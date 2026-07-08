package tui

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTask_SubmitSuccessPath(t *testing.T) {
	m := newNewTask("unix:///nonexistent")
	m.repo.SetValue("owner/repo")
	m.description.SetValue("add OAuth2 login")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	nm := updated.(newTaskModel)
	assert.True(t, nm.sent)

	msg := cmd()
	require.IsType(t, submitProblemMsg{}, msg)
	submit := msg.(submitProblemMsg)
	// The socket does not exist, so the API call should fail.
	require.Error(t, submit.err)

	updated2, cmd2 := nm.Update(submit)
	nm2 := updated2.(newTaskModel)
	assert.False(t, nm2.sent)
	assert.Equal(t, submit.err, nm2.err)
	assert.Nil(t, cmd2)
}

func TestNewTask_SubmitValidationError(t *testing.T) {
	m := newNewTask("unix:///nonexistent")
	m.repo.SetValue("invalid")
	m.description.SetValue("add OAuth2 login")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	nm := updated.(newTaskModel)
	assert.False(t, nm.sent)
	assert.Nil(t, cmd)
	require.Error(t, nm.err)
	assert.Contains(t, nm.err.Error(), "owner/name")
}

func TestNewTask_SubmitRequiresDescription(t *testing.T) {
	m := newNewTask("unix:///nonexistent")
	m.repo.SetValue("owner/repo")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	nm := updated.(newTaskModel)
	assert.False(t, nm.sent)
	assert.Nil(t, cmd)
	require.Error(t, nm.err)
	assert.Contains(t, nm.err.Error(), "description")
}

func TestNewTask_DisplaysError(t *testing.T) {
	m := newNewTask("unix:///nonexistent")
	m.err = errors.New("boom")
	view := m.View()
	assert.Contains(t, view, "Error: boom")
}

func TestNewTask_SubmitSuccessReturnsToDashboard(t *testing.T) {
	m := newNewTask("unix:///nonexistent")
	updated, cmd := m.Update(submitProblemMsg{})
	nm := updated.(newTaskModel)
	assert.True(t, nm.done)

	msg := cmd()
	require.IsType(t, switchToDashboardMsg{}, msg)
}

func TestNewTask_ViewShowsSplitInputsAndLoading(t *testing.T) {
	m := newNewTask("unix:///nonexistent")
	view := m.View()
	assert.Contains(t, view, "Repo owner/name")
	assert.Contains(t, view, "Description")

	m.sent = true
	assert.Contains(t, m.View(), "Submitting")
}

func TestModel_NewTaskResetsOnOpen(t *testing.T) {
	m := New("unix:///nonexistent")
	m.newTask.repo.SetValue("old/repo")
	m.newTask.description.SetValue("old description")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	nm := updated.(*Model)
	assert.Equal(t, "newtask", nm.Screen)
	assert.Empty(t, nm.newTask.repo.Value())
	assert.Empty(t, nm.newTask.description.Value())
	assert.Nil(t, cmd)
}
