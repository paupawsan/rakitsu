# 03 — ReAct Team (Modular)

Same as [single/03-react-team](../../single/03-react-team/) but split into modular layout.

```bash
rakitsu run examples/modular/03-react-team/config.yaml "API latency spiked to 5s, investigate"
```

## Structure

```
config.yaml          # Settings, provider, orchestrator
agents/              # Auto-discovered agents
  log-analyzer.md
  infra-engineer.md
  api-specialist.md
tools/               # Auto-discovered tools
  read_file.yaml
  search_logs.yaml
  run_command.yaml
skills/              # Auto-discovered skills
  incident_triage.yaml
```
