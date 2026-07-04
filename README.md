# cttw — Claudivicus Take The Wheel

A TUI CLI + daemon that coordinates ACP agents to break user problems into tasks, tracks them via GitHub issues/sub-issues, and opens stacked PRs.

## Quick Start

```bash
export GITHUB_TOKEN=...
# Install an ACP-compatible agent adapter, e.g.:
#   https://github.com/zed-industries/codex-acp
cttw daemon start
cttw problem owner/repo "add OAuth2 login"
cttw
```

## Configuration

Environment variables or `~/.config/cttw/config.toml`:

```toml
[[repos]]
owner = "owner"
name = "repo"
default_branch = "main"

[agent]
default_backend = "codex"

[agent.backends.codex]
type = "local"
command = "codex-acp"

github_token = "..."
daemon_socket = "unix:///tmp/cttw.sock"
```

You can also point to a custom config file with `CTTW_CONFIG=/path/to/config.toml`.

## Architecture

- **Daemon** persists state in SQLite, registers repos, exposes a Unix-socket HTTP API, and polls for pending tasks.
- **Coordinator** launches an ACP agent per problem to decompose it into tasks and creates GitHub issues.
- **Worker** launches an ACP agent per task to implement it; the agent produces branches and PRs.
- **CLI/TUI** talk to the daemon over the Unix socket.

cttw does not hold LLM API keys; agents are external processes that implement the [Agent Client Protocol](https://agentclientprotocol.com).

## Development

With [mise](https://mise.jdx.dev):

```bash
mise install
mise run build
mise run test
```
