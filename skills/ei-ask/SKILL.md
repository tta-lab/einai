---
name: ei-ask
description: Ask questions to einai agent runtime — async background queries
category: tool
---

# ei-ask — Ask Questions to Einai

Use `ei ask --async` for background queries. ttal notifies you when the answer is ready.

## Usage

```bash
ei ask "question" --async
ei ask "question" --repo org/name --async
```

Results saved to `~/.einai/outputs/ask/<stem>.md`.

## Monitor

```bash
ei job list          # list jobs
ei job log <id>      # print output
ei job kill <id>     # SIGTERM (+ SIGKILL after 5s)
```

## Examples

```bash
# Ask about current directory
ei ask "how does routing work?" --async

# With project context
ei ask "what is this architecture?" --project myapp --async

# About a GitHub repo
ei ask "what is the architecture?" --repo tta-lab/ttal-cli --async

# Web search
ei ask "latest Go generics syntax?" --web --async
```

## Notes

- Use `--async` for non-blocking execution; the CLI returns immediately.
- Prompt is the positional argument (quoted string).
- On completion: `✅ ask finished (job N). Read: ei job log N`
- Output files: `~/.einai/outputs/ask/<stem>.md`
