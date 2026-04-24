---
name: ei-agent
description: Run einai agents — ei agent run, ei agent list, async execution
category: tool
---

# ei-agent — Run Einai Agents

Use `ei agent run --async` to spawn a named agent with a task prompt. ttal notifies you when done.

## Usage

```bash
ei agent run <name> "task prompt" --async
```

Agents are discovered from `.md` files in `agents_paths` (`~/.config/einai/config.toml`).

## Monitor

```bash
ei job list          # list jobs
ei job log <id>     # print output
ei job kill <id>     # SIGTERM (+ SIGKILL after 5s)
```

## Stdin Piping

```bash
# Pipe a plan from a file
cat plan.md | ei agent run coder --async

# Pipe and add extra context
cat plan.md | ei agent run coder "implement this plan" --async
```

## List Available Agents

```bash
ei agent list
```

## Daemon

```bash
ei daemon restart  # restart via launchd (recommended)
ei daemon status  # health check
```

## Notes

- `--async` is the default — always use it for non-blocking execution.
- Prompt can be a positional argument (quoted string) OR piped via stdin.
- Daemon socket: `~/.einai/daemon.sock`
