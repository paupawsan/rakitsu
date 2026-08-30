# 05 — Dev Team Pipeline

Plan, implement, verify, review — a 4-stage development pipeline.

## Run

```bash
rakitsu run examples/single/05-dev-team/config.yaml "Add a health check endpoint to the API"
```

## What's Here

- 4 agents: `Planner` -> `Developer` -> `Verifier` -> `Reviewer`
- Tools: `read-file`, `write-file`, `run-command`
- Sequential pipeline where each stage builds on the previous

## Demonstrates

- Sequential pipeline for software development workflows
- Agent role specialization (planning vs implementation vs QA)
- Tool reuse across agents
