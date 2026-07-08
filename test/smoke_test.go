package test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/llin/cttw/internal/config"
	"github.com/llin/cttw/internal/coordinator"
	"github.com/llin/cttw/internal/github"
	"github.com/llin/cttw/internal/launcher"
	"github.com/llin/cttw/internal/repo"
	"github.com/llin/cttw/internal/store"
	"github.com/llin/cttw/internal/worker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type smokeGH struct {
	issueCount int
	prHead     string
}

func (s *smokeGH) CreateIssue(ctx context.Context, owner, repo, title, body string) (int, error) {
	s.issueCount++
	return s.issueCount, nil
}
func (s *smokeGH) CreateSubIssue(ctx context.Context, owner, repo string, parentNumber, childNumber int) error {
	return nil
}
func (s *smokeGH) CreateBranch(ctx context.Context, owner, repo, branch, base string) error {
	return nil
}
func (s *smokeGH) CreatePullRequest(ctx context.Context, owner, repo, title, body, head, base string) (int, error) {
	s.prHead = head
	return 1, nil
}
func (s *smokeGH) GetPullRequest(ctx context.Context, owner, repo string, number int) (*github.PullRequest, error) {
	return &github.PullRequest{Number: number, Head: struct {
		Ref string `json:"ref"`
	}{Ref: s.prHead}}, nil
}

func TestIntegration_FakeACPAgent(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	repoDir := initSmokeGitRepo(t, filepath.Join(dir, "repo"))

	fakeBin := buildFakeAgent(t, dir, repoDir)
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

	_, err = s.CreateRepo(ctx, "o", "r", repoDir, "main", "")
	require.NoError(t, err)

	reg := &repo.Registry{Root: filepath.Join(dir, "repos")}
	ln := launcher.NewCodexLauncher(cfg)
	gh := &smokeGH{}
	coord := coordinator.New(s, ln, reg, gh, "codex", time.Minute)
	w := worker.New(s, ln, reg, gh, "codex", time.Minute)

	problem, err := coord.CreateProblem(ctx, "o", "r", "add smoke test")
	require.NoError(t, err)
	assert.Equal(t, "pending", problem.Status)

	require.Eventually(t, func() bool {
		p, err := s.GetProblem(ctx, problem.ID)
		if err != nil {
			return false
		}
		return p.Status == "ready"
	}, 2*time.Second, 10*time.Millisecond)

	tasks, err := s.ListTasksByProblem(ctx, problem.ID)
	require.NoError(t, err)
	require.Len(t, tasks, 1)

	require.NoError(t, w.ExecuteTask(ctx, &tasks[0]))

	completed, err := s.GetTask(ctx, tasks[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", completed.Status)
	assert.Equal(t, 1, completed.PRNumber)
	assert.NotEmpty(t, completed.Branch)
	assert.Equal(t, completed.Branch, gh.prHead)
	assert.FileExists(t, filepath.Join(repoDir, "smoke.txt"))
}

func initSmokeGitRepo(t *testing.T, dir string) string {
	t.Helper()
	root := filepath.Dir(dir)
	bare := filepath.Join(root, "origin.git")
	cmd := exec.Command("git", "init", "--bare", bare)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	require.NoError(t, os.MkdirAll(dir, 0755))
	run("init", "-b", "main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	run("remote", "add", "origin", bare)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("base\n"), 0644))
	run("add", ".")
	run("commit", "-m", "initial")
	run("push", "-u", "origin", "main")
	return dir
}

func buildFakeAgent(t *testing.T, dir, repoDir string) string {
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
		case "session/new":
			res, _ = json.Marshal(env{JSONRPC: "2.0", ID: e.ID, Result: json.RawMessage(` + "`{\"sessionId\":\"s1\"}`" + `)})
		case "session/close":
			res, _ = json.Marshal(env{JSONRPC: "2.0", ID: e.ID, Result: json.RawMessage(` + "`{}`" + `)})
		case "session/prompt":
			call++
			_ = os.WriteFile(counterPath, []byte(fmt.Sprintf("%d", call)), 0644)
			if call == 1 {
				res, _ = json.Marshal(env{JSONRPC: "2.0", ID: e.ID, Result: json.RawMessage(` + "`{\"stopReason\":\"end_turn\",\"content\":\"[{\\\"title\\\":\\\"add test\\\",\\\"description\\\":\\\"add a smoke test\\\"}]\"}`" + `)})
			} else {
				_ = os.WriteFile(` + fmt.Sprintf("%q", filepath.Join(repoDir, "smoke.txt")) + `, []byte("smoke\n"), 0644)
				res, _ = json.Marshal(env{JSONRPC: "2.0", ID: e.ID, Result: json.RawMessage(` + "`{\"stopReason\":\"end_turn\",\"content\":\"{\\\"status\\\":\\\"completed\\\",\\\"summary\\\":\\\"added smoke file\\\",\\\"key_changes_made\\\":[\\\"smoke file\\\"],\\\"key_learnings\\\":[],\\\"verification\\\":[\\\"fake smoke verification\\\"]}\"}`" + `)})
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
