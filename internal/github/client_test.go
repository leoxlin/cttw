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

func TestGetPullRequest(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/o/r/pulls/42", r.URL.Path)
		assert.Equal(t, "token gh_token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"number":42,"head":{"ref":"feat/x"}}`))
	}))
	defer ts.Close()

	c := newWithURL("gh_token", ts.URL, ts.Client())
	pr, err := c.GetPullRequest(context.Background(), "o", "r", 42)
	require.NoError(t, err)
	require.NotNil(t, pr)
	assert.Equal(t, 42, pr.Number)
	assert.Equal(t, "feat/x", pr.Head.Ref)
}

func TestGetPullRequest_NotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/o/r/pulls/42", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Not Found"}`))
	}))
	defer ts.Close()

	c := newWithURL("gh_token", ts.URL, ts.Client())
	_, err := c.GetPullRequest(context.Background(), "o", "r", 42)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestNew_DefaultTimeout(t *testing.T) {
	c := New("token", nil)
	require.NotNil(t, c)
	// The concrete type stores the configured http client.
	cc, ok := c.(*client)
	require.True(t, ok)
	assert.Equal(t, defaultHTTPTimeout, cc.http.Timeout)
}

func TestRepoPathEscapesSpecialCharacters(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/o%2Fr/r/issues", r.URL.EscapedPath())
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"number":1}`))
	}))
	defer ts.Close()
	c := newWithURL("token", ts.URL, ts.Client())
	_, err := c.CreateIssue(context.Background(), "o/r", "r", "title", "body")
	require.NoError(t, err)
}
