package api

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateProblemRequest_JSON(t *testing.T) {
	req := CreateProblemRequest{Owner: "llin", Repo: "cttw", Description: "build API"}
	b, err := json.Marshal(req)
	require.NoError(t, err)
	assert.Contains(t, string(b), `"owner":"llin"`)
	assert.Contains(t, string(b), `"repo":"cttw"`)
	assert.Contains(t, string(b), `"description":"build API"`)
}

func TestProblemResponse_JSON(t *testing.T) {
	resp := ProblemResponse{
		ID:          "p1",
		Description: "build API",
		Status:      "ready",
		RepoID:      "r1",
		IssueNumber: 5,
		Tasks: []TaskResponse{
			{ID: "t1", Title: "add handler", Description: "impl POST", Status: "pending"},
		},
	}
	b, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.Contains(t, string(b), `"id":"p1"`)
	assert.Contains(t, string(b), `"tasks":[`)
}

func TestTaskResponse_JSON(t *testing.T) {
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	resp := TaskResponse{
		ID:          "t1",
		Title:       "add handler",
		Description: "impl POST",
		Status:      "pending",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	b, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.Contains(t, string(b), `"id":"t1"`)
	assert.Contains(t, string(b), `"created_at":"2026-07-05T12:00:00Z"`)
	assert.Contains(t, string(b), `"updated_at":"2026-07-05T12:00:00Z"`)
}

func TestTaskResponse_CompletedResponseIncludesPRNumber(t *testing.T) {
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	resp := TaskResponse{
		ID:          "t1",
		Title:       "add handler",
		Description: "impl POST",
		Status:      "completed",
		PRNumber:    42,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	b, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.Contains(t, string(b), `"pr_number":42`)

	var got map[string]any
	require.NoError(t, json.Unmarshal(b, &got))
	assert.Equal(t, "t1", got["id"])
	assert.Equal(t, "add handler", got["title"])
	assert.Equal(t, "impl POST", got["description"])
	assert.Equal(t, "completed", got["status"])
	assert.Equal(t, float64(42), got["pr_number"])
	assert.Equal(t, "2026-07-05T12:00:00Z", got["created_at"])
	assert.Equal(t, "2026-07-05T12:00:00Z", got["updated_at"])
}
