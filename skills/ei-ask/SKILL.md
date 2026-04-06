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
```

Results saved to `~/.einai/outputs/ei/` (`.md`). Session logs at `~/.einai/sessions/ei/`.

## Examples

```bash
# Ask about current directory
ei ask "how does routing work?" --async

# With project context
ei ask "what is this architecture?" --project myapp --async

# Web search
ei ask "latest Go generics syntax?" --web --async
```

## Notes

- `--async` is the default — always use it for non-blocking execution.
- Prompt is the positional argument (quoted string).
- Output files: `~/.einai/outputs/<runtime>/` (`.md` results), `~/.einai/sessions/ei/` (`.jsonl` logs).
