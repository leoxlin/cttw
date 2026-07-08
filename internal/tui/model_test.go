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

	createResponse     *api.ProblemResponse
	createErr          error
	createCalls        int
	createdOwner       string
	createdRepo        string
	createdDescription string
}

func (s *stubProblemAPIClient) ListProblems() ([]api.ProblemResponse, error) {
	s.listCalls++
	return s.listProblems, s.listErr
}

func (s *stubProblemAPIClient) CreateProblem(owner, repo, description string) (*api.ProblemResponse, error) {
	s.createCalls++
	s.createdOwner = owner
	s.createdRepo = repo
	s.createdDescription = description
	return s.createResponse, s.createErr
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
	assert.Equal(t, 1, client.listCalls)
	assert.Equal(t, want, got.Problems)
	assert.Contains(t, got.View(), "Problems:")
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
	require.Equal(t, "newtask", got.Screen)
	assert.Empty(t, got.newTask.textarea.Value())
	assert.Contains(t, got.View(), "New Problem")

	updated, cmd = got.Update(tea.KeyMsg{Type: tea.KeyEsc})
	require.NotNil(t, cmd)
	got = updated.(*Model)
	assert.Equal(t, "dashboard", got.Screen)

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
	m.Screen = "newtask"

	updated, cmd := m.Update(switchToDashboardMsg{})
	require.NotNil(t, cmd)
	got := updated.(*Model)
	assert.Equal(t, "dashboard", got.Screen)

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

	assert.Contains(t, m.View(), "No problems yet.")

	m.Err = errors.New("daemon offline")
	view := m.View()
	assert.Contains(t, view, "Error: daemon offline")
	assert.Contains(t, view, "No problems yet.")
}
