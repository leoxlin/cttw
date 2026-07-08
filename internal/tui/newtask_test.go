package tui

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/llin/cttw/internal/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTask_SubmitAPIError(t *testing.T) {
	wantErr := errors.New("daemon 500: create failed")
	stubCreateProblem(t, func(socket, owner, repo, description string) (*api.ProblemResponse, error) {
		assert.Equal(t, "test-socket", socket)
		assert.Equal(t, "owner", owner)
		assert.Equal(t, "repo", repo)
		assert.Equal(t, "add OAuth2 login", description)
		return nil, wantErr
	})

	m := newNewTask("test-socket")
	m.textarea.SetValue("owner/repo add OAuth2 login")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	nm := updated.(newTaskModel)
	assert.True(t, nm.sent)

	msg := cmd()
	require.IsType(t, submitProblemMsg{}, msg)
	submit := msg.(submitProblemMsg)
	require.Equal(t, wantErr, submit.err)

	updated2, cmd2 := nm.Update(submit)
	nm2 := updated2.(newTaskModel)
	assert.False(t, nm2.sent)
	assert.Equal(t, submit.err, nm2.err)
	assert.Nil(t, cmd2)
}

func TestNewTask_SubmitSuccessPath(t *testing.T) {
	stubCreateProblem(t, func(socket, owner, repo, description string) (*api.ProblemResponse, error) {
		assert.Equal(t, "test-socket", socket)
		assert.Equal(t, "owner", owner)
		assert.Equal(t, "repo", repo)
		assert.Equal(t, "add OAuth2 login", description)
		return &api.ProblemResponse{
			ID:          "p1",
			Description: description,
			Status:      "pending",
			RepoID:      "r1",
		}, nil
	})

	m := newNewTask("test-socket")
	m.textarea.SetValue("owner/repo add OAuth2 login")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	nm := updated.(newTaskModel)
	require.NotNil(t, cmd)
	assert.True(t, nm.sent)

	msg := cmd()
	require.IsType(t, submitProblemMsg{}, msg)
	submit := msg.(submitProblemMsg)
	require.NoError(t, submit.err)

	updated2, cmd2 := nm.Update(submit)
	nm2 := updated2.(newTaskModel)
	assert.False(t, nm2.sent)
	assert.True(t, nm2.done)
	assert.NoError(t, nm2.err)
	require.NotNil(t, cmd2)
	require.IsType(t, switchToDashboardMsg{}, cmd2())
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
