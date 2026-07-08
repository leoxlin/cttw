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

type problemSort string

const (
	sortCreatedAt   problemSort = "created"
	sortStatus      problemSort = "status"
	sortDescription problemSort = "description"
)

type Model struct {
	Screen    string // dashboard | newtask
	Socket    string
	Problems  []api.ProblemResponse
	Err       error
	Loading   bool
	Search    string
	searching bool
	Sort      problemSort
	SortDesc  bool
	newTask   newTaskModel
}

func New(socket string) *Model {
	return &Model{
		Screen:   "dashboard",
		Socket:   socket,
		Loading:  true,
		Sort:     sortCreatedAt,
		SortDesc: true,
		newTask:  newNewTask(socket),
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
		if m.Screen == "dashboard" {
			if handled, cmd := m.updateDashboardKey(msg); handled {
				return m, cmd
			}
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "n":
			m.Screen = "newtask"
			return m, nil
		case "esc":
			m.Screen = "dashboard"
			m.Loading = true
			m.Err = nil
			return m, m.fetchProblems
		}
	case problemsMsg:
		m.Problems = msg.problems
		m.Err = msg.err
		m.Loading = false
	case switchToDashboardMsg:
		m.Screen = "dashboard"
		m.Loading = true
		m.Err = nil
		return m, m.fetchProblems
	}
	if m.Screen == "newtask" {
		updated, cmd := m.newTask.Update(msg)
		m.newTask = updated.(newTaskModel)
		return m, cmd
	}
	return m, nil
}

func (m *Model) updateDashboardKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	if m.searching {
		switch msg.String() {
		case "enter", "esc":
			m.searching = false
		case "backspace", "ctrl+h":
			runes := []rune(m.Search)
			if len(runes) > 0 {
				m.Search = string(runes[:len(runes)-1])
			}
		case "ctrl+u":
			m.Search = ""
		case "ctrl+c":
			return false, nil
		default:
			if len(msg.Runes) > 0 {
				m.Search += string(msg.Runes)
			}
		}
		return true, nil
	}

	switch msg.String() {
	case "/":
		m.searching = true
		return true, nil
	case "ctrl+u":
		m.Search = ""
		return true, nil
	case "s":
		m.cycleSort()
		return true, nil
	case "a":
		m.SortDesc = !m.SortDesc
		return true, nil
	case "esc":
		m.Loading = true
		m.Err = nil
		return true, m.fetchProblems
	}
	return false, nil
}

func (m *Model) cycleSort() {
	switch m.Sort {
	case sortCreatedAt:
		m.Sort = sortStatus
	case sortStatus:
		m.Sort = sortDescription
	default:
		m.Sort = sortCreatedAt
	}
}

func (m *Model) View() string {
	switch m.Screen {
	case "newtask":
		return m.newTask.View()
	default:
		return dashboardView(m)
	}
}
