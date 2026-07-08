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

	view := dashboardView(m)

	assert.Contains(t, view, "Loading problems...")
	assert.NotContains(t, view, "No problems yet.")
}

func TestDashboardView_EmptyState(t *testing.T) {
	m := New("unix:///nonexistent")
	m.Loading = false

	view := dashboardView(m)

	assert.Contains(t, view, "No problems yet.")
}

func TestDashboardView_ErrorState(t *testing.T) {
	m := New("unix:///nonexistent")
	m.Loading = false
	m.Err = errors.New("daemon unavailable")

	view := dashboardView(m)

	assert.Contains(t, view, "Error: daemon unavailable")
	assert.NotContains(t, view, "No problems yet.")
}

func TestDashboardView_SearchFiltersProblems(t *testing.T) {
	m := New("unix:///nonexistent")
	m.Loading = false
	m.Search = "oauth"
	m.Problems = []api.ProblemResponse{
		problem("p1", "pending", "add OAuth login", 2),
		problem("p2", "pending", "fix billing export", 1),
	}

	view := dashboardView(m)

	assert.Contains(t, view, "Problems (1 of 2):")
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

	view := dashboardView(m)

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

	view := dashboardView(m)

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
	assert.Equal(t, "dashboard", m.Screen)

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
