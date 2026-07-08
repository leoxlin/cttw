package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/llin/cttw/internal/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDashboardView_LoadingState(t *testing.T) {
	m := New("unix:///nonexistent")

	view := dashboardView(m, 80)

	assert.Contains(t, view, "Loading problems...")
	assert.NotContains(t, view, "No problems yet.")
}

func TestDashboardView_RespectsNarrowWidth(t *testing.T) {
	m := New("unix:///nonexistent")
	m.Loading = false
	m.Problems = []api.ProblemResponse{
		{
			ID:          "1234567890abcdef",
			Status:      "in_progress",
			Description: "implement a very long responsive terminal layout",
		},
	}

	view := dashboardView(m, 32)

	assertMaxLineWidth(t, view, 32)
	assert.Contains(t, view, "Keys:")
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
	assert.Contains(t, view, "[enter] Details")
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

func TestDashboardViewHighlightsCursorAndShowsDetailAction(t *testing.T) {
	m := &Model{
		Problems: []api.ProblemResponse{
			{ID: "problem-1", Description: "first", Status: "pending"},
			{ID: "problem-2", Description: "second", Status: "ready"},
		},
		Cursor: 1,
	}

	view := dashboardView(m, 80)

	assert.Contains(t, view, "  problem-  pending")
	assert.Contains(t, view, "> problem-  ready")
	assert.Contains(t, view, "[enter] details")
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

func TestModelDashboardSelectionAndDetailView(t *testing.T) {
	m := New("unix:///nonexistent")
	m.Loading = false
	m.SortDesc = false
	m.Problems = []api.ProblemResponse{
		{ID: "problem-1", Description: "first", Status: "pending"},
		{ID: "problem-2", Description: "second", Status: "ready"},
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	model := updated.(*Model)
	require.Nil(t, cmd)
	assert.Equal(t, 1, model.Cursor)

	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(*Model)
	require.NotNil(t, cmd)
	assert.Equal(t, screenDetail, model.Screen)
	assert.True(t, model.DetailLoading)
	require.NotNil(t, model.Detail)
	assert.Equal(t, "problem-2", model.Detail.ID)

	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	detail := &api.ProblemResponse{
		ID:          "problem-2",
		Description: "ship detail view",
		Status:      "ready",
		RepoID:      "repo-2",
		IssueNumber: 12,
		CreatedAt:   now,
		UpdatedAt:   now,
		Tasks: []api.TaskResponse{
			{
				ID:          "task-1",
				Title:       "render metadata",
				Description: "show timestamps and related work",
				Status:      "completed",
				PRNumber:    34,
			},
		},
	}

	updated, cmd = model.Update(problemDetailMsg{problem: detail})
	model = updated.(*Model)
	require.Nil(t, cmd)
	assert.False(t, model.DetailLoading)
	assert.Same(t, detail, model.Detail)

	view := model.View()
	assert.Contains(t, view, "Problem Details")
	assert.Contains(t, view, "ID: problem-2")
	assert.Contains(t, view, "Repo: repo-2")
	assert.Contains(t, view, "Issue: #12")
	assert.Contains(t, view, "Description:")
	assert.Contains(t, view, "ship detail view")
	assert.Contains(t, view, "Tasks (1):")
	assert.Contains(t, view, "render metadata")
	assert.Contains(t, view, "PR: #34")
	assert.Contains(t, view, "Actions: [r] refresh  [b/esc] back  [n] new problem  [q] quit")
}

func TestProblemListRefreshClampsCursor(t *testing.T) {
	m := New("unix:///nonexistent")
	m.Cursor = 5

	updated, cmd := m.Update(problemsMsg{
		problems: []api.ProblemResponse{{ID: "problem-1", Description: "only", Status: "pending"}},
	})

	model := updated.(*Model)
	require.Nil(t, cmd)
	assert.Equal(t, 0, model.Cursor)
}

func TestDetailViewEmptyState(t *testing.T) {
	m := New("unix:///nonexistent")
	m.Screen = screenDetail

	view := m.View()

	assert.Contains(t, view, "No problem selected.")
	assert.Contains(t, view, "Actions: [b/esc] back  [q] quit")
}

func assertMaxLineWidth(t *testing.T, view string, width int) {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSuffix(view, "\n"), "\n") {
		assert.LessOrEqual(t, lipgloss.Width(line), width, line)
	}
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
