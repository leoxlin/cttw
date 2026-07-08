package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/llin/cttw/internal/api"
)

const (
	formCreate = "create"
	formEdit   = "edit"
)

type newTaskModel struct {
	mode        string
	problem     api.ProblemResponse
	ownerInput  textinput.Model
	repoInput   textinput.Model
	description textarea.Model
	focus       int
	socket      string
	err         error
	sent        bool
	done        bool
}

type submitProblemMsg struct {
	err error
}

func newNewTask(socket string) newTaskModel {
	owner := textinput.New()
	owner.Placeholder = "owner"
	owner.Focus()

	repo := textinput.New()
	repo.Placeholder = "repo"

	desc := textarea.New()
	desc.Placeholder = "Describe the problem..."
	desc.ShowLineNumbers = false

	return newTaskModel{
		mode:        formCreate,
		ownerInput:  owner,
		repoInput:   repo,
		description: desc,
		socket:      socket,
	}
}

func newEditTask(socket string, problem api.ProblemResponse) newTaskModel {
	owner := textinput.New()
	repo := textinput.New()
	desc := textarea.New()
	desc.Placeholder = "Describe the problem..."
	desc.ShowLineNumbers = false
	desc.SetValue(problem.Description)
	desc.Focus()

	return newTaskModel{
		mode:        formEdit,
		problem:     problem,
		ownerInput:  owner,
		repoInput:   repo,
		description: desc,
		socket:      socket,
		focus:       2,
	}
}

func (n newTaskModel) Init() tea.Cmd { return textarea.Blink }

func (n newTaskModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return n, func() tea.Msg { return switchToDashboardMsg{} }
		case "tab", "shift+tab", "up", "down":
			n.moveFocus(msg.String())
		case "ctrl+d":
			value, err := n.validate()
			if err != nil {
				n.err = err
				n.sent = false
				n.done = false
				return n, nil
			}
			n.sent = true
			n.err = nil
			n.done = false
			return n, n.submit(value)
		}
	case submitProblemMsg:
		n.sent = false
		if msg.err != nil {
			n.err = msg.err
			n.done = false
			return n, nil
		}
		n.err = nil
		n.done = true
		notice := "Problem created."
		if n.mode == formEdit {
			notice = "Problem updated."
		}
		return n, func() tea.Msg { return switchToDashboardMsg{notice: notice} }
	}

	var cmd tea.Cmd
	if n.mode == formCreate && n.focus == 0 {
		n.ownerInput, cmd = n.ownerInput.Update(msg)
		cmds = append(cmds, cmd)
	} else if n.mode == formCreate && n.focus == 1 {
		n.repoInput, cmd = n.repoInput.Update(msg)
		cmds = append(cmds, cmd)
	} else {
		n.description, cmd = n.description.Update(msg)
		cmds = append(cmds, cmd)
	}
	return n, tea.Batch(cmds...)
}

func (n *newTaskModel) moveFocus(key string) {
	if n.mode == formEdit {
		n.focus = 2
		n.applyFocus()
		return
	}

	if key == "shift+tab" || key == "up" {
		n.focus--
	} else {
		n.focus++
	}
	if n.focus < 0 {
		n.focus = 2
	}
	if n.focus > 2 {
		n.focus = 0
	}
	n.applyFocus()
}

func (n *newTaskModel) applyFocus() {
	n.ownerInput.Blur()
	n.repoInput.Blur()
	n.description.Blur()
	if n.mode == formCreate && n.focus == 0 {
		n.ownerInput.Focus()
	} else if n.mode == formCreate && n.focus == 1 {
		n.repoInput.Focus()
	} else {
		n.description.Focus()
	}
}

func (n newTaskModel) validate() (problemFormValue, error) {
	value := problemFormValue{
		owner:       strings.TrimSpace(n.ownerInput.Value()),
		repo:        strings.TrimSpace(n.repoInput.Value()),
		description: strings.TrimSpace(n.description.Value()),
	}
	if n.mode == formEdit {
		value.id = n.problem.ID
		if value.description == "" {
			return value, fmt.Errorf("description is required")
		}
		return value, nil
	}
	if value.owner == "" {
		return value, fmt.Errorf("owner is required")
	}
	if value.repo == "" {
		return value, fmt.Errorf("repo is required")
	}
	if strings.Contains(value.owner, "/") || strings.Contains(value.repo, "/") {
		return value, fmt.Errorf("repo must be split into owner and name")
	}
	if value.description == "" {
		return value, fmt.Errorf("description is required")
	}
	return value, nil
}

func (n newTaskModel) View() string {
	if n.done {
		title := "Problem created"
		if n.mode == formEdit {
			title = "Problem updated"
		}
		return sectionTitleStyle.Render(title) + "\n\n" + helpStyle.Render("[esc] back")
	}
	if n.sent {
		return sectionTitleStyle.Render("Submitting...") + "\n\n" + helpStyle.Render("[esc] back")
	}

	title := "New Problem"
	if n.mode == formEdit {
		title = "Edit Problem"
	}
	view := sectionTitleStyle.Render(title) + "\n" +
		helpStyle.Render("ctrl+d to submit, esc to cancel") + "\n\n"
	if n.mode == formCreate {
		view += "Owner\n" + n.ownerInput.View() + "\n\n"
		view += "Repo\n" + n.repoInput.View() + "\n\n"
	}
	view += "Description\n" + n.description.View()
	if n.err != nil {
		view += "\n\n" + errorStyle.Render("Error: "+n.err.Error())
	}
	return view
}

func (n *newTaskModel) SetSize(width, height int) {
	width = maxInt(width, 24)
	n.ownerInput.Width = width
	n.repoInput.Width = width
	n.description.SetWidth(width)
	n.description.SetHeight(maxInt(height-8, 5))
}

type problemFormValue struct {
	id          string
	owner       string
	repo        string
	description string
}

func (n newTaskModel) submit(value problemFormValue) tea.Cmd {
	return func() tea.Msg {
		client := api.NewClient(n.socket)
		if n.mode == formEdit {
			_, err := client.UpdateProblem(value.id, value.description)
			return submitProblemMsg{err: err}
		}
		_, err := client.CreateProblem(value.owner, value.repo, value.description)
		return submitProblemMsg{err: err}
	}
}
