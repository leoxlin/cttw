package coordinator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/llin/cttw/internal/acp"
	"github.com/llin/cttw/internal/github"
	"github.com/llin/cttw/internal/launcher"
	"github.com/llin/cttw/internal/repo"
	"github.com/llin/cttw/internal/store"
)

// ErrRepoNotRegistered is returned when a requested repo has not been registered.
var ErrRepoNotRegistered = errors.New("repo not registered")

type Coordinator struct {
	store    *store.Store
	launcher launcher.Launcher
	repos    *repo.Registry
	gh       github.Client
	backend  string
}

func New(store *store.Store, launcher launcher.Launcher, repos *repo.Registry, gh github.Client, backend string) *Coordinator {
	if backend == "" {
		backend = "codex"
	}
	return &Coordinator{store: store, launcher: launcher, repos: repos, gh: gh, backend: backend}
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

	// Create GitHub issue for the problem.
	parentNumber, err := c.gh.CreateIssue(ctx, owner, name, description, "Coordinated by cttw.")
	if err != nil {
		return nil, fmt.Errorf("create parent issue: %w", err)
	}
	problem.ParentIssueNumber = parentNumber
	if err := c.store.UpdateProblem(ctx, problem); err != nil {
		return nil, fmt.Errorf("update problem issue number: %w", err)
	}

	// Launch coordinator agent.
	agent, err := c.launcher.Launch(ctx, launcher.LaunchSpec{
		Backend: c.backend,
		Repo:    launcher.RepoContext{Owner: owner, Name: name, DefaultBranch: r.DefaultBranch, LocalDir: r.LocalDir},
		Task:    launcher.TaskContext{ProblemDescription: description},
	})
	if err != nil {
		return nil, fmt.Errorf("launch coordinator agent: %w", err)
	}
	defer agent.Close(ctx)

	if err := agent.Initialize(ctx); err != nil {
		return nil, fmt.Errorf("initialize agent: %w", err)
	}
	if err := agent.NewSession(ctx, acp.NewSessionRequest{CWD: r.LocalDir}); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	// Prompt for decomposition.
	prompt := fmt.Sprintf(`You are a software engineering coordinator. Break the following problem into small, implementable tasks for a single repository.

Repository: %s/%s
Problem: %s

Return ONLY a JSON array of objects, each with "title" and "description" fields. Example:
[{"title":"add handler","description":"implement POST endpoint"}]

Do not include markdown fences or explanation.`, owner, name, description)

	res, err := agent.Prompt(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("prompt agent: %w", err)
	}

	tasks, err := parseTasks(res.Content)
	if err != nil {
		return nil, fmt.Errorf("parse tasks: %w", err)
	}

	// Create tasks and child issues.
	for _, task := range tasks {
		t, err := c.store.CreateTask(ctx, problem.ID, r.ID, task.Title, task.Description)
		if err != nil {
			return nil, fmt.Errorf("create task: %w", err)
		}
		childNumber, err := c.gh.CreateIssue(ctx, owner, name, task.Title, task.Description)
		if err != nil {
			return nil, fmt.Errorf("create task issue: %w", err)
		}
		t.IssueNumber = childNumber
		if err := c.store.UpdateTask(ctx, t); err != nil {
			return nil, fmt.Errorf("update task issue number: %w", err)
		}
		if err := c.gh.CreateSubIssue(ctx, owner, name, parentNumber, childNumber); err != nil {
			return nil, fmt.Errorf("link sub-issue: %w", err)
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
	start := findOutermost(content, '[', ']')
	if start.Start < 0 {
		return nil, fmt.Errorf("no JSON array found in response")
	}
	var tasks []taskSpec
	if err := json.Unmarshal([]byte(content[start.Start:start.End+1]), &tasks); err != nil {
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

type span struct {
	Start int
	End   int
}

// findOutermost returns the indices of the outermost balanced open/close pair,
// or -1 if none exists.
func findOutermost(s string, open, close byte) span {
	depth := 0
	start := -1
	for i := 0; i < len(s); i++ {
		if s[i] == open {
			if depth == 0 {
				start = i
			}
			depth++
		} else if s[i] == close {
			if depth > 0 {
				depth--
				if depth == 0 {
					return span{Start: start, End: i}
				}
			}
		}
	}
	return span{Start: -1, End: -1}
}
