# 08 — Full Featured

Kitchen sink config demonstrating every Rakitsu feature in one file.

## Run

```bash
rakitsu run examples/single/08-full-featured/config.yaml "Analyze the market trends for AI infrastructure"
```

## What's Here

- 4 agents: `Researcher`, `VisionAnalyst`, `Writer`, `QAChecker`
- Pipeline with synthesis step
- Provider fallback chain (openai -> anthropic)
- Full security sandbox with Docker

## Demonstrates

- Provider fallback chains
- Reflection modes (`after_tool`, `always`)
- Ground check for fact verification
- Token and cost budget limits
- Context management with auto-compression
- Vision-capable agents
- Skills and inline tools
- Docker sandbox for tool execution
- Model config overrides (`temperature`, `top_p`)
- Pricing configuration
