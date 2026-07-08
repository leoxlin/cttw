# Project Structure Audit

## App Framework

cttw is a Go 1.25 command-line application. The executable lives at
`cmd/cttw/main.go` and dispatches to either the interactive CLI/TUI or the
daemon process.

- CLI framework: Cobra (`github.com/spf13/cobra`) in `internal/cli`.
- Terminal UI framework: Bubble Tea, Bubbles, and Lip Gloss in `internal/tui`.
- Persistence: SQLite via `modernc.org/sqlite` in `internal/store`.
- Local daemon API: Go `net/http` served over a Unix socket by default.
- Agent integration: Agent Client Protocol (ACP) JSON-RPC over local
  subprocess transports in `internal/acp` and `internal/launcher`.

There is no browser frontend framework, package manager manifest, or client-side
route tree in this repository.

## Existing UI Routes and Screens

CLI command routes are registered in `internal/cli/root.go`:

- `cttw`: launches the dashboard TUI.
- `cttw tui`: launches the dashboard TUI explicitly.
- `cttw problem <owner>/<repo> <description>`: creates a problem through the
  daemon API.
- `cttw problems`: lists known problems through the daemon API.
- `cttw daemon start`: starts the daemon in `--daemon` mode.
- `cttw daemon stop`: asks the daemon to shut down.
- `cttw daemon status`: checks daemon health.

TUI screens are modeled by `internal/tui.Model.Screen`:

- `dashboard`: fetches and displays problems from the daemon.
- `newtask`: accepts `owner/repo description` input and submits a new problem.

## Backend API Consumed by the UI

The UI consumes the local daemon through `internal/api.Client`. The daemon
registers these HTTP routes in `internal/daemon/daemon.go`:

- `GET /api/v1/status`: returns `{"status":"ok"}`.
- `POST /api/v1/shutdown`: shuts the daemon down and returns `204 No Content`.
- `POST /api/v1/problems`: accepts `CreateProblemRequest` and returns
  `202 Accepted` with a `ProblemResponse`.
- `GET /api/v1/problems`: returns `[]ProblemResponse`.
- `GET /api/v1/problems/{id}`: returns a `ProblemResponse` with `tasks`
  populated.

The default daemon socket is `unix:///tmp/cttw.sock`; non-Unix socket values are
treated as TCP host addresses by the API client.

## Data Models

SQLite tables are migrated in `internal/store/store.go`:

- `repos`: registered GitHub repositories with owner, name, local checkout,
  default branch, optional clone URL, and timestamps.
- `problems`: top-level user requests with description, status, repository ID,
  parent GitHub issue number, and timestamps.
- `tasks`: decomposed work items with problem/repo IDs, title, description,
  status, ACP session ID, branch/base branch, PR and issue numbers, output,
  retry counters, and timestamps.

Primary Go domain structs:

- `config.Config`, `RepoConfig`, `AgentConfig`, and `BackendConfig` in
  `internal/config`.
- `store.Repo`, `store.Problem`, and `store.Task` in `internal/store`.
- `api.CreateProblemRequest`, `api.ProblemResponse`, and `api.TaskResponse` in
  `internal/api`.
- ACP request/response structs in `internal/acp/types.go`.
- Worker agent result JSON is represented by `worker.taskResult`.

Common status values observed in the store and worker flow are `pending`,
`running`, `ready`, `completed`, and `failed`.

## External Backend APIs and Services

cttw talks to GitHub through `internal/github.Client`:

- `POST /repos/{owner}/{repo}/issues` to create parent and task issues.
- `GET/PATCH /repos/{owner}/{repo}/issues/{number}` to append sub-issue links.
- `GET /repos/{owner}/{repo}/git/ref/heads/{base}` and
  `POST /repos/{owner}/{repo}/git/refs` to create task branches.
- `POST /repos/{owner}/{repo}/pulls` to create pull requests.
- `GET /repos/{owner}/{repo}/pulls/{number}` to verify PR head branches.

The daemon also launches an external ACP-compatible agent command, defaulting to
`codex-acp`, to decompose problems and implement tasks.

## Build and Development Scripts

Build and test scripts are defined in `mise.toml`:

- `mise run build`: `go build -o bin/cttw ./cmd/cttw`.
- `mise run test`: `go test ./...`.
- `mise run dev`: `go run ./cmd/cttw`.

The README documents `GITHUB_TOKEN`, `CTTW_REPO`, and `CTTW_CONFIG` as the main
runtime configuration inputs. `CTTW_DB`, `CTTW_REPOS`, and `DAEMON_SOCKET` are
also read by code paths in the daemon and config packages.
