package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/llin/cttw/internal/api"
)

type problemsMsg struct {
	problems []api.ProblemResponse
	err      error
}

type problemMsg struct {
	problem *api.ProblemResponse
	err     error
}

type switchToDashboardMsg struct{}

type Model struct {
	Screen   string // dashboard | newtask | detail
	Socket   string
	Problems []api.ProblemResponse
	Cursor   int
	Detail   *api.ProblemResponse
	Err      error
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
	problems, err := listProblems(m.Socket)
	return problemsMsg{problems: problems, err: err}
}

func (m *Model) fetchProblem(id string) tea.Cmd {
	return func() tea.Msg {
		problem, err := getProblem(m.Socket, id)
		return problemMsg{problem: problem, err: err}
	}
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "n":
			m.Screen = "newtask"
		case "esc":
			m.Screen = "dashboard"
			return m, m.fetchProblems
		case "up", "k":
			if m.Screen == "dashboard" && m.Cursor > 0 {
				m.Cursor--
			}
		case "down", "j":
			if m.Screen == "dashboard" && m.Cursor < len(m.Problems)-1 {
				m.Cursor++
			}
		case "enter":
			if m.Screen == "dashboard" && len(m.Problems) > 0 {
				m.Screen = "detail"
				m.Detail = nil
				m.Err = nil
				return m, m.fetchProblem(m.Problems[m.Cursor].ID)
			}
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
	case problemMsg:
		m.Detail = msg.problem
		m.Err = msg.err
	case switchToDashboardMsg:
		m.Screen = "dashboard"
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
	case "detail":
		return detailView(m)
	default:
		return dashboardView(m)
	}
}
