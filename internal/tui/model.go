package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/llin/cttw/internal/api"
)

type problemsMsg struct {
	problems []api.ProblemResponse
	err      error
}

type switchToDashboardMsg struct{}

const (
	ScreenDashboard  = "dashboard"
	ScreenNewProblem = "newtask"
)

type Model struct {
	Screen   string // dashboard | newtask
	Socket   string
	Problems []api.ProblemResponse
	Err      error
	newTask  newTaskModel
}

func New(socket string) *Model {
	return &Model{
		Screen:  ScreenDashboard,
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
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "n":
			m.Screen = ScreenNewProblem
		case "esc":
			m.Screen = ScreenDashboard
			return m, m.fetchProblems
		}
	case problemsMsg:
		m.Problems = msg.problems
		m.Err = msg.err
	case switchToDashboardMsg:
		m.Screen = ScreenDashboard
		return m, m.fetchProblems
	}
	if m.Screen == ScreenNewProblem {
		updated, cmd := m.newTask.Update(msg)
		m.newTask = updated.(newTaskModel)
		return m, cmd
	}
	return m, nil
}

func (m *Model) View() string {
	switch m.Screen {
	case ScreenNewProblem:
		return m.newTask.View()
	default:
		return dashboardView(m)
	}
}
