package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_CreateProblem_Accepts202(t *testing.T) {
	want := ProblemResponse{
		ID:          "p1",
		Description: "build API",
		Status:      "pending",
		RepoID:      "r1",
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/problems", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		require.NoError(t, json.NewEncoder(w).Encode(want))
	}))
	defer server.Close()

	client := NewClient(server.Listener.Addr().String())
	got, err := client.CreateProblem("llin", "cttw", "build API")
	require.NoError(t, err)
	assert.Equal(t, want, *got)
}

func TestClient_CreateProblem(t *testing.T) {
	want := ProblemResponse{
		ID:          "p1",
		Description: "build API",
		Status:      "ready",
		RepoID:      "r1",
		IssueNumber: 5,
		Tasks: []TaskResponse{
			{ID: "t1", Title: "add handler", Description: "impl POST", Status: "pending"},
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/problems", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var req CreateProblemRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "llin", req.Owner)
		assert.Equal(t, "cttw", req.Repo)
		assert.Equal(t, "build API", req.Description)

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(want))
	}))
	defer server.Close()

	client := NewClient(server.Listener.Addr().String())
	got, err := client.CreateProblem("llin", "cttw", "build API")
	require.NoError(t, err)
	assert.Equal(t, want, *got)
}

func TestClient_ListProblems(t *testing.T) {
	want := []ProblemResponse{
		{ID: "p1", Description: "one", Status: "ready", RepoID: "r1", IssueNumber: 1},
		{ID: "p2", Description: "two", Status: "done", RepoID: "r2", IssueNumber: 2},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/problems", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(want))
	}))
	defer server.Close()

	client := NewClient(server.Listener.Addr().String())
	got, err := client.ListProblems()
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestClient_GetProblem(t *testing.T) {
	want := ProblemResponse{
		ID:          "p/with spaces",
		Description: "x",
		Status:      "ready",
		RepoID:      "r1",
		IssueNumber: 5,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/problems/p/with spaces", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(want))
	}))
	defer server.Close()

	client := NewClient(server.Listener.Addr().String())
	got, err := client.GetProblem("p/with spaces")
	require.NoError(t, err)
	assert.Equal(t, want, *got)
}

func TestClient_GetProblemEscapesID(t *testing.T) {
	client := NewClient("localhost:12345")
	assert.Equal(t, "http://localhost:12345/api/v1/problems/p%2Fwith%20spaces", client.url("/api/v1/problems/"+url.PathEscape("p/with spaces")))
}

func TestClient_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request details", http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewClient(server.Listener.Addr().String())
	_, err := client.ListProblems()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "400")
	assert.Contains(t, err.Error(), "bad request details")
}

func TestClient_NewClientTimeout(t *testing.T) {
	client := NewClient("localhost:12345")
	require.NotNil(t, client.http)
	assert.Equal(t, defaultRequestTimeout, client.http.Timeout)
}

func TestClient_NewClientUnixTimeout(t *testing.T) {
	client := NewClient("unix:///tmp/cttw.sock")
	require.NotNil(t, client.http)
	assert.Equal(t, defaultRequestTimeout, client.http.Timeout)
	assert.NotNil(t, client.http.Transport)
}

func TestClient_GetProblemURLTimeout(t *testing.T) {
	// Ensure the client URL helper still works with the default timeout.
	client := NewClient("localhost:12345")
	assert.Equal(t, "http://localhost:12345/api/v1/problems/p1", client.url("/api/v1/problems/p1"))
}

func TestClient_TimeoutApplies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.Listener.Addr().String())
	client.http.Timeout = 10 * time.Millisecond

	_, err := client.ListProblems()
	require.Error(t, err)
}
