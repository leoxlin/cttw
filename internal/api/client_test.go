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

func TestClient_CreateTask(t *testing.T) {
	want := TaskResponse{ID: "t1", Description: "do it", Status: "pending"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/tasks", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var req CreateTaskRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "do it", req.Description)

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(want))
	}))
	defer server.Close()

	client := NewClient(server.Listener.Addr().String())
	got, err := client.CreateTask("do it")
	require.NoError(t, err)
	assert.Equal(t, want.ID, got.ID)
	assert.Equal(t, want.Description, got.Description)
	assert.Equal(t, want.Status, got.Status)
}

func TestClient_ListTasks(t *testing.T) {
	want := []TaskResponse{
		{ID: "t1", Description: "one", Status: "pending"},
		{ID: "t2", Description: "two", Status: "done"},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/tasks", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(want))
	}))
	defer server.Close()

	client := NewClient(server.Listener.Addr().String())
	got, err := client.ListTasks()
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestClient_GetTask(t *testing.T) {
	want := TaskResponse{ID: "t/with spaces", Description: "x", Status: "pending"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/tasks/t/with spaces", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(want))
	}))
	defer server.Close()

	client := NewClient(server.Listener.Addr().String())
	got, err := client.GetTask("t/with spaces")
	require.NoError(t, err)
	assert.Equal(t, want.ID, got.ID)
}

func TestClient_GetTaskEscapesID(t *testing.T) {
	client := NewClient("localhost:12345")
	assert.Equal(t, "http://localhost:12345/api/v1/tasks/t%2Fwith%20spaces", client.url("/api/v1/tasks/"+url.PathEscape("t/with spaces")))
}

func TestClient_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request details", http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewClient(server.Listener.Addr().String())
	_, err := client.ListTasks()
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

func TestClient_GetTaskURLTimeout(t *testing.T) {
	// Ensure the client URL helper still works with the default timeout.
	client := NewClient("localhost:12345")
	assert.Equal(t, "http://localhost:12345/api/v1/tasks/t1", client.url("/api/v1/tasks/t1"))
}

func TestClient_TimeoutApplies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.Listener.Addr().String())
	client.http.Timeout = 10 * time.Millisecond

	_, err := client.ListTasks()
	require.Error(t, err)
}
