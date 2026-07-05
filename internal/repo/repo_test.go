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
	bare := filepath.Join(dir, "bare")
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
	// runs remote update. Rewrite the github.com URL back to the local bare repo
	// so the update succeeds without network access.
	cloneDir := reg.Dir("llin", "cttw")
	require.NoError(t, exec.Command("git", "clone", bare, cloneDir).Run())
	require.NoError(t, exec.Command("git", "-C", cloneDir, "remote", "set-url", "origin", "https://github.com/llin/cttw.git").Run())
	require.NoError(t, exec.Command("git", "-C", cloneDir, "config", "url."+bare+".insteadOf", "https://github.com/llin/cttw.git").Run())

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

func TestRegistry_EnsureRejectsMismatchedRepo(t *testing.T) {
	dir := t.TempDir()
	bare := filepath.Join(dir, "bare")
	require.NoError(t, exec.Command("git", "init", "--bare", bare).Run())

	work := filepath.Join(dir, "seed")
	require.NoError(t, exec.Command("git", "clone", bare, work).Run())
	require.NoError(t, exec.Command("git", "-C", work, "remote", "set-url", "origin", "https://github.com/other/project.git").Run())

	reg := &Registry{Root: filepath.Join(dir, "repos")}
	require.NoError(t, exec.Command("git", "clone", bare, reg.Dir("llin", "cttw")).Run())

	_, err := reg.Ensure(context.Background(), "llin", "cttw", "main", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match")
}

func TestRemoteMatches(t *testing.T) {
	cases := []struct {
		remote string
		owner  string
		name   string
		want   bool
	}{
		{"https://github.com/llin/cttw.git", "llin", "cttw", true},
		{"https://github.com/llin/cttw", "llin", "cttw", true},
		{"git@github.com:llin/cttw.git", "llin", "cttw", true},
		{"git@github.com:llin/cttw", "llin", "cttw", true},
		{"http://github.com/llin/cttw.git", "llin", "cttw", true},
		{"https://github.com/llin/other.git", "llin", "cttw", false},
		{"https://github.com/acme/cttw.git", "llin", "cttw", false},
		{"", "llin", "cttw", false},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, remoteMatches(tc.remote, tc.owner, tc.name), tc.remote)
	}
}
