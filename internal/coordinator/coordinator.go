package coordinator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
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
	wg            sync.WaitGroup
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

// Wait blocks until all in-flight problem decomposition goroutines finish.
func (c *Coordinator) Wait() {
	c.wg.Wait()
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

	// Return a copy so the caller does not race with the background goroutine.
	pending := *problem

	c.wg.Add(1)
	go c.decomposeProblem(context.Background(), problem, r, owner, name, description)

	return &pending, nil
}

func (c *Coordinator) decomposeProblem(bg context.Context, problem *store.Problem, r *store.Repo, owner, name, description string) {
	defer c.wg.Done()

	// Helper to mark the problem failed and fail any tasks already created.
	markFailed := func() {
		problem.Status = "failed"
		if err := c.store.UpdateProblem(bg, problem); err != nil {
			log.Printf("update problem status to failed: %v", err)
		}
		if err := c.store.FailTasksByProblem(bg, problem.ID); err != nil {
			log.Printf("fail tasks by problem: %v", err)
		}
	}

	// Launch coordinator agent.
	agent, err := c.launcher.Launch(bg, launcher.LaunchSpec{
		Backend: c.backend,
		Repo:    launcher.RepoContext{Owner: owner, Name: name, DefaultBranch: r.DefaultBranch, LocalDir: r.LocalDir},
		Task:    launcher.TaskContext{ProblemDescription: description},
	})
	if err != nil {
		markFailed()
		log.Printf("coordinator launch agent: %v", err)
		return
	}
	defer agent.Close(bg)

	setupCtx, cancel := context.WithTimeout(bg, c.promptTimeout)
	defer cancel()
	if err := agent.Initialize(setupCtx); err != nil {
		markFailed()
		log.Printf("coordinator initialize agent: %v", err)
		return
	}
	if err := agent.NewSession(setupCtx, acp.NewSessionRequest{CWD: r.LocalDir}); err != nil {
		markFailed()
		log.Printf("coordinator create session: %v", err)
		return
	}

	// Prompt for decomposition with a bounded timeout.
	prompt := fmt.Sprintf(`You are a software engineering coordinator. Break the following problem into small, implementable tasks for a single repository.

Repository: %s/%s
Problem: %s

Return ONLY a JSON array of objects, each with "title" and "description" fields. Example:
[{"title":"add handler","description":"implement POST endpoint"}]

Do not include markdown fences or explanation.`, owner, name, description)

	promptCtx, cancel := context.WithTimeout(bg, c.promptTimeout)
	defer cancel()
	res, err := agent.Prompt(promptCtx, prompt)
	if err != nil {
		markFailed()
		log.Printf("coordinator prompt agent: %v", err)
		return
	}

	tasks, err := parseTasks(res.Content)
	if err != nil {
		markFailed()
		log.Printf("coordinator parse tasks: %v", err)
		return
	}
	if len(tasks) == 0 {
		markFailed()
		log.Printf("coordinator decomposition returned no tasks")
		return
	}

	// Create tasks in the store before exposing any GitHub state.
	createdTasks := make([]*store.Task, 0, len(tasks))
	for _, task := range tasks {
		t, err := c.store.CreateTask(bg, problem.ID, r.ID, task.Title, task.Description)
		if err != nil {
			markFailed()
			log.Printf("coordinator create task: %v", err)
			return
		}
		createdTasks = append(createdTasks, t)
	}

	// Create the parent GitHub issue only after decomposition succeeds.
	parentNumber, err := c.gh.CreateIssue(bg, owner, name, description, "Coordinated by cttw.")
	if err != nil {
		markFailed()
		log.Printf("coordinator create parent issue: %v", err)
		return
	}
	problem.ParentIssueNumber = parentNumber
	if err := c.store.UpdateProblem(bg, problem); err != nil {
		markFailed()
		log.Printf("coordinator update problem issue number: %v", err)
		return
	}

	// Create child issues and link them to the parent.
	for _, t := range createdTasks {
		childNumber, err := c.gh.CreateIssue(bg, owner, name, t.Title, t.Description)
		if err != nil {
			markFailed()
			log.Printf("coordinator create task issue: %v", err)
			return
		}
		t.IssueNumber = childNumber
		if err := c.store.UpdateTask(bg, t); err != nil {
			markFailed()
			log.Printf("coordinator update task issue number: %v", err)
			return
		}
		if err := c.gh.CreateSubIssue(bg, owner, name, parentNumber, childNumber); err != nil {
			markFailed()
			log.Printf("coordinator link sub-issue: %v", err)
			return
		}
	}

	problem.Status = "ready"
	if err := c.store.UpdateProblem(bg, problem); err != nil {
		log.Printf("coordinator update problem status: %v", err)
		return
	}
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
