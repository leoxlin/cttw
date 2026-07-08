package tui

import (
	"testing"
	"time"

	"github.com/llin/cttw/internal/api"
	"github.com/stretchr/testify/assert"
)

func TestDashboardView_GroupsProblemsByRepository(t *testing.T) {
	updated := time.Date(2026, 7, 7, 15, 4, 0, 0, time.Local)
	m := &Model{
		Problems: []api.ProblemResponse{
			{
				ID:          "problem-1",
				Description: "build API",
				Status:      "ready",
				RepoID:      "repo-1",
				RepoOwner:   "llin",
				RepoName:    "cttw",
				IssueNumber: 42,
				UpdatedAt:   updated,
				Tasks: []api.TaskResponse{
					{ID: "task-1", Status: "pending"},
					{ID: "task-2", Status: "completed"},
				},
			},
		},
	}

	view := dashboardView(m)

	assert.Contains(t, view, "Repositories")
	assert.Contains(t, view, "llin/cttw")
	assert.Contains(t, view, "1 problem")
	assert.Contains(t, view, "2 tasks")
	assert.Contains(t, view, "STATUS")
	assert.Contains(t, view, "ISSUE")
	assert.Contains(t, view, "TASKS")
	assert.Contains(t, view, "UPDATED")
	assert.Contains(t, view, "ready")
	assert.Contains(t, view, "#42")
	assert.Contains(t, view, "2026-07-07 15:04")
	assert.Contains(t, view, "build API")
}

func TestDashboardView_FallsBackToRepoID(t *testing.T) {
	m := &Model{
		Problems: []api.ProblemResponse{
			{ID: "problem-1", Description: "x", Status: "pending", RepoID: "1234567890abcdef"},
		},
	}

	view := dashboardView(m)

	assert.Contains(t, view, "repo 12345678")
	assert.Contains(t, view, "-")
	assert.Contains(t, view, "n/a")
}
