package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/llin/cttw/internal/api"
)

type problemsMsg struct {
	problems []api.ProblemResponse
	err      error
}

type problemDetailMsg struct {
	problem *api.ProblemResponse
	err     error
}

type switchToDashboardMsg struct{}

type Model struct {
	Screen        string // dashboard | detail | newtask
	Socket        string
	Problems      []api.ProblemResponse
	Cursor        int
	Detail        *api.ProblemResponse
	DetailLoading bool
	DetailErr     error
	Err           error
	newTask       newTaskModel
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
		return problemDetailMsg{problem: problem, err: err}
	}
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
		switch m.Screen {
		case "newtask":
			if msg.String() == "esc" {
				m.Screen = "dashboard"
				return m, m.fetchProblems
			}
		case "detail":
			switch msg.String() {
			case "esc", "b":
				m.Screen = "dashboard"
				m.Detail = nil
				m.DetailErr = nil
				m.DetailLoading = false
				return m, m.fetchProblems
			case "r":
				if m.Detail != nil {
					m.DetailLoading = true
					m.DetailErr = nil
					return m, m.fetchProblem(m.Detail.ID)
				}
			case "n":
				m.Screen = "newtask"
				return m, nil
			}
		default:
			switch msg.String() {
			case "n":
				m.Screen = "newtask"
				return m, nil
			case "esc", "r":
				return m, m.fetchProblems
			case "up", "k":
				if m.Cursor > 0 {
					m.Cursor--
				}
			case "down", "j":
				if m.Cursor < len(m.Problems)-1 {
					m.Cursor++
				}
			case "enter", "o":
				if len(m.Problems) > 0 {
					selected := m.Problems[m.Cursor]
					m.Screen = "detail"
					m.Detail = &selected
					m.DetailErr = nil
					m.DetailLoading = true
					return m, m.fetchProblem(selected.ID)
				}
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
	case problemDetailMsg:
		m.DetailLoading = false
		m.DetailErr = msg.err
		if msg.err == nil {
			m.Detail = msg.problem
		}
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
