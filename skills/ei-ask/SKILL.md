---
name: ei-ask
description: Ask questions to einai agent runtime
category: tool
---

# ei-ask — Ask Questions to Einai

Use `ei ask` for local, project, repo, or URL-scoped questions. Use `ei fetch` for web research.

## Usage

```bash
ei ask "question"
ei ask "question" --repo org/name
ei fetch "web research question"
```

Results are saved under `~/.einai/outputs/lenos/` when the lenos runtime is used.

## Examples

```bash
# Ask about current directory
ei ask "how does routing work?"

# With project context
ei ask "what is this architecture?" --project myapp

# About a GitHub repo
ei ask "what is the architecture?" --repo tta-lab/ttal-cli

# About a URL
ei ask "what is this page about?" --url https://docs.example.com

# Web research
ei fetch "latest Go generics syntax?"
```

## Notes

- Prompt can be a positional argument (quoted string) OR piped via stdin.
- `ei fetch` also accepts stdin for extra context.
- Daemon socket: `~/.einai/daemon.sock`
