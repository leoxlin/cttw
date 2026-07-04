package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/llin/cttw/internal/api"
)

type newTaskModel struct {
	textarea textarea.Model
	socket   string
	err      error
	sent     bool
}

func newNewTask(socket string) newTaskModel {
	ta := textarea.New()
	ta.Placeholder = "owner/repo describe the problem..."
	ta.Focus()
	return newTaskModel{textarea: ta, socket: socket}
}

func (n newTaskModel) Init() tea.Cmd { return textarea.Blink }

func (n newTaskModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return n, func() tea.Msg { return switchToDashboardMsg{} }
		case "ctrl+d":
			value := n.textarea.Value()
			if value != "" {
				go n.submit(value)
				n.sent = true
			}
		}
	}
	m, cmd := n.textarea.Update(msg)
	n.textarea = m
	cmds = append(cmds, cmd)
	return n, tea.Batch(cmds...)
}

func (n newTaskModel) View() string {
	if n.sent {
		return "Submitting...\n\n[esc] back"
	}
	return "New Problem (ctrl+d to submit, esc to cancel)\n\n" + n.textarea.View()
}

func (n *newTaskModel) submit(value string) {
	parts := strings.SplitN(value, " ", 2)
	if len(parts) != 2 {
		n.err = fmt.Errorf("input must be owner/repo description")
		return
	}
	repoParts := strings.Split(parts[0], "/")
	if len(repoParts) != 2 {
		n.err = fmt.Errorf("repo must be owner/name")
		return
	}
	client := api.NewClient(n.socket)
	_, n.err = client.CreateProblem(repoParts[0], repoParts[1], parts[1])
}
