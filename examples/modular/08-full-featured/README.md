# 08 — Full Featured (Modular)

Same as [single/08-full-featured](../../single/08-full-featured/) but split into modular layout.

```bash
rakitsu run examples/modular/08-full-featured/config.yaml "Analyze market trends for AI infrastructure"
```

## Structure

```
config.yaml          # Settings, providers, budgets, context management
agents/              # 4 agents with full config (reflection, vision, etc.)
tools/               # 5 tools including Docker sandbox
skills/              # Reusable prompt templates
```

See [single/08-full-featured](../../single/08-full-featured/) for the full feature list.
