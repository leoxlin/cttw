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

	// The auth header should be configured for github.com.
	header, err := (&Runner{Dir: cloneDir}).Output("config", "--get", "http.https://github.com/.extraHeader")
	require.NoError(t, err)
	wantAuth := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
	assert.Contains(t, string(header), "Authorization: Basic "+wantAuth)
}
