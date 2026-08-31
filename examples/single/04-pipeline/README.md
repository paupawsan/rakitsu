# 04 — Content Publishing Pipeline

Sequential pipeline with parallel translation and final synthesis.

## Run

```bash
rakitsu run examples/single/04-pipeline/config.yaml "Write a blog post about WebAssembly"
```

## What's Here

- 6 agents: `Researcher` -> `Drafter` -> `Editor` -> 3 translators (ES/JP/FR) in parallel -> synthesis
- `Publisher` orchestrator with `strategy: Pipeline`
- No tools — every agent works purely on the context the pipeline passes it; the final synthesis step returns the result directly

## Demonstrates

- Pipeline orchestration with sequential steps
- Parallel step execution (3 translators run simultaneously)
- Synthesis step (combines parallel outputs)
- How pipeline passes context between steps
