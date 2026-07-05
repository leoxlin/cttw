package test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/llin/cttw/internal/config"
	"github.com/llin/cttw/internal/coordinator"
	"github.com/llin/cttw/internal/launcher"
	"github.com/llin/cttw/internal/repo"
	"github.com/llin/cttw/internal/store"
	"github.com/llin/cttw/internal/worker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type smokeGH struct{ issueCount int }

func (s *smokeGH) CreateIssue(ctx context.Context, owner, repo, title, body string) (int, error) {
	s.issueCount++
	return s.issueCount, nil
}
func (s *smokeGH) CreateSubIssue(ctx context.Context, owner, repo string, parentNumber, childNumber int) error {
	return nil
}
func (s *smokeGH) CreateBranch(ctx context.Context, owner, repo, branch, base string) error { return nil }
func (s *smokeGH) CreatePullRequest(ctx context.Context, owner, repo, title, body, head, base string) (int, error) {
	return 1, nil
}

func TestIntegration_FakeACPAgent(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	fakeBin := buildFakeAgent(t, dir)
	cfg := &config.Config{
		GitHubToken: "token",
		Agent: config.AgentConfig{
			DefaultBackend: "codex",
			Backends: map[string]config.BackendConfig{
				"codex": {Type: "local", Command: fakeBin},
			},
		},
	}

	s, err := store.New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	_, err = s.CreateRepo(ctx, "o", "r", dir, "main", "")
	require.NoError(t, err)

	reg := &repo.Registry{Root: filepath.Join(dir, "repos")}
	ln := launcher.NewCodexLauncher(cfg)
	gh := &smokeGH{}
	coord := coordinator.New(s, ln, reg, gh, "codex")
	w := worker.New(s, ln, reg, gh, "codex")

	problem, err := coord.CreateProblem(ctx, "o", "r", "add smoke test")
	require.NoError(t, err)
	assert.Equal(t, "ready", problem.Status)

	tasks, err := s.ListTasksByProblem(ctx, problem.ID)
	require.NoError(t, err)
	require.Len(t, tasks, 1)

	require.NoError(t, w.ExecuteTask(ctx, &tasks[0]))

	completed, err := s.GetTask(ctx, tasks[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", completed.Status)
	assert.Equal(t, 42, completed.PRNumber)
	assert.Equal(t, "feat/smoke", completed.Branch)
}

func buildFakeAgent(t *testing.T, dir string) string {
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
	counterPath := ` + fmt.Sprintf("%q", filepath.Join(dir, "call_count")) + `
	call := 0
	if data, err := os.ReadFile(counterPath); err == nil {
		fmt.Sscanf(string(data), "%d", &call)
	}
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
			call++
			_ = os.WriteFile(counterPath, []byte(fmt.Sprintf("%d", call)), 0644)
			if call == 1 {
				res, _ = json.Marshal(env{JSONRPC: "2.0", ID: e.ID, Result: json.RawMessage(` + "`{\"stopReason\":\"end_turn\",\"content\":\"[{\\\"title\\\":\\\"add test\\\",\\\"description\\\":\\\"add a smoke test\\\"}]\"}`" + `)})
			} else {
				res, _ = json.Marshal(env{JSONRPC: "2.0", ID: e.ID, Result: json.RawMessage(` + "`{\"stopReason\":\"end_turn\",\"content\":\"{\\\"pr_number\\\":42,\\\"branch\\\":\\\"feat/smoke\\\",\\\"status\\\":\\\"completed\\\"}\"}`" + `)})
			}
		default:
			res, _ = json.Marshal(env{JSONRPC: "2.0", ID: e.ID, Result: json.RawMessage(` + "`{}`" + `)})
		}
		fmt.Println(string(res))
	}
}
`
	mainFile := filepath.Join(dir, "fake_acp.go")
	require.NoError(t, os.WriteFile(mainFile, []byte(code), 0o644))
	bin := filepath.Join(dir, "fake-acp")
	cmd := exec.Command("go", "build", "-o", bin, mainFile)
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Run())
	return bin
}
