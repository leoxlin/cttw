package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/llin/cttw/internal/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProblemCmd_PrintsAsyncMessage(t *testing.T) {
	want := api.ProblemResponse{
		ID:          "p1",
		Description: "build API",
		Status:      "pending",
		RepoID:      "r1",
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/problems", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		require.NoError(t, json.NewEncoder(w).Encode(want))
	}))
	defer server.Close()

	t.Setenv("CTTW_REPO", "llin/cttw")
	t.Setenv("DAEMON_SOCKET", server.Listener.Addr().String())

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	cmd := problemCmd()
	cmd.SetArgs([]string{"llin/cttw", "build API"})
	execErr := cmd.Execute()

	require.NoError(t, w.Close())
	os.Stdout = oldStdout
	require.NoError(t, execErr)

	out, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Contains(t, string(out), "Problem p1 created")
	assert.Contains(t, string(out), "decomposition in progress")
}
