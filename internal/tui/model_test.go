package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/llin/cttw/internal/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDashboardKeyboardNavigationAndSelection(t *testing.T) {
	m := New("unix:///nonexistent")
	m.Problems = []api.ProblemResponse{
		{ID: "problem-one", Status: "ready", Description: "first problem"},
		{ID: "problem-two", Status: "pending", Description: "second problem"},
	}

	updated, cmd := m.Update(key("j"))
	m = updated.(*Model)
	require.Nil(t, cmd)
	assert.Equal(t, 1, m.Cursor)

	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(*Model)
	require.Nil(t, cmd)
	assert.Equal(t, 1, m.Cursor)

	updated, cmd = m.Update(key("k"))
	m = updated.(*Model)
	require.Nil(t, cmd)
	assert.Equal(t, 0, m.Cursor)

	view := m.View()
	assert.Contains(t, view, "> problem-")
	assert.Contains(t, view, "[enter] select")

	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(*Model)
	require.NotNil(t, cmd)
	assert.Equal(t, "problem", m.Screen)
	require.NotNil(t, m.Problem)
	assert.Equal(t, "problem-one", m.Problem.ID)
}

func TestDashboardRefreshAndBackShortcuts(t *testing.T) {
	m := New("unix:///nonexistent")

	updated, cmd := m.Update(key("r"))
	m = updated.(*Model)
	require.NotNil(t, cmd)
	assert.Equal(t, "dashboard", m.Screen)

	m.Screen = "problem"
	m.Problem = &api.ProblemResponse{ID: "problem-one", Status: "ready", Description: "first problem"}
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(*Model)
	require.NotNil(t, cmd)
	assert.Equal(t, "dashboard", m.Screen)
	assert.Nil(t, m.Problem)
}

func TestNewTaskKeepsPrintableKeysInTextarea(t *testing.T) {
	m := New("unix:///nonexistent")

	updated, cmd := m.Update(key("n"))
	m = updated.(*Model)
	require.Nil(t, cmd)
	require.Equal(t, "newtask", m.Screen)

	updated, cmd = m.Update(key("n"))
	m = updated.(*Model)
	require.NotNil(t, cmd)
	assert.Equal(t, "newtask", m.Screen)
	assert.Equal(t, "n", m.newTask.textarea.Value())
	assert.Contains(t, m.View(), "[ctrl+d] submit")
	assert.Contains(t, m.View(), "[ctrl+c] quit")
}

func TestProblemViewShowsBackRefreshNewAndQuitHints(t *testing.T) {
	m := New("unix:///nonexistent")
	m.Screen = "problem"
	m.Problem = &api.ProblemResponse{
		ID:          "problem-one",
		Status:      "ready",
		Description: "first problem",
		Tasks: []api.TaskResponse{
			{ID: "task-one", Status: "pending", Title: "write tests"},
		},
	}

	view := m.View()
	assert.Contains(t, view, "Tasks:")
	assert.Contains(t, view, "write tests")
	assert.Contains(t, view, "[b/esc] back")
	assert.Contains(t, view, "[r] refresh")
	assert.Contains(t, view, "[n] new")
	assert.Contains(t, view, "[q] quit")
}

func key(value string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(value)}
}
