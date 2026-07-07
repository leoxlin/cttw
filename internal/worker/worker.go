package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/llin/cttw/internal/acp"
	"github.com/llin/cttw/internal/gitexec"
	"github.com/llin/cttw/internal/github"
	"github.com/llin/cttw/internal/jsonutil"
	"github.com/llin/cttw/internal/launcher"
	"github.com/llin/cttw/internal/repo"
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

type Option func(*Worker)

type Worker struct {
	store         *store.Store
	launcher      launcher.Launcher
	repos         *repo.Registry
	gh            github.Client
	backend       string
	promptTimeout time.Duration
	gitHubToken   string
	newGitRunner  gitRunnerFactory
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

	git := w.newGitRunner(r.LocalDir, w.gitHubToken)
	branch := task.Branch
	if branch == "" {
		branch = taskBranchName(task)
		task.Branch = branch
	}
	task.BaseBranch = r.DefaultBranch
	if err := ensureTaskBranch(task, git, branch, r.DefaultBranch); err != nil {
		task.Status = "failed"
		task.Output = err.Error()
		return err
	}

	out, _, err := w.runTaskLifecycle(ctx, git, r, problem, task, branch)
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
	return nil
}

func (w *Worker) runTaskLifecycle(ctx context.Context, git gitRunner, r *store.Repo, problem *store.Problem, task *store.Task, branch string) (taskResult, bool, error) {
	resume, err := shouldResumePostCommit(task, git, r.DefaultBranch)
	if err != nil {
		return taskResult{}, false, err
	}

	out := taskResult{}
	committedBranch := resume
	if resume {
		summary, err := headSummary(git)
		if err != nil {
			return taskResult{}, false, fmt.Errorf("resume task from branch: %w", err)
		}
		out.Status = "completed"
		out.Summary = summary
	} else {
		var commitCreated bool
		out, commitCreated, err = w.runAgentAndCommit(ctx, git, r, problem, task)
		if err != nil {
			return taskResult{}, commitCreated, err
		}
		committedBranch = commitCreated
	}

	if task.PRNumber == 0 {
		if err := git.PushSetUpstream(branch); err != nil {
			return taskResult{}, committedBranch, fmt.Errorf("push branch: %w", err)
		}

		prNumber, err := w.gh.CreatePullRequest(ctx, r.Owner, r.Name, task.Title, prBody(task, out), branch, r.DefaultBranch)
		if err != nil {
			return taskResult{}, committedBranch, fmt.Errorf("create pull request: %w", err)
		}
		task.PRNumber = prNumber
	}

	pr, err := w.gh.GetPullRequest(ctx, r.Owner, r.Name, task.PRNumber)
	if err != nil {
		return taskResult{}, committedBranch, fmt.Errorf("verify pull request: %w", err)
	}
	if pr == nil {
		return taskResult{}, committedBranch, fmt.Errorf("verify pull request: pull request %d was nil", task.PRNumber)
	}
	if pr.Head.Ref != branch {
		return taskResult{}, committedBranch, fmt.Errorf("task branch %q does not match pull request head %q", branch, pr.Head.Ref)
	}
	return out, committedBranch, nil
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

func taskBranchName(task *store.Task) string {
	slug := strings.ToLower(strings.TrimSpace(task.Title))
	slug = branchUnsafe.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "task"
	}
	if len(slug) > 48 {
		slug = strings.Trim(slug[:48], "-")
	}
	if slug == "" {
		slug = "task"
	}
	return fmt.Sprintf("cttw/%s-%s", slug, strutil.ShortID(task.ID))
}

func commitMessage(task *store.Task, out taskResult) string {
	const prefix = "cttw: "
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

func prBody(task *store.Task, out taskResult) string {
	var b strings.Builder
	b.WriteString("Coordinated by cttw.\n\n")
	if out.Summary != "" {
		b.WriteString(out.Summary)
		b.WriteString("\n\n")
	}
	if len(out.Verification) > 0 {
		b.WriteString("Verification:\n")
		for _, cmd := range out.Verification {
			b.WriteString("- ")
			b.WriteString(cmd)
			b.WriteByte('\n')
		}
	}
	return b.String()
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

func ensureTaskBranch(task *store.Task, git gitRunner, branch, baseBranch string) error {
	currentBranch, err := git.CurrentBranch()
	if err != nil {
		return fmt.Errorf("current branch: %w", err)
	}
	changed, err := git.HasChanges()
	if err != nil {
		return fmt.Errorf("inspect git changes: %w", err)
	}
	if changed && currentBranch != branch {
		return terminalTaskError(fmt.Errorf("workspace is dirty on branch %q; cannot start task branch %q", currentBranch, branch))
	}
	if currentBranch == branch {
		return nil
	}
	if err := git.CheckoutNew(branch, baseBranch); err != nil {
		if checkoutErr := git.Checkout(branch); checkoutErr != nil {
			return fmt.Errorf("checkout task branch: %w", err)
		}
	}
	return nil
}

func shouldResumePostCommit(task *store.Task, git gitRunner, baseBranch string) (bool, error) {
	changed, err := git.HasChanges()
	if err != nil {
		return false, fmt.Errorf("inspect git changes: %w", err)
	}
	if changed {
		return false, nil
	}
	if task.PRNumber != 0 {
		return true, nil
	}
	out, err := git.Output("rev-list", "--count", fmt.Sprintf("%s..HEAD", baseBranch))
	if err != nil {
		return false, fmt.Errorf("inspect branch commits: %w", err)
	}
	count, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return false, fmt.Errorf("parse branch commit count: %w", err)
	}
	return count > 0, nil
}

func headSummary(git gitRunner) (string, error) {
	out, err := git.Output("log", "-1", "--pretty=%s")
	if err != nil {
		return "", err
	}
	summary := strings.TrimSpace(string(out))
	summary = strings.TrimPrefix(summary, "cttw: ")
	return summary, nil
}
