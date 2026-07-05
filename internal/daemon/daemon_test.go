package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/llin/cttw/internal/coordinator"
	"github.com/llin/cttw/internal/launcher"
	"github.com/llin/cttw/internal/repo"
	"github.com/llin/cttw/internal/store"
	"github.com/llin/cttw/internal/worker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockGH struct {
	issues    map[string]int
	subIssues [][2]int
}

func (m *mockGH) CreateIssue(ctx context.Context, owner, repo, title, body string) (int, error) {
	m.issues[title] = len(m.issues) + 1
	return m.issues[title], nil
}
func (m *mockGH) CreateSubIssue(ctx context.Context, owner, repo string, parentNumber, childNumber int) error {
	m.subIssues = append(m.subIssues, [2]int{parentNumber, childNumber})
	return nil
}
func (m *mockGH) CreateBranch(ctx context.Context, owner, repo, branch, base string) error { return nil }
func (m *mockGH) CreatePullRequest(ctx context.Context, owner, repo, title, body, head, base string) (int, error) {
	return 0, nil
}

func TestServer_ShutdownWaitsForWorkerLoop(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	// run() closes the store, so do not defer s.Close() here.

	ctx := context.Background()
	repoDir := filepath.Join(t.TempDir(), "repo")
	require.NoError(t, os.MkdirAll(repoDir, 0755))
	r, err := s.CreateRepo(ctx, "llin", "cttw", repoDir, "main", "")
	require.NoError(t, err)
	problem, err := s.CreateProblem(ctx, "build API", r.ID)
	require.NoError(t, err)
	_, err = s.CreateTask(ctx, problem.ID, r.ID, "add handler", "implement POST")
	require.NoError(t, err)

	ml := &launcher.MockLauncher{}
	gh := &mockGH{issues: make(map[string]int)}
	regRoot := filepath.Join(t.TempDir(), "repos")

	promptStarted := make(chan struct{})
	continuePrompt := make(chan struct{})
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{
			Responses: []string{""},
			OnPrompt: func(prompt string) {
				close(promptStarted)
				<-continuePrompt
			},
		}, nil
	}

	w := worker.New(s, ml, &repo.Registry{Root: regRoot}, gh, "codex", time.Minute)

	sockFile := filepath.Join(t.TempDir(), "cttw.sock")
	srv := &Server{
		Store:              s,
		Coordinator:        coordinator.New(s, ml, &repo.Registry{Root: regRoot}, gh, "codex", time.Minute),
		Worker:             w,
		Socket:             "unix://" + sockFile,
		shutdown:           make(chan struct{}),
		workerTickInterval: 50 * time.Millisecond,
	}

	go func() { _ = srv.run() }()
	t.Cleanup(srv.Shutdown)
	waitForSocket(t, sockFile)

	// Wait for the worker loop to start a RunOnce call.
	select {
	case <-promptStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not start task")
	}

	// Shut down while RunOnce is blocked inside the agent prompt.
	srv.Shutdown()

	// Allow the agent prompt to return.
	close(continuePrompt)

	// The worker loop should have exited before the test cleanup runs.
	srv.workerWg.Wait()
}

func TestServer_CreateProblem_MapsClientErrorsTo400(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	repoDir := filepath.Join(t.TempDir(), "repo")
	require.NoError(t, os.MkdirAll(repoDir, 0755))
	_, err = s.CreateRepo(ctx, "llin", "cttw", repoDir, "main", "")
	require.NoError(t, err)

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{
			Responses: []string{`not valid json`},
		}, nil
	}
	gh := &mockGH{issues: make(map[string]int)}
	regRoot := filepath.Join(t.TempDir(), "repos")
	coord := coordinator.New(s, ml, &repo.Registry{Root: regRoot}, gh, "codex", time.Minute)
	w := worker.New(s, ml, &repo.Registry{Root: regRoot}, gh, "codex", time.Minute)

	sockFile := filepath.Join(t.TempDir(), "cttw.sock")
	srv := &Server{
		Store:       s,
		Coordinator: coord,
		Worker:      w,
		Socket:      "unix://" + sockFile,
		shutdown:    make(chan struct{}),
	}

	go func() { _ = srv.Serve() }()
	t.Cleanup(srv.Shutdown)
	waitForSocket(t, sockFile)

	body, _ := json.Marshal(map[string]string{"owner": "llin", "repo": "cttw", "description": "build API"})
	resp, err := unixPost(sockFile, "/api/v1/problems", body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadGateway, resp.StatusCode)
}

func TestServer_CreateProblem_MapsRepoErrorTo400(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ml := &launcher.MockLauncher{}
	gh := &mockGH{issues: make(map[string]int)}
	regRoot := filepath.Join(t.TempDir(), "repos")
	coord := coordinator.New(s, ml, &repo.Registry{Root: regRoot}, gh, "codex", time.Minute)
	w := worker.New(s, ml, &repo.Registry{Root: regRoot}, gh, "codex", time.Minute)

	sockFile := filepath.Join(t.TempDir(), "cttw.sock")
	srv := &Server{
		Store:       s,
		Coordinator: coord,
		Worker:      w,
		Socket:      "unix://" + sockFile,
		shutdown:    make(chan struct{}),
	}

	go func() { _ = srv.Serve() }()
	t.Cleanup(srv.Shutdown)
	waitForSocket(t, sockFile)

	body, _ := json.Marshal(map[string]string{"owner": "llin", "repo": "cttw", "description": "build API"})
	resp, err := unixPost(sockFile, "/api/v1/problems", body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestServer_CreateAndGetProblem(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	repoDir := filepath.Join(t.TempDir(), "repo")
	require.NoError(t, os.MkdirAll(repoDir, 0755))
	_, err = s.CreateRepo(ctx, "llin", "cttw", repoDir, "main", "")
	require.NoError(t, err)

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{
			Responses: []string{`[{"title":"t1","description":"d1"}]`},
		}, nil
	}

	gh := &mockGH{issues: make(map[string]int)}
	regRoot := filepath.Join(t.TempDir(), "repos")
	coord := coordinator.New(s, ml, &repo.Registry{Root: regRoot}, gh, "codex", time.Minute)
	w := worker.New(s, ml, &repo.Registry{Root: regRoot}, gh, "codex", time.Minute)

	sockFile := filepath.Join(t.TempDir(), "cttw.sock")
	srv := &Server{
		Store:       s,
		Coordinator: coord,
		Worker:      w,
		Socket:      "unix://" + sockFile,
		shutdown:    make(chan struct{}),
	}

	go func() { _ = srv.Serve() }()
	t.Cleanup(srv.Shutdown)
	waitForSocket(t, sockFile)

	body, _ := json.Marshal(map[string]string{"owner": "llin", "repo": "cttw", "description": "build API"})
	resp, err := unixPost(sockFile, "/api/v1/problems", body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var problemResp problemResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&problemResp))
	assert.Equal(t, "build API", problemResp.Description)
	assert.Equal(t, "ready", problemResp.Status)
	assert.Greater(t, problemResp.IssueNumber, 0)

	resp, err = unixGet(sockFile, "/api/v1/problems/"+problemResp.ID)
	require.NoError(t, err)
	var got problemResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Len(t, got.Tasks, 1)
}

func waitForSocket(t *testing.T, path string) {
	require.Eventually(t, func() bool {
		_, err := os.Stat(path)
		return err == nil
	}, time.Second, 10*time.Millisecond)
}

func unixPost(sock, path string, body []byte) (*http.Response, error) {
	c := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", sock)
			},
		},
		Timeout: 5 * time.Second,
	}
	req, _ := http.NewRequest("POST", "http://unix"+path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return c.Do(req)
}

func unixGet(sock, path string) (*http.Response, error) {
	c := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", sock)
			},
		},
		Timeout: 5 * time.Second,
	}
	return c.Get("http://unix" + path)
}
