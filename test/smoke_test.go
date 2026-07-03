package test

import (
	"context"
	"testing"

	"github.com/llin/cttw/internal/coordinator"
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

type smokeLLM struct{}

func (s *smokeLLM) Chat(ctx context.Context, system, user string) (string, error) {
	return `[{"title":"add test","description":"add a smoke test"}]`, nil
}

func TestSmoke_CreateTask(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	coord := &coordinator.Coordinator{
		LLM:   &smokeLLM{},
		GH:    &smokeGH{},
		Store: s,
		Owner: "o",
		Repo:  "r",
	}
	task, err := coord.StartTask(context.Background(), "add smoke test")
	require.NoError(t, err)
	assert.Equal(t, "add smoke test", task.Description)

	chunks, err := s.ListChunksByTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Len(t, chunks, 1)
}
