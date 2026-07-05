package launcher

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/llin/cttw/internal/acp"
	"github.com/llin/cttw/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeCodexAgent is a minimal ACP-speaking fake process written as a Go test helper.
// It reads JSON-RPC lines from stdin and writes responses to stdout.
func TestCodexLauncher_LaunchAndPrompt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dir := t.TempDir()
	fakeBin := filepath.Join(dir, "codex-acp")
	// Build a tiny fake ACP agent binary.
	code := `package main
import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)
type env struct {
	JSONRPC string          ` + "`json:\"jsonrpc\"`" + `
	ID      json.RawMessage ` + "`json:\"id,omitempty\"`" + `
	Method  string          ` + "`json:\"method,omitempty\"`" + `
	Params  json.RawMessage ` + "`json:\"params,omitempty\"`" + `
	Result  json.RawMessage ` + "`json:\"result,omitempty\"`" + `
}
func main() {
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		var e env
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			continue
		}
		var res []byte
		switch e.Method {
		case "initialize":
			res, _ = json.Marshal(env{JSONRPC: "2.0", ID: e.ID, Result: json.RawMessage(` + "`{\"protocolVersion\":1,\"agentCapabilities\":{},\"agentInfo\":{\"name\":\"fake\",\"version\":\"1\"},\"authMethods\":[]}`" + `)})
		case "newSession":
			res, _ = json.Marshal(env{JSONRPC: "2.0", ID: e.ID, Result: json.RawMessage(` + "`{\"sessionId\":\"s1\"}`" + `)})
		case "closeSession":
			res, _ = json.Marshal(env{JSONRPC: "2.0", ID: e.ID, Result: json.RawMessage(` + "`{}`" + `)})
		case "prompt":
			res, _ = json.Marshal(env{JSONRPC: "2.0", ID: e.ID, Result: json.RawMessage(` + "`{\"stopReason\":\"end_turn\",\"content\":\"done\"}`" + `)})
		default:
			res, _ = json.Marshal(env{JSONRPC: "2.0", ID: e.ID, Result: json.RawMessage(` + "`{}`" + `)})
		}
		fmt.Println(string(res))
	}
}
`
	mainFile := filepath.Join(dir, "main.go")
	require.NoError(t, os.WriteFile(mainFile, []byte(code), 0o644))
	require.NoError(t, runCmd("go", "build", "-o", fakeBin, mainFile))

	cfg := &config.Config{
		Agent: config.AgentConfig{
			DefaultBackend: "codex",
			Backends: map[string]config.BackendConfig{
				"codex": {Type: "local", Command: fakeBin},
			},
		},
	}

	l := NewCodexLauncher(cfg)
	agent, err := l.Launch(ctx, LaunchSpec{
		Backend: "codex",
		Repo:    RepoContext{Owner: "llin", Name: "cttw", DefaultBranch: "main", LocalDir: dir},
		Task:    TaskContext{ProblemDescription: "p", TaskTitle: "t", TaskDescription: "d"},
	})
	require.NoError(t, err)
	require.NoError(t, agent.Initialize(ctx))
	require.NoError(t, agent.NewSession(ctx, acp.NewSessionRequest{CWD: dir}))

	res, err := agent.Prompt(ctx, "do work")
	require.NoError(t, err)
	assert.Equal(t, "end_turn", res.StopReason)
	assert.Equal(t, "done", res.Content)

	require.NoError(t, agent.Close(ctx))
}

func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func TestDefaultHandler_ResolvePath(t *testing.T) {
	cwd := t.TempDir()
	d := &defaultHandler{cwd: cwd}

	// Relative path inside cwd resolves normally.
	got, err := d.resolvePath("foo/bar.txt")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(cwd, "foo", "bar.txt"), got)

	// Absolute path inside cwd resolves normally.
	absInside := filepath.Join(cwd, "baz.txt")
	got, err = d.resolvePath(absInside)
	require.NoError(t, err)
	assert.Equal(t, absInside, got)

	// Path traversal escapes are rejected.
	_, err = d.resolvePath("../etc/passwd")
	require.Error(t, err)

	// Absolute path outside cwd is rejected.
	_, err = d.resolvePath("/etc/passwd")
	require.Error(t, err)

	// Brackets in cleaned path are rejected.
	_, err = d.resolvePath("foo/../../etc/passwd")
	require.Error(t, err)
}

func TestCodexAgent_CloseTerminatesProcess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dir := t.TempDir()
	fakeBin := filepath.Join(dir, "codex-acp")
	// Fake agent that ignores SIGTERM until SIGKILL.
	code := `package main
import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)
type env struct {
	JSONRPC string          ` + "`json:\"jsonrpc\"`" + `
	ID      json.RawMessage ` + "`json:\"id,omitempty\"`" + `
	Method  string          ` + "`json:\"method,omitempty\"`" + `
	Params  json.RawMessage ` + "`json:\"params,omitempty\"`" + `
	Result  json.RawMessage ` + "`json:\"result,omitempty\"`" + `
}
func main() {
	sc := bufio.NewScanner(os.Stdin)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		// Ignore SIGTERM/SIGINT; wait for SIGKILL.
		for {
			time.Sleep(100 * time.Millisecond)
		}
	}()
	for sc.Scan() {
		var e env
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			continue
		}
		var res []byte
		switch e.Method {
		case "initialize":
			res, _ = json.Marshal(env{JSONRPC: "2.0", ID: e.ID, Result: json.RawMessage(` + "`{\"protocolVersion\":1,\"agentCapabilities\":{},\"agentInfo\":{\"name\":\"fake\",\"version\":\"1\"},\"authMethods\":[]}`" + `)})
		case "newSession":
			res, _ = json.Marshal(env{JSONRPC: "2.0", ID: e.ID, Result: json.RawMessage(` + "`{\"sessionId\":\"s1\"}`" + `)})
		case "closeSession":
			res, _ = json.Marshal(env{JSONRPC: "2.0", ID: e.ID, Result: json.RawMessage(` + "`{}`" + `)})
		case "prompt":
			res, _ = json.Marshal(env{JSONRPC: "2.0", ID: e.ID, Result: json.RawMessage(` + "`{\"stopReason\":\"end_turn\",\"content\":\"done\"}`" + `)})
		default:
			res, _ = json.Marshal(env{JSONRPC: "2.0", ID: e.ID, Result: json.RawMessage(` + "`{}`" + `)})
		}
		fmt.Println(string(res))
	}
}
`
	mainFile := filepath.Join(dir, "main.go")
	require.NoError(t, os.WriteFile(mainFile, []byte(code), 0o644))
	require.NoError(t, runCmd("go", "build", "-o", fakeBin, mainFile))

	cfg := &config.Config{
		Agent: config.AgentConfig{
			DefaultBackend: "codex",
			Backends: map[string]config.BackendConfig{
				"codex": {Type: "local", Command: fakeBin},
			},
		},
	}

	l := NewCodexLauncher(cfg)
	agent, err := l.Launch(ctx, LaunchSpec{
		Backend: "codex",
		Repo:    RepoContext{Owner: "llin", Name: "cttw", DefaultBranch: "main", LocalDir: dir},
		Task:    TaskContext{ProblemDescription: "p", TaskTitle: "t", TaskDescription: "d"},
	})
	require.NoError(t, err)
	require.NoError(t, agent.Initialize(ctx))
	require.NoError(t, agent.NewSession(ctx, acp.NewSessionRequest{CWD: dir}))

	closeCtx, closeCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer closeCancel()
	require.NoError(t, agent.Close(closeCtx))

	// After Close returns, the process must not be running.
	ca, ok := agent.(*codexAgent)
	require.True(t, ok)
	require.NotNil(t, ca.cmd.Process)
	proc, err := os.FindProcess(ca.cmd.Process.Pid)
	require.NoError(t, err)
	err = proc.Signal(syscall.Signal(0))
	assert.Error(t, err, "process should no longer exist after Close")
}
