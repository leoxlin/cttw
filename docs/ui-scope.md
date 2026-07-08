# UI Scope and Data Model

This document audits the current CLI, daemon API, TUI, and store model to define
the minimum project UI needed to make cttw usable. It describes the UI contract
as it exists today and calls out the daemon fields/endpoints that should be
exposed before a richer UI is built.

## Current Surfaces

### CLI

- `cttw` and `cttw tui` launch the Bubble Tea TUI.
- `cttw daemon start` starts the daemon, waits for `/api/v1/status`, and reports
  the configured socket.
- `cttw daemon stop` calls `/api/v1/shutdown`.
- `cttw daemon status` calls `/api/v1/status` and reports running/not running.
- `cttw problem <owner>/<repo> <description>` creates a problem and prints the
  returned problem ID.
- `cttw problems` lists short problem ID, status, and description.

### TUI

- Dashboard screen:
  - Fetches `GET /api/v1/problems` on startup and on refresh.
  - Shows daemon/API errors inline.
  - Lists short problem ID, status, and description.
  - Actions: `n` opens new problem, `esc` refreshes, `q`/`ctrl+c` quits.
- New problem screen:
  - Accepts one text area value formatted as `owner/repo description`.
  - Submits with `ctrl+d` to `POST /api/v1/problems`.
  - Actions: `esc` cancels/back, successful submit returns to dashboard.

### Daemon API

- `GET /api/v1/status`
  - Response: `{ "status": "ok" }`.
- `POST /api/v1/problems`
  - Request fields: `owner`, `repo`, `description`.
  - Validation: all three fields are required; description must not be blank.
  - Response: `202 Accepted` with a problem object.
- `GET /api/v1/problems`
  - Response: newest-first problem objects without embedded tasks.
- `GET /api/v1/problems/{id}`
  - Response: a problem object with embedded tasks.
- `POST /api/v1/shutdown`
  - Response: `204 No Content`.

## Required Screens

### Daemon Status

Purpose: make connection state and daemon lifecycle understandable before the
user creates work.

Required fields:

- socket address from config
- daemon status from `/api/v1/status`
- last error message, when unavailable

Required actions:

- refresh status
- start daemon outside the UI for now, since the current API only supports
  status and shutdown
- stop daemon only if the UI intentionally exposes the existing shutdown API

### Problem List

Purpose: show all top-level user requests and their progress.

Required fields from current API:

- `id`
- `description`
- `status`
- `repo_id`
- `issue_number`
- `created_at`
- `updated_at`

Required display fields not exposed by current API:

- repository owner/name
- task counts by status
- parent GitHub issue URL or repository URL

Required actions:

- refresh
- create problem
- open problem detail

### New Problem

Purpose: create a problem for a registered or lazily ensured repository.

Required fields:

- `owner`
- `repo`
- `description`

Required actions:

- submit
- cancel

Recommended UI behavior:

- Split `owner/repo` and description into separate controls instead of the
  current single text area input.
- Keep the daemon's validation errors visible on failed submit.

### Problem Detail

Purpose: inspect decomposition progress, child tasks, and external GitHub work.

Required fields from current API:

- Problem: `id`, `description`, `status`, `repo_id`, `issue_number`,
  `created_at`, `updated_at`.
- Tasks: `id`, `title`, `description`, `status`, `pr_number`, `created_at`,
  `updated_at`.

Required display fields not exposed by current API:

- repository owner/name
- task `issue_number`
- task `branch`
- task `base_branch`
- task `attempts`
- task `max_attempts`
- task `output`
- GitHub issue and pull request URLs

Required actions:

- refresh
- return to problem list
- open issue/PR links when URLs are exposed

### Task Detail

Purpose: diagnose running, failed, and completed task work.

Required fields not exposed by current API:

- `problem_id`
- `repo_id`
- `agent_session_id`
- `branch`
- `base_branch`
- `issue_number`
- `output`
- `attempts`
- `max_attempts`

Required actions:

- return to problem detail
- open branch/issue/PR links when URLs are exposed

## Data Model

### Problem

Current API fields:

- `id`: UUID string.
- `description`: user request.
- `status`: `pending`, `ready`, or `failed`.
- `repo_id`: internal repository UUID.
- `issue_number`: parent GitHub issue number, zero when not yet created.
- `tasks`: task list, only present on `GET /api/v1/problems/{id}`.
- `created_at`, `updated_at`: RFC3339 timestamps.

Needed UI additions:

- `repo`: nested object with `id`, `owner`, `name`, `default_branch`.
- `issue_url`: URL for the parent GitHub issue.
- `task_counts`: counts keyed by task status for list views.

### Task

Current API fields:

- `id`: UUID string.
- `title`: decomposition title.
- `description`: implementation instructions.
- `status`: `pending`, `running`, `completed`, or `failed`.
- `pr_number`: GitHub pull request number, zero when absent.
- `created_at`, `updated_at`: RFC3339 timestamps.

Needed UI additions:

- `problem_id`
- `repo_id`
- `issue_number`
- `branch`
- `base_branch`
- `agent_session_id`
- `output`
- `attempts`
- `max_attempts`
- `issue_url`
- `pr_url`

### Repo

Current store fields, not exposed by API:

- `id`
- `owner`
- `name`
- `local_dir`
- `default_branch`
- `clone_url`
- `created_at`
- `updated_at`

Needed UI additions:

- `html_url`
- problem count
- active task count

## API Gaps Before Rich UI

- Add repository data to problem responses or expose `GET /api/v1/repos`.
- Add task detail fields currently stored but omitted from `TaskResponse`.
- Add link fields for GitHub issues and pull requests, or enough repository
  metadata for clients to build them consistently.
- Add a task detail endpoint if task drill-down should avoid refetching the
  whole problem.
- Add filtering parameters for problem and task status once lists grow beyond a
  short dashboard.
