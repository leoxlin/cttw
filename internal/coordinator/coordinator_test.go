package coordinator

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

type mockGitHub struct {
	issues        map[string]int
	subIssues     [][2]int
	failNextIssue bool
}

func (m *mockGitHub) CreateIssue(ctx context.Context, owner, repo, title, body string) (int, error) {
	if m.failNextIssue {
		return 0, errors.New("github create issue failed")
	}
	m.issues[title] = len(m.issues) + 1
	return m.issues[title], nil
}

func (m *mockGitHub) CreateSubIssue(ctx context.Context, owner, repo string, parentNumber, childNumber int) error {
	m.subIssues = append(m.subIssues, [2]int{parentNumber, childNumber})
	return nil
}

func (m *mockGitHub) CreateBranch(ctx context.Context, owner, repo, branch, base string) error {
	return nil
}

func (m *mockGitHub) CreatePullRequest(ctx context.Context, owner, repo, title, body, head, base string) (int, error) {
	return 0, nil
}
func (m *mockGitHub) GetPullRequest(ctx context.Context, owner, repo string, number int) (*github.PullRequest, error) {
	return nil, nil
}

func waitForProblemStatus(t *testing.T, ctx context.Context, s *store.Store, id, want string) *store.Problem {
	t.Helper()
	var p *store.Problem
	require.Eventually(t, func() bool {
		var err error
		p, err = s.GetProblem(ctx, id)
		if err != nil {
			return false
		}
		return p.Status == want
	}, 2*time.Second, 10*time.Millisecond)
	return p
}

func TestCoordinator_CreateProblem_BracketsInStrings(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	_, err = s.CreateRepo(ctx, "llin", "cttw", t.TempDir(), "main", "")
	require.NoError(t, err)

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{
			Responses: []string{`[{"title":"a]b","description":"d1"},{"title":"c}d","description":"d2"}]`},
		}, nil
	}

	coord := New(s, ml, &repo.Registry{Root: t.TempDir()}, &mockGitHub{issues: make(map[string]int)}, "codex", time.Minute)
	problem, err := coord.CreateProblem(ctx, "llin", "cttw", "build the API")
	require.NoError(t, err)
	assert.Equal(t, "pending", problem.Status)

	problem = waitForProblemStatus(t, ctx, s, problem.ID, "ready")
	tasks, err := s.ListTasksByProblem(ctx, problem.ID)
	require.NoError(t, err)
	require.Len(t, tasks, 2)
	assert.Equal(t, "a]b", tasks[0].Title)
	assert.Equal(t, "c}d", tasks[1].Title)
}

func TestCoordinator_CreateProblem(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	// Seed repo record.
	ctx := context.Background()
	r, err := s.CreateRepo(ctx, "llin", "cttw", t.TempDir(), "main", "")
	require.NoError(t, err)

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{
			Responses: []string{`[{"title":"add handler","description":"implement POST /api/tasks"},{"title":"add tests","description":"write unit tests"}]`},
		}, nil
	}

	coord := New(s, ml, &repo.Registry{Root: t.TempDir()}, &mockGitHub{issues: make(map[string]int)}, "codex", time.Minute)
	problem, err := coord.CreateProblem(ctx, "llin", "cttw", "build the API")
	require.NoError(t, err)
	assert.Equal(t, "pending", problem.Status)

	problem = waitForProblemStatus(t, ctx, s, problem.ID, "ready")
	assert.Greater(t, problem.ParentIssueNumber, 0)

	tasks, err := s.ListTasksByProblem(ctx, problem.ID)
	require.NoError(t, err)
	require.Len(t, tasks, 2)
	assert.Equal(t, "add handler", tasks[0].Title)
	assert.Equal(t, "implement POST /api/tasks", tasks[0].Description)
	assert.Equal(t, "pending", tasks[0].Status)
	assert.Equal(t, r.ID, tasks[0].RepoID)
}

func TestCoordinator_CreateProblem_AsyncSuccess(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	_, err = s.CreateRepo(ctx, "llin", "cttw", t.TempDir(), "main", "")
	require.NoError(t, err)

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{
			Responses: []string{`[{"title":"t1","description":"d1"}]`},
		}, nil
	}

	coord := New(s, ml, &repo.Registry{Root: t.TempDir()}, &mockGitHub{issues: make(map[string]int)}, "codex", time.Minute)
	problem, err := coord.CreateProblem(ctx, "llin", "cttw", "build the API")
	require.NoError(t, err)
	assert.Equal(t, "pending", problem.Status)

	problem = waitForProblemStatus(t, ctx, s, problem.ID, "ready")
	tasks, err := s.ListTasksByProblem(ctx, problem.ID)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, "t1", tasks[0].Title)
}

func TestCoordinator_CreateProblem_EmptyTasks(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	_, err = s.CreateRepo(ctx, "llin", "cttw", t.TempDir(), "main", "")
	require.NoError(t, err)

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{Responses: []string{"[]"}}, nil
	}

	coord := New(s, ml, &repo.Registry{Root: t.TempDir()}, &mockGitHub{issues: make(map[string]int)}, "codex", time.Minute)
	problem, err := coord.CreateProblem(ctx, "llin", "cttw", "build the API")
	require.NoError(t, err)
	assert.Equal(t, "pending", problem.Status)

	waitForProblemStatus(t, ctx, s, problem.ID, "failed")
}

func TestCoordinator_CreateProblem_AsyncFailure(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	_, err = s.CreateRepo(ctx, "llin", "cttw", t.TempDir(), "main", "")
	require.NoError(t, err)

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{
			Responses: []string{`[{"title":"t1","description":"d1"}]`},
		}, nil
	}

	gh := &mockGitHub{issues: make(map[string]int), failNextIssue: true}
	coord := New(s, ml, &repo.Registry{Root: t.TempDir()}, gh, "codex", time.Minute)
	problem, err := coord.CreateProblem(ctx, "llin", "cttw", "build the API")
	require.NoError(t, err)
	assert.Equal(t, "pending", problem.Status)

	problem = waitForProblemStatus(t, ctx, s, problem.ID, "failed")
	assert.Equal(t, 0, problem.ParentIssueNumber)

	tasks, err := s.ListTasksByProblem(ctx, problem.ID)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, "failed", tasks[0].Status)
}

func TestCoordinator_CreateProblem_MarksFailedOnGitHubError(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	_, err = s.CreateRepo(ctx, "llin", "cttw", t.TempDir(), "main", "")
	require.NoError(t, err)

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{Responses: []string{`[{"title":"t1","description":"d1"}]`}}, nil
	}

	gh := &mockGitHub{issues: make(map[string]int), failNextIssue: true}
	coord := New(s, ml, &repo.Registry{Root: t.TempDir()}, gh, "codex", time.Minute)
	problem, err := coord.CreateProblem(ctx, "llin", "cttw", "build the API")
	require.NoError(t, err)

	problem = waitForProblemStatus(t, ctx, s, problem.ID, "failed")

	tasks, err := s.ListTasksByProblem(ctx, problem.ID)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, "failed", tasks[0].Status)
	assert.Equal(t, 0, problem.ParentIssueNumber)
}

func TestCoordinator_CreateProblem_MarksFailedOnUpdateAfterIssueCreation(t *testing.T) {
	errUpdate := errors.New("update problem failed")
	failOnce := true
	s, err := store.New(":memory:", store.WithUpdateProblemErrorFunc(func() error {
		if failOnce {
			failOnce = false
			return errUpdate
		}
		return nil
	}))
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	_, err = s.CreateRepo(ctx, "llin", "cttw", t.TempDir(), "main", "")
	require.NoError(t, err)

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{Responses: []string{`[{"title":"t1","description":"d1"}]`}}, nil
	}

	gh := &mockGitHub{issues: make(map[string]int)}
	coord := New(s, ml, &repo.Registry{Root: t.TempDir()}, gh, "codex", time.Minute)
	problem, err := coord.CreateProblem(ctx, "llin", "cttw", "build the API")
	require.NoError(t, err)

	problem = waitForProblemStatus(t, ctx, s, problem.ID, "failed")
	assert.Greater(t, problem.ParentIssueNumber, 0)
}

func TestCoordinator_CreateProblem_MarksFailedOnTaskUpdateAfterChildIssueCreation(t *testing.T) {
	errUpdate := errors.New("update task failed")
	s, err := store.New(":memory:", store.WithUpdateTaskError(errUpdate))
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	_, err = s.CreateRepo(ctx, "llin", "cttw", t.TempDir(), "main", "")
	require.NoError(t, err)

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{Responses: []string{`[{"title":"t1","description":"d1"}]`}}, nil
	}

	gh := &mockGitHub{issues: make(map[string]int)}
	coord := New(s, ml, &repo.Registry{Root: t.TempDir()}, gh, "codex", time.Minute)
	problem, err := coord.CreateProblem(ctx, "llin", "cttw", "build the API")
	require.NoError(t, err)

	problem = waitForProblemStatus(t, ctx, s, problem.ID, "failed")
	assert.Greater(t, problem.ParentIssueNumber, 0)

	tasks, err := s.ListTasksByProblem(ctx, problem.ID)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, "failed", tasks[0].Status)
}
