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
	Screen         string // dashboard | problem | newtask
	Socket         string
	Problems       []api.ProblemResponse
	Cursor         int
	Err            error
	Problem        *api.ProblemResponse
	ProblemErr     error
	ProblemLoading bool
	newTask        newTaskModel
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

func (m *Model) fetchProblem(id string) tea.Cmd {
	return func() tea.Msg {
		client := api.NewClient(m.Socket)
		problem, err := client.GetProblem(id)
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
			if m.Screen == "dashboard" {
				m.Screen = "newtask"
			}
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
				problemID := m.Problems[m.Cursor].ID
				m.Screen = "problem"
				m.Problem = nil
				m.ProblemErr = nil
				m.ProblemLoading = true
				return m, m.fetchProblem(problemID)
			}
		case "esc":
			m.Screen = "dashboard"
			m.ProblemLoading = false
			return m, m.fetchProblems
		}
	case problemsMsg:
		m.Problems = msg.problems
		m.Err = msg.err
		if len(m.Problems) == 0 {
			m.Cursor = 0
		} else if m.Cursor >= len(m.Problems) {
			m.Cursor = len(m.Problems) - 1
		}
	case problemMsg:
		m.ProblemLoading = false
		m.Problem = msg.problem
		m.ProblemErr = msg.err
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
	case "problem":
		return problemDetailView(m)
	default:
		return dashboardView(m)
	}
}
