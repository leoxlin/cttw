package gitexec

import (
	"os"
	"path/filepath"
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

	out, err := r.runOutput("rev-parse", "--abbrev-ref", "HEAD")
	require.NoError(t, err)
	assert.Equal(t, "feat/x\n", string(out))
}
