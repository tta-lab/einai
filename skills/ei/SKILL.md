---
name: ei
description: Einai agent runtime CLI — ask, agent run, agent list, daemon
category: tool
---

# ei — Einai Agent Runtime

## Ask

```bash
ei ask "question" --project <alias>      # ask about a project
ei ask "question" --repo org/repo        # ask about a GitHub repo
ei ask "question" --url https://...      # ask about a web page
ei ask "question" --web                  # search the web
ei ask "question"                        # ask with CWD context
```

## Agent Run

```bash
ei agent run <name> "prompt"             # run agent with positional prompt
echo "prompt" | ei agent run <name>      # run agent with stdin prompt
ei agent run <name> "prompt" --project <alias>
ei agent run <name> "prompt" --repo org/repo
```

## Agent List

```bash
ei agent list    # list all available agents
```

## Daemon

```bash
ei daemon run     # start daemon (blocks)
ei daemon status  # health check
```

## Sandbox

```bash
ei sandbox sync            # sync CC settings.json from sandbox.toml
ei sandbox sync --dry-run  # preview without writing
```
