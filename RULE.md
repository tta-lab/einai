# Einai Quick Reference

Einai (`ei`) is the native agent runtime dispatcher for ttal.

## Commands

```bash
# Ask a question
ei ask "how does routing work?" --project myapp
ei ask "what is this?" --url https://docs.example.com
ei ask "latest Go generics syntax?" --web
ei ask "summarize this project" --save   # save answer to flicknote

# Run an agent
ei agent run coder "implement the auth module"
ei agent run coder "$(cat plan.md)"   # pipe from stdin

# List available agents
ei agent list

# Daemon management
ei daemon run     # start in foreground
ei daemon status  # check health

```

## Notes

- Use `ei ask` instead of `ttal ask`
- Use `ei agent run` for agent execution
- Prompt can be positional arg OR piped via stdin
- Agents are discovered from `agents_paths` in `~/.config/einai/config.toml`
- Daemon socket: `~/.einai/daemon.sock`

## Async

`ei ask --async` and `ei agent run --async` submit jobs to the ei job queue for background execution. ttal send notification on completion. Monitor with `ei job list`, view output with `ei job log <id>`, and kill with `ei job kill <id>`.

**Files:**
- `~/.einai/queue.jsonl` — job queue (JSONL)
- `~/.einai/outputs/lenos/` — results for lenos runs (`.md`)
- `~/.einai/outputs/claude-code/` — results for claude-code runs (`.md`)
