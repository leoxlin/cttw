package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/llin/cttw/internal/api"
)

type newTaskModel struct {
	repo        textinput.Model
	description textarea.Model
	focus       int
	socket      string
	err         error
	sent        bool
	done        bool
}

type submitProblemMsg struct {
	err error
}

func newNewTask(socket string) newTaskModel {
	repo := textinput.New()
	repo.Placeholder = "owner/repo"
	repo.CharLimit = 160
	repo.Width = 40

	description := textarea.New()
	description.Placeholder = "Describe the problem..."

	n := newTaskModel{repo: repo, description: description, socket: socket}
	n.setFocus(0)
	return n
}

func (n newTaskModel) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, textarea.Blink)
}

func (n newTaskModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return n, func() tea.Msg { return switchToDashboardMsg{} }
		case "tab", "shift+tab", "up", "down":
			if msg.String() == "shift+tab" || msg.String() == "up" {
				n.setFocus(0)
			} else {
				n.setFocus(1)
			}
			return n, nil
		case "enter":
			if n.focus == 0 {
				n.setFocus(1)
				return n, nil
			}
		case "ctrl+d":
			owner, repo, description, err := n.validate()
			if err != nil {
				n.err = err
				n.done = false
				return n, nil
			}
			n.sent = true
			n.err = nil
			n.done = false
			return n, n.submit(owner, repo, description)
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
	if n.sent {
		return n, nil
	}
	if n.focus == 0 {
		m, cmd := n.repo.Update(msg)
		n.repo = m
		cmds = append(cmds, cmd)
		return n, tea.Batch(cmds...)
	}
	m, cmd := n.description.Update(msg)
	n.description = m
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
	view := "New Problem (ctrl+d to submit, tab to switch, esc to cancel)\n\n"
	view += "Repo owner/name\n" + n.repo.View()
	view += "\n\nDescription\n" + n.description.View()
	if n.err != nil {
		view += "\n\nError: " + n.err.Error()
	}
	return view
}

func (n *newTaskModel) setFocus(focus int) {
	n.focus = focus
	if focus == 0 {
		n.description.Blur()
		n.repo.Focus()
		return
	}
	n.repo.Blur()
	n.description.Focus()
}

func (n newTaskModel) validate() (owner, repo, description string, err error) {
	repoSpec := strings.TrimSpace(n.repo.Value())
	description = strings.TrimSpace(n.description.Value())
	if repoSpec == "" {
		return "", "", "", fmt.Errorf("repo is required")
	}
	repoParts := strings.Split(repoSpec, "/")
	if len(repoParts) != 2 || strings.TrimSpace(repoParts[0]) == "" || strings.TrimSpace(repoParts[1]) == "" {
		return "", "", "", fmt.Errorf("repo must be owner/name")
	}
	if description == "" {
		return "", "", "", fmt.Errorf("description is required")
	}
	return strings.TrimSpace(repoParts[0]), strings.TrimSpace(repoParts[1]), description, nil
}

func (n newTaskModel) submit(owner, repo, description string) tea.Cmd {
	return func() tea.Msg {
		client := api.NewClient(n.socket)
		_, err := client.CreateProblem(owner, repo, description)
		return submitProblemMsg{err: err}
	}
}
