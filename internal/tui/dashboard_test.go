package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/llin/cttw/internal/api"
	"github.com/stretchr/testify/assert"
)

func TestDashboardView_LoadingState(t *testing.T) {
	m := New("unix:///nonexistent")

	view := dashboardView(m, 80)

	assert.Contains(t, view, "Loading problems...")
	assert.NotContains(t, view, "No problems yet.")
}

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

func TestDashboardView_SearchFiltersProblems(t *testing.T) {
	m := New("unix:///nonexistent")
	m.Loading = false
	m.Search = "oauth"
	m.Problems = []api.ProblemResponse{
		problem("p1", "pending", "add OAuth login", 2),
		problem("p2", "pending", "fix billing export", 1),
	}

	view := dashboardView(m, 80)

	assert.Contains(t, view, "Problems (1 of 2)")
	assert.Contains(t, view, "add OAuth login")
	assert.NotContains(t, view, "fix billing export")
}

func TestDashboardView_SearchEmptyState(t *testing.T) {
	m := New("unix:///nonexistent")
	m.Loading = false
	m.Search = "missing"
	m.Problems = []api.ProblemResponse{
		problem("p1", "pending", "add OAuth login", 1),
	}

	view := dashboardView(m, 80)

	assert.Contains(t, view, "No matching problems.")
}

func TestDashboardView_SortsProblems(t *testing.T) {
	m := New("unix:///nonexistent")
	m.Loading = false
	m.Sort = sortDescription
	m.SortDesc = false
	m.Problems = []api.ProblemResponse{
		problem("p1", "pending", "zebra work", 1),
		problem("p2", "pending", "alpha work", 2),
	}

	view := dashboardView(m, 80)

	assert.Less(t, strings.Index(view, "alpha work"), strings.Index(view, "zebra work"))
}

func TestModel_DashboardSearchKeyHandling(t *testing.T) {
	m := New("unix:///nonexistent")
	m.Loading = false

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = updated.(*Model)
	assert.True(t, m.searching)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = updated.(*Model)
	assert.Equal(t, "n", m.Search)
	assert.Equal(t, screenDashboard, m.Screen)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(*Model)
	assert.False(t, m.searching)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = updated.(*Model)
	assert.Equal(t, sortStatus, m.Sort)
}

func problem(id, status, description string, days int) api.ProblemResponse {
	when := time.Date(2026, 7, days, 12, 0, 0, 0, time.UTC)
	return api.ProblemResponse{
		ID:          id,
		Description: description,
		Status:      status,
		RepoID:      "repo-" + id,
		IssueNumber: days,
		CreatedAt:   when,
		UpdatedAt:   when,
	}
}
