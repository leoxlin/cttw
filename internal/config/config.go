package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	GitHubToken  string       `toml:"github_token"`
	DaemonSocket string       `toml:"daemon_socket"`
	Repos        []RepoConfig `toml:"repos"`
	Agent        AgentConfig  `toml:"agent"`
}

type RepoConfig struct {
	Owner         string `toml:"owner"`
	Name          string `toml:"name"`
	DefaultBranch string `toml:"default_branch"`
}

type AgentConfig struct {
	DefaultBackend string                   `toml:"default_backend"`
	Backends       map[string]BackendConfig `toml:"backends"`
}

type BackendConfig struct {
	Type    string `toml:"type"`
	Command string `toml:"command"`
	URL     string `toml:"url"`
}

func Load(path string, env map[string]string) (*Config, error) {
	if env == nil {
		env = envMap()
	}
	cfg := &Config{
		DaemonSocket: "unix:///tmp/cttw.sock",
		Agent: AgentConfig{
			DefaultBackend: "codex",
			Backends: map[string]BackendConfig{
				"codex": {Type: "local", Command: "codex-acp"},
			},
		},
	}
	if path == "" {
		path = defaultConfigPath()
	}
	if _, err := toml.DecodeFile(path, cfg); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load config: %w", err)
	}
	if v := env["GITHUB_TOKEN"]; v != "" {
		cfg.GitHubToken = v
	}
	if v := env["DAEMON_SOCKET"]; v != "" {
		cfg.DaemonSocket = v
	}
	if err := validate(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func validate(cfg *Config) error {
	if cfg.GitHubToken == "" {
		return fmt.Errorf("github_token is required")
	}
	if len(cfg.Repos) == 0 {
		return fmt.Errorf("at least one repo is required")
	}
	for i, r := range cfg.Repos {
		if r.Owner == "" || r.Name == "" {
			return fmt.Errorf("repo %d must have owner and name", i)
		}
		if r.DefaultBranch == "" {
			cfg.Repos[i].DefaultBranch = "main"
		}
	}
	if cfg.Agent.DefaultBackend == "" {
		return fmt.Errorf("agent.default_backend is required")
	}
	backend, ok := cfg.Agent.Backends[cfg.Agent.DefaultBackend]
	if !ok {
		return fmt.Errorf("agent default backend %q not configured", cfg.Agent.DefaultBackend)
	}
	switch backend.Type {
	case "local":
		if backend.Command == "" {
			return fmt.Errorf("local backend %q requires command", cfg.Agent.DefaultBackend)
		}
	default:
		return fmt.Errorf("backend %q has unsupported type %q", cfg.Agent.DefaultBackend, backend.Type)
	}
	return nil
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
