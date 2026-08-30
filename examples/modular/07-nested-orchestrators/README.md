# 07 — Nested Orchestrators (Modular)

Same as [single/07-nested-orchestrators](../../single/07-nested-orchestrators/) but split into modular layout.

```bash
rakitsu run examples/modular/07-nested-orchestrators/config.yaml "Build a user profile page"
```

## Structure

```
config.yaml          # Root orchestrator
agents/              # All 7 agents
orchestrators/       # Sub-orchestrators (backend, frontend pipelines)
tools/
  read_file.yaml
  write_file.yaml
  run_command.yaml
```

Demonstrates the `orchestrators/` auto-discovery directory for nested orchestrator definitions.
