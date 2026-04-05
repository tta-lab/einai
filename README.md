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
Ask a question with access to projects, repos, URLs, or the web.

|  Flag | Description |
|------|-------------|
| `--project` | Ask about a registered ttal project |
| `--repo` | Ask about a GitHub/Forgejo repo (auto-clones) |
| `--url` | Ask about a web page (fetches content) |
| `--web` | Search the web to answer the question |
| `--save` | Save the final answer to flicknote |
| `--async` | Submit as async job (non-blocking, see Async below) |

Examples:
```bash
ei ask "how does the auth middleware work?"
ei ask "how does routing work?" --project myapp
ei ask "explain the pipeline syntax" --repo woodpecker-ci/woodpecker
ei ask "what auth methods?" --url https://docs.example.com
ei ask "latest Go generics syntax?" --web
ei ask "summarize this project" --save
```

### Async

Both `ei ask --async` and `ei agent run --async` submit the request to [pueue](https://github.com/Nukesor/pueue) for background execution. The CLI returns immediately with a confirmation message; the job notifies via tmux on completion.

**Files written:**
- `~/.einai/jobs/<runtime>/<stem>.sh` — the job script (runtime is `ask`, `claude-code`, or `ei-native`)
- `~/.einai/outputs/<runtime>/<stem>.md` — the result when complete
- `~/.einai/sessions/ei/<stem>.jsonl` — session log (ei-native runs only)
- `~/.einai/errors/ei/<stem>.jsonl` — error log (ei-native runs only)

**tmux callback:** If running inside tmux, the job sends a message to the current pane on completion:
- ✅ on success: `ei ask finished. Read result: cat ~/.einai/outputs/...`
- ❌ on failure: `ei ask failed (exit N). Read result: cat ~/.einai/outputs/...`

**Pueue config** (`~/.config/einai/config.toml`):
```toml
[pueue]
group = "einai"    # pueue group name (default: "einai")
parallel = 3       # max concurrent jobs (default: 3)
```

```bash
ei ask "research X" --async
# Queued. You'll be notified here when it completes.

# Monitor with:
pueue status
pueue log -f <job_id>
cat ~/.einai/outputs/ask/<stem>.md
```

### Agent
```bash
ei agent run <name> [prompt] [flags]  # run a named agent
ei agent list                          # list discovered agents
```

| Flag | Description |
|------|-------------|
| `--project` | Run in a registered project directory |
| `--repo` | Run in a cloned repo (read-only) |
| `--runtime` | Runtime: `ei-native` or `claude-code` (default: config or `claude-code`) |
| `--env` | Extra env vars for the sandbox (KEY=VALUE, can repeat) |

Examples:
```bash
ei agent run coder "implement auth"
echo "implement X" | ei agent run coder
ei agent run coder --runtime ei-native "implement auth"
ei agent run coder --env OPENAI_KEY=xxx --env DEBUG=true
```

### Daemon
```bash
ei daemon run      # start daemon in foreground
ei daemon status   # check daemon health
```


## Architecture

The daemon listens on a unix socket at `~/.einai/daemon.sock`. CLI commands send requests to the daemon, which runs sessions and returns a blocking JSON response.

**Endpoints:**
- `POST /ask` — blocking JSON `AskResponse{result, duration_ms, error}`
- `POST /agent/run` — blocking JSON `AgentResponse{result, duration_ms, error}`
- `GET /health` — liveness check

## Configuration

Config is read from `~/.config/einai/config.toml`.

```toml
model = "claude-sonnet-4-6"         # default model
max_steps = 100                     # agent loop iteration limit
max_tokens = 131072                 # maximum output tokens per step
agents_paths = ["~/.einai/agents"]  # directories to discover agents
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
