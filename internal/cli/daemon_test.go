package cli

import (
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/llin/cttw/internal/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWaitForDaemon_AlreadyRunning(t *testing.T) {
	sock := tempSocket(t)
	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)
	defer ln.Close()

	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})}
	go srv.Serve(ln)
	defer srv.Close()

	client := api.NewClient("unix://" + sock)
	err = waitForDaemon(client, time.Second, 10*time.Millisecond)
	assert.NoError(t, err)
}

func TestWaitForDaemon_TimesOut(t *testing.T) {
	sock := tempSocket(t)
	client := api.NewClient("unix://" + sock)
	err := waitForDaemon(client, 50*time.Millisecond, 10*time.Millisecond)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
}

func tempSocket(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "cttw.sock")
}
