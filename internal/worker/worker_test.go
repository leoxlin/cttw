package worker

import (
	"context"
	"errors"
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
	prs            []struct{ Head, Base string }
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
	m.prs = append(m.prs, struct{ Head, Base string }{head, base})
	return 42, nil
}
func (m *mockGH) GetPullRequest(ctx context.Context, owner, repo string, number int) (*github.PullRequest, error) {
	m.getPRCalledFor = number
	if m.getPRError != nil {
		return nil, m.getPRError
	}
	return &github.PullRequest{Number: number, Head: struct {
		Ref string `json:"ref"`
	}{Ref: m.getPRBranch}}, nil
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

func TestParseTaskResult_RejectsLegacyPRFieldsAsCompletionContract(t *testing.T) {
	out, err := parseTaskResult(`{"status":"completed","pr_number":42,"branch":"feat/x"}`)
	require.NoError(t, err)
	assert.Equal(t, "completed", out.Status)
	assert.Empty(t, out.Summary)
	assert.Empty(t, out.Error)
	assert.Empty(t, out.KeyChanges)
	assert.Empty(t, out.KeyLearnings)
	assert.Empty(t, out.Verification)
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

func TestWorker_ExecuteTask_BracketsInStrings(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	r, err := s.CreateRepo(ctx, "llin", "cttw", t.TempDir(), "main", "")
	require.NoError(t, err)
	problem, err := s.CreateProblem(ctx, "build API", r.ID)
	require.NoError(t, err)
	task, err := s.CreateTask(ctx, problem.ID, r.ID, "add handler", "implement POST")
	require.NoError(t, err)

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{
			Responses: []string{`{"status":"completed","pr_number":42,"branch":"feat/add-handler","error":"]}{["}`},
		}, nil
	}

	gh := &mockGH{getPRBranch: "feat/add-handler"}
	w := New(s, ml, &repo.Registry{Root: t.TempDir()}, gh, "codex", time.Minute)
	require.NoError(t, w.ExecuteTask(ctx, task))

	got, err := s.GetTask(ctx, task.ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", got.Status)
	assert.Equal(t, 42, got.PRNumber)
	assert.Equal(t, 42, gh.getPRCalledFor)
}

func TestWorker_ExecuteTask(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	r, err := s.CreateRepo(ctx, "llin", "cttw", t.TempDir(), "main", "")
	require.NoError(t, err)
	problem, err := s.CreateProblem(ctx, "build API", r.ID)
	require.NoError(t, err)
	task, err := s.CreateTask(ctx, problem.ID, r.ID, "add handler", "implement POST")
	require.NoError(t, err)

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{
			Responses: []string{`{"pr_number":42,"branch":"feat/add-handler","status":"completed"}`},
		}, nil
	}

	gh := &mockGH{getPRBranch: "feat/add-handler"}
	w := New(s, ml, &repo.Registry{Root: t.TempDir()}, gh, "codex", time.Minute)
	require.NoError(t, w.ExecuteTask(ctx, task))

	got, err := s.GetTask(ctx, task.ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", got.Status)
	assert.Equal(t, 42, got.PRNumber)
	assert.Equal(t, "feat/add-handler", got.Branch)
	assert.Equal(t, 42, gh.getPRCalledFor)
}

func TestWorker_RunOnce_AttemptCountAfterFailure(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	r, err := s.CreateRepo(ctx, "llin", "cttw", t.TempDir(), "main", "")
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

	gh := &mockGH{}
	w := New(s, ml, &repo.Registry{Root: t.TempDir()}, gh, "codex", time.Minute)

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
	r, err := s.CreateRepo(ctx, "llin", "cttw", t.TempDir(), "main", "")
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

	gh := &mockGH{}
	w := New(s, ml, &repo.Registry{Root: t.TempDir()}, gh, "codex", time.Minute)

	err = w.RunOnce(ctx)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUpdate)
	assert.Contains(t, err.Error(), "execute task")
}

func TestWorker_RunOnce_MissingCompletedFields(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	r, err := s.CreateRepo(ctx, "llin", "cttw", t.TempDir(), "main", "")
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
			Responses: []string{`{"status":"completed"}`},
		}, nil
	}

	gh := &mockGH{}
	w := New(s, ml, &repo.Registry{Root: t.TempDir()}, gh, "codex", time.Minute)
	require.Error(t, w.RunOnce(ctx))

	got, err := s.GetTask(ctx, task.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", got.Status)
	assert.Contains(t, got.Output, "missing pr_number or branch")
}

func TestWorker_ExecuteTask_FailsWhenPullRequestVerificationFails(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	r, err := s.CreateRepo(ctx, "llin", "cttw", t.TempDir(), "main", "")
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
			Responses: []string{`{"pr_number":42,"branch":"feat/add-handler","status":"completed"}`},
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

func TestWorker_RunOnceForRepo(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	r1, _ := s.CreateRepo(ctx, "o1", "r1", t.TempDir(), "main", "")
	r2, _ := s.CreateRepo(ctx, "o2", "r2", t.TempDir(), "main", "")
	p1, _ := s.CreateProblem(ctx, "x", r1.ID)
	p2, _ := s.CreateProblem(ctx, "y", r2.ID)
	_, _ = s.CreateTask(ctx, p1.ID, r1.ID, "t1", "d1")
	t2, _ := s.CreateTask(ctx, p2.ID, r2.ID, "t2", "d2")

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{Responses: []string{`{"status":"completed","pr_number":42,"branch":"feat/t2"}`}}, nil
	}
	gh := &mockGH{getPRBranch: "feat/t2"}
	w := New(s, ml, &repo.Registry{Root: t.TempDir()}, gh, "codex", time.Minute)

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
	r, err := s.CreateRepo(ctx, "llin", "cttw", t.TempDir(), "main", "")
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
			Responses: []string{`{"pr_number":42,"branch":"feat/add-handler","status":"completed"}`},
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
