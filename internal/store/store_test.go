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
