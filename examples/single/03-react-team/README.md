# 03 — ReAct Team (Incident Triage)

Supervisor dynamically delegates to specialist agents using the ReAct strategy.

## Run

```bash
rakitsu run examples/single/03-react-team/config.yaml "API latency spiked to 5s, investigate root cause"
```

## What's Here

- 3 specialist workers: `LogAnalyzer`, `InfraEngineer`, `APISpecialist`
- `IncidentLead` orchestrator with `strategy: ReAct`
- Tools: `read_file`, `search_logs`, `run_command`
- Skills for structured output

## Demonstrates

- Multi-agent orchestration with dynamic delegation
- ReAct strategy: supervisor reasons about which agent to call next
- Agent specialization via system prompts
- Skills (reusable prompt templates)
