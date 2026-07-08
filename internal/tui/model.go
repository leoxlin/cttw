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
	Screen   string // dashboard | newtask | problem
	Socket   string
	Problems []api.ProblemResponse
	Cursor   int
	Problem  *api.ProblemResponse
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
	case problemsMsg:
		m.Problems = msg.problems
		m.Err = msg.err
		m.clampCursor()
		return m, nil
	case problemMsg:
		if msg.problem != nil {
			m.Problem = msg.problem
		}
		m.Err = msg.err
		return m, nil
	case switchToDashboardMsg:
		m.Screen = "dashboard"
		m.Problem = nil
		return m, m.fetchProblems
	}

	if m.Screen == "newtask" {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
		}
		updated, cmd := m.newTask.Update(msg)
		m.newTask = updated.(newTaskModel)
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch key := msg.String(); key {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "n":
			m.newTask = newNewTask(m.Socket)
			m.Screen = "newtask"
		case "r", "f5":
			if m.Screen == "problem" && m.Problem != nil {
				return m, m.fetchProblem(m.Problem.ID)
			}
			return m, m.fetchProblems
		case "esc", "b":
			if m.Screen == "problem" {
				m.Screen = "dashboard"
				m.Problem = nil
				return m, m.fetchProblems
			}
		case "up", "k":
			if m.Screen == "dashboard" {
				m.moveCursor(-1)
			}
		case "down", "j":
			if m.Screen == "dashboard" {
				m.moveCursor(1)
			}
		case "home", "g":
			if m.Screen == "dashboard" {
				m.Cursor = 0
			}
		case "end", "G":
			if m.Screen == "dashboard" && len(m.Problems) > 0 {
				m.Cursor = len(m.Problems) - 1
			}
		case "enter":
			if m.Screen == "dashboard" && len(m.Problems) > 0 {
				problem := m.Problems[m.Cursor]
				m.Screen = "problem"
				m.Problem = &problem
				m.Err = nil
				return m, m.fetchProblem(problem.ID)
			}
		}
	}
	return m, nil
}

func (m *Model) View() string {
	switch m.Screen {
	case "newtask":
		return m.newTask.View()
	case "problem":
		return problemView(m)
	default:
		return dashboardView(m)
	}
}

func (m *Model) moveCursor(delta int) {
	if len(m.Problems) == 0 {
		m.Cursor = 0
		return
	}
	m.Cursor += delta
	m.clampCursor()
}

func (m *Model) clampCursor() {
	if len(m.Problems) == 0 || m.Cursor < 0 {
		m.Cursor = 0
		return
	}
	if m.Cursor >= len(m.Problems) {
		m.Cursor = len(m.Problems) - 1
	}
}
