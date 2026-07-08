# cttw

> Claudivicus! Take the wheel.

A TUI CLI + daemon that coordinates ACP agents to break user problems into
tasks, tracks them via GitHub issues/sub-issues, and opens stacked PRs.

## Quick Start

```bash
export GITHUB_TOKEN=...   # optional if `gh auth token` is available
export CTTW_REPO=owner/repo
cttw daemon start
cttw problem owner/repo "add OAuth2 login"
cttw
```

`CTTW_REPO` accepts a comma-separated list of repositories in `owner/name[:branch]`
form; the branch defaults to `main` when omitted.

## Configuration

Environment variables or `~/.config/cttw/config.toml`.
If `github_token` is not set, cttw will try to infer it from `gh auth token`:

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

github_token = "..."        # optional if `gh auth token` is available
daemon_socket = "unix:///tmp/cttw.sock"
```

You can also point to a custom config file with `CTTW_CONFIG=/path/to/config.toml`.

## Architecture

- **Daemon** persists state in SQLite, registers repos, exposes a Unix-socket HTTP API, and polls for pending tasks.
- **Coordinator** launches an ACP agent per problem to decompose it into tasks and creates GitHub issues.
- **Worker** launches an ACP agent per task to edit and validate code, while cttw owns branch creation, commits, rollback, push, and pull request creation. Agents return structured JSON describing the result; cttw commits successful diffs and resets failed or no-op diffs.
- **CLI/TUI** talk to the daemon over the Unix socket.

cttw does not hold LLM API keys; agents are external processes that implement the [Agent Client Protocol](https://agentclientprotocol.com).

## Security and Trust Model

cttw runs ACP agents as local subprocesses with the same privileges as the user
running the daemon. Agents have full read/write access to the configured
repository directories and can execute terminal commands. Because cttw is
designed for unattended daemon execution, the local handler auto-approves all
`permission/request` calls from the agent. Only run cttw in environments where
this level of access is acceptable.
Agents still run with local repository access through ACP, but they are not asked to perform git management actions. cttw treats the local checkout as a managed transaction boundary and handles commit, reset, push, and PR creation itself.

> **Note:** This branch uses a new SQLite schema (`repos`, `problems`, `tasks`). Legacy tables from earlier versions (`chunks`, `jobs`, `config`, and any old `tasks`) are left in place when opening an existing database but are no longer used.

## Development

With [mise](https://mise.jdx.dev):

```bash
mise install
mise run build
mise run test
```

To run the TUI locally from source:

```bash
export GITHUB_TOKEN=...   # optional if `gh auth token` is available
export CTTW_REPO=owner/repo[:branch]
mise run dev              # equivalent to `go run ./cmd/cttw`
```

The default command launches the interactive TUI. Start the local daemon first
when you want the TUI to show live problem/task state:

```bash
go run ./cmd/cttw daemon start
go run ./cmd/cttw tui
go run ./cmd/cttw daemon stop
```

### Environment variables

- `GITHUB_TOKEN`: GitHub API token. Required unless `gh auth token` is available.
- `CTTW_REPO`: comma-separated repositories in `owner/name[:branch]` form. The
  branch defaults to `main`.
- `CTTW_CONFIG`: optional path to a TOML config file. Defaults to
  `~/.config/cttw/config.toml`.
- `DAEMON_SOCKET`: optional daemon API socket override. Defaults to
  `unix:///tmp/cttw.sock`.
- `CTTW_DB`: optional daemon SQLite database path. Defaults to
  `~/.local/share/cttw/cttw.db`.
- `CTTW_REPOS`: optional daemon checkout root. Defaults to
  `~/.local/share/cttw/repos`.
