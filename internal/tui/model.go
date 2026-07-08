package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/llin/cttw/internal/api"
)

type problemsMsg struct {
	problems []api.ProblemResponse
	err      error
}

type switchToDashboardMsg struct {
	notice string
}

type Model struct {
	Screen   string // dashboard | newtask
	Socket   string
	Problems []api.ProblemResponse
	Cursor   int
	Err      error
	Notice   string
	newTask  newTaskModel
}

func New(socket string) *Model {
	return &Model{
		Screen:  "dashboard",
		Socket:  socket,
		newTask: newNewTask(socket),
	}
}

func (m *Model) Init() tea.Cmd {
	return m.fetchProblems
}

func (m *Model) fetchProblems() tea.Msg {
	client := api.NewClient(m.Socket)
	problems, err := client.ListProblems()
	return problemsMsg{problems: problems, err: err}
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.Screen == "newtask" {
			if msg.String() == "q" || msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			updated, cmd := m.newTask.Update(msg)
			m.newTask = updated.(newTaskModel)
			return m, cmd
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "n":
			m.Notice = ""
			m.newTask = newNewTask(m.Socket)
			m.Screen = "newtask"
			return m, nil
		case "e":
			if len(m.Problems) > 0 {
				m.Notice = ""
				m.newTask = newEditTask(m.Socket, m.Problems[m.Cursor])
				m.Screen = "newtask"
			}
			return m, nil
		case "j", "down":
			if m.Cursor < len(m.Problems)-1 {
				m.Cursor++
			}
			return m, nil
		case "k", "up":
			if m.Cursor > 0 {
				m.Cursor--
			}
			return m, nil
		case "esc":
			m.Screen = "dashboard"
			return m, m.fetchProblems
		}
	case problemsMsg:
		m.Problems = msg.problems
		m.Err = msg.err
		if m.Cursor >= len(m.Problems) {
			m.Cursor = len(m.Problems) - 1
		}
		if m.Cursor < 0 {
			m.Cursor = 0
		}
	case switchToDashboardMsg:
		m.Screen = "dashboard"
		m.Notice = msg.notice
		return m, m.fetchProblems
	}
	if m.Screen == "newtask" {
		updated, cmd := m.newTask.Update(msg)
		m.newTask = updated.(newTaskModel)
		return m, cmd
	}
	return m, nil
}

func (m *Model) View() string {
	switch m.Screen {
	case "newtask":
		return m.newTask.View()
	default:
		return dashboardView(m)
	}
}
