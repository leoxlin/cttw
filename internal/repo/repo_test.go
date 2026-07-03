package repo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_EnsureClones(t *testing.T) {
	dir := t.TempDir()
	bare := filepath.Join(dir, "bare.git")
	require.NoError(t, exec.Command("git", "init", "--bare", bare).Run())

	// seed bare repo with a main branch
	work := filepath.Join(dir, "seed")
	require.NoError(t, exec.Command("git", "clone", bare, work).Run())
	require.NoError(t, exec.Command("git", "-C", work, "config", "user.email", "t@t").Run())
	require.NoError(t, exec.Command("git", "-C", work, "config", "user.name", "T").Run())
	require.NoError(t, exec.Command("git", "-C", work, "commit", "--allow-empty", "-m", "init").Run())
	require.NoError(t, exec.Command("git", "-C", work, "push", "origin", "main").Run())

	reg := &Registry{Root: filepath.Join(dir, "repos")}
	// Pre-clone from the bare repo so Ensure sees an existing repository and only
	// runs remote update, avoiding a network call to github.com.
	require.NoError(t, exec.Command("git", "clone", bare, reg.Dir("llin", "cttw")).Run())

	repo, err := reg.Ensure(context.Background(), "llin", "cttw", "main", "")
	require.NoError(t, err)
	assert.Equal(t, "llin", repo.Owner)
	assert.Equal(t, "cttw", repo.Name)
	assert.Equal(t, "main", repo.DefaultBranch)
	assert.Equal(t, reg.Dir("llin", "cttw"), repo.Dir)

	gitDir := filepath.Join(repo.Dir, ".git")
	info, err := os.Stat(gitDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}
