# Einai Quick Reference

Einai (`ei`) is the native agent runtime dispatcher for ttal.

## Commands

```bash
# Ask a question
ei ask "how does routing work?" --project myapp
ei ask "what is this?" --url https://docs.example.com
ei ask "summarize this project" --save   # save answer to flicknote

# Research the web
ei fetch "latest Go generics syntax?"
cat notes.md | ei fetch "check these claims"

# Run an agent
ei agent run coder "implement the auth module"
cat plan.md | ei agent run coder "implement this plan"

# List available agents
ei agent list

# Daemon management
ei daemon run     # start in foreground
ei daemon status  # check health
```

## Notes

- Use `ei ask` instead of `ttal ask`
- Use `ei fetch` for web research
- Use `ei agent run` for agent execution
- Prompt can be positional arg OR piped via stdin
- Agents are embedded in einai and synced with `ei agent sync`
- Daemon socket: `~/.einai/daemon.sock`
