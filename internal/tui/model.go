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

	screenDashboard = ScreenDashboard
	screenNewTask   = ScreenNewProblem
)

type Model struct {
	Screen   string // dashboard | newtask
	Socket   string
	Width    int
	Height   int
	Problems []api.ProblemResponse
	Err      error
	newTask  newTaskModel
}

func New(socket string) *Model {
	m := &Model{
		Screen:  ScreenDashboard,
		Socket:  socket,
		Width:   defaultShellWidth,
		Height:  defaultShellHeight,
		newTask: newNewTask(socket),
	}
	m.resize(defaultShellWidth, defaultShellHeight)
	return m
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
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "q":
			if m.Screen == screenDashboard {
				return m, tea.Quit
			}
		case "n":
			if m.Screen == screenDashboard {
				m.navigate(screenNewTask)
				return m, nil
			}
		case "esc":
			if m.Screen != screenDashboard {
				m.navigate(screenDashboard)
				return m, m.fetchProblems
			}
			return m, m.fetchProblems
		}
	case problemsMsg:
		m.Problems = msg.problems
		m.Err = msg.err
	case switchToDashboardMsg:
		m.navigate(screenDashboard)
		return m, m.fetchProblems
	}
	if m.Screen == screenNewTask {
		updated, cmd := m.newTask.Update(msg)
		m.newTask = updated.(newTaskModel)
		return m, cmd
	}
	return m, nil
}

func (m *Model) View() string {
	content := ""
	switch m.Screen {
	case screenNewTask:
		content = m.newTask.View()
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
