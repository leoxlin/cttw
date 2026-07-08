package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/llin/cttw/internal/api"
	"github.com/stretchr/testify/assert"
)

func TestModel_NewProblemKeyDoesNotTypeIntoForm(t *testing.T) {
	m := New("unix:///nonexistent")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	nm := updated.(*Model)

	assert.Nil(t, cmd)
	assert.Equal(t, "newtask", nm.Screen)
	assert.Empty(t, nm.newTask.ownerInput.Value())
}

func TestModel_EditProblemKeyOpensSelectedProblem(t *testing.T) {
	m := New("unix:///nonexistent")
	m.Problems = []api.ProblemResponse{
		{ID: "p1", Description: "first"},
		{ID: "p2", Description: "second"},
	}
	m.Cursor = 1

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	nm := updated.(*Model)

	assert.Nil(t, cmd)
	assert.Equal(t, "newtask", nm.Screen)
	assert.Equal(t, formEdit, nm.newTask.mode)
	assert.Equal(t, "p2", nm.newTask.problem.ID)
	assert.Equal(t, "second", nm.newTask.description.Value())
}
