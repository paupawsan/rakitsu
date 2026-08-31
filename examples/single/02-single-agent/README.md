# 02 — Single Agent with Tools

One agent equipped with file system and shell tools for codebase exploration.

## Run

```bash
rakitsu run examples/single/02-single-agent/config.yaml "List all Go files and summarize the project structure"
```

## What's Here

- `Explorer` agent with 4 tools: `list_files`, `read_file`, `search_files`, `run_command`
- CLI tool with command whitelist (security sandbox)
- File system tool with path restrictions

## Demonstrates

- Tool definitions (`type: fs`, `type: cli`)
- Tool security: `allowed_commands`, `allowed_paths`
- Agent-tool wiring via `tools: [...]`
- ReAct loop: agent reasons, picks a tool, observes result, repeats
