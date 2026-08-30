# 02 — Single Agent with Tools (Modular)

Same as [single/02-single-agent](../../single/02-single-agent/) but split into modular layout.

```bash
rakitsu run examples/modular/02-single-agent/config.yaml "List all Go files and summarize the project"
```

## Structure

```
config.yaml          # Settings and provider config
agents/              # Auto-discovered agents
  explorer.md
tools/               # Auto-discovered tool definitions
  list_files.yaml
  read_file.yaml
  search_files.yaml
  run_command.yaml
```
