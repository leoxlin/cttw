package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/llin/cttw/internal/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModel_ShellRendersDashboardNavigation(t *testing.T) {
	m := New("unix:///nonexistent")
	m.Loading = false
	m.Problems = []api.ProblemResponse{{
		ID:          "1234567890abcdef",
		Status:      "pending",
		Description: "add OAuth2 login",
	}}

	view := m.View()

	assert.Contains(t, view, "cttw")
	assert.Contains(t, view, "Problems")
	assert.Contains(t, view, "New problem")
	assert.Contains(t, view, "12345678")
	assert.Contains(t, view, "add OAuth2 login")
}

func TestModel_NavigatesToNewProblem(t *testing.T) {
	m := New("unix:///nonexistent")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	require.Nil(t, cmd)

	nm := updated.(*Model)
	assert.Equal(t, screenNewTask, nm.Screen)
	assert.Contains(t, nm.View(), "New Problem")
}

func TestModel_NewProblemKeyDoesNotTypeIntoForm(t *testing.T) {
	m := New("unix:///nonexistent")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	nm := updated.(*Model)

	assert.Nil(t, cmd)
	assert.Equal(t, screenNewTask, nm.Screen)
	assert.Empty(t, nm.newTask.ownerInput.Value())
}

func TestModel_NewProblemReceivesTextInput(t *testing.T) {
	m := New("unix:///nonexistent")
	m.Screen = screenNewTask

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})

	nm := updated.(*Model)
	assert.Equal(t, screenNewTask, nm.Screen)
	assert.Equal(t, "n", nm.newTask.ownerInput.Value())
}

func TestModel_EditProblemKeyOpensSelectedProblem(t *testing.T) {
	m := New("unix:///nonexistent")
	m.Loading = false
	m.SortDesc = false
	m.Problems = []api.ProblemResponse{
		{ID: "p1", Description: "first"},
		{ID: "p2", Description: "second"},
	}
	m.Cursor = 1

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	nm := updated.(*Model)

	assert.Nil(t, cmd)
	assert.Equal(t, screenNewTask, nm.Screen)
	assert.Equal(t, formEdit, nm.newTask.mode)
	assert.Equal(t, "p2", nm.newTask.problem.ID)
	assert.Equal(t, "second", nm.newTask.description.Value())
}

func TestModel_WindowSizeResizesShell(t *testing.T) {
	m := New("unix:///nonexistent")

	updated, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	require.Nil(t, cmd)

	nm := updated.(*Model)
	assert.Equal(t, 120, nm.Width)
	assert.Equal(t, 40, nm.Height)
	assert.GreaterOrEqual(t, nm.newTask.ownerInput.Width, 24)
	assert.GreaterOrEqual(t, nm.newTask.description.Width(), 24)
	assert.GreaterOrEqual(t, nm.newTask.description.Height(), 5)
}
