package worker

import (
	"context"
	"testing"

	"github.com/llin/cttw/internal/launcher"
	"github.com/llin/cttw/internal/repo"
	"github.com/llin/cttw/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockGH struct {
	prs []struct{ Head, Base string }
}

func (m *mockGH) CreateIssue(ctx context.Context, owner, repo, title, body string) (int, error) { return 0, nil }
func (m *mockGH) CreateSubIssue(ctx context.Context, owner, repo string, parentNumber, childNumber int) error { return nil }
func (m *mockGH) CreateBranch(ctx context.Context, owner, repo, branch, base string) error { return nil }
func (m *mockGH) CreatePullRequest(ctx context.Context, owner, repo, title, body, head, base string) (int, error) {
	m.prs = append(m.prs, struct{ Head, Base string }{head, base})
	return 42, nil
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

	gh := &mockGH{}
	w := New(s, ml, &repo.Registry{Root: t.TempDir()}, gh, "codex")
	require.NoError(t, w.ExecuteTask(ctx, task))

	got, err := s.GetTask(ctx, task.ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", got.Status)
	assert.Equal(t, 42, got.PRNumber)
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

	gh := &mockGH{}
	w := New(s, ml, &repo.Registry{Root: t.TempDir()}, gh, "codex")
	require.NoError(t, w.ExecuteTask(ctx, task))

	got, err := s.GetTask(ctx, task.ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", got.Status)
	assert.Equal(t, 42, got.PRNumber)
	assert.Equal(t, "feat/add-handler", got.Branch)
}
