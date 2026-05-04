# CLAUDE.md

This file provides guidance to Claude Code when working with code in this repository.

## Project Overview

Einai is a thin runtime dispatcher that shells out to `lenos` or `claude` for agent execution. It does NOT handle CC/Codex worker spawning — that stays in ttal.

## Essential Commands

```bash
# Format, tidy, qlty, and build
make all

# Run all CI checks (qlty, test, build)
make ci

# Run tests
make test

# Build binary
make build

# Install to GOPATH/bin
make install
```

## Architecture

Einai is a Go CLI + daemon. The binary is `ei`.

### Packages (by plane)

**Shared (used by all)**
- `internal/config` — EinaiConfig (TOML), TaskrcPath()
- `internal/runtime` — Runtime type (lenos, claude-code), Parse()
- `internal/provider` — BuildProvider() wrapping fantasy
- `internal/repo` — ResolveRepoRef(), EnsureRepo()
- `internal/project` — GetProjectPath(), List()
- `internal/agent` — Discover(), Find(), ValidateAccess(), ValidateRuntime()

**Manager Plane (long-running)**
- `internal/session` — RunAsk(), RunAgent(), RunLenos(), RunClaudeCode() — the core dispatch functions
- `internal/daemon` — HTTP server on unix socket, handlers for /ask, /agent/run, /health

**CLI (thin wrappers)**
- `cmd/` — ei daemon, ei ask, ei agent

## Daemon Management

**Restart via launchd** (not `run &`):
```bash
ei daemon restart
```

The daemon listens on a unix socket at `~/.einai/daemon.sock`. All CLI commands send requests to the daemon and receive a blocking JSON response.

Endpoints:
- POST /ask — blocking JSON AskResponse (calls session.RunAsk)
- POST /agent/run — blocking JSON AgentResponse (calls session.RunAgent)
- GET /health — liveness check

### Testing Patterns

- Unit tests in `_test.go` files alongside the package
- Use table-driven tests
- Mock external calls (ttal project list, lenos binary) with interfaces
- Run: `make test`

## Key Features

### Pluggable Runtime

`ei agent run` dispatches to one of two backends based on `--runtime` flag > config `default_runtime` > `lenos` default:

- **`claude-code`** — spawns `claude -p --agent <name> --output-format json`. Session logs saved to `~/.einai/sessions/cc/`.
- **`lenos`** — spawns `lenos run --agent <name> ...`. No session jsonl (lenos handles its own). Output saved to `~/.einai/outputs/lenos/`.

Agents are discovered if they have a `lenos:` block, a `claude-code:` block, or both.

### Blocking JSON Responses

Both `/ask` and `/agent/run` return a single blocking JSON response (`AskResponse` / `AgentResponse`). No streaming. Server-side timeout configured via `max_run_timeout` in config (default 20 min).

### Cross-Project Read Access

All einai agents always have read-only access to all projects from `ttal project list`.

For `ei ask`, the `--project` flag sets the agent's cwd to that project. When the agent has rw access, cwd is read-write; all other projects remain read-only for cross-project reads.

For `ei agent run`, working directory is always the caller's current working directory.
