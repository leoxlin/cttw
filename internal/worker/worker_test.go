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

	"github.com/llin/cttw/internal/gitexec"
	"github.com/llin/cttw/internal/github"
	"github.com/llin/cttw/internal/launcher"
	"github.com/llin/cttw/internal/repo"
	"github.com/llin/cttw/internal/stack"
	"github.com/llin/cttw/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockGH struct{}

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
	return 42, nil
}

func (m *mockGH) GetPullRequest(ctx context.Context, owner, repo string, number int) (*github.PullRequest, error) {
	return &github.PullRequest{Number: number}, nil
}

func (m *mockGH) ListPullRequests(ctx context.Context, owner, repo, head, base string) ([]github.PullRequest, error) {
	return nil, nil
}

func (m *mockGH) UpdatePullRequest(ctx context.Context, owner, repo string, number int, title, body, base string) error {
	return nil
}

type mockStack struct {
	submitCalls [][]stack.Group
	submitError error
}

func (m *mockStack) Submit(ctx context.Context, groups []stack.Group) error {
	m.submitCalls = append(m.submitCalls, groups)
	return m.submitError
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

func createGroupAndTask(ctx context.Context, t *testing.T, s *store.Store, problemID, repoID, title, description string, stackOrder, groupOrder, sequence int) (*store.PRGroup, *store.Task) {
	t.Helper()
	g, err := s.CreatePRGroup(ctx, problemID, repoID, title, description, stackOrder)
	require.NoError(t, err)
	task, err := s.CreateTaskInGroup(ctx, problemID, repoID, g.ID, title, description, groupOrder, sequence)
	require.NoError(t, err)
	return g, task
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

func TestGroupBranchName_SanitizesArbitraryTitles(t *testing.T) {
	testCases := []struct {
		title string
		want  string
	}{
		{title: " Add Handler!!! / POST ", want: "cttw/add-handler-post-12345678"},
		{title: "..", want: "cttw/group-12345678"},
		{title: "feature//subsystem", want: "cttw/feature-subsystem-12345678"},
		{title: ".lock", want: "cttw/lock-12345678"},
		{title: "trailing.", want: "cttw/trailing-12345678"},
		{title: "--leading dash-like segments--", want: "cttw/leading-dash-like-segments-12345678"},
		{title: "", want: "cttw/group-12345678"},
	}

	for _, tc := range testCases {
		group := &store.PRGroup{ID: "1234567890abcdef", Title: tc.title}
		assert.Equal(t, tc.want, groupBranchName(group), tc.title)
	}
}

func TestCommitMessage_PrefersSummary(t *testing.T) {
	task := &store.Task{ID: "task-id-123", Title: "fallback title"}
	out := taskResult{Summary: "add POST handler"}
	assert.Equal(t, "cttw[task-id-]: add POST handler", commitMessage(task, out))
}

func TestCommitMessage_TruncatesTo72CharsIncludingPrefix(t *testing.T) {
	task := &store.Task{ID: "task-id-123", Title: "fallback title"}
	out := taskResult{Summary: strings.Repeat("a", 80)}
	got := commitMessage(task, out)
	assert.Len(t, got, 72)
	assert.Equal(t, "cttw[task-id-]: "+strings.Repeat("a", 56), got)
}

func TestWorker_ExecuteTask_ManagedLifecycleCommitsPushesAndSubmitsStack(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	dir := initGitRepo(t)
	r, err := s.CreateRepo(ctx, "llin", "cttw", dir, "main", "")
	require.NoError(t, err)
	problem, err := s.CreateProblem(ctx, "build API", r.ID)
	require.NoError(t, err)
	group, task := createGroupAndTask(ctx, t, s, problem.ID, r.ID, "add handler", "implement POST", 0, 0, 0)

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{
			OnPrompt: func(prompt string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "handler.go"), []byte("package main\n"), 0644))
			},
			Responses: []string{`{"status":"completed","summary":"added handler","key_changes_made":["handler"],"key_learnings":[],"verification":["go test ./..."]}`},
		}, nil
	}

	ms := &mockStack{}
	w := New(s, ml, &repo.Registry{Root: t.TempDir()}, &mockGH{}, "codex", time.Minute,
		withStackRunnerFactory(func(dir, owner, name, token string) StackRunner { return ms }),
	)
	require.NoError(t, w.ExecuteTask(ctx, task))

	got, err := s.GetTask(ctx, task.ID)
	require.NoError(t, err)
	branch := groupBranchName(group)
	assert.Equal(t, "completed", got.Status)
	assert.Equal(t, "added handler", got.Output)

	gotGroup, err := s.GetPRGroup(ctx, group.ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", gotGroup.Status)
	assert.Equal(t, branch, gotGroup.Branch)
	assert.Equal(t, "main", gotGroup.BaseBranch)

	assert.Equal(t, branch, runGit(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))
	assert.Contains(t, runGit(t, dir, "branch", "-r"), "origin/"+branch)
	assert.Empty(t, gitStatus(t, dir))

	require.Len(t, ms.submitCalls, 1)
	require.Len(t, ms.submitCalls[0], 1)
	assert.Equal(t, branch, ms.submitCalls[0][0].Branch)
	assert.Equal(t, "main", ms.submitCalls[0][0].BaseBranch)
	assert.Equal(t, "add handler", ms.submitCalls[0][0].Title)
}

func TestWorker_ExecuteTask_ManagedLifecycleUsesGitHubTokenForRunner(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	dir := initGitRepo(t)
	r, err := s.CreateRepo(ctx, "llin", "cttw", dir, "main", "")
	require.NoError(t, err)
	problem, err := s.CreateProblem(ctx, "build API", r.ID)
	require.NoError(t, err)
	_, task := createGroupAndTask(ctx, t, s, problem.ID, r.ID, "add handler", "implement POST", 0, 0, 0)

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{
			OnPrompt: func(prompt string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "handler.go"), []byte("package main\n"), 0644))
			},
			Responses: []string{`{"status":"completed","summary":"added handler","key_changes_made":["handler"],"key_learnings":[],"verification":["go test ./..."]}`},
		}, nil
	}

	var gotToken string
	w := New(
		s,
		ml,
		&repo.Registry{Root: t.TempDir()},
		&mockGH{},
		"codex",
		time.Minute,
		WithGitHubToken("gh-token"),
		withGitRunnerFactory(func(dir, token string) gitRunner {
			gotToken = token
			return &gitexec.Runner{Dir: dir, Token: token}
		}),
		withStackRunnerFactory(func(dir, owner, name, token string) StackRunner { return &mockStack{} }),
	)

	require.NoError(t, w.ExecuteTask(ctx, task))
	assert.Equal(t, "gh-token", gotToken)
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
	_, task := createGroupAndTask(ctx, t, s, problem.ID, r.ID, "add handler", "implement POST", 0, 0, 0)

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{
			OnPrompt: func(prompt string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.go"), []byte("bad"), 0644))
			},
			Responses: []string{`{"status":"failed","error":"not today"}`},
		}, nil
	}

	w := New(s, ml, &repo.Registry{Root: t.TempDir()}, &mockGH{}, "codex", time.Minute,
		withStackRunnerFactory(func(dir, owner, name, token string) StackRunner { return &mockStack{} }),
	)
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
	_, task := createGroupAndTask(ctx, t, s, problem.ID, r.ID, "add handler", "implement POST", 0, 0, 0)

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{
			OnPrompt: func(prompt string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "partial.go"), []byte("package main\n"), 0644))
			},
		}, nil
	}

	w := New(s, ml, &repo.Registry{Root: t.TempDir()}, &mockGH{}, "codex", time.Minute,
		withStackRunnerFactory(func(dir, owner, name, token string) StackRunner { return &mockStack{} }),
	)
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
	_, task := createGroupAndTask(ctx, t, s, problem.ID, r.ID, "add handler", "implement POST", 0, 0, 0)

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{
			OnPrompt: func(prompt string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "broken.go"), []byte("package main\n"), 0644))
			},
			Responses: []string{`not json`},
		}, nil
	}

	w := New(s, ml, &repo.Registry{Root: t.TempDir()}, &mockGH{}, "codex", time.Minute,
		withStackRunnerFactory(func(dir, owner, name, token string) StackRunner { return &mockStack{} }),
	)
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
	_, task := createGroupAndTask(ctx, t, s, problem.ID, r.ID, "add handler", "implement POST", 0, 0, 0)

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{
			OnPrompt: func(prompt string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "handler.go"), []byte("package main\n"), 0644))
			},
			Responses: []string{`{"status":"completed","summary":"nothing changed","key_changes_made":[],"key_learnings":["no-op"],"verification":[]}`},
		}, nil
	}

	w := New(s, ml, &repo.Registry{Root: t.TempDir()}, &mockGH{}, "codex", time.Minute,
		withStackRunnerFactory(func(dir, owner, name, token string) StackRunner { return &mockStack{} }),
	)
	require.NoError(t, w.ExecuteTask(ctx, task))

	got, err := s.GetTask(ctx, task.ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", got.Status)
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
	_, task := createGroupAndTask(ctx, t, s, problem.ID, r.ID, "add handler", "implement POST", 0, 0, 0)

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{
			Responses: []string{`{"status":"completed","summary":"added handler","key_changes_made":["handler"],"key_learnings":[],"verification":["go test ./..."]}`},
		}, nil
	}

	w := New(s, ml, &repo.Registry{Root: t.TempDir()}, &mockGH{}, "codex", time.Minute,
		withStackRunnerFactory(func(dir, owner, name, token string) StackRunner { return &mockStack{} }),
	)
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
	hooks := filepath.Join(t.TempDir(), "githooks")
	require.NoError(t, os.MkdirAll(hooks, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte("#!/bin/sh\nexit 1\n"), 0755))
	runGit(t, dir, "config", "core.hooksPath", hooks)

	r, err := s.CreateRepo(ctx, "llin", "cttw", dir, "main", "")
	require.NoError(t, err)
	problem, err := s.CreateProblem(ctx, "build API", r.ID)
	require.NoError(t, err)
	_, task := createGroupAndTask(ctx, t, s, problem.ID, r.ID, "add handler", "implement POST", 0, 0, 0)

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{
			OnPrompt: func(prompt string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "handler.go"), []byte("package main\n"), 0644))
			},
			Responses: []string{`{"status":"completed","summary":"added handler","key_changes_made":["handler"],"key_learnings":[],"verification":["go test ./..."]}`},
		}, nil
	}

	w := New(s, ml, &repo.Registry{Root: t.TempDir()}, &mockGH{}, "codex", time.Minute,
		withStackRunnerFactory(func(dir, owner, name, token string) StackRunner { return &mockStack{} }),
	)
	err = w.ExecuteTask(ctx, task)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "commit failed")
	assert.FileExists(t, filepath.Join(dir, "handler.go"))
	assert.NotEmpty(t, gitStatus(t, dir))
}

func TestWorker_ExecuteTask_FailsNewTaskWhenOtherBranchIsDirty(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	dir := initGitRepo(t)
	r, err := s.CreateRepo(ctx, "llin", "cttw", dir, "main", "")
	require.NoError(t, err)

	problemA, err := s.CreateProblem(ctx, "build API", r.ID)
	require.NoError(t, err)
	groupA, taskA := createGroupAndTask(ctx, t, s, problemA.ID, r.ID, "first task", "break commit", 0, 0, 0)
	branchA := groupBranchName(groupA)
	taskA.Branch = branchA
	taskA.Status = "completed"
	taskA.Output = "done"
	require.NoError(t, s.UpdateTask(ctx, taskA))
	groupA.Branch = branchA
	groupA.Status = "failed"
	groupA.Output = "left branch dirty"
	require.NoError(t, s.UpdatePRGroup(ctx, groupA))

	runGit(t, dir, "checkout", "-b", branchA, "main")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "handler.go"), []byte("package main\n"), 0644))
	require.NotEmpty(t, gitStatus(t, dir))

	problemB, err := s.CreateProblem(ctx, "build UI", r.ID)
	require.NoError(t, err)
	_, taskB := createGroupAndTask(ctx, t, s, problemB.ID, r.ID, "second task", "should not start", 0, 0, 1)
	taskB.MaxAttempts = 1
	require.NoError(t, s.UpdateTask(ctx, taskB))

	w := New(s, &launcher.MockLauncher{}, &repo.Registry{Root: t.TempDir()}, &mockGH{}, "codex", time.Minute,
		withStackRunnerFactory(func(dir, owner, name, token string) StackRunner { return &mockStack{} }),
	)
	err = w.RunOnce(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace is dirty")
	assert.Contains(t, err.Error(), branchA)
	assert.NotEmpty(t, gitStatus(t, dir))

	got, err := s.GetTask(ctx, taskB.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", got.Status)
	assert.Contains(t, got.Output, "workspace is dirty")
}

func TestWorker_RunOnce_PreservesCommitFailureAsFailed(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	dir := initGitRepo(t)
	hooks := filepath.Join(t.TempDir(), "githooks")
	require.NoError(t, os.MkdirAll(hooks, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte("#!/bin/sh\nexit 1\n"), 0755))
	runGit(t, dir, "config", "core.hooksPath", hooks)

	r, err := s.CreateRepo(ctx, "llin", "cttw", dir, "main", "")
	require.NoError(t, err)
	problem, err := s.CreateProblem(ctx, "build API", r.ID)
	require.NoError(t, err)
	_, task := createGroupAndTask(ctx, t, s, problem.ID, r.ID, "add handler", "implement POST", 0, 0, 0)

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

	w := New(s, ml, &repo.Registry{Root: t.TempDir()}, &mockGH{}, "codex", time.Minute,
		withStackRunnerFactory(func(dir, owner, name, token string) StackRunner { return &mockStack{} }),
	)
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
	hooks := filepath.Join(t.TempDir(), "githooks")
	require.NoError(t, os.MkdirAll(hooks, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte("#!/bin/sh\nexit 1\n"), 0755))
	runGit(t, dir, "config", "core.hooksPath", hooks)

	r, err := s.CreateRepo(ctx, "llin", "cttw", dir, "main", "")
	require.NoError(t, err)
	problem, err := s.CreateProblem(ctx, "build API", r.ID)
	require.NoError(t, err)
	_, task := createGroupAndTask(ctx, t, s, problem.ID, r.ID, "add handler", "implement POST", 0, 0, 0)

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

	w := New(s, ml, &repo.Registry{Root: t.TempDir()}, &mockGH{}, "codex", time.Minute,
		withStackRunnerFactory(func(dir, owner, name, token string) StackRunner { return &mockStack{} }),
	)
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
	_, task := createGroupAndTask(ctx, t, s, problem.ID, r.ID, "add handler", "implement POST", 0, 0, 0)

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{
			Responses: []string{`{"status":"failed","error":"not today"}`},
		}, nil
	}

	w := New(s, ml, &repo.Registry{Root: t.TempDir()}, &mockGH{}, "codex", time.Minute,
		withStackRunnerFactory(func(dir, owner, name, token string) StackRunner { return &mockStack{} }),
	)

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
	_, _ = createGroupAndTask(ctx, t, s, problem.ID, r.ID, "add handler", "implement POST", 0, 0, 0)

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{
			Responses: []string{`{"status":"failed","error":"not today"}`},
		}, nil
	}

	w := New(s, ml, &repo.Registry{Root: t.TempDir()}, &mockGH{}, "codex", time.Minute,
		withStackRunnerFactory(func(dir, owner, name, token string) StackRunner { return &mockStack{} }),
	)
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
	_, task := createGroupAndTask(ctx, t, s, problem.ID, r.ID, "add handler", "implement POST", 0, 0, 0)
	task.MaxAttempts = 1
	require.NoError(t, s.UpdateTask(ctx, task))

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{
			Responses: []string{`{"status":"completed","summary":"added handler","key_changes_made":["handler"],"key_learnings":[],"verification":["go test ./..."]}`},
		}, nil
	}

	w := New(s, ml, &repo.Registry{Root: t.TempDir()}, &mockGH{}, "codex", time.Minute,
		withStackRunnerFactory(func(dir, owner, name, token string) StackRunner { return &mockStack{} }),
	)
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
	_, _ = createGroupAndTask(ctx, t, s, p1.ID, r1.ID, "t1", "d1", 0, 0, 0)
	_, t2 := createGroupAndTask(ctx, t, s, p2.ID, r2.ID, "t2", "d2", 0, 0, 0)

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{
			OnPrompt: func(prompt string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir2, "task.go"), []byte("package main\n"), 0644))
			},
			Responses: []string{`{"status":"completed","summary":"did task 2","key_changes_made":["task"],"key_learnings":[],"verification":["go test ./..."]}`},
		}, nil
	}
	w := New(s, ml, &repo.Registry{Root: t.TempDir()}, &mockGH{}, "codex", time.Minute,
		withStackRunnerFactory(func(dir, owner, name, token string) StackRunner { return &mockStack{} }),
	)

	require.NoError(t, w.RunOnceForRepo(ctx, r2.ID))

	got, err := s.GetTask(ctx, t2.ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", got.Status)

	tasks, err := s.ListTasksByProblem(ctx, p1.ID)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, "pending", tasks[0].Status)
}

func TestWorker_GroupsStackCommitsAndWaitsForPreviousGroup(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	dir := initGitRepo(t)
	r, err := s.CreateRepo(ctx, "llin", "cttw", dir, "main", "")
	require.NoError(t, err)
	problem, err := s.CreateProblem(ctx, "build API", r.ID)
	require.NoError(t, err)

	group1, err := s.CreatePRGroup(ctx, problem.ID, r.ID, "group one", "first group", 0)
	require.NoError(t, err)
	task1, err := s.CreateTaskInGroup(ctx, problem.ID, r.ID, group1.ID, "task one", "", 0, 0)
	require.NoError(t, err)
	task2, err := s.CreateTaskInGroup(ctx, problem.ID, r.ID, group1.ID, "task two", "", 1, 1)
	require.NoError(t, err)
	group2, err := s.CreatePRGroup(ctx, problem.ID, r.ID, "group two", "second group", 1)
	require.NoError(t, err)
	task3, err := s.CreateTaskInGroup(ctx, problem.ID, r.ID, group2.ID, "task three", "", 0, 2)
	require.NoError(t, err)

	branch1 := groupBranchName(group1)
	branch2 := groupBranchName(group2)

	launches := 0
	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		launches++
		return &launcher.MockAgent{
			OnPrompt: func(prompt string) {
				file := filepath.Join(dir, "task"+string(rune('0'+launches))+".go")
				require.NoError(t, os.WriteFile(file, []byte("package main\n"), 0644))
			},
			Responses: []string{`{"status":"completed","summary":"did work","key_changes_made":["work"],"key_learnings":[],"verification":["go test ./..."]}`},
		}, nil
	}

	ms := &mockStack{}
	w := New(s, ml, &repo.Registry{Root: t.TempDir()}, &mockGH{}, "codex", time.Minute,
		withStackRunnerFactory(func(dir, owner, name, token string) StackRunner { return ms }),
	)

	// First run: task1 commits to group1 branch, group1 stays incomplete.
	require.NoError(t, w.RunOnce(ctx))
	assert.Equal(t, 1, launches)

	got1, err := s.GetTask(ctx, task1.ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", got1.Status)

	gotG1, err := s.GetPRGroup(ctx, group1.ID)
	require.NoError(t, err)
	assert.Equal(t, "pending", gotG1.Status)
	assert.Equal(t, branch1, gotG1.Branch)

	// Second run: task2 commits to group1 branch, group1 completes and pushes.
	require.NoError(t, w.RunOnce(ctx))
	assert.Equal(t, 2, launches)

	gotG1, err = s.GetPRGroup(ctx, group1.ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", gotG1.Status)
	assert.Contains(t, runGit(t, dir, "branch", "-r"), "origin/"+branch1)

	// Third run: task3 starts group2 branch based on group1 branch.
	require.NoError(t, w.RunOnce(ctx))
	assert.Equal(t, 3, launches)

	got3, err := s.GetTask(ctx, task3.ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", got3.Status)

	gotG2, err := s.GetPRGroup(ctx, group2.ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", gotG2.Status)
	assert.Equal(t, branch1, gotG2.BaseBranch)
	assert.Equal(t, group1.ID, gotG2.BaseGroupID)
	assert.Contains(t, runGit(t, dir, "branch", "-r"), "origin/"+branch2)

	// Both group branches were initialized and the stack was submitted.
	require.Len(t, ms.submitCalls, 1)
	require.Len(t, ms.submitCalls[0], 2)
	assert.Equal(t, branch1, ms.submitCalls[0][0].Branch)
	assert.Equal(t, "main", ms.submitCalls[0][0].BaseBranch)
	assert.Equal(t, "group one", ms.submitCalls[0][0].Title)
	assert.Equal(t, branch2, ms.submitCalls[0][1].Branch)
	assert.Equal(t, branch1, ms.submitCalls[0][1].BaseBranch)
	assert.Equal(t, "group two", ms.submitCalls[0][1].Title)

	// Verify group1 has two commits from the two tasks.
	logOut := runGit(t, dir, "log", "--pretty=%s", branch1)
	assert.Contains(t, logOut, taskCommitPrefix(task1))
	assert.Contains(t, logOut, taskCommitPrefix(task2))

	// Verify task3 commit is on group2 branch.
	logOut2 := runGit(t, dir, "log", "--pretty=%s", branch2)
	assert.Contains(t, logOut2, taskCommitPrefix(task3))
	assert.Empty(t, gitStatus(t, dir))
}
