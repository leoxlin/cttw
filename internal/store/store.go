package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// Store provides persistent storage for repos, problems, and tasks backed by SQLite.
type Store struct {
	db *sql.DB
	// test-only hooks; when non-nil the named method returns this error instead
	// of executing. They are intended for tests that need to simulate persistence
	// failures while keeping the database readable.
	updateProblemErr  func() error
	updateTaskErr     error
	updatePRGroupErr  error
}

// StoreOption configures a Store during construction.
type StoreOption func(*Store)

// WithUpdateProblemError returns a StoreOption that makes UpdateProblem return
// err on every call. It is intended for tests.
func WithUpdateProblemError(err error) StoreOption {
	return func(s *Store) { s.updateProblemErr = func() error { return err } }
}

// WithUpdateProblemErrorFunc returns a StoreOption that makes UpdateProblem
// return the result of fn on each call. It is intended for tests that need to
// fail a specific invocation, e.g. the update after a GitHub issue is created.
func WithUpdateProblemErrorFunc(fn func() error) StoreOption {
	return func(s *Store) { s.updateProblemErr = fn }
}

// WithUpdateTaskError returns a StoreOption that makes UpdateTask return err.
// It is intended for tests.
func WithUpdateTaskError(err error) StoreOption {
	return func(s *Store) { s.updateTaskErr = err }
}

// WithUpdatePRGroupError returns a StoreOption that makes UpdatePRGroup return err.
// It is intended for tests.
func WithUpdatePRGroupError(err error) StoreOption {
	return func(s *Store) { s.updatePRGroupErr = err }
}

// New opens the SQLite database at dbPath and runs migrations.
func New(dbPath string, opts ...StoreOption) (*Store, error) {
	isMemory := dbPath == ":memory:" || strings.HasPrefix(dbPath, "file::memory:")
	if !isMemory {
		if err := ensureDBDir(dbPath); err != nil {
			return nil, err
		}
	}
	if !isMemory && !strings.Contains(dbPath, "_pragma=busy_timeout") {
		if strings.Contains(dbPath, "?") {
			dbPath += "&_pragma=busy_timeout(5000)"
		} else {
			dbPath += "?_pragma=busy_timeout(5000)"
		}
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// SQLite in-memory databases are per-connection; limit the pool to one
	// connection so that all operations share the same database.
	if isMemory {
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
	}
	if err := migrate(db); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	s := &Store{db: db}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

// ensureDBDir creates the parent directory for a file-backed SQLite database
// if it does not already exist. It skips in-memory databases and ignores
// relative paths that resolve to the current directory.
func ensureDBDir(dbPath string) error {
	filePart := dbPath
	if i := strings.Index(filePart, "?"); i >= 0 {
		filePart = filePart[:i]
	}
	if strings.HasPrefix(filePart, "file:") {
		filePart = strings.TrimPrefix(filePart, "file:")
	}
	dir := filepath.Dir(filePart)
	if dir == "" || dir == "." {
		return nil
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create db directory: %w", err)
	}
	return nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

type migration struct {
	version int
	name    string
	stmts   []string
}

var migrations = []migration{
	{
		version: 1,
		name:    "repos_problems_tasks",
		stmts: []string{
			`CREATE TABLE IF NOT EXISTS repos (
				id TEXT PRIMARY KEY,
				owner TEXT NOT NULL,
				name TEXT NOT NULL,
				local_dir TEXT NOT NULL,
				default_branch TEXT NOT NULL,
				clone_url TEXT,
				created_at DATETIME NOT NULL,
				updated_at DATETIME NOT NULL,
				UNIQUE(owner, name)
			);`,
			`CREATE TABLE IF NOT EXISTS problems (
				id TEXT PRIMARY KEY,
				description TEXT NOT NULL,
				status TEXT NOT NULL,
				repo_id TEXT NOT NULL REFERENCES repos(id),
				parent_issue_number INTEGER,
				created_at DATETIME NOT NULL,
				updated_at DATETIME NOT NULL
			);`,
			`CREATE TABLE IF NOT EXISTS tasks (
				id TEXT PRIMARY KEY,
				problem_id TEXT NOT NULL REFERENCES problems(id),
				repo_id TEXT NOT NULL REFERENCES repos(id),
				title TEXT NOT NULL,
				description TEXT NOT NULL,
				status TEXT NOT NULL,
				agent_session_id TEXT,
				branch TEXT,
				base_branch TEXT,
				pr_number INTEGER,
				issue_number INTEGER,
				output TEXT,
				attempts INTEGER NOT NULL DEFAULT 0,
				max_attempts INTEGER NOT NULL DEFAULT 3,
				created_at DATETIME NOT NULL,
				updated_at DATETIME NOT NULL
			);`,
		},
	},
	{
		version: 2,
		name:    "pr_groups",
		stmts: []string{
			`CREATE TABLE IF NOT EXISTS pr_groups (
				id TEXT PRIMARY KEY,
				problem_id TEXT NOT NULL REFERENCES problems(id),
				repo_id TEXT NOT NULL REFERENCES repos(id),
				title TEXT NOT NULL,
				description TEXT NOT NULL DEFAULT '',
				status TEXT NOT NULL,
				branch TEXT,
				base_branch TEXT,
				base_group_id TEXT REFERENCES pr_groups(id),
				stack_order INTEGER NOT NULL,
				pr_number INTEGER,
				issue_number INTEGER,
				output TEXT,
				attempts INTEGER NOT NULL DEFAULT 0,
				max_attempts INTEGER NOT NULL DEFAULT 3,
				created_at DATETIME NOT NULL,
				updated_at DATETIME NOT NULL
			);`,
			`ALTER TABLE tasks ADD COLUMN pr_group_id TEXT REFERENCES pr_groups(id);`,
			`ALTER TABLE tasks ADD COLUMN depends_on_task_id TEXT REFERENCES tasks(id);`,
			`ALTER TABLE tasks ADD COLUMN group_order INTEGER NOT NULL DEFAULT 0;`,
			`ALTER TABLE tasks ADD COLUMN sequence INTEGER NOT NULL DEFAULT 0;`,
		},
	},
}

func migrate(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at DATETIME NOT NULL
	);`); err != nil {
		return fmt.Errorf("create schema_version: %w", err)
	}

	var current int
	row := db.QueryRow(`SELECT version FROM schema_version ORDER BY version DESC LIMIT 1`)
	if err := row.Scan(&current); err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("read schema_version: %w", err)
	}

	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		for _, stmt := range m.stmts {
			if _, err := db.Exec(stmt); err != nil {
				return fmt.Errorf("migration %d (%s): %w", m.version, m.name, err)
			}
		}
		if _, err := db.Exec(
			`INSERT INTO schema_version (version, name, applied_at) VALUES (?, ?, ?)`,
			m.version, m.name, time.Now().UTC(),
		); err != nil {
			return fmt.Errorf("record migration %d: %w", m.version, err)
		}
		current = m.version
	}
	return nil
}

// Repo represents a registered GitHub repository.
type Repo struct {
	ID            string
	Owner         string
	Name          string
	LocalDir      string
	DefaultBranch string
	CloneURL      string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// CreateRepo inserts a new repo and returns it.
func (s *Store) CreateRepo(ctx context.Context, owner, name, localDir, defaultBranch, cloneURL string) (*Repo, error) {
	now := time.Now().UTC()
	r := &Repo{
		ID:            uuid.New().String(),
		Owner:         owner,
		Name:          name,
		LocalDir:      localDir,
		DefaultBranch: defaultBranch,
		CloneURL:      cloneURL,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO repos (id, owner, name, local_dir, default_branch, clone_url, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Owner, r.Name, r.LocalDir, r.DefaultBranch, r.CloneURL, r.CreatedAt, r.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create repo: %w", err)
	}
	return r, nil
}

// UpdateRepo persists changes to an existing repo.
func (s *Store) UpdateRepo(ctx context.Context, r *Repo) error {
	r.UpdatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`UPDATE repos SET owner=?, name=?, local_dir=?, default_branch=?, clone_url=?, updated_at=? WHERE id = ?`,
		r.Owner, r.Name, r.LocalDir, r.DefaultBranch, r.CloneURL, r.UpdatedAt, r.ID,
	)
	if err != nil {
		return fmt.Errorf("update repo: %w", err)
	}
	return nil
}

// DeleteRepo removes a repo and its associated problems and tasks.
func (s *Store) DeleteRepo(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete repo: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.ExecContext(ctx, `DELETE FROM tasks WHERE repo_id = ?`, id); err != nil {
		return fmt.Errorf("delete repo tasks: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM pr_groups WHERE repo_id = ?`, id); err != nil {
		return fmt.Errorf("delete repo pr groups: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM problems WHERE repo_id = ?`, id); err != nil {
		return fmt.Errorf("delete repo problems: %w", err)
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM repos WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete repo: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete repo rows affected: %w", err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete repo: %w", err)
	}
	committed = true
	return nil
}

// GetRepo retrieves a repo by its ID.
func (s *Store) GetRepo(ctx context.Context, id string) (*Repo, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, owner, name, local_dir, default_branch, clone_url, created_at, updated_at FROM repos WHERE id = ?`, id)
	return scanRepo(row)
}

// GetRepoByOwnerName retrieves a repo by owner and name.
func (s *Store) GetRepoByOwnerName(ctx context.Context, owner, name string) (*Repo, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, owner, name, local_dir, default_branch, clone_url, created_at, updated_at FROM repos WHERE owner = ? AND name = ?`, owner, name)
	return scanRepo(row)
}

// ListRepos returns all repos ordered by creation time, newest first.
func (s *Store) ListRepos(ctx context.Context) ([]Repo, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, owner, name, local_dir, default_branch, clone_url, created_at, updated_at FROM repos ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var repos []Repo
	for rows.Next() {
		r, err := scanRepo(rows)
		if err != nil {
			return nil, err
		}
		repos = append(repos, *r)
	}
	return repos, rows.Err()
}

func scanRepo(s scanner) (*Repo, error) {
	r := &Repo{}
	err := s.Scan(&r.ID, &r.Owner, &r.Name, &r.LocalDir, &r.DefaultBranch, &r.CloneURL, &r.CreatedAt, &r.UpdatedAt)
	return r, err
}

// Problem represents a top-level user request tracked by cttw.
type Problem struct {
	ID                string
	Description       string
	Status            string
	RepoID            string
	ParentIssueNumber int
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// CreateProblem inserts a new pending problem and returns it.
func (s *Store) CreateProblem(ctx context.Context, description, repoID string) (*Problem, error) {
	now := time.Now().UTC()
	p := &Problem{
		ID:          uuid.New().String(),
		Description: description,
		Status:      "pending",
		RepoID:      repoID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO problems (id, description, status, repo_id, parent_issue_number, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Description, p.Status, p.RepoID, nil, p.CreatedAt, p.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create problem: %w", err)
	}
	return p, nil
}

// GetProblem retrieves a problem by its ID.
func (s *Store) GetProblem(ctx context.Context, id string) (*Problem, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, description, status, repo_id, parent_issue_number, created_at, updated_at FROM problems WHERE id = ?`, id)
	return scanProblem(row)
}

// ListProblems returns all problems ordered by creation time, newest first.
func (s *Store) ListProblems(ctx context.Context) ([]Problem, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, description, status, repo_id, parent_issue_number, created_at, updated_at FROM problems ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var problems []Problem
	for rows.Next() {
		p, err := scanProblem(rows)
		if err != nil {
			return nil, err
		}
		problems = append(problems, *p)
	}
	return problems, rows.Err()
}

// ListProblemsByRepo returns all problems for a repo ordered by creation time, newest first.
func (s *Store) ListProblemsByRepo(ctx context.Context, repoID string) ([]Problem, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, description, status, repo_id, parent_issue_number, created_at, updated_at FROM problems WHERE repo_id = ? ORDER BY created_at DESC`, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var problems []Problem
	for rows.Next() {
		p, err := scanProblem(rows)
		if err != nil {
			return nil, err
		}
		problems = append(problems, *p)
	}
	return problems, rows.Err()
}

// UpdateProblem persists changes to an existing problem.
func (s *Store) UpdateProblem(ctx context.Context, p *Problem) error {
	if s.updateProblemErr != nil {
		if err := s.updateProblemErr(); err != nil {
			return err
		}
	}
	p.UpdatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`UPDATE problems SET description=?, status=?, repo_id=?, parent_issue_number=?, updated_at=? WHERE id = ?`,
		p.Description, p.Status, p.RepoID, p.ParentIssueNumber, p.UpdatedAt, p.ID,
	)
	return err
}

// FailTasksByProblem marks every task belonging to a problem as failed.
func (s *Store) FailTasksByProblem(ctx context.Context, problemID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET status = 'failed', updated_at = ? WHERE problem_id = ?`,
		time.Now().UTC(), problemID,
	)
	if err != nil {
		return fmt.Errorf("fail tasks by problem: %w", err)
	}
	return nil
}

func scanProblem(s scanner) (*Problem, error) {
	p := &Problem{}
	var parent sql.NullInt64
	err := s.Scan(&p.ID, &p.Description, &p.Status, &p.RepoID, &parent, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if parent.Valid {
		p.ParentIssueNumber = int(parent.Int64)
	}
	return p, nil
}

// PRGroup represents a feature-complete pull request within a problem.
// It contains one or more tasks/commits and is part of a stack.
type PRGroup struct {
	ID              string
	ProblemID       string
	RepoID          string
	Title           string
	Description     string
	Status          string
	Branch          string
	BaseBranch      string
	BaseGroupID     string
	StackOrder      int
	PRNumber        int
	IssueNumber     int
	Output          string
	Attempts        int
	MaxAttempts     int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// CreatePRGroup inserts a new pending PR group and returns it.
func (s *Store) CreatePRGroup(ctx context.Context, problemID, repoID, title, description string, stackOrder int) (*PRGroup, error) {
	now := time.Now().UTC()
	g := &PRGroup{
		ID:          uuid.New().String(),
		ProblemID:   problemID,
		RepoID:      repoID,
		Title:       title,
		Description: description,
		Status:      "pending",
		StackOrder:  stackOrder,
		MaxAttempts: 3,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO pr_groups (id, problem_id, repo_id, title, description, status, branch, base_branch, base_group_id, stack_order, pr_number, issue_number, output, attempts, max_attempts, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		g.ID, g.ProblemID, g.RepoID, g.Title, g.Description, g.Status, g.Branch, g.BaseBranch, g.BaseGroupID, g.StackOrder, g.PRNumber, g.IssueNumber, g.Output, g.Attempts, g.MaxAttempts, g.CreatedAt, g.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create pr group: %w", err)
	}
	return g, nil
}

// GetPRGroup retrieves a PR group by its ID.
func (s *Store) GetPRGroup(ctx context.Context, id string) (*PRGroup, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, problem_id, repo_id, title, description, status, branch, base_branch, base_group_id, stack_order, pr_number, issue_number, output, attempts, max_attempts, created_at, updated_at FROM pr_groups WHERE id = ?`, id)
	return scanPRGroup(row)
}

// ListPRGroupsByProblem returns all PR groups for a problem ordered by stack_order.
func (s *Store) ListPRGroupsByProblem(ctx context.Context, problemID string) ([]PRGroup, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, problem_id, repo_id, title, description, status, branch, base_branch, base_group_id, stack_order, pr_number, issue_number, output, attempts, max_attempts, created_at, updated_at FROM pr_groups WHERE problem_id = ? ORDER BY stack_order`, problemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []PRGroup
	for rows.Next() {
		g, err := scanPRGroup(rows)
		if err != nil {
			return nil, err
		}
		groups = append(groups, *g)
	}
	return groups, rows.Err()
}

// UpdatePRGroup persists changes to an existing PR group.
func (s *Store) UpdatePRGroup(ctx context.Context, g *PRGroup) error {
	if s.updatePRGroupErr != nil {
		return s.updatePRGroupErr
	}
	g.UpdatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`UPDATE pr_groups SET problem_id=?, repo_id=?, title=?, description=?, status=?, branch=?, base_branch=?, base_group_id=?, stack_order=?, pr_number=?, issue_number=?, output=?, attempts=?, max_attempts=?, updated_at=? WHERE id = ?`,
		g.ProblemID, g.RepoID, g.Title, g.Description, g.Status, g.Branch, g.BaseBranch, g.BaseGroupID, g.StackOrder, g.PRNumber, g.IssueNumber, g.Output, g.Attempts, g.MaxAttempts, g.UpdatedAt, g.ID,
	)
	return err
}

func scanPRGroup(s scanner) (*PRGroup, error) {
	g := &PRGroup{}
	var pr, issue sql.NullInt64
	var baseGroupID sql.NullString
	err := s.Scan(&g.ID, &g.ProblemID, &g.RepoID, &g.Title, &g.Description, &g.Status, &g.Branch, &g.BaseBranch, &baseGroupID, &g.StackOrder, &pr, &issue, &g.Output, &g.Attempts, &g.MaxAttempts, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if baseGroupID.Valid {
		g.BaseGroupID = baseGroupID.String
	}
	if pr.Valid {
		g.PRNumber = int(pr.Int64)
	}
	if issue.Valid {
		g.IssueNumber = int(issue.Int64)
	}
	return g, nil
}

// Task represents a unit of work within a problem, mapping to one commit within a PR group.
type Task struct {
	ID                string
	ProblemID         string
	RepoID            string
	PRGroupID         string
	DependsOnTaskID   string
	Title             string
	Description       string
	Status            string
	AgentSessionID    string
	Branch            string
	BaseBranch        string
	GroupOrder        int
	Sequence          int
	PRNumber          int
	IssueNumber       int
	Output            string
	Attempts          int
	MaxAttempts       int
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// CreateTask inserts a new pending task and returns it.
func (s *Store) CreateTask(ctx context.Context, problemID, repoID, title, description string) (*Task, error) {
	return s.CreateTaskInGroup(ctx, problemID, repoID, "", title, description, 0, 0)
}

// CreateTaskInGroup inserts a new pending task inside a PR group and returns it.
func (s *Store) CreateTaskInGroup(ctx context.Context, problemID, repoID, prGroupID, title, description string, groupOrder, sequence int) (*Task, error) {
	now := time.Now().UTC()
	t := &Task{
		ID:          uuid.New().String(),
		ProblemID:   problemID,
		RepoID:      repoID,
		PRGroupID:   prGroupID,
		Title:       title,
		Description: description,
		Status:      "pending",
		GroupOrder:  groupOrder,
		Sequence:    sequence,
		MaxAttempts: 3,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO tasks (id, problem_id, repo_id, pr_group_id, depends_on_task_id, title, description, status, agent_session_id, branch, base_branch, pr_number, issue_number, output, attempts, max_attempts, group_order, sequence, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.ProblemID, t.RepoID, t.PRGroupID, t.DependsOnTaskID, t.Title, t.Description, t.Status, t.AgentSessionID, t.Branch, t.BaseBranch, t.PRNumber, t.IssueNumber, t.Output, t.Attempts, t.MaxAttempts, t.GroupOrder, t.Sequence, t.CreatedAt, t.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create task: %w", err)
	}
	return t, nil
}

// GetTask retrieves a task by its ID.
func (s *Store) GetTask(ctx context.Context, id string) (*Task, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, problem_id, repo_id, pr_group_id, depends_on_task_id, title, description, status, agent_session_id, branch, base_branch, pr_number, issue_number, output, attempts, max_attempts, group_order, sequence, created_at, updated_at FROM tasks WHERE id = ?`, id)
	return scanTask(row)
}

// UpdateTask persists changes to an existing task.
func (s *Store) UpdateTask(ctx context.Context, t *Task) error {
	if s.updateTaskErr != nil {
		return s.updateTaskErr
	}
	t.UpdatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET problem_id=?, repo_id=?, pr_group_id=?, depends_on_task_id=?, title=?, description=?, status=?, agent_session_id=?, branch=?, base_branch=?, pr_number=?, issue_number=?, output=?, attempts=?, max_attempts=?, group_order=?, sequence=?, updated_at=? WHERE id = ?`,
		t.ProblemID, t.RepoID, t.PRGroupID, t.DependsOnTaskID, t.Title, t.Description, t.Status, t.AgentSessionID, t.Branch, t.BaseBranch, t.PRNumber, t.IssueNumber, t.Output, t.Attempts, t.MaxAttempts, t.GroupOrder, t.Sequence, t.UpdatedAt, t.ID,
	)
	return err
}

// ListTasksByProblem returns all tasks for a problem ordered by sequence.
func (s *Store) ListTasksByProblem(ctx context.Context, problemID string) ([]Task, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, problem_id, repo_id, pr_group_id, depends_on_task_id, title, description, status, agent_session_id, branch, base_branch, pr_number, issue_number, output, attempts, max_attempts, group_order, sequence, created_at, updated_at FROM tasks WHERE problem_id = ? ORDER BY sequence, created_at`, problemID)
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

// NextPendingTask selects the pending task with the lowest sequence whose earlier
// sequence tasks are all completed, atomically marks it running, and returns it.
func (s *Store) NextPendingTask(ctx context.Context) (*Task, error) {
	row := s.db.QueryRowContext(ctx,
		`WITH next AS (
			SELECT t.id FROM tasks t
			WHERE t.status = 'pending' AND t.attempts < t.max_attempts
			  AND NOT EXISTS (
				  SELECT 1 FROM tasks blocker
				  WHERE blocker.sequence < t.sequence AND blocker.status != 'completed'
			  )
			ORDER BY t.sequence LIMIT 1
		)
		UPDATE tasks SET status = 'running', updated_at = ?
		WHERE id = (SELECT id FROM next) AND status = 'pending'
		RETURNING id, problem_id, repo_id, pr_group_id, depends_on_task_id, title, description, status, agent_session_id, branch, base_branch, pr_number, issue_number, output, attempts, max_attempts, group_order, sequence, created_at, updated_at`,
		time.Now().UTC())
	t, err := scanTask(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

// NextPendingTaskForRepo selects the next pending task for a specific repo,
// respecting serial dependency order, atomically marks it running, and returns it.
func (s *Store) NextPendingTaskForRepo(ctx context.Context, repoID string) (*Task, error) {
	row := s.db.QueryRowContext(ctx,
		`WITH next AS (
			SELECT t.id FROM tasks t
			WHERE t.status = 'pending' AND t.attempts < t.max_attempts AND t.repo_id = ?
			  AND NOT EXISTS (
				  SELECT 1 FROM tasks blocker
				  WHERE blocker.sequence < t.sequence AND blocker.status != 'completed'
			  )
			ORDER BY t.sequence LIMIT 1
		)
		UPDATE tasks SET status = 'running', updated_at = ?
		WHERE id = (SELECT id FROM next) AND status = 'pending'
		RETURNING id, problem_id, repo_id, pr_group_id, depends_on_task_id, title, description, status, agent_session_id, branch, base_branch, pr_number, issue_number, output, attempts, max_attempts, group_order, sequence, created_at, updated_at`,
		repoID, time.Now().UTC())
	t, err := scanTask(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

// ResetRunningTasks resets tasks that were running at startup to pending so they
// can be retried after a crash or unclean shutdown.
func (s *Store) ResetRunningTasks(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET status='pending' WHERE status='running' AND attempts < max_attempts`)
	return err
}

// CountIncompleteTasksByPRGroup returns the number of tasks in a group that are not completed.
func (s *Store) CountIncompleteTasksByPRGroup(ctx context.Context, prGroupID string) (int, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tasks WHERE pr_group_id = ? AND status != 'completed'`, prGroupID)
	var n int
	if err := row.Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// CountIncompleteTasksByProblem returns the number of tasks in a problem that are not completed.
func (s *Store) CountIncompleteTasksByProblem(ctx context.Context, problemID string) (int, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tasks WHERE problem_id = ? AND status != 'completed'`, problemID)
	var n int
	if err := row.Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func scanTask(s scanner) (*Task, error) {
	t := &Task{}
	var pr, issue sql.NullInt64
	var prGroupID, dependsOnID sql.NullString
	err := s.Scan(&t.ID, &t.ProblemID, &t.RepoID, &prGroupID, &dependsOnID, &t.Title, &t.Description, &t.Status, &t.AgentSessionID, &t.Branch, &t.BaseBranch, &pr, &issue, &t.Output, &t.Attempts, &t.MaxAttempts, &t.GroupOrder, &t.Sequence, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if prGroupID.Valid {
		t.PRGroupID = prGroupID.String
	}
	if dependsOnID.Valid {
		t.DependsOnTaskID = dependsOnID.String
	}
	if pr.Valid {
		t.PRNumber = int(pr.Int64)
	}
	if issue.Valid {
		t.IssueNumber = int(issue.Int64)
	}
	return t, nil
}

type scanner interface {
	Scan(dest ...any) error
}
