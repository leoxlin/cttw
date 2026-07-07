package worker

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/llin/cttw/internal/github"
	"github.com/llin/cttw/internal/launcher"
	"github.com/llin/cttw/internal/repo"
	"github.com/llin/cttw/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockGH struct {
	prs []struct {
		Title string
		Body  string
		Head  string
		Base  string
	}
	createPRError  error
	getPRError     error
	getPRBranch    string
	getPRCalledFor int
}

func (m *mockGH) CreateIssue(ctx context.Context, owner, repo, title, body string) (int, error) {
	return 0, nil
}

func (m *mockGH) CreateSubIssue(ctx context.Context, owner, repo string, parentNumber, childNumber int) error {
	return nil
}

func (m *mockGH) CreateBranch(ctx context.Context, owner, repo, branch, base string) error {
	return nil
}

func (m *mockGH) CreatePullRequest(ctx context.Context, owner, repo, title, body, head, base string) (int, error) {
	if m.createPRError != nil {
		return 0, m.createPRError
	}
	m.prs = append(m.prs, struct {
		Title string
		Body  string
		Head  string
		Base  string
	}{Title: title, Body: body, Head: head, Base: base})
	return 42, nil
}

func (m *mockGH) GetPullRequest(ctx context.Context, owner, repo string, number int) (*github.PullRequest, error) {
	m.getPRCalledFor = number
	if m.getPRError != nil {
		return nil, m.getPRError
	}
	ref := m.getPRBranch
	if ref == "" && len(m.prs) > 0 {
		ref = m.prs[len(m.prs)-1].Head
	}
	return &github.PullRequest{
		Number: number,
		Head: struct {
			Ref string `json:"ref"`
		}{Ref: ref},
	}, nil
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return strings.TrimSpace(string(out))
}

func gitStatus(t *testing.T, dir string) string {
	t.Helper()
	return runGit(t, dir, "status", "--porcelain")
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	bare := filepath.Join(root, "origin.git")
	work := filepath.Join(root, "repo")

	runGit(t, root, "init", "--bare", bare)
	runGit(t, root, "init", "-b", "main", work)
	runGit(t, work, "config", "user.email", "test@example.com")
	runGit(t, work, "config", "user.name", "Test")
	runGit(t, work, "remote", "add", "origin", bare)
	require.NoError(t, os.WriteFile(filepath.Join(work, "README.md"), []byte("base\n"), 0644))
	runGit(t, work, "add", ".")
	runGit(t, work, "commit", "-m", "initial")
	runGit(t, work, "push", "-u", "origin", "main")
	return work
}

func TestParseTaskResult_ManagedSchema(t *testing.T) {
	out, err := parseTaskResult(`{
		"status":"completed",
		"summary":"added handler",
		"key_changes_made":["new route"],
		"key_learnings":["router tests cover auth"],
		"verification":["go test ./internal/api"]
	}`)
	require.NoError(t, err)
	assert.Equal(t, "completed", out.Status)
	assert.Equal(t, "added handler", out.Summary)
	assert.Equal(t, []string{"new route"}, out.KeyChanges)
	assert.Equal(t, []string{"router tests cover auth"}, out.KeyLearnings)
	assert.Equal(t, []string{"go test ./internal/api"}, out.Verification)
	assert.Empty(t, out.Error)
}

func TestBuildTaskPrompt_ForbidsGitManagement(t *testing.T) {
	prompt := buildTaskPrompt("llin", "cttw", "main", "add handler", "implement POST")
	assert.Contains(t, prompt, "Do not create branches.")
	assert.Contains(t, prompt, "Do not make git commits.")
	assert.Contains(t, prompt, "Do not push.")
	assert.Contains(t, prompt, "Do not open pull requests.")
	assert.Contains(t, prompt, `"key_changes_made"`)
	assert.Contains(t, prompt, `"verification"`)
}

func TestTaskBranchName_SlugifiesTitle(t *testing.T) {
	task := &store.Task{ID: "1234567890abcdef", Title: " Add Handler!!! / POST "}
	assert.Equal(t, "cttw/add-handler-/-post-12345678", taskBranchName(task))
}

func TestCommitMessage_PrefersSummary(t *testing.T) {
	task := &store.Task{Title: "fallback title"}
	out := taskResult{Summary: "add POST handler"}
	assert.Equal(t, "cttw: add POST handler", commitMessage(task, out))
}

func TestWorker_ExecuteTask_ManagedLifecycleCommitsPushesAndCreatesPR(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	dir := initGitRepo(t)
	r, err := s.CreateRepo(ctx, "llin", "cttw", dir, "main", "")
	require.NoError(t, err)
	problem, err := s.CreateProblem(ctx, "build API", r.ID)
	require.NoError(t, err)
	task, err := s.CreateTask(ctx, problem.ID, r.ID, "add handler", "implement POST")
	require.NoError(t, err)

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{
			OnPrompt: func(prompt string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "handler.go"), []byte("package main\n"), 0644))
			},
			Responses: []string{`{"status":"completed","summary":"added handler","key_changes_made":["handler"],"key_learnings":[],"verification":["go test ./..."]}`},
		}, nil
	}

	gh := &mockGH{}
	w := New(s, ml, &repo.Registry{Root: t.TempDir()}, gh, "codex", time.Minute)
	require.NoError(t, w.ExecuteTask(ctx, task))

	got, err := s.GetTask(ctx, task.ID)
	require.NoError(t, err)
	branch := taskBranchName(task)
	assert.Equal(t, "completed", got.Status)
	assert.Equal(t, branch, got.Branch)
	assert.Equal(t, "main", got.BaseBranch)
	assert.Equal(t, 42, got.PRNumber)
	require.Len(t, gh.prs, 1)
	assert.Equal(t, got.Branch, gh.prs[0].Head)
	assert.Equal(t, "main", gh.prs[0].Base)
	assert.Contains(t, gh.prs[0].Body, "added handler")
	assert.Contains(t, gh.prs[0].Body, "Verification:")
	assert.Equal(t, 42, gh.getPRCalledFor)
	assert.Equal(t, branch, runGit(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))
	assert.Contains(t, runGit(t, dir, "branch", "-r"), "origin/"+branch)
	assert.Empty(t, gitStatus(t, dir))
}

func TestWorker_ExecuteTask_ResetsOnAgentFailure(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	dir := initGitRepo(t)
	r, err := s.CreateRepo(ctx, "llin", "cttw", dir, "main", "")
	require.NoError(t, err)
	problem, err := s.CreateProblem(ctx, "build API", r.ID)
	require.NoError(t, err)
	task, err := s.CreateTask(ctx, problem.ID, r.ID, "add handler", "implement POST")
	require.NoError(t, err)

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{
			OnPrompt: func(prompt string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.go"), []byte("bad"), 0644))
			},
			Responses: []string{`{"status":"failed","error":"not today"}`},
		}, nil
	}

	w := New(s, ml, &repo.Registry{Root: t.TempDir()}, &mockGH{}, "codex", time.Minute)
	err = w.ExecuteTask(ctx, task)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "task failed")
	assert.NoFileExists(t, filepath.Join(dir, "bad.go"))
	assert.Empty(t, gitStatus(t, dir))
}

func TestWorker_ExecuteTask_ResetsPromptErrorAfterEdits(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	dir := initGitRepo(t)
	r, err := s.CreateRepo(ctx, "llin", "cttw", dir, "main", "")
	require.NoError(t, err)
	problem, err := s.CreateProblem(ctx, "build API", r.ID)
	require.NoError(t, err)
	task, err := s.CreateTask(ctx, problem.ID, r.ID, "add handler", "implement POST")
	require.NoError(t, err)

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{
			OnPrompt: func(prompt string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "partial.go"), []byte("package main\n"), 0644))
			},
		}, nil
	}

	w := New(s, ml, &repo.Registry{Root: t.TempDir()}, &mockGH{}, "codex", time.Minute)
	err = w.ExecuteTask(ctx, task)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "prompt agent")
	assert.NoFileExists(t, filepath.Join(dir, "partial.go"))
	assert.Empty(t, gitStatus(t, dir))
}

func TestWorker_ExecuteTask_ResetsMalformedJSONAfterEdits(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	dir := initGitRepo(t)
	r, err := s.CreateRepo(ctx, "llin", "cttw", dir, "main", "")
	require.NoError(t, err)
	problem, err := s.CreateProblem(ctx, "build API", r.ID)
	require.NoError(t, err)
	task, err := s.CreateTask(ctx, problem.ID, r.ID, "add handler", "implement POST")
	require.NoError(t, err)

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{
			OnPrompt: func(prompt string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "broken.go"), []byte("package main\n"), 0644))
			},
			Responses: []string{`not json`},
		}, nil
	}

	w := New(s, ml, &repo.Registry{Root: t.TempDir()}, &mockGH{}, "codex", time.Minute)
	err = w.ExecuteTask(ctx, task)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse task result")
	assert.NoFileExists(t, filepath.Join(dir, "broken.go"))
	assert.Empty(t, gitStatus(t, dir))
}

func TestWorker_ExecuteTask_CompletesWithRealDiffDespiteEmptyKeyChanges(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	dir := initGitRepo(t)
	r, err := s.CreateRepo(ctx, "llin", "cttw", dir, "main", "")
	require.NoError(t, err)
	problem, err := s.CreateProblem(ctx, "build API", r.ID)
	require.NoError(t, err)
	task, err := s.CreateTask(ctx, problem.ID, r.ID, "add handler", "implement POST")
	require.NoError(t, err)

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{
			OnPrompt: func(prompt string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "handler.go"), []byte("package main\n"), 0644))
			},
			Responses: []string{`{"status":"completed","summary":"nothing changed","key_changes_made":[],"key_learnings":["no-op"],"verification":[]}`},
		}, nil
	}

	w := New(s, ml, &repo.Registry{Root: t.TempDir()}, &mockGH{}, "codex", time.Minute)
	require.NoError(t, w.ExecuteTask(ctx, task))

	got, err := s.GetTask(ctx, task.ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", got.Status)
	assert.Equal(t, 42, got.PRNumber)
	assert.FileExists(t, filepath.Join(dir, "handler.go"))
	assert.Empty(t, gitStatus(t, dir))
}

func TestWorker_ExecuteTask_ResetsWhenCompletedTaskProducesNoDiff(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	dir := initGitRepo(t)
	r, err := s.CreateRepo(ctx, "llin", "cttw", dir, "main", "")
	require.NoError(t, err)
	problem, err := s.CreateProblem(ctx, "build API", r.ID)
	require.NoError(t, err)
	task, err := s.CreateTask(ctx, problem.ID, r.ID, "add handler", "implement POST")
	require.NoError(t, err)

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{
			Responses: []string{`{"status":"completed","summary":"added handler","key_changes_made":["handler"],"key_learnings":[],"verification":["go test ./..."]}`},
		}, nil
	}

	w := New(s, ml, &repo.Registry{Root: t.TempDir()}, &mockGH{}, "codex", time.Minute)
	err = w.ExecuteTask(ctx, task)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no changes")
	assert.Empty(t, gitStatus(t, dir))
}

func TestWorker_ExecuteTask_PreservesWorkspaceOnCommitFailure(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	dir := initGitRepo(t)
	hooks := filepath.Join(dir, ".githooks")
	require.NoError(t, os.MkdirAll(hooks, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte("#!/bin/sh\nexit 1\n"), 0755))
	runGit(t, dir, "config", "core.hooksPath", hooks)

	r, err := s.CreateRepo(ctx, "llin", "cttw", dir, "main", "")
	require.NoError(t, err)
	problem, err := s.CreateProblem(ctx, "build API", r.ID)
	require.NoError(t, err)
	task, err := s.CreateTask(ctx, problem.ID, r.ID, "add handler", "implement POST")
	require.NoError(t, err)

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{
			OnPrompt: func(prompt string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "handler.go"), []byte("package main\n"), 0644))
			},
			Responses: []string{`{"status":"completed","summary":"added handler","key_changes_made":["handler"],"key_learnings":[],"verification":["go test ./..."]}`},
		}, nil
	}

	w := New(s, ml, &repo.Registry{Root: t.TempDir()}, &mockGH{}, "codex", time.Minute)
	err = w.ExecuteTask(ctx, task)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "commit failed")
	assert.FileExists(t, filepath.Join(dir, "handler.go"))
	assert.NotEmpty(t, gitStatus(t, dir))
}

func TestWorker_RunOnce_PreservesCommitFailureAsFailed(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	dir := initGitRepo(t)
	hooks := filepath.Join(dir, ".githooks")
	require.NoError(t, os.MkdirAll(hooks, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte("#!/bin/sh\nexit 1\n"), 0755))
	runGit(t, dir, "config", "core.hooksPath", hooks)

	r, err := s.CreateRepo(ctx, "llin", "cttw", dir, "main", "")
	require.NoError(t, err)
	problem, err := s.CreateProblem(ctx, "build API", r.ID)
	require.NoError(t, err)
	task, err := s.CreateTask(ctx, problem.ID, r.ID, "add handler", "implement POST")
	require.NoError(t, err)

	launches := 0
	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		launches++
		return &launcher.MockAgent{
			OnPrompt: func(prompt string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "handler.go"), []byte("package main\n"), 0644))
			},
			Responses: []string{`{"status":"completed","summary":"added handler","key_changes_made":["handler"],"key_learnings":[],"verification":["go test ./..."]}`},
		}, nil
	}

	w := New(s, ml, &repo.Registry{Root: t.TempDir()}, &mockGH{}, "codex", time.Minute)
	err = w.RunOnce(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "commit failed")

	got, err := s.GetTask(ctx, task.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", got.Status)
	assert.Equal(t, 1, got.Attempts)
	assert.FileExists(t, filepath.Join(dir, "handler.go"))
	assert.NotEmpty(t, gitStatus(t, dir))

	require.NoError(t, w.RunOnce(ctx))
	assert.Equal(t, 1, launches)
}

func TestWorker_RunOnceForRepo_PreservesCommitFailureAsFailed(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	dir := initGitRepo(t)
	hooks := filepath.Join(dir, ".githooks")
	require.NoError(t, os.MkdirAll(hooks, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte("#!/bin/sh\nexit 1\n"), 0755))
	runGit(t, dir, "config", "core.hooksPath", hooks)

	r, err := s.CreateRepo(ctx, "llin", "cttw", dir, "main", "")
	require.NoError(t, err)
	problem, err := s.CreateProblem(ctx, "build API", r.ID)
	require.NoError(t, err)
	task, err := s.CreateTask(ctx, problem.ID, r.ID, "add handler", "implement POST")
	require.NoError(t, err)

	launches := 0
	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		launches++
		return &launcher.MockAgent{
			OnPrompt: func(prompt string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "handler.go"), []byte("package main\n"), 0644))
			},
			Responses: []string{`{"status":"completed","summary":"added handler","key_changes_made":["handler"],"key_learnings":[],"verification":["go test ./..."]}`},
		}, nil
	}

	w := New(s, ml, &repo.Registry{Root: t.TempDir()}, &mockGH{}, "codex", time.Minute)
	err = w.RunOnceForRepo(ctx, r.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "commit failed")

	got, err := s.GetTask(ctx, task.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", got.Status)
	assert.Equal(t, 1, got.Attempts)
	assert.FileExists(t, filepath.Join(dir, "handler.go"))
	assert.NotEmpty(t, gitStatus(t, dir))

	require.NoError(t, w.RunOnceForRepo(ctx, r.ID))
	assert.Equal(t, 1, launches)
}

func TestWorker_ExecuteTask_FailsWhenPullRequestVerificationFails(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	dir := initGitRepo(t)
	r, err := s.CreateRepo(ctx, "llin", "cttw", dir, "main", "")
	require.NoError(t, err)
	problem, err := s.CreateProblem(ctx, "build API", r.ID)
	require.NoError(t, err)
	task, err := s.CreateTask(ctx, problem.ID, r.ID, "add handler", "implement POST")
	require.NoError(t, err)
	task.MaxAttempts = 1
	require.NoError(t, s.UpdateTask(ctx, task))

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{
			OnPrompt: func(prompt string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "handler.go"), []byte("package main\n"), 0644))
			},
			Responses: []string{`{"status":"completed","summary":"added handler","key_changes_made":["handler"],"key_learnings":[],"verification":["go test ./..."]}`},
		}, nil
	}

	gh := &mockGH{getPRError: errors.New("pull request not found")}
	w := New(s, ml, &repo.Registry{Root: t.TempDir()}, gh, "codex", time.Minute)
	require.Error(t, w.RunOnce(ctx))

	got, err := s.GetTask(ctx, task.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", got.Status)
	assert.Contains(t, got.Output, "verify pull request")
	assert.Equal(t, 42, gh.getPRCalledFor)
}

func TestWorker_RunOnce_AttemptCountAfterFailure(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	dir := initGitRepo(t)
	r, err := s.CreateRepo(ctx, "llin", "cttw", dir, "main", "")
	require.NoError(t, err)
	problem, err := s.CreateProblem(ctx, "build API", r.ID)
	require.NoError(t, err)
	task, err := s.CreateTask(ctx, problem.ID, r.ID, "add handler", "implement POST")
	require.NoError(t, err)

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{
			Responses: []string{`{"status":"failed","error":"not today"}`},
		}, nil
	}

	w := New(s, ml, &repo.Registry{Root: t.TempDir()}, &mockGH{}, "codex", time.Minute)

	require.Error(t, w.RunOnce(ctx))
	got, err := s.GetTask(ctx, task.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, got.Attempts)
	assert.Equal(t, "pending", got.Status)

	require.Error(t, w.RunOnce(ctx))
	got, err = s.GetTask(ctx, task.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, got.Attempts)
	assert.Equal(t, "pending", got.Status)

	require.Error(t, w.RunOnce(ctx))
	got, err = s.GetTask(ctx, task.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, got.Attempts)
	assert.Equal(t, "failed", got.Status)
}

func TestWorker_RunOnce_ReturnsUpdateError(t *testing.T) {
	errUpdate := errors.New("update task failed")
	s, err := store.New(":memory:", store.WithUpdateTaskError(errUpdate))
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	dir := initGitRepo(t)
	r, err := s.CreateRepo(ctx, "llin", "cttw", dir, "main", "")
	require.NoError(t, err)
	problem, err := s.CreateProblem(ctx, "build API", r.ID)
	require.NoError(t, err)
	_, err = s.CreateTask(ctx, problem.ID, r.ID, "add handler", "implement POST")
	require.NoError(t, err)

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{
			Responses: []string{`{"status":"failed","error":"not today"}`},
		}, nil
	}

	w := New(s, ml, &repo.Registry{Root: t.TempDir()}, &mockGH{}, "codex", time.Minute)
	err = w.RunOnce(ctx)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUpdate)
	assert.Contains(t, err.Error(), "execute task")
}

func TestWorker_RunOnce_CompletedWithoutDiffFailsTask(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	dir := initGitRepo(t)
	r, err := s.CreateRepo(ctx, "llin", "cttw", dir, "main", "")
	require.NoError(t, err)
	problem, err := s.CreateProblem(ctx, "build API", r.ID)
	require.NoError(t, err)
	task, err := s.CreateTask(ctx, problem.ID, r.ID, "add handler", "implement POST")
	require.NoError(t, err)
	task.MaxAttempts = 1
	require.NoError(t, s.UpdateTask(ctx, task))

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{
			Responses: []string{`{"status":"completed","summary":"added handler","key_changes_made":["handler"],"key_learnings":[],"verification":["go test ./..."]}`},
		}, nil
	}

	w := New(s, ml, &repo.Registry{Root: t.TempDir()}, &mockGH{}, "codex", time.Minute)
	require.Error(t, w.RunOnce(ctx))

	got, err := s.GetTask(ctx, task.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", got.Status)
	assert.Contains(t, got.Output, "no changes")
}

func TestWorker_RunOnceForRepo(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	r1, _ := s.CreateRepo(ctx, "o1", "r1", t.TempDir(), "main", "")
	dir2 := initGitRepo(t)
	r2, _ := s.CreateRepo(ctx, "o2", "r2", dir2, "main", "")
	p1, _ := s.CreateProblem(ctx, "x", r1.ID)
	p2, _ := s.CreateProblem(ctx, "y", r2.ID)
	_, _ = s.CreateTask(ctx, p1.ID, r1.ID, "t1", "d1")
	t2, _ := s.CreateTask(ctx, p2.ID, r2.ID, "t2", "d2")

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{
			OnPrompt: func(prompt string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir2, "task.go"), []byte("package main\n"), 0644))
			},
			Responses: []string{`{"status":"completed","summary":"did task 2","key_changes_made":["task"],"key_learnings":[],"verification":["go test ./..."]}`},
		}, nil
	}
	w := New(s, ml, &repo.Registry{Root: t.TempDir()}, &mockGH{}, "codex", time.Minute)

	require.NoError(t, w.RunOnceForRepo(ctx, r2.ID))

	got, err := s.GetTask(ctx, t2.ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", got.Status)
	assert.Equal(t, 42, got.PRNumber)

	tasks, err := s.ListTasksByProblem(ctx, p1.ID)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, "pending", tasks[0].Status)
}

func TestWorker_ExecuteTask_FailsWhenBranchMismatch(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	dir := initGitRepo(t)
	r, err := s.CreateRepo(ctx, "llin", "cttw", dir, "main", "")
	require.NoError(t, err)
	problem, err := s.CreateProblem(ctx, "build API", r.ID)
	require.NoError(t, err)
	task, err := s.CreateTask(ctx, problem.ID, r.ID, "add handler", "implement POST")
	require.NoError(t, err)
	task.MaxAttempts = 1
	require.NoError(t, s.UpdateTask(ctx, task))

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{
			OnPrompt: func(prompt string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "handler.go"), []byte("package main\n"), 0644))
			},
			Responses: []string{`{"status":"completed","summary":"added handler","key_changes_made":["handler"],"key_learnings":[],"verification":["go test ./..."]}`},
		}, nil
	}

	gh := &mockGH{getPRBranch: "different-branch"}
	w := New(s, ml, &repo.Registry{Root: t.TempDir()}, gh, "codex", time.Minute)
	require.Error(t, w.RunOnce(ctx))

	got, err := s.GetTask(ctx, task.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", got.Status)
	assert.Contains(t, got.Output, "does not match pull request")
	assert.Equal(t, 42, gh.getPRCalledFor)
}
