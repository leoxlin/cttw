package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTask_CRUD(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	task, err := s.CreateTask(ctx, "add OAuth2", "owner", "repo")
	require.NoError(t, err)
	assert.Equal(t, "add OAuth2", task.Description)
	assert.Equal(t, "pending", task.Status)
	assert.Equal(t, "owner", task.RepoOwner)
	assert.Equal(t, "repo", task.RepoName)

	got, err := s.GetTask(ctx, task.ID)
	require.NoError(t, err)
	assert.Equal(t, task.ID, got.ID)

	tasks, err := s.ListTasks(ctx)
	require.NoError(t, err)
	assert.Len(t, tasks, 1)

	task.Status = "running"
	require.NoError(t, s.UpdateTask(ctx, task))
	got, err = s.GetTask(ctx, task.ID)
	require.NoError(t, err)
	assert.Equal(t, "running", got.Status)
}

func TestChunk_CRUD(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err)
	defer s.Close()
	ctx := context.Background()

	task, err := s.CreateTask(ctx, "x", "o", "r")
	require.NoError(t, err)

	chunk, err := s.CreateChunk(ctx, Chunk{TaskID: task.ID, Title: "c1", Description: "d1", SortOrder: 1})
	require.NoError(t, err)
	assert.Equal(t, task.ID, chunk.TaskID)

	got, err := s.GetChunk(ctx, chunk.ID)
	require.NoError(t, err)
	assert.Equal(t, "c1", got.Title)

	chunks, err := s.ListChunksByTask(ctx, task.ID)
	require.NoError(t, err)
	assert.Len(t, chunks, 1)

	chunk.Status = "completed"
	chunk.PRNumber = 42
	require.NoError(t, s.UpdateChunk(ctx, chunk))
	got, err = s.GetChunk(ctx, chunk.ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", got.Status)
	assert.Equal(t, 42, got.PRNumber)
}

func TestJob_CRUD(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err)
	defer s.Close()
	ctx := context.Background()

	task, _ := s.CreateTask(ctx, "x", "o", "r")
	chunk, _ := s.CreateChunk(ctx, Chunk{TaskID: task.ID, Title: "c", Description: "d", SortOrder: 1})

	job, err := s.CreateJob(ctx, Job{ChunkID: chunk.ID, Type: "execute"})
	require.NoError(t, err)
	assert.Equal(t, "pending", job.Status)

	got, err := s.GetJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, chunk.ID, got.ChunkID)

	job.Status = "running"
	require.NoError(t, s.UpdateJob(ctx, job))
}

func TestConfig(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err)
	defer s.Close()
	ctx := context.Background()

	require.NoError(t, s.SetConfigValue(ctx, "github_token", "abc"))
	v, err := s.GetConfigValue(ctx, "github_token")
	require.NoError(t, err)
	assert.Equal(t, "abc", v)
}
