package config

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// ghAuthToken runs `gh auth token` to infer a GitHub token from the GitHub CLI.
// It is a package variable so tests can stub it out.
var ghAuthToken = func() (string, error) {
	out, err := exec.Command("gh", "auth", "token").Output()
	if err != nil {
		return "", err
	}
	return string(bytes.TrimSpace(out)), nil
}

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
	PromptTimeout  string                   `toml:"prompt_timeout"`
	Backends       map[string]BackendConfig `toml:"backends"`
}

// PromptTimeoutDuration parses PromptTimeout as a time.Duration. It returns the
// default of 10 minutes if unset or invalid.
func (a AgentConfig) PromptTimeoutDuration() time.Duration {
	if a.PromptTimeout == "" {
		return 10 * time.Minute
	}
	d, err := time.ParseDuration(a.PromptTimeout)
	if err != nil {
		return 10 * time.Minute
	}
	return d
}

type BackendConfig struct {
	Type    string `toml:"type"`
	Command string `toml:"command"`
}

func Load(path string, env map[string]string) (*Config, error) {
	return load(path, env, true)
}

// LoadClient loads only the settings needed by commands that talk to an
// already-running daemon. It intentionally skips daemon-only validation such as
// github_token and agent backend checks.
func LoadClient(path string, env map[string]string) (*Config, error) {
	return load(path, env, false)
}

func load(path string, env map[string]string, validateDaemon bool) (*Config, error) {
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
	if path != "" {
		if _, err := toml.DecodeFile(path, cfg); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("load config: %w", err)
		}
	}
	if v := env["GITHUB_TOKEN"]; v != "" {
		cfg.GitHubToken = v
	}
	if validateDaemon && cfg.GitHubToken == "" {
		if tok, err := ghAuthToken(); err == nil && tok != "" {
			cfg.GitHubToken = tok
		}
	}
	if v := env["DAEMON_SOCKET"]; v != "" {
		cfg.DaemonSocket = v
	}
	// Environment variables override config file values. Precedence is
	// flag > env > config file > default; flags are not handled here.
	if v := env["CTTW_REPO"]; v != "" {
		repos, err := parseRepoEnv(v)
		if err != nil {
			return nil, fmt.Errorf("CTTW_REPO: %w", err)
		}
		cfg.Repos = repos
	}
	if validateDaemon {
		if err := validate(cfg); err != nil {
			return nil, err
		}
	} else {
		defaultRepoBranches(cfg.Repos)
	}
	return cfg, nil
}

func validate(cfg *Config) error {
	if cfg.GitHubToken == "" {
		return fmt.Errorf("github_token is required")
	}
	if err := validateRepos(cfg.Repos); err != nil {
		return err
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

func validateRepos(repos []RepoConfig) error {
	for i, r := range repos {
		if r.Owner == "" || r.Name == "" {
			return fmt.Errorf("repo %d must have owner and name", i)
		}
		if r.DefaultBranch == "" {
			repos[i].DefaultBranch = "main"
		}
	}
	return nil
}

func defaultRepoBranches(repos []RepoConfig) {
	for i := range repos {
		if repos[i].DefaultBranch == "" {
			repos[i].DefaultBranch = "main"
		}
	}
}

func parseRepoEnv(v string) ([]RepoConfig, error) {
	var repos []RepoConfig
	for _, part := range strings.Split(v, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		owner, name, branch, err := splitRepoSpec(part)
		if err != nil {
			return nil, err
		}
		repos = append(repos, RepoConfig{Owner: owner, Name: name, DefaultBranch: branch})
	}
	return repos, nil
}

func splitRepoSpec(spec string) (owner, name, branch string, err error) {
	branch = "main"
	if idx := strings.LastIndex(spec, ":"); idx >= 0 {
		branch = spec[idx+1:]
		spec = spec[:idx]
	}
	parts := strings.Split(spec, "/")
	if len(parts) != 2 {
		return "", "", "", fmt.Errorf("repo %q must be owner/name", spec)
	}
	owner, name = parts[0], parts[1]
	if owner == "" || name == "" {
		return "", "", "", fmt.Errorf("repo %q must have owner and name", spec)
	}
	return owner, name, branch, nil
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
