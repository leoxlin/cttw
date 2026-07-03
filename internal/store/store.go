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
