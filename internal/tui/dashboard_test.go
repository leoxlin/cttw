package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/llin/cttw/internal/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDashboardViewHighlightsCursorAndShowsDetailAction(t *testing.T) {
	m := &Model{
		Problems: []api.ProblemResponse{
			{ID: "problem-1", Description: "first", Status: "pending"},
			{ID: "problem-2", Description: "second", Status: "ready"},
		},
		Cursor: 1,
	}

	view := dashboardView(m)
	assert.Contains(t, view, "  problem-  pending")
	assert.Contains(t, view, "> problem-  ready")
	assert.Contains(t, view, "[enter] details")
}

func TestModelDashboardSelectionAndDetailView(t *testing.T) {
	m := New("unix:///nonexistent")
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
	assert.Equal(t, "detail", model.Screen)
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
	m := &Model{Screen: "detail"}

	view := m.View()
	assert.Contains(t, view, "No problem selected.")
	assert.Contains(t, view, "Actions: [b/esc] back  [q] quit")
}
