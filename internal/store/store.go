package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// Store provides persistent storage for tasks backed by SQLite.
type Store struct {
	db *sql.DB
}

// Task represents a unit of work tracked by cttw.
type Task struct {
	ID                string
	Description       string
	Status            string
	RepoOwner         string
	RepoName          string
	ParentIssueNumber int
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// New opens the SQLite database at dbPath and runs migrations.
func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// SQLite in-memory databases are per-connection; limit the pool to one
	// connection so that all operations share the same database.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := migrate(db); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db}, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

func migrate(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			description TEXT NOT NULL,
			status TEXT NOT NULL,
			repo_owner TEXT NOT NULL,
			repo_name TEXT NOT NULL,
			parent_issue_number INTEGER,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS chunks (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			title TEXT NOT NULL,
			description TEXT NOT NULL,
			status TEXT NOT NULL,
			depends_on_chunk_id TEXT,
			output TEXT,
			branch TEXT,
			base_branch TEXT,
			issue_number INTEGER,
			pr_number INTEGER,
			sort_order INTEGER NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS jobs (
			id TEXT PRIMARY KEY,
			chunk_id TEXT NOT NULL,
			type TEXT NOT NULL,
			status TEXT NOT NULL,
			error TEXT,
			attempts INTEGER NOT NULL DEFAULT 0,
			max_attempts INTEGER NOT NULL DEFAULT 3,
			created_at DATETIME,
			started_at DATETIME,
			completed_at DATETIME
		);`,
		`CREATE TABLE IF NOT EXISTS config (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

// CreateTask inserts a new pending task and returns it.
func (s *Store) CreateTask(ctx context.Context, description, owner, name string) (*Task, error) {
	now := time.Now().UTC()
	t := &Task{
		ID:          uuid.New().String(),
		Description: description,
		Status:      "pending",
		RepoOwner:   owner,
		RepoName:    name,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO tasks (id, description, status, repo_owner, repo_name, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Description, t.Status, t.RepoOwner, t.RepoName, t.CreatedAt, t.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create task: %w", err)
	}
	return t, nil
}

// GetTask retrieves a task by its ID.
func (s *Store) GetTask(ctx context.Context, id string) (*Task, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, description, status, repo_owner, repo_name, parent_issue_number, created_at, updated_at
		 FROM tasks WHERE id = ?`, id)
	return scanTask(row)
}

// ListTasks returns all tasks ordered by creation time, newest first.
func (s *Store) ListTasks(ctx context.Context) ([]Task, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, description, status, repo_owner, repo_name, parent_issue_number, created_at, updated_at
		 FROM tasks ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, *t)
	}
	return tasks, rows.Err()
}

// UpdateTask persists changes to an existing task.
func (s *Store) UpdateTask(ctx context.Context, task *Task) error {
	task.UpdatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET description=?, status=?, repo_owner=?, repo_name=?, parent_issue_number=?, updated_at=?
		 WHERE id = ?`,
		task.Description, task.Status, task.RepoOwner, task.RepoName, task.ParentIssueNumber,
		task.UpdatedAt, task.ID,
	)
	return err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanTask(s scanner) (*Task, error) {
	t := &Task{}
	var parent sql.NullInt64
	err := s.Scan(&t.ID, &t.Description, &t.Status, &t.RepoOwner, &t.RepoName,
		&parent, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if parent.Valid {
		t.ParentIssueNumber = int(parent.Int64)
	}
	return t, nil
}

// Chunk represents a discrete unit of work within a task.
type Chunk struct {
	ID               string
	TaskID           string
	Title            string
	Description      string
	Status           string
	DependsOnChunkID string
	Output           string
	Branch           string
	BaseBranch       string
	IssueNumber      int
	PRNumber         int
	SortOrder        int
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// CreateChunk inserts a new pending chunk and returns it.
func (s *Store) CreateChunk(ctx context.Context, c Chunk) (*Chunk, error) {
	now := time.Now().UTC()
	c.ID = uuid.New().String()
	c.Status = "pending"
	c.CreatedAt = now
	c.UpdatedAt = now
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO chunks (id, task_id, title, description, status, depends_on_chunk_id, output, branch, base_branch, issue_number, pr_number, sort_order, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.TaskID, c.Title, c.Description, c.Status, c.DependsOnChunkID, c.Output,
		c.Branch, c.BaseBranch, c.IssueNumber, c.PRNumber, c.SortOrder, c.CreatedAt, c.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create chunk: %w", err)
	}
	return &c, nil
}

// GetChunk retrieves a chunk by its ID.
func (s *Store) GetChunk(ctx context.Context, id string) (*Chunk, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, task_id, title, description, status, depends_on_chunk_id, output, branch, base_branch, issue_number, pr_number, sort_order, created_at, updated_at
		 FROM chunks WHERE id = ?`, id)
	return scanChunk(row)
}

// ListChunksByTask returns all chunks for a task ordered by sort_order.
func (s *Store) ListChunksByTask(ctx context.Context, taskID string) ([]Chunk, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, task_id, title, description, status, depends_on_chunk_id, output, branch, base_branch, issue_number, pr_number, sort_order, created_at, updated_at
		 FROM chunks WHERE task_id = ? ORDER BY sort_order`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var chunks []Chunk
	for rows.Next() {
		c, err := scanChunk(rows)
		if err != nil {
			return nil, err
		}
		chunks = append(chunks, *c)
	}
	return chunks, rows.Err()
}

// UpdateChunk persists changes to an existing chunk.
func (s *Store) UpdateChunk(ctx context.Context, c *Chunk) error {
	c.UpdatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`UPDATE chunks SET title=?, description=?, status=?, depends_on_chunk_id=?, output=?, branch=?, base_branch=?, issue_number=?, pr_number=?, sort_order=?, updated_at=?
		 WHERE id = ?`,
		c.Title, c.Description, c.Status, c.DependsOnChunkID, c.Output, c.Branch, c.BaseBranch,
		c.IssueNumber, c.PRNumber, c.SortOrder, c.UpdatedAt, c.ID,
	)
	return err
}

func scanChunk(s scanner) (*Chunk, error) {
	c := &Chunk{}
	err := s.Scan(&c.ID, &c.TaskID, &c.Title, &c.Description, &c.Status, &c.DependsOnChunkID,
		&c.Output, &c.Branch, &c.BaseBranch, &c.IssueNumber, &c.PRNumber, &c.SortOrder,
		&c.CreatedAt, &c.UpdatedAt)
	return c, err
}

// Job represents a queued operation for a chunk.
type Job struct {
	ID          string
	ChunkID     string
	Type        string
	Status      string
	Error       string
	Attempts    int
	MaxAttempts int
	CreatedAt   *time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
}

// CreateJob inserts a new pending job and returns it.
func (s *Store) CreateJob(ctx context.Context, j Job) (*Job, error) {
	now := time.Now().UTC()
	j.ID = uuid.New().String()
	j.Status = "pending"
	j.CreatedAt = &now
	if j.MaxAttempts == 0 {
		j.MaxAttempts = 3
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO jobs (id, chunk_id, type, status, error, attempts, max_attempts, created_at, started_at, completed_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		j.ID, j.ChunkID, j.Type, j.Status, j.Error, j.Attempts, j.MaxAttempts,
		j.CreatedAt, j.StartedAt, j.CompletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create job: %w", err)
	}
	return &j, nil
}

// GetJob retrieves a job by its ID.
func (s *Store) GetJob(ctx context.Context, id string) (*Job, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, chunk_id, type, status, error, attempts, max_attempts, created_at, started_at, completed_at
		 FROM jobs WHERE id = ?`, id)
	return scanJob(row)
}

// UpdateJob persists changes to an existing job.
func (s *Store) UpdateJob(ctx context.Context, j *Job) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE jobs SET chunk_id=?, type=?, status=?, error=?, attempts=?, max_attempts=?, created_at=?, started_at=?, completed_at=?
		 WHERE id = ?`,
		j.ChunkID, j.Type, j.Status, j.Error, j.Attempts, j.MaxAttempts,
		j.CreatedAt, j.StartedAt, j.CompletedAt, j.ID,
	)
	return err
}

// NextPendingJob selects the oldest pending job whose chunk dependencies are
// satisfied, marks it as running, and returns it. If no job is available it
// returns nil, nil.
func (s *Store) NextPendingJob(ctx context.Context) (*Job, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT j.id, j.chunk_id, j.type, j.status, j.error, j.attempts, j.max_attempts, j.created_at, j.started_at, j.completed_at
		 FROM jobs j
		 JOIN chunks c ON c.id = j.chunk_id
		 WHERE j.status = 'pending'
		   AND j.attempts < j.max_attempts
		   AND (c.depends_on_chunk_id IS NULL OR c.depends_on_chunk_id = '' OR
		        (SELECT status FROM chunks WHERE id = c.depends_on_chunk_id) = 'completed')
		 ORDER BY j.created_at LIMIT 1`)
	j, err := scanJob(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	j.Status = "running"
	return j, s.UpdateJob(ctx, j)
}

func scanJob(s scanner) (*Job, error) {
	j := &Job{}
	err := s.Scan(&j.ID, &j.ChunkID, &j.Type, &j.Status, &j.Error, &j.Attempts, &j.MaxAttempts,
		&j.CreatedAt, &j.StartedAt, &j.CompletedAt)
	return j, err
}

// SetConfigValue stores or updates a config value by key.
func (s *Store) SetConfigValue(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO config (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// GetConfigValue retrieves a config value by key.
func (s *Store) GetConfigValue(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM config WHERE key = ?`, key).Scan(&value)
	return value, err
}
