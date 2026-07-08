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
	m.textarea.SetValue("owner/repo add OAuth2 login")

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
	m.textarea.SetValue("invalid")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	nm := updated.(newTaskModel)
	assert.True(t, nm.sent)

	msg := cmd()
	require.IsType(t, submitProblemMsg{}, msg)
	submit := msg.(submitProblemMsg)
	require.Error(t, submit.err)
	assert.Contains(t, submit.err.Error(), "owner/repo")
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

	updated, cmd := m.Update(tea.WindowSizeMsg{Width: 32, Height: 12})
	nm := updated.(newTaskModel)

	assert.Nil(t, cmd)
	assert.Equal(t, 32, nm.width)
	assert.Equal(t, 5, nm.textarea.Height())
	assertMaxLineWidth(t, nm.View(), nm.width)
	assert.Contains(t, nm.View(), "[ctrl+d] submit")
}
