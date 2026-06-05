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
Ask a question with access to the current directory, registered projects, repos, or a URL.

|  Flag | Description |
|------|-------------|
| `--project` | Ask about a registered ttal project |
| `--repo` | Ask about a GitHub/Forgejo repo (auto-clones) |
| `--url` | Ask about a web page (fetches content) |
| `--save` | Save the final answer to flicknote |

Examples:
```bash
ei ask "how does the auth middleware work?"
ei ask "how does routing work?" --project myapp
ei ask "explain the pipeline syntax" --repo woodpecker-ci/woodpecker
ei ask "what auth methods?" --url https://docs.example.com
ei ask "summarize this project" --save
```

### Fetch

```bash
ei fetch [prompt]
```

Research the web with the embedded `webdiver` agent. The prompt can be a positional argument, piped via stdin, or both.

Examples:
```bash
ei fetch "latest Go generics syntax?"
cat notes.md | ei fetch "check these claims against current docs"
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
default_runtime = "lenos"
model = "deepseek/deepseek-v4-flash"
max_run_timeout = 1200
references_path = "~/.einai/references"
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
