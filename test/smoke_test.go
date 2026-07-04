package test

import (
	"context"
	"testing"

	"github.com/llin/cttw/internal/coordinator"
	"github.com/llin/cttw/internal/launcher"
	"github.com/llin/cttw/internal/repo"
	"github.com/llin/cttw/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type smokeGH struct{ issueCount int }

func (s *smokeGH) CreateIssue(ctx context.Context, owner, repo, title, body string) (int, error) {
	s.issueCount++
	return s.issueCount, nil
}
func (s *smokeGH) CreateSubIssue(ctx context.Context, owner, repo string, parentNumber, childNumber int) error {
	return nil
}
func (s *smokeGH) CreateBranch(ctx context.Context, owner, repo, branch, base string) error {
	return nil
}
func (s *smokeGH) CreatePullRequest(ctx context.Context, owner, repo, title, body, head, base string) (int, error) {
	return 1, nil
}

func TestSmoke_CreateProblem(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	_, err = s.CreateRepo(ctx, "o", "r", "/tmp/r", "main", "")
	require.NoError(t, err)

	ml := &launcher.MockLauncher{
		OnLaunch: func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
			return &launcher.MockAgent{
				Responses: []string{`[{"title":"add test","description":"add a smoke test"}]`},
			}, nil
		},
	}

	coord := coordinator.New(s, ml, &repo.Registry{Root: "/tmp/repos"}, &smokeGH{})
	problem, err := coord.CreateProblem(ctx, "o", "r", "add smoke test")
	require.NoError(t, err)
	assert.Equal(t, "add smoke test", problem.Description)
	assert.Equal(t, "ready", problem.Status)

	tasks, err := s.ListTasksByProblem(ctx, problem.ID)
	require.NoError(t, err)
	assert.Len(t, tasks, 1)
}
