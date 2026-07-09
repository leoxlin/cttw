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
	"github.com/llin/cttw/internal/config"
	"github.com/llin/cttw/internal/github"
	"github.com/llin/cttw/internal/jsonutil"
	"github.com/llin/cttw/internal/launcher"
	"github.com/llin/cttw/internal/repo"
	"github.com/llin/cttw/internal/store"
)

// RepoRegistry abstracts the local git registry so tests can avoid real clones.
type RepoRegistry interface {
	Ensure(ctx context.Context, owner, name, defaultBranch, token string) (*repo.Repo, error)
}

// Option configures a Coordinator.
type Option func(*Coordinator)

// WithToken provides the GitHub token used when lazily cloning repos.
func WithToken(token string) Option {
	return func(c *Coordinator) { c.token = token }
}

// WithRepoConfigs records configured default branches for repos.
func WithRepoConfigs(configs []config.RepoConfig) Option {
	return func(c *Coordinator) {
		for _, rc := range configs {
			key := rc.Owner + "/" + rc.Name
			branch := rc.DefaultBranch
			if branch == "" {
				branch = "main"
			}
			c.repoBranches[key] = branch
		}
	}
}

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
	repos         RepoRegistry
	gh            github.Client
	backend       string
	promptTimeout time.Duration
	token         string
	repoBranches  map[string]string
	wg            sync.WaitGroup
}

func New(store *store.Store, launcher launcher.Launcher, repos RepoRegistry, gh github.Client, backend string, promptTimeout time.Duration, opts ...Option) *Coordinator {
	if backend == "" {
		backend = "codex"
	}
	if promptTimeout <= 0 {
		promptTimeout = 15 * time.Minute
	}
	c := &Coordinator{
		store:         store,
		launcher:      launcher,
		repos:         repos,
		gh:            gh,
		backend:       backend,
		promptTimeout: promptTimeout,
		repoBranches:  make(map[string]string),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Coordinator) defaultBranch(owner, name string) string {
	if b, ok := c.repoBranches[owner+"/"+name]; ok {
		return b
	}
	return "main"
}

// Wait blocks until all in-flight problem decomposition goroutines finish.
func (c *Coordinator) Wait() {
	c.wg.Wait()
}

func (c *Coordinator) CreateProblem(ctx context.Context, owner, name, description string) (*store.Problem, error) {
	// Ensure repo is registered locally.
	r, err := c.store.GetRepoByOwnerName(ctx, owner, name)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}

		branch := c.defaultBranch(owner, name)
		localRepo, ensureErr := c.repos.Ensure(ctx, owner, name, branch, c.token)
		if ensureErr != nil {
			return nil, fmt.Errorf("ensure repo %s/%s: %w", owner, name, ensureErr)
		}

		created, createErr := c.store.CreateRepo(ctx, owner, name, localRepo.Dir, localRepo.DefaultBranch, "")
		if createErr != nil {
			// The unique (owner, name) constraint may race with another
			// concurrent registration. Re-query once before failing.
			r, err = c.store.GetRepoByOwnerName(ctx, owner, name)
			if err != nil {
				return nil, fmt.Errorf("create repo %s/%s: %w", owner, name, createErr)
			}
		} else {
			r = created
		}
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
	prompt := fmt.Sprintf(`You are a software engineering coordinator. Break the following problem into a small number of feature-complete pull requests for a single repository.

Each pull request should be a complete, reviewable feature and may contain multiple dependent tasks that are committed together.

Repository: %s/%s
Problem: %s

Return ONLY a JSON object with this shape:
{"pr_groups":[{"title":"Feature title for the PR","description":"What this PR implements","tasks":[{"title":"Task title","description":"Task details"}]}]}

Rules:
- Groups are ordered from first (base of the stack) to last (top of the stack). Later groups build on earlier groups.
- Tasks within a group are executed in the order given; each task becomes one commit in the group's branch.
- Keep the number of PRs small; group related tasks into one PR rather than opening one PR per task.
- Do not include markdown fences or explanation.`, owner, name, description)

	promptCtx, cancel := context.WithTimeout(bg, c.promptTimeout)
	defer cancel()
	res, err := agent.Prompt(promptCtx, prompt)
	if err != nil {
		markFailed()
		log.Printf("coordinator prompt agent: %v", err)
		return
	}

	groups, err := parsePRGroups(res.Content)
	if err != nil {
		markFailed()
		log.Printf("coordinator parse pr groups: %v", err)
		return
	}
	if len(groups) == 0 {
		markFailed()
		log.Printf("coordinator decomposition returned no pr groups")
		return
	}

	// Create PR groups and their tasks in the store before exposing any GitHub state.
	createdGroups := make([]*store.PRGroup, 0, len(groups))
	for i, g := range groups {
		pg, err := c.store.CreatePRGroup(bg, problem.ID, r.ID, g.Title, g.Description, i)
		if err != nil {
			markFailed()
			log.Printf("coordinator create pr group: %v", err)
			return
		}
		createdGroups = append(createdGroups, pg)

		for j, task := range g.Tasks {
			sequence := i*1000 + j
			if _, err := c.store.CreateTaskInGroup(bg, problem.ID, r.ID, pg.ID, task.Title, task.Description, j, sequence); err != nil {
				markFailed()
				log.Printf("coordinator create task: %v", err)
				return
			}
		}
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

	// Create child issues per PR group and link them to the parent.
	for _, g := range createdGroups {
		childNumber, err := c.gh.CreateIssue(bg, owner, name, g.Title, g.Description)
		if err != nil {
			markFailed()
			log.Printf("coordinator create group issue: %v", err)
			return
		}
		g.IssueNumber = childNumber
		if err := c.store.UpdatePRGroup(bg, g); err != nil {
			markFailed()
			log.Printf("coordinator update group issue number: %v", err)
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

type prGroupSpec struct {
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Tasks       []taskSpec  `json:"tasks"`
}

type decompositionSpec struct {
	PRGroups []prGroupSpec `json:"pr_groups"`
}

func parsePRGroups(content string) ([]prGroupSpec, error) {
	content = strings.TrimSpace(content)
	raw, err := jsonutil.ExtractOutermost([]byte(content), '{')
	if err != nil {
		return nil, fmt.Errorf("no JSON object found in response: %w", err)
	}
	var spec decompositionSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return nil, err
	}
	for i, g := range spec.PRGroups {
		if strings.TrimSpace(g.Title) == "" {
			return nil, fmt.Errorf("pr group %d has empty title", i)
		}
		if strings.TrimSpace(g.Description) == "" {
			return nil, fmt.Errorf("pr group %d has empty description", i)
		}
		if len(g.Tasks) == 0 {
			return nil, fmt.Errorf("pr group %d has no tasks", i)
		}
		for j, t := range g.Tasks {
			if strings.TrimSpace(t.Title) == "" {
				return nil, fmt.Errorf("pr group %d task %d has empty title", i, j)
			}
			if strings.TrimSpace(t.Description) == "" {
				return nil, fmt.Errorf("pr group %d task %d has empty description", i, j)
			}
		}
	}
	return spec.PRGroups, nil
}
