package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/llin/cttw/internal/acp"
	"github.com/llin/cttw/internal/gitexec"
	"github.com/llin/cttw/internal/github"
	"github.com/llin/cttw/internal/jsonutil"
	"github.com/llin/cttw/internal/launcher"
	"github.com/llin/cttw/internal/repo"
	"github.com/llin/cttw/internal/stack"
	"github.com/llin/cttw/internal/store"
	"github.com/llin/cttw/internal/strutil"
)

type gitRunner interface {
	Checkout(branch string) error
	CheckoutNew(branch, base string) error
	CommitAll(message string) (bool, error)
	CurrentBranch() (string, error)
	HasChanges() (bool, error)
	Output(args ...string) ([]byte, error)
	PushSetUpstream(branch string) error
	ResetHardClean() error
}

type gitRunnerFactory func(dir, token string) gitRunner

// StackRunner abstracts the native stacked PR publisher used by the worker.
type StackRunner interface {
	Submit(ctx context.Context, groups []stack.Group) error
}

type stackRunnerFactory func(dir, owner, name, token string) StackRunner

type Option func(*Worker)

type Worker struct {
	store            *store.Store
	launcher         launcher.Launcher
	repos            *repo.Registry
	gh               github.Client
	backend          string
	promptTimeout    time.Duration
	gitHubToken      string
	newGitRunner     gitRunnerFactory
	newStackRunner   stackRunnerFactory
}

func New(store *store.Store, launcher launcher.Launcher, repos *repo.Registry, gh github.Client, backend string, promptTimeout time.Duration, opts ...Option) *Worker {
	if backend == "" {
		backend = "codex"
	}
	if promptTimeout <= 0 {
		promptTimeout = 10 * time.Minute
	}
	w := &Worker{
		store:         store,
		launcher:      launcher,
		repos:         repos,
		gh:            gh,
		backend:       backend,
		promptTimeout: promptTimeout,
		newGitRunner: func(dir, token string) gitRunner {
			return &gitexec.Runner{Dir: dir, Token: token}
		},
		newStackRunner: func(dir, owner, name, token string) StackRunner {
			return &stack.Runner{
				Git:    &gitexec.Runner{Dir: dir, Token: token},
				GitHub: gh,
				Owner:  owner,
				Name:   name,
			}
		},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(w)
		}
	}
	return w
}

func WithGitHubToken(token string) Option {
	return func(w *Worker) {
		w.gitHubToken = token
	}
}

func withGitRunnerFactory(factory gitRunnerFactory) Option {
	return func(w *Worker) {
		if factory != nil {
			w.newGitRunner = factory
		}
	}
}

func WithStackRunnerFactory(factory func(dir, owner, name, token string) StackRunner) Option {
	return func(w *Worker) {
		if factory != nil {
			w.newStackRunner = factory
		}
	}
}

func withStackRunnerFactory(factory stackRunnerFactory) Option {
	return func(w *Worker) {
		if factory != nil {
			w.newStackRunner = factory
		}
	}
}

type taskExecutionError struct {
	err      error
	terminal bool
}

func (e *taskExecutionError) Error() string {
	return e.err.Error()
}

func (e *taskExecutionError) Unwrap() error {
	return e.err
}

func terminalTaskError(err error) error {
	if err == nil {
		return nil
	}
	return &taskExecutionError{err: err, terminal: true}
}

func isTerminalTaskError(err error) bool {
	var taskErr *taskExecutionError
	return errors.As(err, &taskErr) && taskErr.terminal
}

func (w *Worker) RunOnce(ctx context.Context) error {
	task, err := w.store.NextPendingTask(ctx)
	if err != nil {
		return err
	}
	if task == nil {
		return nil
	}
	if err := w.ExecuteTask(ctx, task); err != nil {
		task.Output = err.Error()
		task.Attempts++
		if isTerminalTaskError(err) || task.Attempts >= task.MaxAttempts {
			task.Status = "failed"
		} else {
			task.Status = "pending"
		}
		if updateErr := w.store.UpdateTask(ctx, task); updateErr != nil {
			return fmt.Errorf("execute task: %w; update task state: %w", err, updateErr)
		}
		return err
	}
	return nil
}

// RunOnceForRepo picks and executes the next pending task for a single repo.
func (w *Worker) RunOnceForRepo(ctx context.Context, repoID string) error {
	task, err := w.store.NextPendingTaskForRepo(ctx, repoID)
	if err != nil {
		return err
	}
	if task == nil {
		return nil
	}
	if err := w.ExecuteTask(ctx, task); err != nil {
		task.Output = err.Error()
		task.Attempts++
		if isTerminalTaskError(err) || task.Attempts >= task.MaxAttempts {
			task.Status = "failed"
		} else {
			task.Status = "pending"
		}
		if updateErr := w.store.UpdateTask(ctx, task); updateErr != nil {
			return fmt.Errorf("execute task: %w; update task state: %w", err, updateErr)
		}
		return err
	}
	return nil
}

var branchUnsafe = regexp.MustCompile(`[^a-z0-9]+`)

func (w *Worker) ExecuteTask(ctx context.Context, task *store.Task) error {
	task.Status = "running"
	if err := w.store.UpdateTask(ctx, task); err != nil {
		return fmt.Errorf("mark running: %w", err)
	}

	r, err := w.store.GetRepo(ctx, task.RepoID)
	if err != nil {
		return fmt.Errorf("get repo: %w", err)
	}

	problem, err := w.store.GetProblem(ctx, task.ProblemID)
	if err != nil {
		return fmt.Errorf("get problem: %w", err)
	}

	group, err := w.store.GetPRGroup(ctx, task.PRGroupID)
	if err != nil {
		return fmt.Errorf("get pr group: %w", err)
	}

	git := w.newGitRunner(r.LocalDir, w.gitHubToken)
	sr := w.newStackRunner(r.LocalDir, r.Owner, r.Name, w.gitHubToken)

	branch := group.Branch
	if branch == "" {
		branch = groupBranchName(group)
		group.Branch = branch
	}

	baseBranch := r.DefaultBranch
	if group.StackOrder > 0 {
		prev, err := w.previousGroup(ctx, group)
		if err != nil {
			return fmt.Errorf("resolve previous group: %w", err)
		}
		if prev != nil && prev.Branch != "" {
			baseBranch = prev.Branch
			group.BaseGroupID = prev.ID
		}
	}
	group.BaseBranch = baseBranch

	if err := ensureGroupBranch(group, git, branch, baseBranch); err != nil {
		task.Status = "failed"
		task.Output = err.Error()
		return err
	}
	if err := w.store.UpdatePRGroup(ctx, group); err != nil {
		return fmt.Errorf("update group branch: %w", err)
	}

	out, err := w.runTaskLifecycle(ctx, git, r, problem, group, task, branch)
	if err != nil {
		task.Status = "failed"
		task.Output = err.Error()
		if strings.Contains(err.Error(), "commit failed") {
			return terminalTaskError(err)
		}
		return err
	}

	task.Status = "completed"
	task.Output = out.Summary
	if err := w.store.UpdateTask(ctx, task); err != nil {
		return fmt.Errorf("update task: %w", err)
	}

	groupComplete, err := w.isGroupComplete(ctx, group.ID)
	if err != nil {
		return fmt.Errorf("check group completion: %w", err)
	}
	if groupComplete {
		if err := git.PushSetUpstream(group.Branch); err != nil {
			return fmt.Errorf("push group branch: %w", err)
		}
		group.Status = "completed"
		if err := w.store.UpdatePRGroup(ctx, group); err != nil {
			return fmt.Errorf("update group status: %w", err)
		}
	}

	problemComplete, err := w.isProblemComplete(ctx, problem.ID)
	if err != nil {
		return fmt.Errorf("check problem completion: %w", err)
	}
	if problemComplete {
		if err := w.submitStack(ctx, sr, r, problem); err != nil {
			return fmt.Errorf("submit stack: %w", err)
		}
	}

	return nil
}

func (w *Worker) previousGroup(ctx context.Context, group *store.PRGroup) (*store.PRGroup, error) {
	groups, err := w.store.ListPRGroupsByProblem(ctx, group.ProblemID)
	if err != nil {
		return nil, err
	}
	for _, g := range groups {
		if g.StackOrder == group.StackOrder-1 {
			return &g, nil
		}
	}
	return nil, nil
}

func (w *Worker) isGroupComplete(ctx context.Context, groupID string) (bool, error) {
	n, err := w.store.CountIncompleteTasksByPRGroup(ctx, groupID)
	if err != nil {
		return false, err
	}
	return n == 0, nil
}

func (w *Worker) isProblemComplete(ctx context.Context, problemID string) (bool, error) {
	n, err := w.store.CountIncompleteTasksByProblem(ctx, problemID)
	if err != nil {
		return false, err
	}
	return n == 0, nil
}

func (w *Worker) submitStack(ctx context.Context, sr StackRunner, r *store.Repo, problem *store.Problem) error {
	groups, err := w.store.ListPRGroupsByProblem(ctx, problem.ID)
	if err != nil {
		return fmt.Errorf("list groups: %w", err)
	}
	if len(groups) == 0 {
		return nil
	}
	stackGroups := make([]stack.Group, 0, len(groups))
	for _, g := range groups {
		if g.Branch == "" {
			return fmt.Errorf("group %s has no branch", g.ID)
		}
		base := g.BaseBranch
		if base == "" {
			base = r.DefaultBranch
		}
		stackGroups = append(stackGroups, stack.Group{
			ID:          g.ID,
			Title:       g.Title,
			Description: g.Description,
			Branch:      g.Branch,
			BaseBranch:  base,
			PRNumber:    g.PRNumber,
		})
	}
	if err := sr.Submit(ctx, stackGroups); err != nil {
		return fmt.Errorf("submit stack: %w", err)
	}
	// Persist the PR numbers assigned by the stack runner.
	for i, sg := range stackGroups {
		if sg.PRNumber == 0 {
			continue
		}
		g := &groups[i]
		if g.PRNumber != sg.PRNumber {
			g.PRNumber = sg.PRNumber
			if err := w.store.UpdatePRGroup(ctx, g); err != nil {
				return fmt.Errorf("update group pr number: %w", err)
			}
		}
	}
	return nil
}

func (w *Worker) runTaskLifecycle(ctx context.Context, git gitRunner, r *store.Repo, problem *store.Problem, group *store.PRGroup, task *store.Task, branch string) (taskResult, error) {
	resume, err := taskCommitExists(git, task)
	if err != nil {
		return taskResult{}, err
	}

	out := taskResult{}
	if resume {
		summary, err := taskSummaryFromCommit(git, task)
		if err != nil {
			return taskResult{}, fmt.Errorf("resume task from branch: %w", err)
		}
		out.Status = "completed"
		out.Summary = summary
		return out, nil
	}

	out, _, err = w.runAgentAndCommit(ctx, git, r, problem, task)
	if err != nil {
		return taskResult{}, err
	}
	return out, nil
}

func (w *Worker) runAgentAndCommit(ctx context.Context, git gitRunner, r *store.Repo, problem *store.Problem, task *store.Task) (taskResult, bool, error) {
	agent, err := w.launcher.Launch(ctx, launcher.LaunchSpec{
		Backend: w.backend,
		Repo:    launcher.RepoContext{Owner: r.Owner, Name: r.Name, DefaultBranch: r.DefaultBranch, LocalDir: r.LocalDir},
		Task: launcher.TaskContext{
			ProblemDescription: problem.Description,
			TaskTitle:          task.Title,
			TaskDescription:    task.Description,
			BaseBranch:         r.DefaultBranch,
		},
	})
	if err != nil {
		return taskResult{}, false, fmt.Errorf("launch task agent: %w", err)
	}
	defer agent.Close(ctx)

	setupCtx, cancel := context.WithTimeout(ctx, w.promptTimeout)
	defer cancel()
	if err := agent.Initialize(setupCtx); err != nil {
		return taskResult{}, false, fmt.Errorf("initialize agent: %w", err)
	}
	if err := agent.NewSession(setupCtx, acp.NewSessionRequest{CWD: r.LocalDir}); err != nil {
		return taskResult{}, false, fmt.Errorf("create session: %w", err)
	}
	task.AgentSessionID = agent.SessionID()

	prompt := buildTaskPrompt(r.Owner, r.Name, r.DefaultBranch, task.Title, task.Description)

	promptCtx, cancel := context.WithTimeout(ctx, w.promptTimeout)
	defer cancel()
	res, err := agent.Prompt(promptCtx, prompt)
	if err != nil {
		if cleanupErr := resetIfChanged(git); cleanupErr != nil {
			return taskResult{}, false, fmt.Errorf("prompt agent: %w; cleanup workspace: %v", err, cleanupErr)
		}
		return taskResult{}, false, fmt.Errorf("prompt agent: %w", err)
	}

	out, err := parseTaskResult(res.Content)
	if err != nil {
		if cleanupErr := resetIfChanged(git); cleanupErr != nil {
			return taskResult{}, false, fmt.Errorf("parse task result: %w; cleanup workspace: %v", err, cleanupErr)
		}
		return taskResult{}, false, fmt.Errorf("parse task result: %w", err)
	}

	if out.Status == "failed" {
		if err := git.ResetHardClean(); err != nil {
			return taskResult{}, false, fmt.Errorf("task failed: %s; cleanup workspace: %v", out.Error, err)
		}
		return taskResult{}, false, fmt.Errorf("task failed: %s", out.Error)
	}

	changed, err := git.HasChanges()
	if err != nil {
		return taskResult{}, false, fmt.Errorf("inspect git changes: %w", err)
	}
	if !changed {
		if err := git.ResetHardClean(); err != nil {
			return taskResult{}, false, fmt.Errorf("completed task produced no changes; cleanup workspace: %v", err)
		}
		return taskResult{}, false, fmt.Errorf("completed task produced no changes")
	}

	committed, err := git.CommitAll(commitMessage(task, out))
	if err != nil {
		return taskResult{}, false, err
	}
	if !committed {
		if err := git.ResetHardClean(); err != nil {
			return taskResult{}, false, fmt.Errorf("completed task produced no staged changes; cleanup workspace: %v", err)
		}
		return taskResult{}, false, fmt.Errorf("completed task produced no staged changes")
	}

	return out, true, nil
}

func resetIfChanged(git gitRunner) error {
	changed, err := git.HasChanges()
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	return git.ResetHardClean()
}

func groupBranchName(group *store.PRGroup) string {
	slug := strings.ToLower(strings.TrimSpace(group.Title))
	slug = branchUnsafe.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "group"
	}
	if len(slug) > 48 {
		slug = strings.Trim(slug[:48], "-")
	}
	if slug == "" {
		slug = "group"
	}
	return fmt.Sprintf("cttw/%s-%s", slug, strutil.ShortID(group.ID))
}

func commitMessage(task *store.Task, out taskResult) string {
	prefix := fmt.Sprintf("cttw[%s]: ", strutil.ShortID(task.ID))
	summary := strings.TrimSpace(out.Summary)
	if summary == "" {
		summary = task.Title
	}
	maxSummary := 72 - len(prefix)
	if maxSummary < 0 {
		maxSummary = 0
	}
	if len(summary) > maxSummary {
		summary = strings.TrimSpace(summary[:maxSummary])
	}
	return prefix + summary
}

func taskCommitPrefix(task *store.Task) string {
	return fmt.Sprintf("cttw[%s]:", strutil.ShortID(task.ID))
}

func taskCommitExists(git gitRunner, task *store.Task) (bool, error) {
	prefix := taskCommitPrefix(task)
	out, err := git.Output("log", "--grep", prefix, "--pretty=%s")
	if err != nil {
		return false, fmt.Errorf("search task commit: %w", err)
	}
	return strings.TrimSpace(string(out)) != "", nil
}

func taskSummaryFromCommit(git gitRunner, task *store.Task) (string, error) {
	prefix := taskCommitPrefix(task)
	out, err := git.Output("log", "--grep", prefix, "-1", "--pretty=%s")
	if err != nil {
		return "", err
	}
	summary := strings.TrimSpace(string(out))
	summary = strings.TrimPrefix(summary, prefix)
	return strings.TrimSpace(summary), nil
}

func ensureGroupBranch(group *store.PRGroup, git gitRunner, branch, baseBranch string) error {
	currentBranch, err := git.CurrentBranch()
	if err != nil {
		return fmt.Errorf("current branch: %w", err)
	}
	changed, err := git.HasChanges()
	if err != nil {
		return fmt.Errorf("inspect git changes: %w", err)
	}
	if changed && currentBranch != branch {
		return terminalTaskError(fmt.Errorf("workspace is dirty on branch %q; cannot start group branch %q", currentBranch, branch))
	}
	if currentBranch == branch {
		return nil
	}
	if err := git.CheckoutNew(branch, baseBranch); err != nil {
		if checkoutErr := git.Checkout(branch); checkoutErr != nil {
			return fmt.Errorf("checkout group branch: %w", err)
		}
	}
	return nil
}

type taskResult struct {
	Status       string   `json:"status"`
	Summary      string   `json:"summary"`
	KeyChanges   []string `json:"key_changes_made"`
	KeyLearnings []string `json:"key_learnings"`
	Verification []string `json:"verification"`
	Error        string   `json:"error"`
}

func parseTaskResult(content string) (taskResult, error) {
	content = strings.TrimSpace(content)
	raw, err := jsonutil.ExtractOutermost([]byte(content), '{')
	if err != nil {
		return taskResult{}, fmt.Errorf("no JSON object found in response: %w", err)
	}
	var out taskResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return taskResult{}, err
	}
	switch out.Status {
	case "completed", "failed":
	default:
		return taskResult{}, fmt.Errorf("unrecognized task status %q", out.Status)
	}
	return out, nil
}

func buildTaskPrompt(owner, name, baseBranch, title, description string) string {
	return fmt.Sprintf(`You are a software engineer. Implement the following task in the repository.

Repository: %s/%s
Base branch: %s
Task: %s
Description: %s

cttw owns repository management for this run:
- Do not create branches.
- Do not make git commits.
- Do not push.
- Do not open pull requests.

Make the smallest verifiable code change that completes this task. Run the focused tests, build, linters, or formatters that validate the changed code when available. If you start long-running processes, stop them before finishing.

When the task is complete, return ONLY this JSON object:
{"status":"completed","summary":"<one sentence>","key_changes_made":["<logical change>"],"key_learnings":["<learning for future runs>"],"verification":["<command you ran>"]}

If you cannot complete the task, return ONLY:
{"status":"failed","error":"<reason>","summary":"<what happened>","key_learnings":["<learning for future runs>"],"verification":["<command you ran, if any>"]}

No markdown fences. No prose outside the JSON object.`, owner, name, baseBranch, title, description)
}


