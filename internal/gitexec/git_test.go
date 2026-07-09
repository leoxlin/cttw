package gitexec

import (
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunner_Commit(t *testing.T) {
	dir := t.TempDir()
	r := &Runner{Dir: dir}
	require.NoError(t, r.run("init"))
	require.NoError(t, r.run("config", "user.email", "test@example.com"))
	require.NoError(t, r.run("config", "user.name", "Test"))

	f := filepath.Join(dir, "a.txt")
	require.NoError(t, os.WriteFile(f, []byte("hi"), 0644))
	require.NoError(t, r.Add("a.txt"))
	require.NoError(t, r.Commit("initial"))
	require.NoError(t, r.CheckoutNew("feat/x", "main"))

	out, err := r.Output("rev-parse", "--abbrev-ref", "HEAD")
	require.NoError(t, err)
	assert.Equal(t, "feat/x\n", string(out))
}

func TestRunner_Clone_DoesNotEmbedToken(t *testing.T) {
	dir := t.TempDir()
	bare := filepath.Join(dir, "bare.git")
	require.NoError(t, exec.Command("git", "init", "--bare", bare).Run())

	// Seed the bare repo so clone succeeds.
	work := filepath.Join(dir, "seed")
	require.NoError(t, exec.Command("git", "clone", bare, work).Run())
	require.NoError(t, exec.Command("git", "-C", work, "config", "user.email", "t@t").Run())
	require.NoError(t, exec.Command("git", "-C", work, "config", "user.name", "T").Run())
	require.NoError(t, exec.Command("git", "-C", work, "commit", "--allow-empty", "-m", "init").Run())
	require.NoError(t, exec.Command("git", "-C", work, "push", "origin", "main").Run())

	cloneDir := filepath.Join(dir, "clone")
	token := "super-secret-token"
	r := &Runner{}
	require.NoError(t, r.Clone("file://"+bare, token, cloneDir))

	// The remote URL must not contain the token.
	remoteURL, err := (&Runner{Dir: cloneDir}).Output("config", "--get", "remote.origin.url")
	require.NoError(t, err)
	assert.Equal(t, "file://"+bare, strings.TrimSpace(string(remoteURL)))
	assert.NotContains(t, string(remoteURL), token)

	// The auth header must NOT be persisted in the cloned repo's local config.
	_, err = (&Runner{Dir: cloneDir}).Output("config", "--get", "http.https://github.com/.extraHeader")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exit status 1")

	// The runner must still carry the token and use it per-invocation.
	assert.Equal(t, token, r.Token)
	assert.Equal(t, cloneDir, r.Dir)

	// Verify the per-invocation header is injected by checking the encoded auth
	// appears in the -c argument the runner would pass to git.
	wantAuth := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
	assert.Contains(t, r.extraHeader(), "Authorization: Basic "+wantAuth)
}

func TestRunner_HasChangesAndResetHardClean(t *testing.T) {
	dir := t.TempDir()
	r := &Runner{Dir: dir}
	require.NoError(t, r.run("init", "-b", "main"))
	require.NoError(t, r.run("config", "user.email", "test@example.com"))
	require.NoError(t, r.run("config", "user.name", "Test"))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("one"), 0644))
	committed, err := r.CommitAll("initial")
	require.NoError(t, err)
	require.True(t, committed)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("two"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("temp"), 0644))
	changed, err := r.HasChanges()
	require.NoError(t, err)
	require.True(t, changed)

	require.NoError(t, r.ResetHardClean())
	changed, err = r.HasChanges()
	require.NoError(t, err)
	require.False(t, changed)
	got, err := os.ReadFile(filepath.Join(dir, "tracked.txt"))
	require.NoError(t, err)
	assert.Equal(t, "one", string(got))
	assert.NoFileExists(t, filepath.Join(dir, "untracked.txt"))
}

func TestRunner_CommitAllNoopsWhenNoChanges(t *testing.T) {
	dir := t.TempDir()
	r := &Runner{Dir: dir}
	require.NoError(t, r.run("init", "-b", "main"))
	require.NoError(t, r.run("config", "user.email", "test@example.com"))
	require.NoError(t, r.run("config", "user.name", "Test"))

	committed, err := r.CommitAll("empty")
	require.NoError(t, err)
	assert.False(t, committed)
}

func TestRunner_CommitAllDisablesSigning(t *testing.T) {
	dir := t.TempDir()
	r := &Runner{Dir: dir}
	require.NoError(t, r.run("init", "-b", "main"))
	require.NoError(t, r.run("config", "user.email", "test@example.com"))
	require.NoError(t, r.run("config", "user.name", "Test"))
	require.NoError(t, r.run("config", "commit.gpgsign", "true"))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hi"), 0644))

	committed, err := r.CommitAll("signed disabled")
	require.NoError(t, err)
	assert.True(t, committed)
}

func TestRunner_CurrentBranchAndHead(t *testing.T) {
	dir := t.TempDir()
	r := &Runner{Dir: dir}
	require.NoError(t, r.run("init", "-b", "main"))
	require.NoError(t, r.run("config", "user.email", "test@example.com"))
	require.NoError(t, r.run("config", "user.name", "Test"))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hi"), 0644))
	committed, err := r.CommitAll("initial")
	require.NoError(t, err)
	require.True(t, committed)

	branch, err := r.CurrentBranch()
	require.NoError(t, err)
	assert.Equal(t, "main", branch)
	head, err := r.Head()
	require.NoError(t, err)
	assert.Len(t, head, 40)
}

func TestRunner_PushForce(t *testing.T) {
	root := t.TempDir()
	bare := filepath.Join(root, "origin.git")
	work := filepath.Join(root, "repo")

	require.NoError(t, exec.Command("git", "init", "--bare", bare).Run())
	require.NoError(t, exec.Command("git", "clone", bare, work).Run())
	require.NoError(t, exec.Command("git", "-C", work, "config", "user.email", "t@t").Run())
	require.NoError(t, exec.Command("git", "-C", work, "config", "user.name", "T").Run())
	require.NoError(t, exec.Command("git", "-C", work, "commit", "--allow-empty", "-m", "init").Run())
	require.NoError(t, exec.Command("git", "-C", work, "push", "-u", "origin", "main").Run())

	r := &Runner{Dir: work}
	require.NoError(t, r.CheckoutNew("feat/x", "main"))
	require.NoError(t, os.WriteFile(filepath.Join(work, "feat.txt"), []byte("feature"), 0644))
	committed, err := r.CommitAll("feature commit")
	require.NoError(t, err)
	require.True(t, committed)
	require.NoError(t, r.PushForce("feat/x"))

	out, err := exec.Command("git", "-C", bare, "rev-parse", "refs/heads/feat/x").Output()
	require.NoError(t, err)
	assert.NotEmpty(t, strings.TrimSpace(string(out)))
}
