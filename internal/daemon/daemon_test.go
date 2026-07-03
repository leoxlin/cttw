package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/llin/cttw/internal/api"
	"github.com/llin/cttw/internal/coordinator"
	"github.com/llin/cttw/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeLLM struct{ resp string }

func (f *fakeLLM) Chat(ctx context.Context, system, user string) (string, error) {
	return f.resp, nil
}

type fakeGH struct{ issueCount int }

func (f *fakeGH) CreateIssue(ctx context.Context, owner, repo, title, body string) (int, error) {
	f.issueCount++
	return f.issueCount, nil
}
func (f *fakeGH) CreateSubIssue(ctx context.Context, owner, repo string, parentNumber, childNumber int) error {
	return nil
}
func (f *fakeGH) CreateBranch(ctx context.Context, owner, repo, branch, base string) error {
	return nil
}
func (f *fakeGH) CreatePullRequest(ctx context.Context, owner, repo, title, body, head, base string) (int, error) {
	return 1, nil
}

func TestHandleCreateTask(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	coord := &coordinator.Coordinator{
		LLM:   &fakeLLM{resp: `[{"title":"c1","description":"d1"}]`},
		GH:    &fakeGH{},
		Store: s,
		Owner: "o",
		Repo:  "r",
	}
	d := &Daemon{Store: s, Owner: "o", Name: "r", Coordinator: coord}
	body, _ := json.Marshal(api.CreateTaskRequest{Description: "test task"})
	req := httptest.NewRequest("POST", "/api/v1/tasks", bytes.NewReader(body))
	w := httptest.NewRecorder()
	d.handleCreateTask(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)
	var resp api.TaskResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "test task", resp.Description)
}

func TestHandleStatus(t *testing.T) {
	d := &Daemon{}
	req := httptest.NewRequest("GET", "/api/v1/status", nil)
	w := httptest.NewRecorder()
	d.handleStatus(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "ok", resp["status"])
}

func TestHandleGetTask(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	task, err := s.CreateTask(ctx, "get me", "o", "r")
	require.NoError(t, err)

	d := &Daemon{Store: s}
	req := httptest.NewRequest("GET", "/api/v1/tasks/"+task.ID, nil)
	req.SetPathValue("id", task.ID)
	w := httptest.NewRecorder()
	d.handleGetTask(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp api.TaskResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, task.ID, resp.ID)
	assert.Equal(t, "get me", resp.Description)
}

func TestHandleGetTaskNotFound(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	d := &Daemon{Store: s}
	req := httptest.NewRequest("GET", "/api/v1/tasks/does-not-exist", nil)
	req.SetPathValue("id", "does-not-exist")
	w := httptest.NewRecorder()
	d.handleGetTask(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleListTasks(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	_, err = s.CreateTask(ctx, "first", "o", "r")
	require.NoError(t, err)
	_, err = s.CreateTask(ctx, "second", "o", "r")
	require.NoError(t, err)

	d := &Daemon{Store: s}
	req := httptest.NewRequest("GET", "/api/v1/tasks", nil)
	w := httptest.NewRecorder()
	d.handleListTasks(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp []api.TaskResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp, 2)
}
