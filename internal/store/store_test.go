package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrations_AreIdempotent(t *testing.T) {
	dbPath := "file:" + t.TempDir() + "/cttw.db"

	s, err := New(dbPath)
	require.NoError(t, err)

	var version int
	row := s.db.QueryRow(`SELECT version FROM schema_version ORDER BY version DESC LIMIT 1`)
	require.NoError(t, row.Scan(&version))
	assert.Equal(t, 2, version)

	// Insert a problem to verify tables exist.
	ctx := context.Background()
	repo, err := s.CreateRepo(ctx, "o", "r", "/tmp/r", "main", "")
	require.NoError(t, err)
	_, err = s.CreateProblem(ctx, "x", repo.ID)
	require.NoError(t, err)
	s.Close()

	// Reopening the database should not re-apply migrations or fail.
	s2, err := New(dbPath)
	require.NoError(t, err)
	defer s2.Close()

	var version2 int
	row = s2.db.QueryRow(`SELECT version FROM schema_version ORDER BY version DESC LIMIT 1`)
	require.NoError(t, row.Scan(&version2))
	assert.Equal(t, 2, version2)

	// Data should still be present.
	problems, err := s2.ListProblems(ctx)
	require.NoError(t, err)
	assert.Len(t, problems, 1)
}

func TestNew_CreatesMissingDBDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing", "nested")
	dbPath := filepath.Join(dir, "cttw.db")

	s, err := New(dbPath)
	require.NoError(t, err)
	defer s.Close()

	_, err = os.Stat(dir)
	require.NoError(t, err, "expected database directory to be created")
}

func TestRepo_CRUD(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err)
	defer s.Close()
	ctx := context.Background()

	repo, err := s.CreateRepo(ctx, "llin", "cttw", "/tmp/r", "main", "")
	require.NoError(t, err)
	assert.Equal(t, "llin", repo.Owner)

	got, err := s.GetRepoByOwnerName(ctx, "llin", "cttw")
	require.NoError(t, err)
	assert.Equal(t, repo.ID, got.ID)

	repos, err := s.ListRepos(ctx)
	require.NoError(t, err)
	assert.Len(t, repos, 1)

	repo.Name = "cttw-renamed"
	repo.CloneURL = "https://github.com/llin/cttw-renamed.git"
	require.NoError(t, s.UpdateRepo(ctx, repo))
	updated, err := s.GetRepo(ctx, repo.ID)
	require.NoError(t, err)
	assert.Equal(t, "cttw-renamed", updated.Name)
	assert.Equal(t, "https://github.com/llin/cttw-renamed.git", updated.CloneURL)

	problem, err := s.CreateProblem(ctx, "build API", repo.ID)
	require.NoError(t, err)
	_, err = s.CreateTask(ctx, problem.ID, repo.ID, "add handler", "implement POST")
	require.NoError(t, err)

	require.NoError(t, s.DeleteRepo(ctx, repo.ID))
	_, err = s.GetRepo(ctx, repo.ID)
	assert.ErrorIs(t, err, sql.ErrNoRows)
	problems, err := s.ListProblemsByRepo(ctx, repo.ID)
	require.NoError(t, err)
	assert.Empty(t, problems)
	tasks, err := s.ListTasksByProblem(ctx, problem.ID)
	require.NoError(t, err)
	assert.Empty(t, tasks)
}

func TestProblemAndTasks_CRUD(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err)
	defer s.Close()
	ctx := context.Background()

	repo, _ := s.CreateRepo(ctx, "o", "r", "/tmp/r", "main", "")
	problem, err := s.CreateProblem(ctx, "build API", repo.ID)
	require.NoError(t, err)
	assert.Equal(t, "pending", problem.Status)

	task, err := s.CreateTask(ctx, problem.ID, repo.ID, "add handler", "implement POST")
	require.NoError(t, err)
	assert.Equal(t, "pending", task.Status)
	assert.Equal(t, 3, task.MaxAttempts)

	tasks, err := s.ListTasksByProblem(ctx, problem.ID)
	require.NoError(t, err)
	assert.Len(t, tasks, 1)

	task.Status = "running"
	require.NoError(t, s.UpdateTask(ctx, task))
	got, err := s.GetTask(ctx, task.ID)
	require.NoError(t, err)
	assert.Equal(t, "running", got.Status)
}

func TestFailTasksByProblem(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err)
	defer s.Close()
	ctx := context.Background()

	repo, _ := s.CreateRepo(ctx, "o", "r", "/tmp/r", "main", "")
	problem, _ := s.CreateProblem(ctx, "x", repo.ID)
	t1, _ := s.CreateTask(ctx, problem.ID, repo.ID, "t1", "d1")
	t2, _ := s.CreateTask(ctx, problem.ID, repo.ID, "t2", "d2")

	require.NoError(t, s.FailTasksByProblem(ctx, problem.ID))

	got1, err := s.GetTask(ctx, t1.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", got1.Status)

	got2, err := s.GetTask(ctx, t2.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", got2.Status)
}

func TestNextPendingTaskForRepo(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err)
	defer s.Close()
	ctx := context.Background()

	r1, _ := s.CreateRepo(ctx, "o1", "r1", "/tmp/r1", "main", "")
	r2, _ := s.CreateRepo(ctx, "o2", "r2", "/tmp/r2", "main", "")
	p1, _ := s.CreateProblem(ctx, "x", r1.ID)
	p2, _ := s.CreateProblem(ctx, "y", r2.ID)
	_, _ = s.CreateTask(ctx, p1.ID, r1.ID, "t1", "d1")
	t2, _ := s.CreateTask(ctx, p2.ID, r2.ID, "t2", "d2")

	got, err := s.NextPendingTaskForRepo(ctx, r2.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, t2.ID, got.ID)
	assert.Equal(t, "running", got.Status)

	again, err := s.NextPendingTaskForRepo(ctx, r2.ID)
	require.NoError(t, err)
	assert.Nil(t, again)

	tasks, err := s.ListTasksByProblem(ctx, p1.ID)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, "pending", tasks[0].Status)
}

func TestNextPendingTask(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err)
	defer s.Close()
	ctx := context.Background()

	repo, _ := s.CreateRepo(ctx, "o", "r", "/tmp/r", "main", "")
	problem, _ := s.CreateProblem(ctx, "x", repo.ID)
	task, _ := s.CreateTask(ctx, problem.ID, repo.ID, "t", "d")

	got, err := s.NextPendingTask(ctx)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, task.ID, got.ID)
	assert.Equal(t, "running", got.Status)

	again, err := s.NextPendingTask(ctx)
	require.NoError(t, err)
	assert.Nil(t, again)
}


func TestPRGroup_CRUD(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err)
	defer s.Close()
	ctx := context.Background()

	repo, _ := s.CreateRepo(ctx, "o", "r", "/tmp/r", "main", "")
	problem, _ := s.CreateProblem(ctx, "build API", repo.ID)

	g, err := s.CreatePRGroup(ctx, problem.ID, repo.ID, "add handler", "implement POST", 0)
	require.NoError(t, err)
	assert.Equal(t, "pending", g.Status)
	assert.Equal(t, 0, g.StackOrder)

	got, err := s.GetPRGroup(ctx, g.ID)
	require.NoError(t, err)
	assert.Equal(t, g.ID, got.ID)

	groups, err := s.ListPRGroupsByProblem(ctx, problem.ID)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.Equal(t, "add handler", groups[0].Title)

	g.Status = "completed"
	require.NoError(t, s.UpdatePRGroup(ctx, g))
	updated, err := s.GetPRGroup(ctx, g.ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", updated.Status)
}

func TestCreateTaskInGroup(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err)
	defer s.Close()
	ctx := context.Background()

	repo, _ := s.CreateRepo(ctx, "o", "r", "/tmp/r", "main", "")
	problem, _ := s.CreateProblem(ctx, "build API", repo.ID)
	g, _ := s.CreatePRGroup(ctx, problem.ID, repo.ID, "feature", "desc", 0)

	task, err := s.CreateTaskInGroup(ctx, problem.ID, repo.ID, g.ID, "t1", "d1", 0, 5)
	require.NoError(t, err)
	assert.Equal(t, g.ID, task.PRGroupID)
	assert.Equal(t, 0, task.GroupOrder)
	assert.Equal(t, 5, task.Sequence)
}

func TestNextPendingTask_RespectsSequence(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err)
	defer s.Close()
	ctx := context.Background()

	repo, _ := s.CreateRepo(ctx, "o", "r", "/tmp/r", "main", "")
	problem, _ := s.CreateProblem(ctx, "x", repo.ID)
	g, _ := s.CreatePRGroup(ctx, problem.ID, repo.ID, "g", "d", 0)

	t1, _ := s.CreateTaskInGroup(ctx, problem.ID, repo.ID, g.ID, "t1", "d1", 0, 1)
	t2, _ := s.CreateTaskInGroup(ctx, problem.ID, repo.ID, g.ID, "t2", "d2", 1, 2)

	got, err := s.NextPendingTask(ctx)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, t1.ID, got.ID)

	// t2 is still blocked because t1 is running.
	again, err := s.NextPendingTask(ctx)
	require.NoError(t, err)
	assert.Nil(t, again)

	// Complete t1; t2 becomes available.
	got.Status = "completed"
	require.NoError(t, s.UpdateTask(ctx, got))

	got2, err := s.NextPendingTask(ctx)
	require.NoError(t, err)
	require.NotNil(t, got2)
	assert.Equal(t, t2.ID, got2.ID)
}

func TestCountIncompleteTasksByPRGroup(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err)
	defer s.Close()
	ctx := context.Background()

	repo, _ := s.CreateRepo(ctx, "o", "r", "/tmp/r", "main", "")
	problem, _ := s.CreateProblem(ctx, "x", repo.ID)
	g, _ := s.CreatePRGroup(ctx, problem.ID, repo.ID, "g", "d", 0)

	_, _ = s.CreateTaskInGroup(ctx, problem.ID, repo.ID, g.ID, "t1", "d1", 0, 1)
	t2, _ := s.CreateTaskInGroup(ctx, problem.ID, repo.ID, g.ID, "t2", "d2", 1, 2)

	n, err := s.CountIncompleteTasksByPRGroup(ctx, g.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	t2.Status = "completed"
	require.NoError(t, s.UpdateTask(ctx, t2))

	n, err = s.CountIncompleteTasksByPRGroup(ctx, g.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}
