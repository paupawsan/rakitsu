# 05 — Dev Team Pipeline (Modular)

Same as [single/05-dev-team](../../single/05-dev-team/) but split into modular layout.

```bash
rakitsu run examples/modular/05-dev-team/config.yaml "Add a health check endpoint"
```

## Structure

```
config.yaml          # Settings, provider, orchestrator
agents/              # Auto-discovered agents
  planner.md
  developer.md
  verifier.md
  reviewer.md
tools/
  read_file.yaml
  write_file.yaml
  run_command.yaml
```
