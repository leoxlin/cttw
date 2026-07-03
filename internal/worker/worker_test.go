package worker

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/llin/cttw/internal/gitexec"
	"github.com/llin/cttw/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeGH struct{}

func (f *fakeGH) CreateIssue(ctx context.Context, owner, repo, title, body string) (int, error) { return 1, nil }
func (f *fakeGH) CreateSubIssue(ctx context.Context, owner, repo string, parentNumber, childNumber int) error {
	return nil
}
func (f *fakeGH) CreateBranch(ctx context.Context, owner, repo, branch, base string) error { return nil }
func (f *fakeGH) CreatePullRequest(ctx context.Context, owner, repo, title, body, head, base string) (int, error) {
	return 42, nil
}

func TestWorker_Execute(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()
	ctx := context.Background()

	task, _ := s.CreateTask(ctx, "x", "o", "r")
	chunk, _ := s.CreateChunk(ctx, store.Chunk{TaskID: task.ID, Title: "c", Description: "d", SortOrder: 1})
	job, _ := s.CreateJob(ctx, store.Job{ChunkID: chunk.ID, Type: "execute"})

	dir := t.TempDir()
	bare := filepath.Join(dir, "bare.git")
	work := filepath.Join(dir, "work")
	require.NoError(t, exec.Command("git", "init", "--bare", bare).Run())
	require.NoError(t, exec.Command("git", "clone", bare, work).Run())
	git := &gitexec.Runner{Dir: work}
	require.NoError(t, git.Run("config", "user.email", "t@t"))
	require.NoError(t, git.Run("config", "user.name", "T"))
	require.NoError(t, git.Run("commit", "--allow-empty", "-m", "init"))
	require.NoError(t, git.Run("push", "origin", "main"))

	// Ensure there is a change to commit so the worker's commit succeeds.
	require.NoError(t, os.WriteFile(filepath.Join(work, "dummy.txt"), []byte("hello"), 0644))

	w := &Worker{
		Store: s,
		Git:   git,
		GH:    &fakeGH{},
		Owner: "o", Repo: "r",
	}
	require.NoError(t, w.Execute(ctx, job.ID))

	got, _ := s.GetChunk(ctx, chunk.ID)
	assert.Equal(t, "completed", got.Status)
	assert.Equal(t, 42, got.PRNumber)
}
