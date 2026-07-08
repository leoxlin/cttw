package daemon

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/llin/cttw/internal/config"
	"github.com/llin/cttw/internal/coordinator"
	"github.com/llin/cttw/internal/github"
	"github.com/llin/cttw/internal/launcher"
	"github.com/llin/cttw/internal/repo"
	"github.com/llin/cttw/internal/store"
	"github.com/llin/cttw/internal/worker"
)

type Server struct {
	Store        *store.Store
	Coordinator  *coordinator.Coordinator
	Worker       *worker.Worker
	Socket       string
	shutdown     chan struct{}
	shutdownOnce sync.Once
	workerWg     sync.WaitGroup

	// workerTickInterval is used by tests to speed up the worker loop.
	workerTickInterval time.Duration
}

func Run() error {
	cfg, err := config.Load(os.Getenv("CTTW_CONFIG"), nil)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	s, err := store.New(dbPath())
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}

	gh := github.New(cfg.GitHubToken, nil)
	reg := &repo.Registry{Root: reposRoot()}

	ln := launcher.NewCodexLauncher(cfg)
	coord := coordinator.New(s, ln, reg, gh, cfg.Agent.DefaultBackend, cfg.Agent.PromptTimeoutDuration(),
		coordinator.WithToken(cfg.GitHubToken),
		coordinator.WithRepoConfigs(cfg.Repos),
	)
	w := worker.New(
		s,
		ln,
		reg,
		gh,
		cfg.Agent.DefaultBackend,
		cfg.Agent.PromptTimeoutDuration(),
		worker.WithGitHubToken(cfg.GitHubToken),
	)

	srv := &Server{
		Store:       s,
		Coordinator: coord,
		Worker:      w,
		Socket:      cfg.DaemonSocket,
		shutdown:    make(chan struct{}),
	}
	return srv.run()
}

func (s *Server) run() error {
	defer func() {
		s.Shutdown()
		s.workerWg.Wait()
		s.Coordinator.Wait()
		s.Store.Close()
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Store.ResetRunningTasks(ctx); err != nil {
		return fmt.Errorf("reset running tasks: %w", err)
	}
	s.workerWg.Add(1)
	go s.workerLoop(ctx)
	return s.Serve()
}

func (s *Server) workerLoop(ctx context.Context) {
	defer s.workerWg.Done()
	interval := s.workerTickInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.shutdown:
			return
		case <-ticker.C:
			repos, err := s.Store.ListRepos(ctx)
			if err != nil {
				log.Printf("list repos for worker: %v", err)
				continue
			}
			for _, r := range repos {
				if err := s.Worker.RunOnceForRepo(ctx, r.ID); err != nil {
					log.Printf("worker run %s/%s: %v", r.Owner, r.Name, err)
				}
			}
		}
	}
}

func (s *Server) Serve() error {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/projects", s.handleCreateProject)
	mux.HandleFunc("GET /api/v1/projects", s.handleListProjects)
	mux.HandleFunc("GET /api/v1/projects/{id}", s.handleGetProject)
	mux.HandleFunc("PUT /api/v1/projects/{id}", s.handleUpdateProject)
	mux.HandleFunc("DELETE /api/v1/projects/{id}", s.handleDeleteProject)
	mux.HandleFunc("POST /api/v1/problems", s.handleCreateProblem)
	mux.HandleFunc("GET /api/v1/problems", s.handleListProblems)
	mux.HandleFunc("GET /api/v1/problems/{id}", s.handleGetProblem)
	mux.HandleFunc("PATCH /api/v1/problems/{id}", s.handleUpdateProblem)
	mux.HandleFunc("GET /api/v1/status", s.handleStatus)
	mux.HandleFunc("POST /api/v1/shutdown", s.handleShutdown)

	l, addr, err := s.listen()
	if err != nil {
		return err
	}
	log.Printf("daemon listening on %s", addr)

	srv := &http.Server{Handler: mux}
	go func() {
		<-s.shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("server shutdown: %v", err)
		}
	}()

	if err := srv.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) Shutdown() {
	s.shutdownOnce.Do(func() { close(s.shutdown) })
}

func (s *Server) listen() (net.Listener, string, error) {
	addr := s.Socket
	if strings.HasPrefix(addr, "unix://") {
		path := strings.TrimPrefix(addr, "unix://")
		_ = os.Remove(path)
		l, err := net.Listen("unix", path)
		if err != nil {
			return nil, "", err
		}
		if err := os.Chmod(path, 0600); err != nil {
			_ = l.Close()
			return nil, "", err
		}
		return l, path, nil
	}
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, "", err
	}
	return l, l.Addr().String(), nil
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Owner         string `json:"owner"`
		Name          string `json:"name"`
		LocalDir      string `json:"local_dir"`
		DefaultBranch string `json:"default_branch"`
		CloneURL      string `json:"clone_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateProjectRequest(req.Owner, req.Name, req.LocalDir); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.DefaultBranch) == "" {
		req.DefaultBranch = "main"
	}
	project, err := s.Store.CreateRepo(r.Context(), req.Owner, req.Name, req.LocalDir, req.DefaultBranch, req.CloneURL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(projectToResponse(project))
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.Store.ListRepos(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp := make([]projectResponse, 0, len(projects))
	for i := range projects {
		resp = append(resp, projectToResponse(&projects[i]))
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	project, err := s.Store.GetRepo(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(projectToResponse(project))
}

func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	project, err := s.Store.GetRepo(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var req struct {
		Owner         string `json:"owner"`
		Name          string `json:"name"`
		LocalDir      string `json:"local_dir"`
		DefaultBranch string `json:"default_branch"`
		CloneURL      string `json:"clone_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateProjectRequest(req.Owner, req.Name, req.LocalDir); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.DefaultBranch) == "" {
		req.DefaultBranch = "main"
	}
	project.Owner = req.Owner
	project.Name = req.Name
	project.LocalDir = req.LocalDir
	project.DefaultBranch = req.DefaultBranch
	project.CloneURL = req.CloneURL
	if err := s.Store.UpdateRepo(r.Context(), project); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(projectToResponse(project))
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.DeleteRepo(r.Context(), r.PathValue("id")); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func validateProjectRequest(owner, name, localDir string) error {
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(name) == "" || strings.TrimSpace(localDir) == "" {
		return errors.New("owner, name, and local_dir are required")
	}
	return nil
}

func (s *Server) handleCreateProblem(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Owner       string `json:"owner"`
		Repo        string `json:"repo"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Owner == "" || req.Repo == "" || strings.TrimSpace(req.Description) == "" {
		http.Error(w, "owner, repo, and description are required", http.StatusBadRequest)
		return
	}
	problem, err := s.Coordinator.CreateProblem(r.Context(), req.Owner, req.Repo, req.Description)
	if err != nil {
		switch {
		case errors.Is(err, coordinator.ErrRepoNotRegistered):
			http.Error(w, err.Error(), http.StatusBadRequest)
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(problemToResponse(problem, nil))
}

func (s *Server) handleListProblems(w http.ResponseWriter, r *http.Request) {
	problems, err := s.Store.ListProblems(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp := make([]problemResponse, 0, len(problems))
	for i := range problems {
		resp = append(resp, problemToResponse(&problems[i], nil))
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleGetProblem(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	problem, err := s.Store.GetProblem(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tasks, err := s.Store.ListTasksByProblem(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(problemToResponse(problem, tasks))
}

func (s *Server) handleUpdateProblem(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	description := strings.TrimSpace(req.Description)
	if description == "" {
		http.Error(w, "description is required", http.StatusBadRequest)
		return
	}
	problem, err := s.Store.GetProblem(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	problem.Description = description
	if err := s.Store.UpdateProblem(r.Context(), problem); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(problemToResponse(problem, nil))
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	s.Shutdown()
	w.WriteHeader(http.StatusNoContent)
}

type projectResponse struct {
	ID            string    `json:"id"`
	Owner         string    `json:"owner"`
	Name          string    `json:"name"`
	LocalDir      string    `json:"local_dir"`
	DefaultBranch string    `json:"default_branch"`
	CloneURL      string    `json:"clone_url,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type problemResponse struct {
	ID          string         `json:"id"`
	Description string         `json:"description"`
	Status      string         `json:"status"`
	RepoID      string         `json:"repo_id"`
	IssueNumber int            `json:"issue_number"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	Tasks       []taskResponse `json:"tasks,omitempty"`
}

type taskResponse struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	PRNumber    int       `json:"pr_number,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func projectToResponse(r *store.Repo) projectResponse {
	return projectResponse{
		ID:            r.ID,
		Owner:         r.Owner,
		Name:          r.Name,
		LocalDir:      r.LocalDir,
		DefaultBranch: r.DefaultBranch,
		CloneURL:      r.CloneURL,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}
}

func problemToResponse(p *store.Problem, tasks []store.Task) problemResponse {
	resp := problemResponse{
		ID:          p.ID,
		Description: p.Description,
		Status:      p.Status,
		RepoID:      p.RepoID,
		IssueNumber: p.ParentIssueNumber,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
	for _, t := range tasks {
		resp.Tasks = append(resp.Tasks, taskResponse{
			ID:          t.ID,
			Title:       t.Title,
			Description: t.Description,
			Status:      t.Status,
			PRNumber:    t.PRNumber,
			CreatedAt:   t.CreatedAt,
			UpdatedAt:   t.UpdatedAt,
		})
	}
	return resp
}

func dbPath() string {
	if v := os.Getenv("CTTW_DB"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "cttw", "cttw.db")
}

func reposRoot() string {
	if v := os.Getenv("CTTW_REPOS"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "cttw", "repos")
}
