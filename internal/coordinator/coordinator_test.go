package coordinator

import (
	"context"
	"testing"

	"github.com/llin/cttw/internal/launcher"
	"github.com/llin/cttw/internal/repo"
	"github.com/llin/cttw/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockGitHub struct {
	issues    map[string]int
	subIssues [][2]int
}

func (m *mockGitHub) CreateIssue(ctx context.Context, owner, repo, title, body string) (int, error) {
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

	coord := New(s, ml, &repo.Registry{Root: t.TempDir()}, &mockGitHub{issues: make(map[string]int)}, "codex")
	problem, err := coord.CreateProblem(ctx, "llin", "cttw", "build the API")
	require.NoError(t, err)
	assert.Equal(t, "ready", problem.Status)

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

	coord := New(s, ml, &repo.Registry{Root: t.TempDir()}, &mockGitHub{issues: make(map[string]int)}, "codex")
	problem, err := coord.CreateProblem(ctx, "llin", "cttw", "build the API")
	require.NoError(t, err)
	assert.Equal(t, "ready", problem.Status)
	assert.Greater(t, problem.ParentIssueNumber, 0)

	tasks, err := s.ListTasksByProblem(ctx, problem.ID)
	require.NoError(t, err)
	require.Len(t, tasks, 2)
	assert.Equal(t, "add handler", tasks[0].Title)
	assert.Equal(t, "implement POST /api/tasks", tasks[0].Description)
	assert.Equal(t, "pending", tasks[0].Status)
	assert.Equal(t, r.ID, tasks[0].RepoID)
}
