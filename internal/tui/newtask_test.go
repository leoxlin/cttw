package tui

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/llin/cttw/internal/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTask_SubmitSuccessPath(t *testing.T) {
	m := newNewTask("unix:///nonexistent")
	m.ownerInput.SetValue("owner")
	m.repoInput.SetValue("repo")
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
	m.ownerInput.SetValue("owner/repo")
	m.repoInput.SetValue("repo")
	m.description.SetValue("add OAuth2 login")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	nm := updated.(newTaskModel)
	assert.False(t, nm.sent)
	require.Nil(t, cmd)
	require.Error(t, nm.err)
	assert.Contains(t, nm.err.Error(), "owner and name")
}

func TestNewTask_DisplaysError(t *testing.T) {
	m := newNewTask("unix:///nonexistent")
	m.err = errors.New("boom")
	view := m.View()
	assert.Contains(t, view, "Error: boom")
}

func TestNewTask_DoneView(t *testing.T) {
	m := newNewTask("unix:///nonexistent")
	m.done = true
	assert.Contains(t, m.View(), "Problem created")
}

func TestNewTask_ResizesForNarrowWidth(t *testing.T) {
	m := newNewTask("unix:///nonexistent")
	m.SetSize(32, 12)

	assert.Equal(t, 32, m.ownerInput.Width)
	assert.Equal(t, 32, m.repoInput.Width)
	assert.GreaterOrEqual(t, m.description.Width(), 24)
	assert.LessOrEqual(t, m.description.Width(), 32)
	assert.Equal(t, 5, m.description.Height())
}

func TestNewTask_EditSubmitValidationError(t *testing.T) {
	m := newEditTask("unix:///nonexistent", api.ProblemResponse{ID: "p1", Description: "old"})
	m.description.SetValue(" ")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	nm := updated.(newTaskModel)
	assert.False(t, nm.sent)
	require.Nil(t, cmd)
	require.Error(t, nm.err)
	assert.Contains(t, nm.err.Error(), "description")
}

func TestNewTask_EditDoneView(t *testing.T) {
	m := newEditTask("unix:///nonexistent", api.ProblemResponse{ID: "p1", Description: "old"})
	m.done = true
	assert.Contains(t, m.View(), "Problem updated")
}
