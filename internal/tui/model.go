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

type problemDetailMsg struct {
	problem *api.ProblemResponse
	err     error
}

type problemSort string

const (
	ScreenDashboard  = "dashboard"
	ScreenNewProblem = "newtask"

	screenDashboard = ScreenDashboard
	screenNewTask   = ScreenNewProblem
	screenDetail    = "detail"

	sortCreatedAt   problemSort = "created"
	sortStatus      problemSort = "status"
	sortDescription problemSort = "description"
)

type problemAPIClient interface {
	CreateProblem(owner, repo, description string) (*api.ProblemResponse, error)
	UpdateProblem(id, description string) (*api.ProblemResponse, error)
	ListProblems() ([]api.ProblemResponse, error)
	GetProblem(id string) (*api.ProblemResponse, error)
}

var newProblemAPIClient = func(socket string) problemAPIClient {
	return api.NewClient(socket)
}

type Model struct {
	Screen        string // dashboard | detail | newtask
	Socket        string
	Width         int
	Height        int
	Problems      []api.ProblemResponse
	Cursor        int
	Detail        *api.ProblemResponse
	DetailLoading bool
	DetailErr     error
	Err           error
	Loading       bool
	Search        string
	searching     bool
	Sort          problemSort
	SortDesc      bool
	Notice        string
	newTask       newTaskModel
}

func New(socket string) *Model {
	m := &Model{
		Screen:   ScreenDashboard,
		Socket:   socket,
		Width:    defaultShellWidth,
		Height:   defaultShellHeight,
		Loading:  true,
		Sort:     sortCreatedAt,
		SortDesc: true,
		newTask:  newNewTask(socket),
	}
	m.resize(defaultShellWidth, defaultShellHeight)
	return m
}

func (m *Model) Init() tea.Cmd {
	return m.fetchProblems
}

func (m *Model) fetchProblems() tea.Msg {
	client := newProblemAPIClient(m.Socket)
	problems, err := client.ListProblems()
	return problemsMsg{problems: problems, err: err}
}

func (m *Model) fetchProblem(id string) tea.Cmd {
	return func() tea.Msg {
		client := newProblemAPIClient(m.Socket)
		problem, err := client.GetProblem(id)
		return problemDetailMsg{problem: problem, err: err}
	}
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		}

		switch m.Screen {
		case screenNewTask:
			if msg.String() == "esc" {
				m.navigate(screenDashboard)
				m.Loading = true
				m.Err = nil
				return m, m.fetchProblems
			}
			updated, cmd := m.newTask.Update(msg)
			m.newTask = updated.(newTaskModel)
			return m, cmd
		case screenDetail:
			return m.updateDetailKey(msg)
		default:
			if handled, cmd := m.updateDashboardKey(msg); handled {
				return m, cmd
			}
			switch msg.String() {
			case "q":
				return m, tea.Quit
			case "n":
				m.Notice = ""
				m.newTask = newNewTask(m.Socket)
				m.navigate(screenNewTask)
				return m, nil
			}
		}
	case problemsMsg:
		m.Problems = msg.problems
		m.Err = msg.err
		m.Loading = false
		m.clampCursor()
	case problemDetailMsg:
		m.DetailLoading = false
		m.DetailErr = msg.err
		if msg.err == nil {
			m.Detail = msg.problem
		}
	case switchToDashboardMsg:
		m.navigate(screenDashboard)
		m.Notice = msg.notice
		m.Loading = true
		m.Err = nil
		return m, m.fetchProblems
	}
	return m, nil
}

func (m *Model) updateDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "esc", "b":
		m.navigate(screenDashboard)
		m.Detail = nil
		m.DetailErr = nil
		m.DetailLoading = false
		m.Loading = true
		m.Err = nil
		return m, m.fetchProblems
	case "r":
		if m.Detail != nil {
			m.DetailLoading = true
			m.DetailErr = nil
			return m, m.fetchProblem(m.Detail.ID)
		}
	case "n":
		m.Notice = ""
		m.newTask = newNewTask(m.Socket)
		m.navigate(screenNewTask)
		return m, nil
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
		m.clampCursor()
		return true, nil
	}

	switch msg.String() {
	case "/":
		m.searching = true
		return true, nil
	case "ctrl+u":
		m.Search = ""
		m.clampCursor()
		return true, nil
	case "s":
		m.cycleSort()
		m.clampCursor()
		return true, nil
	case "a":
		m.SortDesc = !m.SortDesc
		return true, nil
	case "up", "k":
		if m.Cursor > 0 {
			m.Cursor--
		}
		return true, nil
	case "down", "j":
		if m.Cursor < len(visibleProblems(m))-1 {
			m.Cursor++
		}
		return true, nil
	case "enter", "o":
		problems := visibleProblems(m)
		if len(problems) > 0 {
			m.clampCursor()
			selected := problems[m.Cursor]
			m.navigate(screenDetail)
			m.Detail = &selected
			m.DetailErr = nil
			m.DetailLoading = true
			return true, m.fetchProblem(selected.ID)
		}
		return true, nil
	case "e":
		problems := visibleProblems(m)
		if len(problems) > 0 {
			m.clampCursor()
			selected := problems[m.Cursor]
			m.Notice = ""
			m.newTask = newEditTask(m.Socket, selected)
			m.navigate(screenNewTask)
		}
		return true, nil
	case "esc", "r":
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

func (m *Model) clampCursor() {
	maxCursor := len(visibleProblems(m)) - 1
	if maxCursor < 0 {
		m.Cursor = 0
		return
	}
	if m.Cursor > maxCursor {
		m.Cursor = maxCursor
	}
	if m.Cursor < 0 {
		m.Cursor = 0
	}
}

func (m *Model) View() string {
	content := ""
	switch m.Screen {
	case screenNewTask:
		content = m.newTask.View()
	case screenDetail:
		content = detailView(m)
	default:
		content = dashboardView(m, contentWidth(m.Width))
	}
	return renderShell(m, content)
}

func (m *Model) navigate(screen string) {
	m.Screen = screen
	m.resize(m.Width, m.Height)
}

func (m *Model) resize(width, height int) {
	if width <= 0 {
		width = defaultShellWidth
	}
	if height <= 0 {
		height = defaultShellHeight
	}
	m.Width = width
	m.Height = height
	m.newTask.SetSize(contentWidth(width), contentHeight(height))
}
