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

	"github.com/llin/cttw/internal/api"
	"github.com/llin/cttw/internal/config"
	"github.com/llin/cttw/internal/coordinator"
	"github.com/llin/cttw/internal/gitexec"
	"github.com/llin/cttw/internal/github"
	"github.com/llin/cttw/internal/llm"
	"github.com/llin/cttw/internal/store"
	"github.com/llin/cttw/internal/worker"
)

type Daemon struct {
	Store        *store.Store
	Coordinator  *coordinator.Coordinator
	Pool         *worker.Pool
	Owner        string
	Name         string
	Socket       string
	shutdown     chan struct{}
	shutdownOnce sync.Once
}

func Run() error {
	cfg, err := config.Load(os.Getenv("CTTW_CONFIG"), nil)
	if err != nil {
		return err
	}
	parts := strings.Split(cfg.Repo, "/")
	owner, name := parts[0], parts[1]

	s, err := store.New(dbPath())
	if err != nil {
		return err
	}
	defer s.Close()

	gh := github.New(cfg.GitHubToken, nil)
	llmClient := llm.New(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel, nil)

	d := &Daemon{
		Store:    s,
		Owner:    owner,
		Name:     name,
		Socket:   cfg.DaemonSocket,
		shutdown: make(chan struct{}),
	}

	coord := &coordinator.Coordinator{LLM: llmClient, GH: gh, Store: s, Owner: owner, Repo: name}
	d.Coordinator = coord

	gitRunner := &gitexec.Runner{Dir: repoDir()}
	w := &worker.Worker{GH: gh, LLM: llmClient, Store: s, Owner: owner, Repo: name, Git: gitRunner}
	d.Pool = &worker.Pool{Worker: w, Store: s}
	defer d.Pool.Stop()

	if err := ensureRepo(owner, name, cfg.GitHubToken, repoDir()); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Pool.Start(ctx)

	return d.serve()
}

func dbPath() string {
	if v := os.Getenv("CTTW_DB"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return fmt.Sprintf("%s/.local/share/cttw/cttw.db", home)
}

func ensureRepo(owner, name, token, dir string) error {
	if _, err := os.Stat(dir); err == nil {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return nil
		}
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("remove invalid repo dir %s: %w", dir, err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0755); err != nil {
		return err
	}
	repo := fmt.Sprintf("https://github.com/%s/%s.git", owner, name)
	return (&gitexec.Runner{}).Clone(repo, token, dir)
}

func repoDir() string {
	if v := os.Getenv("CTTW_REPO_DIR"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return fmt.Sprintf("%s/.local/share/cttw/repo", home)
}

func (d *Daemon) serve() error {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/tasks", d.handleCreateTask)
	mux.HandleFunc("GET /api/v1/tasks", d.handleListTasks)
	mux.HandleFunc("GET /api/v1/tasks/{id}", d.handleGetTask)
	mux.HandleFunc("GET /api/v1/status", d.handleStatus)
	mux.HandleFunc("POST /api/v1/shutdown", d.handleShutdown)

	l, addr, err := d.listen()
	if err != nil {
		return err
	}
	log.Printf("daemon listening on %s", addr)

	srv := &http.Server{Handler: mux}
	if d.shutdown != nil {
		go func() {
			<-d.shutdown
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := srv.Shutdown(ctx); err != nil {
				log.Printf("server shutdown: %v", err)
			}
			d.Pool.Stop()
			d.Store.Close()
		}()
	}

	if err := srv.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (d *Daemon) listen() (net.Listener, string, error) {
	addr := d.Socket
	if strings.HasPrefix(addr, "unix://") {
		path := strings.TrimPrefix(addr, "unix://")
		os.Remove(path)
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

func (d *Daemon) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var req api.CreateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Description) == "" {
		http.Error(w, "description is required", http.StatusBadRequest)
		return
	}
	task, err := d.Coordinator.StartTask(r.Context(), req.Description)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(taskToResponse(task, nil))
}

func (d *Daemon) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (d *Daemon) handleShutdown(w http.ResponseWriter, r *http.Request) {
	if d.shutdown != nil {
		d.shutdownOnce.Do(func() { close(d.shutdown) })
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d *Daemon) handleListTasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := d.Store.ListTasks(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp := make([]api.TaskResponse, 0)
	for i := range tasks {
		resp = append(resp, taskToResponse(&tasks[i], nil))
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (d *Daemon) handleGetTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	task, err := d.Store.GetTask(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	chunks, err := d.Store.ListChunksByTask(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(taskToResponse(task, chunks))
}

func taskToResponse(t *store.Task, chunks []store.Chunk) api.TaskResponse {
	resp := api.TaskResponse{
		ID:                t.ID,
		Description:       t.Description,
		Status:            t.Status,
		RepoOwner:         t.RepoOwner,
		RepoName:          t.RepoName,
		ParentIssueNumber: t.ParentIssueNumber,
		CreatedAt:         t.CreatedAt,
		UpdatedAt:         t.UpdatedAt,
	}
	for _, c := range chunks {
		resp.Chunks = append(resp.Chunks, api.ChunkResponse{
			ID:          c.ID,
			TaskID:      c.TaskID,
			Title:       c.Title,
			Description: c.Description,
			Status:      c.Status,
			Branch:      c.Branch,
			BaseBranch:  c.BaseBranch,
			IssueNumber: c.IssueNumber,
			PRNumber:    c.PRNumber,
			SortOrder:   c.SortOrder,
		})
	}
	return resp
}
