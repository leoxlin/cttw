# cttw — Claudivicus Take The Wheel

A TUI CLI + daemon that coordinates ACP agents to break user problems into tasks, tracks them via GitHub issues/sub-issues, and opens stacked PRs.

## Quick Start

```bash
export GITHUB_TOKEN=...
export CTTW_REPO=owner/repo
# Install an ACP-compatible agent adapter, e.g.:
#   https://github.com/zed-industries/codex-acp
cttw daemon start
cttw problem owner/repo "add OAuth2 login"
cttw
```

`CTTW_REPO` accepts a comma-separated list of repositories in `owner/name[:branch]`
form; the branch defaults to `main` when omitted.

## Configuration

Environment variables or `~/.config/cttw/config.toml`:

```toml
[[repos]]
owner = "owner"
name = "repo"
default_branch = "main"

[agent]
default_backend = "codex"
prompt_timeout = "10m"

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

## Security and Trust Model

cttw runs ACP agents as local subprocesses with the same privileges as the user
running the daemon. Agents have full read/write access to the configured
repository directories and can execute terminal commands. Because cttw is
designed for unattended daemon execution, the local handler auto-approves all
`permission/request` calls from the agent. Only run cttw in environments where
this level of access is acceptable.

> **Note:** This branch uses a new SQLite schema (`repos`, `problems`, `tasks`). Legacy tables from earlier versions (`chunks`, `jobs`, `config`, and any old `tasks`) are left in place when opening an existing database but are no longer used.

## Development

With [mise](https://mise.jdx.dev):

```bash
mise install
mise run build
mise run test
```
