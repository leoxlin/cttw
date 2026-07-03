package worker

import (
	"context"
	"errors"
	"fmt"
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

type fakeGHError struct{}

func (f *fakeGHError) CreateIssue(ctx context.Context, owner, repo, title, body string) (int, error) {
	return 1, nil
}
func (f *fakeGHError) CreateSubIssue(ctx context.Context, owner, repo string, parentNumber, childNumber int) error {
	return nil
}
func (f *fakeGHError) CreateBranch(ctx context.Context, owner, repo, branch, base string) error { return nil }
func (f *fakeGHError) CreatePullRequest(ctx context.Context, owner, repo, title, body, head, base string) (int, error) {
	return 0, errors.New("create PR failed")
}

type fakeLLM struct{}

func (f *fakeLLM) Chat(ctx context.Context, system, user string) (string, error) {
	return "# Implementation\n\nDone.", nil
}

func setupRepo(t *testing.T) (work string, git *gitexec.Runner) {
	t.Helper()
	dir := t.TempDir()
	bare := filepath.Join(dir, "bare.git")
	work = filepath.Join(dir, "work")
	require.NoError(t, exec.Command("git", "init", "--bare", bare).Run())
	require.NoError(t, exec.Command("git", "clone", bare, work).Run())
	git = &gitexec.Runner{Dir: work}
	require.NoError(t, git.Run("config", "user.email", "t@t"))
	require.NoError(t, git.Run("config", "user.name", "T"))
	require.NoError(t, git.Run("commit", "--allow-empty", "-m", "init"))
	require.NoError(t, git.Run("push", "origin", "main"))
	return work, git
}

func TestWorker_Execute(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()
	ctx := context.Background()

	task, err := s.CreateTask(ctx, "x", "o", "r")
	require.NoError(t, err)
	chunk, err := s.CreateChunk(ctx, store.Chunk{TaskID: task.ID, Title: "c", Description: "d", SortOrder: 1})
	require.NoError(t, err)
	job, err := s.CreateJob(ctx, store.Job{ChunkID: chunk.ID, Type: "execute"})
	require.NoError(t, err)

	work, git := setupRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(work, "dummy.txt"), []byte("hello"), 0644))

	w := &Worker{
		Store: s,
		Git:   git,
		GH:    &fakeGH{},
		Owner: "o", Repo: "r",
	}
	require.NoError(t, w.Execute(ctx, job.ID))

	got, err := s.GetChunk(ctx, chunk.ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", got.Status)
	assert.Equal(t, 42, got.PRNumber)
}

func TestWorker_Execute_PRFailure(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()
	ctx := context.Background()

	task, err := s.CreateTask(ctx, "x", "o", "r")
	require.NoError(t, err)
	chunk, err := s.CreateChunk(ctx, store.Chunk{TaskID: task.ID, Title: "c", Description: "d", SortOrder: 1})
	require.NoError(t, err)
	job, err := s.CreateJob(ctx, store.Job{ChunkID: chunk.ID, Type: "execute"})
	require.NoError(t, err)

	work, git := setupRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(work, "dummy.txt"), []byte("hello"), 0644))

	w := &Worker{
		Store: s,
		Git:   git,
		GH:    &fakeGHError{},
		Owner: "o", Repo: "r",
	}
	err = w.Execute(ctx, job.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create PR failed")

	got, err := s.GetChunk(ctx, chunk.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", got.Status)
	assert.Contains(t, got.Output, "create PR failed")

	jobGot, err := s.GetJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", jobGot.Status)
}

func TestWorker_Execute_LLMWritesArtifact(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()
	ctx := context.Background()

	task, err := s.CreateTask(ctx, "x", "o", "r")
	require.NoError(t, err)
	chunk, err := s.CreateChunk(ctx, store.Chunk{TaskID: task.ID, Title: "c", Description: "d", SortOrder: 1})
	require.NoError(t, err)
	job, err := s.CreateJob(ctx, store.Job{ChunkID: chunk.ID, Type: "execute"})
	require.NoError(t, err)

	work, git := setupRepo(t)

	w := &Worker{
		Store: s,
		Git:   git,
		GH:    &fakeGH{},
		LLM:   &fakeLLM{},
		Owner: "o", Repo: "r",
	}
	require.NoError(t, w.Execute(ctx, job.ID))

	artifact := filepath.Join(work, fmt.Sprintf("cttw-chunk-%s.md", shortID(chunk.ID)))
	b, err := os.ReadFile(artifact)
	require.NoError(t, err)
	assert.Contains(t, string(b), "# Implementation")

	got, err := s.GetChunk(ctx, chunk.ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", got.Status)
}

func TestSlug(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Hello World", "hello-world"},
		{"Feat: Add /something", "feat-add-something"},
		{"--leading-trailing--", "leading-trailing"},
		{"a..b@c~d^e*f[g\\h", "a-b-c-d-e-f-g-h"},
		{"", "chunk"},
		{"!!!", "chunk"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, slug(c.in), "input: %q", c.in)
	}
}

func TestShortID(t *testing.T) {
	assert.Equal(t, "abc", shortID("abc"))
	assert.Equal(t, "12345678", shortID("1234567890"))
}
