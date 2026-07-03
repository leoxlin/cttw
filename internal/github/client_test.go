package github

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateIssue(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/o/r/issues", r.URL.Path)
		assert.Equal(t, "token gh_token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"number":123}`))
	}))
	defer ts.Close()

	c := newWithURL("gh_token", ts.URL, ts.Client())
	n, err := c.CreateIssue(context.Background(), "o", "r", "title", "body")
	require.NoError(t, err)
	assert.Equal(t, 123, n)
}

func TestCreateSubIssue(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			assert.Equal(t, "/repos/o/r/issues/1", r.URL.Path)
			assert.Equal(t, "token gh_token", r.Header.Get("Authorization"))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"number":1,"title":"Parent title","body":"Task tracked by cttw."}`))
		case "PATCH":
			assert.Equal(t, "/repos/o/r/issues/1", r.URL.Path)
			body, _ := io.ReadAll(r.Body)
			assert.JSONEq(t, `{"title":"Parent title","body":"Task tracked by cttw.\n- [ ] #2\n"}`, string(body))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"number":1}`))
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer ts.Close()

	c := newWithURL("gh_token", ts.URL, ts.Client())
	err := c.CreateSubIssue(context.Background(), "o", "r", 1, 2)
	require.NoError(t, err)
}

func TestCreateBranch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/o/r/git/ref/heads/main":
			w.Write([]byte(`{"object":{"sha":"abc123"}}`))
		case "/repos/o/r/git/refs":
			w.WriteHeader(http.StatusCreated)
		}
	}))
	defer ts.Close()
	c := newWithURL("token", ts.URL, ts.Client())
	require.NoError(t, c.CreateBranch(context.Background(), "o", "r", "feat/x", "main"))
}

func TestCreatePullRequest(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/o/r/pulls", r.URL.Path)
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"number":99}`))
	}))
	defer ts.Close()
	c := newWithURL("token", ts.URL, ts.Client())
	n, err := c.CreatePullRequest(context.Background(), "o", "r", "title", "body", "feat/x", "main")
	require.NoError(t, err)
	assert.Equal(t, 99, n)
}
