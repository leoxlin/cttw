package api

import (
	"encoding/json"
	"testing"

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
