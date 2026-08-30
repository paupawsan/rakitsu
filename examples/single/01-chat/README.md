# 01 — Chat Assistant

Minimal config: one agent, no tools, no orchestration. Pure conversational LLM.

## Run

```bash
rakitsu run examples/single/01-chat/config.yaml "What is WebAssembly?"
```

## What's Here

- Single `Assistant` agent with a system prompt
- No tools, no orchestration — just a chat completion
- Good starting point for understanding the config format

## Demonstrates

- Basic YAML structure (`settings`, `agents`)
- Provider configuration with env var interpolation (`${OPENAI_API_KEY}`)
- Default model and provider settings
