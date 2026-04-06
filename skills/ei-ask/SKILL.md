---
name: ei-ask
description: Ask questions to einai agent runtime — project, repo, URL, web search, async, save
---

# ei-ask — Ask Questions to Einai

Einai (`ei`) is the native agent runtime for ttal. Use `ei ask` to ask questions with deep context from projects, repos, URLs, or the web.

## Basic Usage

```bash
ei ask "how does routing work?"
```

Ask about the current working directory with filesystem + web access (no flags needed).

## Flags

| Flag | Description |
|------|-------------|
| `--project <alias>` | Set agent's working directory to a ttal project. Use `ttal project list` to find aliases if unsure. |
| `--repo <org>/<name>` | Clone/study a GitHub repo. Format: `org/repo` (e.g. `tta-lab/ttal-cli`). |
| `--url <url>` | Fetch and study a web page. |
| `--web` | Search the web for the answer. |
| `--async` | Submit as background job (non-blocking). Results go to `~/.einai/outputs/` when done. ttal notifies you on completion. |
| `--save` | Save the answer to flicknote for later reference. |

## Examples

```bash
# Ask about a ttal project
ei ask "how does the auth middleware work?" --project myapp

# Ask about a GitHub repo
ei ask "what is the architecture?" --repo tta-lab/ttal-cli

# Ask about a web page
ei ask "summarize this API design" --url https://docs.example.com

# Web search
ei ask "latest Go generics syntax?" --web

# Save answer to flicknote
ei ask "architecture decision notes" --save

# Async background job
ei ask "analyze this codebase thoroughly" --project myapp --async
```

## Notes

- Prompt is the positional argument (quoted string).
- `--async` submits to pueue — the job runs in the background and ttal sends a notification when done.
- Output files: `~/.einai/outputs/<runtime>/` (`.md` results), `~/.einai/sessions/ei/` (`.jsonl` logs).
