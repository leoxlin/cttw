package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

type newTaskModel struct {
	textarea textarea.Model
	socket   string
	err      error
	sent     bool
	done     bool
}

type submitProblemMsg struct {
	err error
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
				n.sent = true
				n.err = nil
				n.done = false
				return n, n.submit(value)
			}
		}
	case submitProblemMsg:
		n.sent = false
		if msg.err != nil {
			n.err = msg.err
			n.done = false
			return n, nil
		}
		n.err = nil
		n.done = true
		return n, func() tea.Msg { return switchToDashboardMsg{} }
	}
	m, cmd := n.textarea.Update(msg)
	n.textarea = m
	cmds = append(cmds, cmd)
	return n, tea.Batch(cmds...)
}

func (n newTaskModel) View() string {
	if n.done {
		return "Problem created.\n\n[esc] back"
	}
	if n.sent {
		return "Submitting...\n\n[esc] back"
	}
	view := "New Problem (ctrl+d to submit, esc to cancel)\n\n" + n.textarea.View()
	if n.err != nil {
		view += "\n\nError: " + n.err.Error()
	}
	return view
}

func (n newTaskModel) submit(value string) tea.Cmd {
	return func() tea.Msg {
		parts := strings.SplitN(value, " ", 2)
		if len(parts) != 2 {
			return submitProblemMsg{err: fmt.Errorf("input must be owner/repo description")}
		}
		repoParts := strings.Split(parts[0], "/")
		if len(repoParts) != 2 {
			return submitProblemMsg{err: fmt.Errorf("repo must be owner/name")}
		}
		_, err := createProblem(n.socket, repoParts[0], repoParts[1], parts[1])
		return submitProblemMsg{err: err}
	}
}
