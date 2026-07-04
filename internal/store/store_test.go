package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
