package tui

import (
	"errors"
	"testing"

	"github.com/llin/cttw/internal/api"
	"github.com/stretchr/testify/assert"
)

func TestDashboardViewSummarizesProjectDataAndWorkflows(t *testing.T) {
	m := &Model{
		Problems: []api.ProblemResponse{
			{
				ID:          "problem-one",
				Description: "add OAuth2 login",
				Status:      "ready",
				RepoID:      "repo-a",
				Tasks: []api.TaskResponse{
					{ID: "task-one", Status: "pending"},
					{ID: "task-two", Status: "running"},
				},
			},
			{
				ID:          "problem-two",
				Description: "fix flaky tests",
				Status:      "pending",
				RepoID:      "repo-a",
				Tasks: []api.TaskResponse{
					{ID: "task-three", Status: "completed"},
				},
			},
			{
				ID:          "problem-three",
				Description: "ship deploy page",
				Status:      "failed",
				RepoID:      "repo-b",
				Tasks: []api.TaskResponse{
					{ID: "task-four", Status: "failed"},
				},
			},
		},
	}

	view := dashboardView(m, 80)

	assert.Contains(t, view, "Project Summary")
	assert.Contains(t, view, "Problems: 3 total, 1 pending, 1 ready, 1 failed")
	assert.Contains(t, view, "Repos:    2 tracked")
	assert.Contains(t, view, "Tasks:    4 total, 1 pending, 1 running, 1 completed, 1 failed")
	assert.Contains(t, view, "Workflows")
	assert.Contains(t, view, "[n]   New problem")
	assert.Contains(t, view, "[esc] Refresh")
	assert.Contains(t, view, "Recent Problems")
	assert.Contains(t, view, "add OAuth2 login  2 tasks")
}

func TestDashboardViewEmptyStatePointsToNewProblemWorkflow(t *testing.T) {
	view := dashboardView(&Model{}, 80)

	assert.Contains(t, view, "Problems: 0 total, 0 pending, 0 ready, 0 failed")
	assert.Contains(t, view, "No problems yet. Press [n] to create one.")
}

func TestDashboardViewDisplaysLoadErrors(t *testing.T) {
	view := dashboardView(&Model{Err: errors.New("daemon unavailable")}, 80)

	assert.Contains(t, view, "Error: daemon unavailable")
}
