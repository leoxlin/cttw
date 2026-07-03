package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_Defaults(t *testing.T) {
	cfg, err := Load("", map[string]string{"REPO": "owner/repo"})
	require.NoError(t, err)
	assert.Equal(t, "owner/repo", cfg.Repo)
	assert.Equal(t, "https://api.openai.com/v1", cfg.LLMBaseURL)
	assert.Equal(t, "gpt-4o", cfg.LLMModel)
	assert.Equal(t, "unix:///tmp/cttw.sock", cfg.DaemonSocket)
}

func TestLoad_FromEnv(t *testing.T) {
	cfg, err := Load("", map[string]string{
		"GITHUB_TOKEN": "gh_token",
		"LLM_API_KEY":  "llm_key",
		"REPO":         "owner/repo",
	})
	require.NoError(t, err)
	assert.Equal(t, "gh_token", cfg.GitHubToken)
	assert.Equal(t, "llm_key", cfg.LLMAPIKey)
	assert.Equal(t, "owner/repo", cfg.Repo)
}

func TestLoad_RepoRequired(t *testing.T) {
	_, err := Load("", map[string]string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "repo is required")
}

func TestLoad_RepoFormat(t *testing.T) {
	cases := []string{"owner", "owner/", "/repo", "owner/repo/extra"}
	for _, repo := range cases {
		_, err := Load("", map[string]string{"REPO": repo})
		require.Error(t, err, "repo: %s", repo)
		assert.Contains(t, err.Error(), "repo must be owner/name")
	}
}

func TestLoad_FromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cttw.toml")
	content := `
repo = "file/repo"
github_token = "file_token"
llm_base_url = "https://example.com/v1"
llm_model = "custom-model"
daemon_socket = "unix:///var/run/cttw.sock"
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	cfg, err := Load(path, map[string]string{
		"REPO":        "env/repo",
		"LLM_API_KEY": "env_key",
	})
	require.NoError(t, err)
	assert.Equal(t, "env/repo", cfg.Repo)
	assert.Equal(t, "file_token", cfg.GitHubToken)
	assert.Equal(t, "https://example.com/v1", cfg.LLMBaseURL)
	assert.Equal(t, "env_key", cfg.LLMAPIKey)
	assert.Equal(t, "custom-model", cfg.LLMModel)
	assert.Equal(t, "unix:///var/run/cttw.sock", cfg.DaemonSocket)
}
