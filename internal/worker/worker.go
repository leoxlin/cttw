package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/llin/cttw/internal/acp"
	"github.com/llin/cttw/internal/github"
	"github.com/llin/cttw/internal/launcher"
	"github.com/llin/cttw/internal/repo"
	"github.com/llin/cttw/internal/store"
)

type Worker struct {
	store    *store.Store
	launcher launcher.Launcher
	repos    *repo.Registry
	gh       github.Client
}

func New(store *store.Store, launcher launcher.Launcher, repos *repo.Registry, gh github.Client) *Worker {
	return &Worker{store: store, launcher: launcher, repos: repos, gh: gh}
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
		task.Status = "failed"
		task.Output = err.Error()
		task.Attempts++
		_ = w.store.UpdateTask(ctx, task)
		return err
	}
	return nil
}

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

	agent, err := w.launcher.Launch(ctx, launcher.LaunchSpec{
		Backend: "codex",
		Repo:    launcher.RepoContext{Owner: r.Owner, Name: r.Name, DefaultBranch: r.DefaultBranch, LocalDir: r.LocalDir},
		Task: launcher.TaskContext{
			ProblemDescription: problem.Description,
			TaskTitle:          task.Title,
			TaskDescription:    task.Description,
			BaseBranch:         r.DefaultBranch,
		},
	})
	if err != nil {
		return fmt.Errorf("launch task agent: %w", err)
	}
	defer agent.Close(ctx)

	if err := agent.Initialize(ctx); err != nil {
		return fmt.Errorf("initialize agent: %w", err)
	}
	if err := agent.NewSession(ctx, acp.NewSessionRequest{CWD: r.LocalDir}); err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	prompt := fmt.Sprintf(`You are a software engineer. Implement the following task in the repository.

Repository: %s/%s
Base branch: %s
Task: %s
Description: %s

Create a feature branch, make the necessary changes, commit, push the branch, and open a pull request targeting the base branch. Then report the result as JSON:

{"pr_number": <number>, "branch": "<branch-name>", "status": "completed"}

If you cannot complete the task, return:
{"status": "failed", "error": "<reason>"}

Return ONLY the JSON object, no markdown fences.`, r.Owner, r.Name, r.DefaultBranch, task.Title, task.Description)

	res, err := agent.Prompt(ctx, prompt)
	if err != nil {
		return fmt.Errorf("prompt agent: %w", err)
	}

	out, err := parseTaskResult(res.Content)
	if err != nil {
		return fmt.Errorf("parse task result: %w", err)
	}

	task.Branch = out.Branch
	task.PRNumber = out.PRNumber
	if out.Status == "failed" {
		task.Status = "failed"
		task.Output = out.Error
		task.Attempts++
		return fmt.Errorf("task failed: %s", out.Error)
	}
	task.Status = "completed"
	if err := w.store.UpdateTask(ctx, task); err != nil {
		return fmt.Errorf("update task: %w", err)
	}
	return nil
}

type taskResult struct {
	Status   string `json:"status"`
	PRNumber int    `json:"pr_number"`
	Branch   string `json:"branch"`
	Error    string `json:"error"`
}

func parseTaskResult(content string) (taskResult, error) {
	content = strings.TrimSpace(content)
	if idx := strings.Index(content, "{"); idx > 0 {
		content = content[idx:]
	}
	if idx := strings.LastIndex(content, "}"); idx > 0 {
		content = content[:idx+1]
	}
	var out taskResult
	if err := json.Unmarshal([]byte(content), &out); err != nil {
		return taskResult{}, err
	}
	return out, nil
}
