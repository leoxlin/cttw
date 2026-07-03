package config

import (
	"os"
	"path/filepath"
	"testing"

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
