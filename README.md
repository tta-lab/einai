# einai

The existence layer for AI agents. Spawns, sandboxes, and sustains sessions on top of temenos and logos.

## Overview

Einai is a daemon that runs agent sessions. It owns the logos+temenos agent loop—receiving prompts, building system prompts, running agents in sandboxed loops, and streaming results back. The CLI binary is `ei`.

## Why einai?

Logos is a library. Temenos is a sandbox. But they need a runtime to wire them together.

Einai is that runtime: it manages agent discovery, prompt construction, sandbox configuration, rate limiting, and output formatting. ttal (the orchestrator) delegates native agent execution to einai.

## Stack

```
ttal       orchestrator — task routing, pipelines, worker spawning
einai      runtime — agent sessions, sandbox config, daemon
logos      agent loop (LLM ↔ command cycle)
temenos    sandbox — filesystem isolation
organon    tools — src, web fetch
```

## Install

**Homebrew**
```bash
brew install tta-lab/ttal/einai
```

**From source**
```bash
go install github.com/tta-lab/einai/cmd/ei@latest
```

**Releases**
See [GitHub Releases](https://github.com/tta-lab/einai/releases) for binaries and version history.

## Quick Start

```bash
ei daemon run &        # start daemon (foreground, background it)
ei daemon status       # check health
ei ask 'how does routing work?' --project myapp
ei agent run coder 'implement the auth module'
ei agent list
```

## CLI Commands

### Ask
```bash
ei ask 'question' [flags]
```
Ask a question. Modes: `--project`, `--repo`, `--url`, `--web`.

### Agent
```bash
ei agent run <name> 'prompt'  # run a named agent with prompt
ei agent list                 # list discovered agents
```

### Daemon
```bash
ei daemon run      # start daemon in foreground
ei daemon status   # check daemon health
```

### Sandbox
```bash
ei sandbox sync    # regenerate CC settings.json
```

## Architecture

The daemon listens on a unix socket at `~/.einai/daemon.sock`. CLI commands send requests to the daemon, which runs sessions via logos and streams NDJSON responses back.

**Endpoints:**
- `POST /ask` — streams agent response
- `POST /agent/run` — streams agent run
- `GET /health` — liveness check
- `POST /sandbox/sync` — regenerates CC settings.json

## Configuration

Config is read from `~/.config/einai/config.toml`.

```toml
agents_paths = ["~/.einai/agents"]  # directories to discover agents
max_steps = 100                     # agent loop iteration limit
model = "claude-sonnet-4-6"  # default model
```

## Ecosystem

| Project | Role |
|---------|------|
| temenos | The boundary — sandbox isolation |
| logos | The reason — agent loop |
| organon | The instruments — perception and action |
| einai | The existence — runtime that ties them together |
| ttal | The orchestrator — task routing, pipelines |

## The Name

Einai (εἶναι) is the Greek infinitive "to be"—existence, being. Where logos provides reason and temenos provides boundaries, einai provides the existence layer: the running process that brings agents into being.

## Development

```bash
make build    # build the binary
make test     # run tests
make ci       # run full CI checks
make install  # install to GOPATH/bin
```

## License

MIT
