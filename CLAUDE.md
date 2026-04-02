# CLAUDE.md

This file provides guidance to Claude Code when working with code in this repository.

## Project Overview

Einai is the native agent runtime daemon for ttal. It owns the logos+temenos agent loop for native ttal agents (ask, subagents). It does NOT handle CC/Codex worker spawning — that stays in ttal.

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
- `internal/event` — NDJSON wire types for streaming
- `internal/provider` — BuildProvider() wrapping fantasy
- `internal/command` — CommandDoc vars for system prompts
- `internal/prompt` — Mode types, BuildSystemPromptForMode()
- `internal/repo` — ResolveRepoRef(), EnsureRepo()
- `internal/project` — GetProjectPath(), List()
- `internal/sandbox` — BuildAgentPaths(): compute per-request CWD + git dir paths; supports additional read-only paths for cross-project access
- `internal/agent` — DiscoverAgents(), FindAgent(), validateAgentAccess()

**Manager Plane (long-running)**
- `internal/session` — RunAsk(), RunAgent() — the core agent loops
  - `task_session.go` — TaskID, SessionHistory, LoadSession(), SaveSession() for taskwarrior-backed persistence
- `internal/daemon` — HTTP server on unix socket, handlers for /ask, /agent/run, /health

**CLI (thin wrappers)**
- `cmd/` — ei daemon, ei ask, ei agent

### Daemon Architecture

The daemon listens on a unix socket at `~/.einai/daemon.sock`. All CLI commands (ei ask, ei agent run) send requests to the daemon and stream NDJSON responses to the terminal.

Endpoints:
- POST /ask — streams agent response (calls session.RunAsk)
- POST /agent/run — streams agent run (calls session.RunAgent)
- GET /health — liveness check

### Testing Patterns

- Unit tests in `_test.go` files alongside the package
- Use table-driven tests
- Mock external calls (ttal project list, logos) with interfaces
- Run: `make test`

## Key Features (PR: feat/agent-task-sessions)

### Taskwarrior Integration

Agents can be run with `--task <id>` to associate the session with a taskwarrior task. The task ID can be:
- 8-character hex (e.g., `abc12345`)
- Full UUID (e.g., `12345678-1234-...`)

The task is validated against taskwarrior before starting (must exist and be pending).

### Session Persistence

When using `--task`, sessions are persisted to `~/.einai/sessions/<agent>-<task>.jsonl`:
- Messages are stored as JSONL (one JSON object per line)
- On re-run with the same task ID, the session is resumed automatically
- The agent receives the full conversation history

### Cross-Project Read Access

All einai agents always have read-only access to all projects from `ttal project list`. The `--project` flag sets the agent's cwd to that project. When the agent has rw access, cwd is read-write; all other projects remain read-only for cross-project reads.
