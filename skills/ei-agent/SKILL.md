---
name: ei-agent
description: Run einai agents — ei agent run, ei agent list, async execution, stdin piping
---

# ei-agent — Run Einai Agents

Einai (`ei`) is the native agent runtime for ttal. Use `ei agent run` to spawn a named agent with a task prompt. Agents are discovered from `agents_paths` in `~/.config/einai/config.toml`.

## Basic Usage

```bash
ei agent run <name> "implement the auth module"
```

## Flags

| Flag | Description |
|------|-------------|
| `--runtime <runtime>` | Agent backend: `ei-native` (built-in loop) or `claude-code` (spawns Claude Code). Defaults to config's `default_runtime`. |
| `--async` | Submit as background job (non-blocking). ttal notifies you on completion. |
| `--env KEY=VALUE` | Extra environment variables to pass to the agent. Repeat for multiple vars. |

## Stdin Piping

```bash
# Pipe a plan from a file
cat plan.md | ei agent run coder

# Pipe and override with additional args
cat plan.md | ei agent run coder "implement this plan"
```

## List Available Agents

```bash
ei agent list
```

Shows all agents with their names, roles, and runtimes. Agents are defined in the `agents/` directory and discovered at runtime.

## Examples

```bash
# Run a named agent with a prompt
ei agent run coder "implement the auth module"

# Async background execution
ei agent run coder "implement the auth module" --async

# Use a specific runtime
ei agent run coder "implement the auth module" --runtime claude-code

# Pipe a plan for the agent to execute
cat plan.md | ei agent run coder

# Combined: pipe plan and add extra context
cat plan.md | ei agent run coder "implement this plan"
```

## Daemon Management

```bash
ei daemon run     # start daemon in foreground (blocks)
ei daemon status  # health check
```

## Notes

- Prompt can be a positional argument (quoted string) OR piped via stdin.
- Async jobs submit to pueue for background execution. Results go to `~/.einai/outputs/<runtime>/`.
- Daemon socket: `~/.einai/daemon.sock`
- Session logs: `~/.einai/sessions/ei/` (ei-native), `~/.einai/sessions/cc/` (claude-code).
