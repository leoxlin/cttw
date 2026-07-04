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
	if err := registerRepos(context.Background(), s, reg, cfg); err != nil {
		s.Close()
		return fmt.Errorf("register repos: %w", err)
	}

	ln := launcher.NewCodexLauncher(cfg)
	coord := coordinator.New(s, ln, reg, gh)
	w := worker.New(s, ln, reg, gh)

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
	defer s.Store.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.workerLoop(ctx)
	return s.Serve()
}

func (s *Server) workerLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.shutdown:
			return
		case <-ticker.C:
			if err := s.Worker.RunOnce(ctx); err != nil {
				log.Printf("worker run: %v", err)
			}
		}
	}
}

func (s *Server) Serve() error {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/problems", s.handleCreateProblem)
	mux.HandleFunc("GET /api/v1/problems", s.handleListProblems)
	mux.HandleFunc("GET /api/v1/problems/{id}", s.handleGetProblem)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
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

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	s.Shutdown()
	w.WriteHeader(http.StatusNoContent)
}

type problemResponse struct {
	ID          string         `json:"id"`
	Description string         `json:"description"`
	Status      string         `json:"status"`
	RepoID      string         `json:"repo_id"`
	IssueNumber int            `json:"issue_number"`
	Tasks       []taskResponse `json:"tasks,omitempty"`
}

type taskResponse struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	PRNumber    int    `json:"pr_number,omitempty"`
}

func problemToResponse(p *store.Problem, tasks []store.Task) problemResponse {
	resp := problemResponse{
		ID:          p.ID,
		Description: p.Description,
		Status:      p.Status,
		RepoID:      p.RepoID,
		IssueNumber: p.ParentIssueNumber,
	}
	for _, t := range tasks {
		resp.Tasks = append(resp.Tasks, taskResponse{
			ID:          t.ID,
			Title:       t.Title,
			Description: t.Description,
			Status:      t.Status,
			PRNumber:    t.PRNumber,
		})
	}
	return resp
}

func registerRepos(ctx context.Context, s *store.Store, reg *repo.Registry, cfg *config.Config) error {
	for _, rc := range cfg.Repos {
		dir := reg.Dir(rc.Owner, rc.Name)
		repo, err := reg.Ensure(ctx, rc.Owner, rc.Name, rc.DefaultBranch, cfg.GitHubToken)
		if err != nil {
			return fmt.Errorf("ensure repo %s/%s: %w", rc.Owner, rc.Name, err)
		}
		existing, err := s.GetRepoByOwnerName(ctx, rc.Owner, rc.Name)
		if err == nil {
			existing.LocalDir = repo.Dir
			existing.DefaultBranch = repo.DefaultBranch
			if err := s.UpdateRepo(ctx, existing); err != nil {
				return fmt.Errorf("update repo %s/%s: %w", rc.Owner, rc.Name, err)
			}
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("lookup repo %s/%s: %w", rc.Owner, rc.Name, err)
		}
		if _, err := s.CreateRepo(ctx, rc.Owner, rc.Name, dir, rc.DefaultBranch, ""); err != nil {
			return fmt.Errorf("register repo %s/%s: %w", rc.Owner, rc.Name, err)
		}
	}
	return nil
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
