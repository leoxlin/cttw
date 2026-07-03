package tui

import tea "github.com/charmbracelet/bubbletea"

// model is a placeholder TUI model.
type model struct{}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "q" {
		return m, tea.Quit
	}
	return m, nil
}

func (m model) View() string { return "cttw TUI (placeholder)\n" }

// New returns a placeholder bubbletea model for the cttw TUI.
func New(socket string) tea.Model {
	return model{}
}
