package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Repo         string `toml:"repo"`
	GitHubToken  string `toml:"github_token"`
	LLMBaseURL   string `toml:"llm_base_url"`
	LLMAPIKey    string `toml:"llm_api_key"`
	LLMModel     string `toml:"llm_model"`
	DaemonSocket string `toml:"daemon_socket"`
}

func Load(path string, env map[string]string) (*Config, error) {
	if env == nil {
		env = envMap()
	}
	cfg := &Config{
		LLMBaseURL:   "https://api.openai.com/v1",
		LLMModel:     "gpt-4o",
		DaemonSocket: "unix:///tmp/cttw.sock",
	}
	if path == "" {
		path = defaultConfigPath()
	}
	if _, err := toml.DecodeFile(path, cfg); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load config: %w", err)
	}
	if v := env["REPO"]; v != "" {
		cfg.Repo = v
	}
	if v := env["GITHUB_TOKEN"]; v != "" {
		cfg.GitHubToken = v
	}
	if v := env["LLM_BASE_URL"]; v != "" {
		cfg.LLMBaseURL = v
	}
	if v := env["LLM_API_KEY"]; v != "" {
		cfg.LLMAPIKey = v
	}
	if v := env["LLM_MODEL"]; v != "" {
		cfg.LLMModel = v
	}
	if v := env["DAEMON_SOCKET"]; v != "" {
		cfg.DaemonSocket = v
	}
	if cfg.Repo == "" {
		return nil, fmt.Errorf("repo is required")
	}
	parts := strings.Split(cfg.Repo, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("repo must be owner/name")
	}
	return cfg, nil
}

func defaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "cttw", "config.toml")
}

func envMap() map[string]string {
	m := make(map[string]string)
	for _, e := range os.Environ() {
		if i := strings.IndexByte(e, '='); i > 0 {
			m[e[:i]] = e[i+1:]
		}
	}
	return m
}
