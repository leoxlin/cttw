package tui

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/llin/cttw/internal/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubProblemAPIClient struct {
	listProblems []api.ProblemResponse
	listErr      error
	listCalls    int

	getResponse *api.ProblemResponse
	getErr      error
	getCalls    int
	gotID       string

	createResponse     *api.ProblemResponse
	createErr          error
	createCalls        int
	createdOwner       string
	createdRepo        string
	createdDescription string

	updateResponse     *api.ProblemResponse
	updateErr          error
	updateCalls        int
	updatedID          string
	updatedDescription string
}

func (s *stubProblemAPIClient) ListProblems() ([]api.ProblemResponse, error) {
	s.listCalls++
	return s.listProblems, s.listErr
}

func (s *stubProblemAPIClient) GetProblem(id string) (*api.ProblemResponse, error) {
	s.getCalls++
	s.gotID = id
	return s.getResponse, s.getErr
}

func (s *stubProblemAPIClient) CreateProblem(owner, repo, description string) (*api.ProblemResponse, error) {
	s.createCalls++
	s.createdOwner = owner
	s.createdRepo = repo
	s.createdDescription = description
	return s.createResponse, s.createErr
}

func (s *stubProblemAPIClient) UpdateProblem(id, description string) (*api.ProblemResponse, error) {
	s.updateCalls++
	s.updatedID = id
	s.updatedDescription = description
	return s.updateResponse, s.updateErr
}

func stubProblemAPI(t *testing.T, client *stubProblemAPIClient) {
	t.Helper()

	original := newProblemAPIClient
	newProblemAPIClient = func(string) problemAPIClient {
		return client
	}
	t.Cleanup(func() {
		newProblemAPIClient = original
	})
}

func TestModel_InitLoadsProblemsFromAPI(t *testing.T) {
	want := []api.ProblemResponse{
		{ID: "problem-123456", Description: "wire dashboard data", Status: "ready", RepoID: "repo-1", IssueNumber: 7},
		{ID: "problem-abcdef", Description: "handle error banners", Status: "failed", RepoID: "repo-2", IssueNumber: 8},
	}
	client := &stubProblemAPIClient{listProblems: want}
	stubProblemAPI(t, client)

	m := New("stub-socket")
	msg := m.Init()()
	require.IsType(t, problemsMsg{}, msg)

	updated, cmd := m.Update(msg)
	require.Nil(t, cmd)
	got := updated.(*Model)

	assert.NoError(t, got.Err)
	assert.False(t, got.Loading)
	assert.Equal(t, 1, client.listCalls)
	assert.Equal(t, want, got.Problems)
	assert.Contains(t, got.View(), "ready")
	assert.Contains(t, got.View(), "wire dashboard data")
}

func TestModel_NavigatesToNewTaskAndEscRefreshesDashboard(t *testing.T) {
	want := []api.ProblemResponse{{ID: "problem-123456", Description: "refreshed item", Status: "ready"}}
	client := &stubProblemAPIClient{listProblems: want}
	stubProblemAPI(t, client)

	m := New("stub-socket")
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	require.Nil(t, cmd)
	got := updated.(*Model)
	require.Equal(t, screenNewTask, got.Screen)
	assert.Empty(t, got.newTask.ownerInput.Value())
	assert.Contains(t, got.View(), "New Problem")

	updated, cmd = got.Update(tea.KeyMsg{Type: tea.KeyEsc})
	require.NotNil(t, cmd)
	got = updated.(*Model)
	assert.Equal(t, screenDashboard, got.Screen)

	msg := cmd()
	require.IsType(t, problemsMsg{}, msg)
	updated, cmd = got.Update(msg)
	require.Nil(t, cmd)
	got = updated.(*Model)

	assert.Equal(t, 1, client.listCalls)
	assert.Equal(t, want, got.Problems)
	assert.Contains(t, got.View(), "refreshed item")
}

func TestModel_SwitchToDashboardRefreshesProblemsAfterSubmit(t *testing.T) {
	want := []api.ProblemResponse{{ID: "problem-123456", Description: "created from form", Status: "pending"}}
	client := &stubProblemAPIClient{listProblems: want}
	stubProblemAPI(t, client)

	m := New("stub-socket")
	m.Screen = screenNewTask

	updated, cmd := m.Update(switchToDashboardMsg{notice: "Problem created."})
	require.NotNil(t, cmd)
	got := updated.(*Model)
	assert.Equal(t, screenDashboard, got.Screen)
	assert.Equal(t, "Problem created.", got.Notice)

	msg := cmd()
	require.IsType(t, problemsMsg{}, msg)
	updated, cmd = got.Update(msg)
	require.Nil(t, cmd)
	got = updated.(*Model)

	assert.Equal(t, want, got.Problems)
	assert.Contains(t, got.View(), "created from form")
}

func TestModel_DashboardShowsLoadingAndErrorStates(t *testing.T) {
	m := New("unix:///nonexistent")

	assert.Contains(t, m.View(), "Loading problems...")

	m.Loading = false
	m.Err = errors.New("daemon offline")
	view := m.View()
	assert.Contains(t, view, "Error: daemon offline")
	assert.Contains(t, view, "No problems yet.")
}

func TestModel_ShellRendersDashboardNavigation(t *testing.T) {
	m := New("unix:///nonexistent")
	m.Loading = false
	m.Problems = []api.ProblemResponse{{
		ID:          "1234567890abcdef",
		Status:      "pending",
		Description: "add OAuth2 login",
	}}

	view := m.View()

	assert.Contains(t, view, "cttw")
	assert.Contains(t, view, "Problems")
	assert.Contains(t, view, "New problem")
	assert.Contains(t, view, "12345678")
	assert.Contains(t, view, "add OAuth2 login")
}

func TestModel_NavigatesToNewProblem(t *testing.T) {
	m := New("unix:///nonexistent")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	require.Nil(t, cmd)

	nm := updated.(*Model)
	assert.Equal(t, screenNewTask, nm.Screen)
	assert.Contains(t, nm.View(), "New Problem")
}

func TestModel_NewProblemKeyDoesNotTypeIntoForm(t *testing.T) {
	m := New("unix:///nonexistent")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	nm := updated.(*Model)

	assert.Nil(t, cmd)
	assert.Equal(t, screenNewTask, nm.Screen)
	assert.Empty(t, nm.newTask.ownerInput.Value())
}

func TestModel_NewProblemReceivesTextInput(t *testing.T) {
	m := New("unix:///nonexistent")
	m.Screen = screenNewTask

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})

	nm := updated.(*Model)
	assert.Equal(t, screenNewTask, nm.Screen)
	assert.Equal(t, "n", nm.newTask.ownerInput.Value())
}

func TestModel_EditProblemKeyOpensSelectedProblem(t *testing.T) {
	m := New("unix:///nonexistent")
	m.Loading = false
	m.SortDesc = false
	m.Problems = []api.ProblemResponse{
		{ID: "p1", Description: "first"},
		{ID: "p2", Description: "second"},
	}
	m.Cursor = 1

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	nm := updated.(*Model)

	assert.Nil(t, cmd)
	assert.Equal(t, screenNewTask, nm.Screen)
	assert.Equal(t, formEdit, nm.newTask.mode)
	assert.Equal(t, "p2", nm.newTask.problem.ID)
	assert.Equal(t, "second", nm.newTask.description.Value())
}

func TestModel_WindowSizeResizesShell(t *testing.T) {
	m := New("unix:///nonexistent")

	updated, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	require.Nil(t, cmd)

	nm := updated.(*Model)
	assert.Equal(t, 120, nm.Width)
	assert.Equal(t, 40, nm.Height)
	assert.GreaterOrEqual(t, nm.newTask.ownerInput.Width, 24)
	assert.GreaterOrEqual(t, nm.newTask.description.Width(), 24)
	assert.GreaterOrEqual(t, nm.newTask.description.Height(), 5)
}
