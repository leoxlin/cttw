package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrations_AreIdempotent(t *testing.T) {
	dbPath := "file:" + t.TempDir() + "/cttw.db"

	s, err := New(dbPath)
	require.NoError(t, err)

	var version int
	row := s.db.QueryRow(`SELECT version FROM schema_version ORDER BY version DESC LIMIT 1`)
	require.NoError(t, row.Scan(&version))
	assert.Equal(t, 1, version)

	// Insert a problem to verify tables exist.
	ctx := context.Background()
	repo, err := s.CreateRepo(ctx, "o", "r", "/tmp/r", "main", "")
	require.NoError(t, err)
	_, err = s.CreateProblem(ctx, "x", repo.ID)
	require.NoError(t, err)
	s.Close()

	// Reopening the database should not re-apply migrations or fail.
	s2, err := New(dbPath)
	require.NoError(t, err)
	defer s2.Close()

	var version2 int
	row = s2.db.QueryRow(`SELECT version FROM schema_version ORDER BY version DESC LIMIT 1`)
	require.NoError(t, row.Scan(&version2))
	assert.Equal(t, 1, version2)

	// Data should still be present.
	problems, err := s2.ListProblems(ctx)
	require.NoError(t, err)
	assert.Len(t, problems, 1)
}

func TestRepo_CRUD(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err)
	defer s.Close()
	ctx := context.Background()

	repo, err := s.CreateRepo(ctx, "llin", "cttw", "/tmp/r", "main", "")
	require.NoError(t, err)
	assert.Equal(t, "llin", repo.Owner)

	got, err := s.GetRepoByOwnerName(ctx, "llin", "cttw")
	require.NoError(t, err)
	assert.Equal(t, repo.ID, got.ID)

	repos, err := s.ListRepos(ctx)
	require.NoError(t, err)
	assert.Len(t, repos, 1)
}

func TestProblemAndTasks_CRUD(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err)
	defer s.Close()
	ctx := context.Background()

	repo, _ := s.CreateRepo(ctx, "o", "r", "/tmp/r", "main", "")
	problem, err := s.CreateProblem(ctx, "build API", repo.ID)
	require.NoError(t, err)
	assert.Equal(t, "pending", problem.Status)

	task, err := s.CreateTask(ctx, problem.ID, repo.ID, "add handler", "implement POST")
	require.NoError(t, err)
	assert.Equal(t, "pending", task.Status)
	assert.Equal(t, 3, task.MaxAttempts)

	tasks, err := s.ListTasksByProblem(ctx, problem.ID)
	require.NoError(t, err)
	assert.Len(t, tasks, 1)

	task.Status = "running"
	require.NoError(t, s.UpdateTask(ctx, task))
	got, err := s.GetTask(ctx, task.ID)
	require.NoError(t, err)
	assert.Equal(t, "running", got.Status)
}

func TestFailTasksByProblem(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err)
	defer s.Close()
	ctx := context.Background()

	repo, _ := s.CreateRepo(ctx, "o", "r", "/tmp/r", "main", "")
	problem, _ := s.CreateProblem(ctx, "x", repo.ID)
	t1, _ := s.CreateTask(ctx, problem.ID, repo.ID, "t1", "d1")
	t2, _ := s.CreateTask(ctx, problem.ID, repo.ID, "t2", "d2")

	require.NoError(t, s.FailTasksByProblem(ctx, problem.ID))

	got1, err := s.GetTask(ctx, t1.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", got1.Status)

	got2, err := s.GetTask(ctx, t2.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", got2.Status)
}

func TestNextPendingTask(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err)
	defer s.Close()
	ctx := context.Background()

	repo, _ := s.CreateRepo(ctx, "o", "r", "/tmp/r", "main", "")
	problem, _ := s.CreateProblem(ctx, "x", repo.ID)
	task, _ := s.CreateTask(ctx, problem.ID, repo.ID, "t", "d")

	got, err := s.NextPendingTask(ctx)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, task.ID, got.ID)
	assert.Equal(t, "running", got.Status)

	again, err := s.NextPendingTask(ctx)
	require.NoError(t, err)
	assert.Nil(t, again)
}
