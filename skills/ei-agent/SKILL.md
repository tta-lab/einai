---
name: ei-agent
description: Run einai agents — ei agent run, ei agent list
category: tool
---

# ei-agent — Run Einai Agents

Use `ei agent run` to run a named agent with a task prompt. The command blocks until the agent finishes.

## Usage

```bash
ei agent run <name> "task prompt"
```

Agents are embedded in einai. Use `ei agent list` to see available agents.

## Stdin Piping

```bash
# Pipe a plan from a file
cat plan.md | ei agent run coder

# Pipe and add extra context
cat plan.md | ei agent run coder "implement this plan"
```

## List Available Agents

```bash
ei agent list
```

## Daemon

```bash
ei daemon restart  # restart via launchd (recommended)
ei daemon status   # health check
```

## Notes

- Prompt can be a positional argument (quoted string) OR piped via stdin.
- Output files: `~/.einai/outputs/<runtime>/<stem>.md` (`claude-code` or `lenos`)
- Daemon socket: `~/.einai/daemon.sock`
