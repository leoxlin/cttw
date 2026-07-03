package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/llin/cttw/internal/api"
)

type tasksMsg struct {
	tasks []api.TaskResponse
	err   error
}

type switchToDashboardMsg struct{}

type Model struct {
	Screen  string // dashboard | newtask
	Socket  string
	Tasks   []api.TaskResponse
	Err     error
	newTask newTaskModel
}

func New(socket string) *Model {
	return &Model{
		Screen:  "dashboard",
		Socket:  socket,
		newTask: newNewTask(socket),
	}
}

func (m *Model) Init() tea.Cmd {
	return m.fetchTasks
}

func (m *Model) fetchTasks() tea.Msg {
	client := api.NewClient(m.Socket)
	tasks, err := client.ListTasks()
	return tasksMsg{tasks: tasks, err: err}
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
			return m, m.fetchTasks
		}
	case tasksMsg:
		m.Tasks = msg.tasks
		m.Err = msg.err
	case switchToDashboardMsg:
		m.Screen = "dashboard"
		return m, m.fetchTasks
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
