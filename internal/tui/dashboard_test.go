package tui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/llin/cttw/internal/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDashboardViewShowsCursorAndIssueMetadata(t *testing.T) {
	m := &Model{
		Problems: []api.ProblemResponse{
			{ID: "problem-1", Description: "first problem", Status: "ready", IssueNumber: 11},
			{ID: "problem-2", Description: "second problem", Status: "pending"},
		},
		Cursor: 1,
	}

	view := dashboardView(m)

	assert.Contains(t, view, "  problem-")
	assert.Contains(t, view, "> problem-")
	assert.Contains(t, view, "Issue: #11")
	assert.Contains(t, view, "Issue: none")
	assert.Contains(t, view, "[enter] details")
}

func TestProblemDetailViewShowsTasksPRsAndIssues(t *testing.T) {
	m := &Model{
		Problem: &api.ProblemResponse{
			ID:          "problem-1",
			Description: "build detail screen",
			Status:      "ready",
			RepoID:      "repo-1",
			IssueNumber: 7,
			Tasks: []api.TaskResponse{
				{
					ID:          "task-1",
					Title:       "render task",
					Description: "show task description",
					Status:      "completed",
					IssueNumber: 8,
					PRNumber:    9,
				},
			},
		},
	}

	view := problemDetailView(m)

	assert.Contains(t, view, "Description: build detail screen")
	assert.Contains(t, view, "Repo: repo-1")
	assert.Contains(t, view, "Issue: #7")
	assert.Contains(t, view, "render task")
	assert.Contains(t, view, "completed")
	assert.Contains(t, view, "Issue: #8")
	assert.Contains(t, view, "PR: #9")
	assert.Contains(t, view, "show task description")
}

func TestModelEnterFetchesSelectedProblem(t *testing.T) {
	want := api.ProblemResponse{
		ID:          "problem-2",
		Description: "selected problem",
		Status:      "ready",
		Tasks: []api.TaskResponse{
			{ID: "task-1", Title: "selected task", Status: "pending"},
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/problems/problem-2", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(want))
	}))
	defer server.Close()

	m := New(server.Listener.Addr().String())
	m.Problems = []api.ProblemResponse{
		{ID: "problem-1", Description: "first"},
		{ID: "problem-2", Description: "second"},
	}
	m.Cursor = 1

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	gotModel := updated.(*Model)
	require.NotNil(t, cmd)
	assert.Equal(t, "problem", gotModel.Screen)
	assert.True(t, gotModel.ProblemLoading)

	msg := cmd()
	problem, ok := msg.(problemMsg)
	require.True(t, ok)
	require.NoError(t, problem.err)
	require.NotNil(t, problem.problem)
	assert.Equal(t, want.ID, problem.problem.ID)

	updated, cmd = gotModel.Update(problem)
	gotModel = updated.(*Model)
	require.Nil(t, cmd)
	assert.False(t, gotModel.ProblemLoading)
	require.NotNil(t, gotModel.Problem)
	assert.Equal(t, want.Tasks[0].Title, gotModel.Problem.Tasks[0].Title)
}
