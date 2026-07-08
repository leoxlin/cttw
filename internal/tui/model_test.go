package tui

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/llin/cttw/internal/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDashboardView_RendersProblemsAndError(t *testing.T) {
	m := New("unix:///nonexistent")
	m.Problems = []api.ProblemResponse{
		{ID: "problem-1", Status: "pending", Description: "first problem"},
		{ID: "problem-2", Status: "ready", Description: "second problem"},
	}
	m.Cursor = 1
	m.Err = errors.New("list failed")

	view := m.View()

	assert.Contains(t, view, "cttw")
	assert.Contains(t, view, "Error: list failed")
	assert.Contains(t, view, "  problem-  pending")
	assert.Contains(t, view, "> problem-  ready")
	assert.Contains(t, view, "[enter] details")
}

func TestModel_KeyboardNavigation(t *testing.T) {
	m := New("unix:///nonexistent")
	m.Problems = []api.ProblemResponse{
		{ID: "p1", Status: "pending", Description: "one"},
		{ID: "p2", Status: "ready", Description: "two"},
	}

	updated, cmd := m.Update(key("j"))
	m = updated.(*Model)
	require.Nil(t, cmd)
	assert.Equal(t, 1, m.Cursor)

	updated, cmd = m.Update(key("j"))
	m = updated.(*Model)
	require.Nil(t, cmd)
	assert.Equal(t, 1, m.Cursor)

	updated, cmd = m.Update(key("k"))
	m = updated.(*Model)
	require.Nil(t, cmd)
	assert.Equal(t, 0, m.Cursor)

	updated, cmd = m.Update(key("n"))
	m = updated.(*Model)
	assert.Equal(t, "newtask", m.Screen)
	assert.Contains(t, m.View(), "New Problem")
}

func TestModel_DetailLoading(t *testing.T) {
	want := api.ProblemResponse{
		ID:          "problem-1",
		Description: "build details screen",
		Status:      "ready",
		IssueNumber: 12,
		Tasks: []api.TaskResponse{
			{ID: "task-1", Title: "wire enter key", Status: "done"},
		},
	}
	stubGetProblem(t, func(socket, id string) (*api.ProblemResponse, error) {
		assert.Equal(t, "test-socket", socket)
		assert.Equal(t, "problem-1", id)
		return &want, nil
	})

	m := New("test-socket")
	m.Problems = []api.ProblemResponse{{ID: "problem-1", Status: "ready", Description: "summary"}}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(*Model)
	require.NotNil(t, cmd)
	assert.Equal(t, "detail", m.Screen)
	assert.Contains(t, m.View(), "Loading problem")

	msg := cmd()
	require.IsType(t, problemMsg{}, msg)

	updated, cmd = m.Update(msg)
	m = updated.(*Model)
	require.Nil(t, cmd)
	require.NotNil(t, m.Detail)
	assert.Equal(t, "build details screen", m.Detail.Description)
	assert.Contains(t, m.View(), "Issue: #12")
	assert.Contains(t, m.View(), "wire enter key")
}

func TestModel_DetailAPIError(t *testing.T) {
	stubGetProblem(t, func(socket, id string) (*api.ProblemResponse, error) {
		assert.Equal(t, "missing", id)
		return nil, errors.New("daemon 404: not found")
	})

	m := New("test-socket")
	m.Problems = []api.ProblemResponse{{ID: "missing", Status: "pending", Description: "missing"}}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(*Model)
	require.NotNil(t, cmd)

	updated, cmd = m.Update(cmd())
	m = updated.(*Model)
	require.Nil(t, cmd)
	require.Error(t, m.Err)
	assert.Contains(t, m.View(), "Error: daemon 404")
	assert.Contains(t, m.View(), "not found")
}

func TestModel_ListAPIError(t *testing.T) {
	stubListProblems(t, func(socket string) ([]api.ProblemResponse, error) {
		assert.Equal(t, "test-socket", socket)
		return nil, errors.New("daemon 500: database offline")
	})

	m := New("test-socket")
	msg := m.Init()()
	require.IsType(t, problemsMsg{}, msg)

	updated, cmd := m.Update(msg)
	m = updated.(*Model)
	require.Nil(t, cmd)
	require.Error(t, m.Err)
	assert.Contains(t, m.View(), "Error: daemon 500")
	assert.Contains(t, m.View(), "database offline")
}

func key(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func stubListProblems(t *testing.T, fn func(string) ([]api.ProblemResponse, error)) {
	t.Helper()
	old := listProblems
	listProblems = fn
	t.Cleanup(func() { listProblems = old })
}

func stubGetProblem(t *testing.T, fn func(string, string) (*api.ProblemResponse, error)) {
	t.Helper()
	old := getProblem
	getProblem = fn
	t.Cleanup(func() { getProblem = old })
}

func stubCreateProblem(t *testing.T, fn func(string, string, string, string) (*api.ProblemResponse, error)) {
	t.Helper()
	old := createProblem
	createProblem = fn
	t.Cleanup(func() { createProblem = old })
}
