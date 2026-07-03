package api

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskResponse_JSON(t *testing.T) {
	tr := TaskResponse{ID: "t1", Description: "x", Status: "pending", RepoOwner: "o", RepoName: "r"}
	b, err := json.Marshal(tr)
	require.NoError(t, err)
	var got TaskResponse
	require.NoError(t, json.Unmarshal(b, &got))
	assert.Equal(t, tr.ID, got.ID)
}
