package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/llin/cttw/internal/coordinator"
	"github.com/llin/cttw/internal/github"
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
func (m *mockGH) CreateBranch(ctx context.Context, owner, repo, branch, base string) error {
	return nil
}
func (m *mockGH) CreatePullRequest(ctx context.Context, owner, repo, title, body, head, base string) (int, error) {
	return 0, nil
}
func (m *mockGH) GetPullRequest(ctx context.Context, owner, repo string, number int) (*github.PullRequest, error) {
	return nil, nil
}

type noopStackRunner struct{}

func (n *noopStackRunner) StackInit(base string, branches []string) error { return nil }
func (n *noopStackRunner) StackSubmit(auto, open bool) error              { return nil }

type mockRegistry struct {
	dir string
}

func (m *mockRegistry) Ensure(ctx context.Context, owner, name, defaultBranch, token string) (*repo.Repo, error) {
	branch := defaultBranch
	if branch == "" {
		branch = "main"
	}
	return &repo.Repo{Owner: owner, Name: name, Dir: m.dir, DefaultBranch: branch}, nil
}

func initDaemonGitRepo(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "repo")
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	require.NoError(t, os.MkdirAll(dir, 0755))
	run("init", "-b", "main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("base\n"), 0644))
	run("add", ".")
	run("commit", "-m", "initial")
	return dir
}

func TestServer_ShutdownWaitsForWorkerLoop(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	// run() closes the store, so do not defer s.Close() here.

	ctx := context.Background()
	repoDir := initDaemonGitRepo(t)
	r, err := s.CreateRepo(ctx, "llin", "cttw", repoDir, "main", "")
	require.NoError(t, err)
	problem, err := s.CreateProblem(ctx, "build API", r.ID)
	require.NoError(t, err)
	g, err := s.CreatePRGroup(ctx, problem.ID, r.ID, "add handler", "implement POST", 0)
	require.NoError(t, err)
	_, err = s.CreateTaskInGroup(ctx, problem.ID, r.ID, g.ID, "add handler", "implement POST", 0, 0)
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

	w := worker.New(s, ml, &repo.Registry{Root: regRoot}, gh, "codex", time.Minute,
		worker.WithStackRunnerFactory(func(dir string) worker.StackRunner { return &noopStackRunner{} }))

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

func TestServer_CreateProblem_AcceptsAsync(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	repoDir := initDaemonGitRepo(t)
	_, err = s.CreateRepo(ctx, "llin", "cttw", repoDir, "main", "")
	require.NoError(t, err)

	decompositionStarted := make(chan struct{})
	continueDecomposition := make(chan struct{})
	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{
			Responses: []string{`{"pr_groups":[{"title":"group one","description":"single group","tasks":[{"title":"t1","description":"d1"}]}]}`},
			OnPrompt: func(prompt string) {
				close(decompositionStarted)
				<-continueDecomposition
			},
		}, nil
	}
	gh := &mockGH{issues: make(map[string]int)}
	regRoot := filepath.Join(t.TempDir(), "repos")
	coord := coordinator.New(s, ml, &repo.Registry{Root: regRoot}, gh, "codex", time.Minute)
	w := worker.New(s, ml, &repo.Registry{Root: regRoot}, gh, "codex", time.Minute,
		worker.WithStackRunnerFactory(func(dir string) worker.StackRunner { return &noopStackRunner{} }))

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
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	var problemResp problemResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&problemResp))
	assert.Equal(t, "build API", problemResp.Description)
	assert.Equal(t, "pending", problemResp.Status)
	assert.NotZero(t, problemResp.CreatedAt)
	assert.NotZero(t, problemResp.UpdatedAt)

	// Wait until decomposition has started so we know the 202 preceded the work.
	select {
	case <-decompositionStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("decomposition did not start")
	}

	// Allow decomposition to finish and verify the problem becomes ready.
	close(continueDecomposition)
	require.Eventually(t, func() bool {
		p, err := s.GetProblem(ctx, problemResp.ID)
		if err != nil {
			return false
		}
		return p.Status == "ready"
	}, 2*time.Second, 10*time.Millisecond)
}

func TestServer_CreateProblem_LazyRegistersRepo(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{Responses: []string{`{"pr_groups":[{"title":"group one","description":"single group","tasks":[{"title":"t1","description":"d1"}]}]}`}}, nil
	}
	gh := &mockGH{issues: make(map[string]int)}
	reg := &mockRegistry{dir: initDaemonGitRepo(t)}
	coord := coordinator.New(s, ml, reg, gh, "codex", time.Minute, coordinator.WithToken("tok"))
	w := worker.New(s, ml, &repo.Registry{Root: t.TempDir()}, gh, "codex", time.Minute,
		worker.WithStackRunnerFactory(func(dir string) worker.StackRunner { return &noopStackRunner{} }))

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
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	var problemResp problemResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&problemResp))

	ctx := context.Background()
	require.Eventually(t, func() bool {
		p, err := s.GetProblem(ctx, problemResp.ID)
		if err != nil {
			return false
		}
		return p.Status == "ready"
	}, 2*time.Second, 10*time.Millisecond)

	r, err := s.GetRepoByOwnerName(ctx, "llin", "cttw")
	require.NoError(t, err)
	assert.Equal(t, "main", r.DefaultBranch)
}

func TestServer_ProjectCRUD(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	sockFile := filepath.Join(t.TempDir(), "cttw.sock")
	srv := &Server{
		Store:    s,
		Socket:   "unix://" + sockFile,
		shutdown: make(chan struct{}),
	}

	go func() { _ = srv.Serve() }()
	t.Cleanup(srv.Shutdown)
	waitForSocket(t, sockFile)

	body, _ := json.Marshal(map[string]string{
		"owner":          "llin",
		"name":           "cttw",
		"local_dir":      "/tmp/cttw",
		"default_branch": "main",
		"clone_url":      "https://github.com/llin/cttw.git",
	})
	resp, err := unixPost(sockFile, "/api/v1/projects", body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var created projectResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	assert.Equal(t, "llin", created.Owner)
	assert.Equal(t, "cttw", created.Name)
	assert.NotZero(t, created.CreatedAt)
	assert.NotZero(t, created.UpdatedAt)

	resp, err = unixGet(sockFile, "/api/v1/projects/"+created.ID)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var got projectResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, created.ID, got.ID)

	updateBody, _ := json.Marshal(map[string]string{
		"owner":          "llin",
		"name":           "cttw-renamed",
		"local_dir":      "/tmp/cttw-renamed",
		"default_branch": "trunk",
	})
	resp, err = unixPut(sockFile, "/api/v1/projects/"+created.ID, updateBody)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var updated projectResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&updated))
	assert.Equal(t, "cttw-renamed", updated.Name)
	assert.Equal(t, "trunk", updated.DefaultBranch)

	resp, err = unixGet(sockFile, "/api/v1/projects")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var projects []projectResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&projects))
	require.Len(t, projects, 1)
	assert.Equal(t, updated.ID, projects[0].ID)

	resp, err = unixDelete(sockFile, "/api/v1/projects/"+created.ID)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	resp, err = unixGet(sockFile, "/api/v1/projects/"+created.ID)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestServer_CreateAndGetProblem(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	repoDir := initDaemonGitRepo(t)
	_, err = s.CreateRepo(ctx, "llin", "cttw", repoDir, "main", "")
	require.NoError(t, err)

	ml := &launcher.MockLauncher{}
	ml.OnLaunch = func(spec launcher.LaunchSpec) (*launcher.MockAgent, error) {
		return &launcher.MockAgent{
			Responses: []string{`{"pr_groups":[{"title":"group one","description":"single group","tasks":[{"title":"t1","description":"d1"}]}]}`},
		}, nil
	}

	gh := &mockGH{issues: make(map[string]int)}
	regRoot := filepath.Join(t.TempDir(), "repos")
	coord := coordinator.New(s, ml, &repo.Registry{Root: regRoot}, gh, "codex", time.Minute)
	w := worker.New(s, ml, &repo.Registry{Root: regRoot}, gh, "codex", time.Minute,
		worker.WithStackRunnerFactory(func(dir string) worker.StackRunner { return &noopStackRunner{} }))

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
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	var problemResp problemResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&problemResp))
	assert.Equal(t, "build API", problemResp.Description)
	assert.Equal(t, "pending", problemResp.Status)

	var got problemResponse
	require.Eventually(t, func() bool {
		resp, err = unixGet(sockFile, "/api/v1/problems/"+problemResp.ID)
		if err != nil {
			return false
		}
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			return false
		}
		return got.Status == "ready" && len(got.Tasks) == 1
	}, 2*time.Second, 10*time.Millisecond)

	assert.NotZero(t, got.CreatedAt)
	assert.NotZero(t, got.UpdatedAt)
	require.Len(t, got.Tasks, 1)
	assert.NotZero(t, got.Tasks[0].CreatedAt)
	assert.NotZero(t, got.Tasks[0].UpdatedAt)
}

func TestServer_ListProblemsIncludesRepoAndTasks(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	r, err := s.CreateRepo(ctx, "llin", "cttw", t.TempDir(), "main", "")
	require.NoError(t, err)
	problem, err := s.CreateProblem(ctx, "build API", r.ID)
	require.NoError(t, err)
	problem.Status = "ready"
	problem.ParentIssueNumber = 42
	require.NoError(t, s.UpdateProblem(ctx, problem))
	_, err = s.CreateTask(ctx, problem.ID, r.ID, "add handler", "implement POST")
	require.NoError(t, err)
	_, err = s.CreateTask(ctx, problem.ID, r.ID, "add tests", "cover POST")
	require.NoError(t, err)

	srv := &Server{Store: s}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/problems", nil)

	srv.handleListProblems(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var got []problemResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
	require.Len(t, got, 1)
	assert.Equal(t, "llin", got[0].RepoOwner)
	assert.Equal(t, "cttw", got[0].RepoName)
	assert.Equal(t, 42, got[0].IssueNumber)
	require.Len(t, got[0].Tasks, 2)
}

func TestServer_UpdateProblem(t *testing.T) {
	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	repoDir := initDaemonGitRepo(t)
	r, err := s.CreateRepo(ctx, "llin", "cttw", repoDir, "main", "")
	require.NoError(t, err)
	problem, err := s.CreateProblem(ctx, "build API", r.ID)
	require.NoError(t, err)

	ml := &launcher.MockLauncher{}
	gh := &mockGH{issues: make(map[string]int)}
	regRoot := filepath.Join(t.TempDir(), "repos")

	sockFile := filepath.Join(t.TempDir(), "cttw.sock")
	srv := &Server{
		Store:       s,
		Coordinator: coordinator.New(s, ml, &repo.Registry{Root: regRoot}, gh, "codex", time.Minute),
		Worker:      worker.New(s, ml, &repo.Registry{Root: regRoot}, gh, "codex", time.Minute,
			worker.WithStackRunnerFactory(func(dir string) worker.StackRunner { return &noopStackRunner{} })),
		Socket:      "unix://" + sockFile,
		shutdown:    make(chan struct{}),
	}

	go func() { _ = srv.Serve() }()
	t.Cleanup(srv.Shutdown)
	waitForSocket(t, sockFile)

	body, _ := json.Marshal(map[string]string{"description": "updated API"})
	resp, err := unixPatch(sockFile, "/api/v1/problems/"+problem.ID, body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var got problemResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, "updated API", got.Description)
	assert.Equal(t, "pending", got.Status)

	stored, err := s.GetProblem(ctx, problem.ID)
	require.NoError(t, err)
	assert.Equal(t, "updated API", stored.Description)
}

func waitForSocket(t *testing.T, path string) {
	require.Eventually(t, func() bool {
		_, err := os.Stat(path)
		return err == nil
	}, 2*time.Second, 10*time.Millisecond)
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

func unixPatch(sock, path string, body []byte) (*http.Response, error) {
	c := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", sock)
			},
		},
		Timeout: 5 * time.Second,
	}
	req, _ := http.NewRequest(http.MethodPatch, "http://unix"+path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return c.Do(req)
}

func unixPut(sock, path string, body []byte) (*http.Response, error) {
	c := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", sock)
			},
		},
		Timeout: 5 * time.Second,
	}
	req, _ := http.NewRequest(http.MethodPut, "http://unix"+path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return c.Do(req)
}

func unixDelete(sock, path string) (*http.Response, error) {
	c := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", sock)
			},
		},
		Timeout: 5 * time.Second,
	}
	req, _ := http.NewRequest(http.MethodDelete, "http://unix"+path, nil)
	return c.Do(req)
}
