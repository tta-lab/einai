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
- `internal/config` — EinaiConfig (TOML)
- `internal/event` — NDJSON wire types for streaming
- `internal/provider` — BuildProvider() wrapping fantasy
- `internal/command` — CommandDoc vars for system prompts
- `internal/prompt` — Mode types, BuildSystemPromptForMode()
- `internal/repo` — ResolveRepoRef(), EnsureRepo()
- `internal/project` — GetProjectPath()
- `internal/sandbox` — BuildAgentPaths(): compute per-request CWD + git dir paths for temenos /run-block
- `internal/agent` — DiscoverAgents(), FindAgent(), validateAgentAccess()

**Manager Plane (long-running)**
- `internal/session` — RunAsk(), RunAgent() — the core agent loops
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
