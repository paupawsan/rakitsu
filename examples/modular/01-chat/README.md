# 01 — Chat Assistant (Modular)

Same as [single/01-chat](../../single/01-chat/) but split into modular layout.

```bash
rakitsu run examples/modular/01-chat/config.yaml "What is WebAssembly?"
```

## Structure

```
config.yaml          # Settings and provider config
agents/              # Auto-discovered agent definitions
  assistant.md       # Agent with system prompt
```

Config defines settings; agents are auto-discovered from subdirectories.
