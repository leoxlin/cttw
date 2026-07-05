package coordinator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/llin/cttw/internal/acp"
	"github.com/llin/cttw/internal/github"
	"github.com/llin/cttw/internal/jsonutil"
	"github.com/llin/cttw/internal/launcher"
	"github.com/llin/cttw/internal/repo"
	"github.com/llin/cttw/internal/store"
)

// Sentinel errors for classes of problems that are client or configuration
// errors rather than internal server bugs.
var (
	// ErrRepoNotRegistered is returned when a requested repo has not been registered.
	ErrRepoNotRegistered = errors.New("repo not registered")
	// ErrGitHubFailed is returned when a GitHub API call fails.
	ErrGitHubFailed = errors.New("github request failed")
	// ErrAgentFailed is returned when launching, prompting, or parsing an ACP agent response fails.
	ErrAgentFailed = errors.New("agent request failed")
)

type Coordinator struct {
	store         *store.Store
	launcher      launcher.Launcher
	repos         *repo.Registry
	gh            github.Client
	backend       string
	promptTimeout time.Duration
}

func New(store *store.Store, launcher launcher.Launcher, repos *repo.Registry, gh github.Client, backend string, promptTimeout time.Duration) *Coordinator {
	if backend == "" {
		backend = "codex"
	}
	if promptTimeout <= 0 {
		promptTimeout = 15 * time.Minute
	}
	return &Coordinator{store: store, launcher: launcher, repos: repos, gh: gh, backend: backend, promptTimeout: promptTimeout}
}

func (c *Coordinator) CreateProblem(ctx context.Context, owner, name, description string) (*store.Problem, error) {
	// Ensure repo is registered locally.
	r, err := c.store.GetRepoByOwnerName(ctx, owner, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRepoNotRegistered
		}
		return nil, err
	}

	// Create parent problem record.
	problem, err := c.store.CreateProblem(ctx, description, r.ID)
	if err != nil {
		return nil, fmt.Errorf("create problem: %w", err)
	}

	// Helper to mark the problem failed and persist the change.
	markFailed := func() {
		problem.Status = "failed"
		if err := c.store.UpdateProblem(ctx, problem); err != nil {
			log.Printf("update problem status to failed: %v", err)
		}
	}

	// Launch coordinator agent.
	agent, err := c.launcher.Launch(ctx, launcher.LaunchSpec{
		Backend: c.backend,
		Repo:    launcher.RepoContext{Owner: owner, Name: name, DefaultBranch: r.DefaultBranch, LocalDir: r.LocalDir},
		Task:    launcher.TaskContext{ProblemDescription: description},
	})
	if err != nil {
		markFailed()
		return nil, fmt.Errorf("%w: launch coordinator agent: %w", ErrAgentFailed, err)
	}
	defer agent.Close(ctx)

	setupCtx, cancel := context.WithTimeout(ctx, c.promptTimeout)
	defer cancel()
	if err := agent.Initialize(setupCtx); err != nil {
		markFailed()
		return nil, fmt.Errorf("%w: initialize agent: %w", ErrAgentFailed, err)
	}
	if err := agent.NewSession(setupCtx, acp.NewSessionRequest{CWD: r.LocalDir}); err != nil {
		markFailed()
		return nil, fmt.Errorf("%w: create session: %w", ErrAgentFailed, err)
	}

	// Prompt for decomposition with a bounded timeout.
	prompt := fmt.Sprintf(`You are a software engineering coordinator. Break the following problem into small, implementable tasks for a single repository.

Repository: %s/%s
Problem: %s

Return ONLY a JSON array of objects, each with "title" and "description" fields. Example:
[{"title":"add handler","description":"implement POST endpoint"}]

Do not include markdown fences or explanation.`, owner, name, description)

	promptCtx, cancel := context.WithTimeout(ctx, c.promptTimeout)
	defer cancel()
	res, err := agent.Prompt(promptCtx, prompt)
	if err != nil {
		markFailed()
		return nil, fmt.Errorf("%w: prompt agent: %w", ErrAgentFailed, err)
	}

	tasks, err := parseTasks(res.Content)
	if err != nil {
		markFailed()
		return nil, fmt.Errorf("%w: parse tasks: %w", ErrAgentFailed, err)
	}
	if len(tasks) == 0 {
		markFailed()
		return nil, fmt.Errorf("%w: decomposition returned no tasks", ErrAgentFailed)
	}

	// Create tasks in the store before exposing any GitHub state.
	createdTasks := make([]*store.Task, 0, len(tasks))
	for _, task := range tasks {
		t, err := c.store.CreateTask(ctx, problem.ID, r.ID, task.Title, task.Description)
		if err != nil {
			markFailed()
			return nil, fmt.Errorf("create task: %w", err)
		}
		createdTasks = append(createdTasks, t)
	}

	// Create the parent GitHub issue only after decomposition succeeds.
	parentNumber, err := c.gh.CreateIssue(ctx, owner, name, description, "Coordinated by cttw.")
	if err != nil {
		markFailed()
		return nil, fmt.Errorf("%w: create parent issue: %w", ErrGitHubFailed, err)
	}
	problem.ParentIssueNumber = parentNumber
	if err := c.store.UpdateProblem(ctx, problem); err != nil {
		markFailed()
		return nil, fmt.Errorf("update problem issue number: %w", err)
	}

	// Create child issues and link them to the parent.
	for _, t := range createdTasks {
		childNumber, err := c.gh.CreateIssue(ctx, owner, name, t.Title, t.Description)
		if err != nil {
			markFailed()
			return nil, fmt.Errorf("%w: create task issue: %w", ErrGitHubFailed, err)
		}
		t.IssueNumber = childNumber
		if err := c.store.UpdateTask(ctx, t); err != nil {
			return nil, fmt.Errorf("update task issue number: %w", err)
		}
		if err := c.gh.CreateSubIssue(ctx, owner, name, parentNumber, childNumber); err != nil {
			markFailed()
			return nil, fmt.Errorf("%w: link sub-issue: %w", ErrGitHubFailed, err)
		}
	}

	problem.Status = "ready"
	if err := c.store.UpdateProblem(ctx, problem); err != nil {
		return nil, fmt.Errorf("update problem status: %w", err)
	}
	return problem, nil
}

type taskSpec struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

func parseTasks(content string) ([]taskSpec, error) {
	content = strings.TrimSpace(content)
	raw, err := jsonutil.ExtractOutermost([]byte(content), '[')
	if err != nil {
		return nil, fmt.Errorf("no JSON array found in response: %w", err)
	}
	var tasks []taskSpec
	if err := json.Unmarshal(raw, &tasks); err != nil {
		return nil, err
	}
	for i, t := range tasks {
		if strings.TrimSpace(t.Title) == "" {
			return nil, fmt.Errorf("task %d has empty title", i)
		}
		if strings.TrimSpace(t.Description) == "" {
			return nil, fmt.Errorf("task %d has empty description", i)
		}
	}
	return tasks, nil
}
