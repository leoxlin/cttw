package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_Defaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cttw.toml")
	content := `
[[repos]]
owner = "llin"
name = "cttw"
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	cfg, err := Load(path, map[string]string{
		"GITHUB_TOKEN": "tok",
	})
	require.NoError(t, err)
	assert.Equal(t, "unix:///tmp/cttw.sock", cfg.DaemonSocket)
	assert.Equal(t, "codex", cfg.Agent.DefaultBackend)
	require.Len(t, cfg.Repos, 1)
	assert.Equal(t, "main", cfg.Repos[0].DefaultBranch)
	assert.Equal(t, "tok", cfg.GitHubToken)
}

func TestLoad_MultiRepoAndBackend(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cttw.toml")
	content := `
[[repos]]
owner = "llin"
name = "cttw"
default_branch = "main"

[[repos]]
owner = "llin"
name = "other-project"

[agent]
default_backend = "codex"

[agent.backends.codex]
type = "local"
command = "codex-acp"
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	cfg, err := Load(path, map[string]string{
		"GITHUB_TOKEN":  "gh_token",
		"DAEMON_SOCKET": "unix:///var/run/cttw.sock",
	})
	require.NoError(t, err)
	require.Len(t, cfg.Repos, 2)
	assert.Equal(t, "llin", cfg.Repos[0].Owner)
	assert.Equal(t, "cttw", cfg.Repos[0].Name)
	assert.Equal(t, "main", cfg.Repos[0].DefaultBranch)
	assert.Equal(t, "other-project", cfg.Repos[1].Name)
	assert.Equal(t, "main", cfg.Repos[1].DefaultBranch) // default
	assert.Equal(t, "codex", cfg.Agent.DefaultBackend)
	assert.Equal(t, "local", cfg.Agent.Backends["codex"].Type)
	assert.Equal(t, "codex-acp", cfg.Agent.Backends["codex"].Command)
	assert.Equal(t, "gh_token", cfg.GitHubToken)
	assert.Equal(t, "unix:///var/run/cttw.sock", cfg.DaemonSocket)
}

func TestLoad_RepoRequired(t *testing.T) {
	_, err := Load("", map[string]string{"GITHUB_TOKEN": "tok"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one repo")
}

func TestLoad_BackendMustExist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cttw.toml")
	content := `
[[repos]]
owner = "o"
name = "r"

[agent]
default_backend = "missing"
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	_, err := Load(path, map[string]string{"GITHUB_TOKEN": "tok"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "backend \"missing\" not configured")
}

func TestLoad_LocalBackendNeedsCommand(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cttw.toml")
	content := `
[[repos]]
owner = "o"
name = "r"

[agent]
default_backend = "codex"

[agent.backends.codex]
type = "local"
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	_, err := Load(path, map[string]string{"GITHUB_TOKEN": "tok"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command")
}

func TestLoad_RepoFromEnv(t *testing.T) {
	cfg, err := Load("", map[string]string{
		"GITHUB_TOKEN": "tok",
		"CTTW_REPO":    "llin/cttw:main,acme/other",
	})
	require.NoError(t, err)
	require.Len(t, cfg.Repos, 2)
	assert.Equal(t, "llin", cfg.Repos[0].Owner)
	assert.Equal(t, "cttw", cfg.Repos[0].Name)
	assert.Equal(t, "main", cfg.Repos[0].DefaultBranch)
	assert.Equal(t, "acme", cfg.Repos[1].Owner)
	assert.Equal(t, "other", cfg.Repos[1].Name)
	assert.Equal(t, "main", cfg.Repos[1].DefaultBranch)
}

func TestLoad_EnvOverridesTOMLRepos(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cttw.toml")
	content := `
[[repos]]
owner = "from"
name = "toml"
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	cfg, err := Load(path, map[string]string{
		"GITHUB_TOKEN": "tok",
		"CTTW_REPO":    "llin/cttw",
	})
	require.NoError(t, err)
	require.Len(t, cfg.Repos, 1)
	assert.Equal(t, "llin", cfg.Repos[0].Owner)
	assert.Equal(t, "cttw", cfg.Repos[0].Name)
}

func TestLoad_InvalidRepoEnv(t *testing.T) {
	_, err := Load("", map[string]string{
		"GITHUB_TOKEN": "tok",
		"CTTW_REPO":    "not-a-repo",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CTTW_REPO")
}

func TestLoad_AgentPromptTimeout(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cttw.toml")
	content := `
[[repos]]
owner = "o"
name = "r"

[agent]
default_backend = "codex"
prompt_timeout = "5m"
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	cfg, err := Load(path, map[string]string{"GITHUB_TOKEN": "tok"})
	require.NoError(t, err)
	assert.Equal(t, "5m", cfg.Agent.PromptTimeout)
	assert.Equal(t, 5*time.Minute, cfg.Agent.PromptTimeoutDuration())
}

func TestLoad_AgentPromptTimeoutDefaults(t *testing.T) {
	cfg, err := Load("", map[string]string{
		"GITHUB_TOKEN": "tok",
		"CTTW_REPO":    "o/r",
	})
	require.NoError(t, err)
	assert.Equal(t, "", cfg.Agent.PromptTimeout)
	assert.Equal(t, 10*time.Minute, cfg.Agent.PromptTimeoutDuration())
}

func TestLoad_MissingHomeDirIsNotError(t *testing.T) {
	// With no config path and a missing home dir, Load should still succeed when
	// environment variables provide the required values.
	cfg, err := Load("__missing_home_dir__", map[string]string{
		"GITHUB_TOKEN": "tok",
		"CTTW_REPO":    "o/r",
	})
	require.NoError(t, err)
	assert.Equal(t, "tok", cfg.GitHubToken)
}
