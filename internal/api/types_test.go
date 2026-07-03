package api

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskResponse_JSON(t *testing.T) {
	tr := TaskResponse{
		ID:                "t1",
		Description:       "x",
		Status:            "pending",
		RepoOwner:         "o",
		RepoName:          "r",
		ParentIssueNumber: 42,
		Chunks: []ChunkResponse{
			{
				ID:          "c1",
				TaskID:      "t1",
				Title:       "chunk title",
				Description: "chunk desc",
				Status:      "in_progress",
				Branch:      "feature/c1",
				BaseBranch:  "main",
				IssueNumber: 7,
				PRNumber:    8,
				SortOrder:   1,
			},
		},
		CreatedAt: time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 7, 3, 13, 0, 0, 0, time.UTC),
	}
	b, err := json.Marshal(tr)
	require.NoError(t, err)
	var got TaskResponse
	require.NoError(t, json.Unmarshal(b, &got))
	assert.Equal(t, tr.ID, got.ID)
	assert.Equal(t, tr.Description, got.Description)
	assert.Equal(t, tr.Status, got.Status)
	assert.Equal(t, tr.RepoOwner, got.RepoOwner)
	assert.Equal(t, tr.RepoName, got.RepoName)
	assert.Equal(t, tr.ParentIssueNumber, got.ParentIssueNumber)
	assert.Equal(t, tr.CreatedAt, got.CreatedAt)
	assert.Equal(t, tr.UpdatedAt, got.UpdatedAt)
	require.Len(t, got.Chunks, 1)
	assert.Equal(t, tr.Chunks[0], got.Chunks[0])
}
