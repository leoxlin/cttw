# cttw — Claudivicus Take The Wheel

A TUI CLI + daemon that breaks tasks into chunks, tracks them via GitHub issues/sub-issues, and opens stacked PRs.

## Quick Start

```bash
export GITHUB_TOKEN=...
export LLM_API_KEY=...
export REPO=owner/repo
cttw daemon start
cttw task "add OAuth2 login"
cttw
```

## Configuration

Environment variables or `~/.config/cttw/config.toml`:

```toml
repo = "owner/repo"
github_token = "..."
llm_api_key = "..."
llm_model = "gpt-4o"
```

## Development

```bash
make build
make test
```
