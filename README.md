# einai

Thin runtime dispatcher for AI agents. Shells out to `lenos` or `claude` — does not run in-process agent loops.

## Overview

Einai is a wrapper daemon that receives agent requests and dispatches them to the right runtime. It manages agent discovery, project resolution, async job queues, and output formatting. The CLI binary is `ei`.

## Why einai?

Lenos and Claude Code are agent runtimes. Einai sits above them: it resolves projects, validates agent access, manages background job queues, and delivers completion notifications. ttal delegates agent execution to einai.

## Stack

```
ttal       orchestrator — task routing, pipelines, worker spawning
einai      dispatcher — agent discovery, project resolution, job queue
lenos      agent runtime — lenos-family agents (internal ttal agents)
claude     agent runtime — Claude Code agents (CC agents)
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

Both `ei ask --async` and `ei agent run --async` submit the request to the einai daemon's job queue for background execution. The CLI returns immediately with a confirmation message; the job notifies via `ttal send` on completion.

**Monitor jobs:**
```bash
ei job list          # list all jobs (newest first)
ei job log <id>      # print job output
ei job kill <id>     # SIGTERM (+ SIGKILL after 5s)
```

**Files written:**
- `~/.einai/queue.jsonl` — job queue (JSONL)
- `~/.einai/outputs/lenos/<stem>.md` — result for agent and ask jobs
- `~/.einai/outputs/claude-code/<stem>.md` — result for claude-code agent jobs
- `~/.einai/errors/lenos/<stem>.jsonl` — error logs for lenos runs

**Note:** `--save` works in async mode too — the result is saved to flicknote after the job completes.

**Completion callback:** When `TTAL_AGENT_NAME` is set (automatically in all agent sessions), the job sends a completion notification via `ttal send --to`. Worker sessions also have `TTAL_JOB_ID` set, enabling precise routing to the originating worker pane.

On success: `✅ <agent> finished (job N). Read: ei job log N`
On failure: `❌ <agent> failed (exit N) (job N). Read: ei job log N`
On kill: `🛑 <agent> killed (job N). Read: ei job log N`

**Job queue config** (`~/.config/einai/config.toml`):
```toml
[jobqueue]
max_parallel = 4   # max concurrent jobs (default: 4)
```

```bash
ei ask "research X" --async
# Queued. You'll be notified here when it completes.

# Monitor with:
ei job list
ei job log <id>
ei job kill <id>
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
| `--runtime` | Runtime: `lenos` or `claude-code` (default: config or `lenos`) |
| `--env` | Extra env vars for the sandbox (KEY=VALUE, can repeat) |

Examples:
```bash
ei agent run coder "implement auth"
echo "implement X" | ei agent run coder
ei agent run coder --runtime lenos "implement auth"
ei agent run coder --env OPENAI_KEY=xxx --env DEBUG=true
```

### Daemon
```bash
ei daemon run      # start daemon in foreground
ei daemon status   # check daemon health
```

## Architecture

The daemon listens on a unix socket at `~/.einai/daemon.sock`. CLI commands send requests to the daemon, which dispatches to the appropriate runtime and returns a blocking JSON response.

**Endpoints:**
- `POST /ask` — blocking JSON `AskResponse{result, duration_ms, error}`
- `POST /agent/run` — blocking JSON `AgentResponse{result, duration_ms, error}`
- `GET /health` — liveness check

## Configuration

Config is read from `~/.config/einai/config.toml`.

```toml
agents_paths = ["~/.einai/agents"]  # directories to discover agents
```

## Development

```bash
make build    # build the binary
make test     # run tests
make ci       # run full CI checks
make install  # install to GOPATH/bin
```

## License

MIT
