# 07 — Nested Orchestrators

3-level hierarchy: root pipeline delegates to backend and frontend sub-pipelines.

## Run

```bash
rakitsu run examples/single/07-nested-orchestrators/config.yaml "Build a user profile page with API and frontend"
```

## What's Here

- 7 specialist agents across backend and frontend
- 2 sub-orchestrators: `BackendPipeline`, `FrontendPipeline`
- Root `ProjectLead` orchestrator coordinates sub-pipelines + integration
- Tools: `read_file`, `write_file`, `run_command`

## Demonstrates

- Nested orchestrators (orchestrators containing other orchestrators)
- 3-level hierarchy: root -> sub-pipeline -> agents
- Complex project decomposition into parallel workstreams
