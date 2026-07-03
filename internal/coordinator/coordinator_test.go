package coordinator

import (
	"context"
	"testing"

	"github.com/llin/cttw/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeGH struct{ issueNum int }

func (f *fakeGH) CreateIssue(ctx context.Context, owner, repo, title, body string) (int, error) {
	f.issueNum++
	return f.issueNum, nil
}
func (f *fakeGH) CreateSubIssue(ctx context.Context, owner, repo string, parentNumber, childNumber int) error {
	return nil
}
func (f *fakeGH) CreateBranch(ctx context.Context, owner, repo, branch, base string) error {
	return nil
}
func (f *fakeGH) CreatePullRequest(ctx context.Context, owner, repo, title, body, head, base string) (int, error) {
	return 0, nil
}

type fakeLLM struct{ resp string }

func (f *fakeLLM) Chat(ctx context.Context, system, user string) (string, error) { return f.resp, nil }

func TestCoordinator_StartTask(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	resp := `[{"title":"c1","description":"d1"}]`
	c := &Coordinator{LLM: &fakeLLM{resp: resp}, GH: &fakeGH{}, Store: s, Owner: "o", Repo: "r"}
	ctx := context.Background()

	task, err := c.StartTask(ctx, "build API")
	require.NoError(t, err)
	assert.Equal(t, "build API", task.Description)

	chunks, err := s.ListChunksByTask(ctx, task.ID)
	require.NoError(t, err)
	assert.Len(t, chunks, 1)
	assert.Equal(t, "c1", chunks[0].Title)
}
