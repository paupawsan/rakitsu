# 04 — Content Publishing Pipeline (Modular)

Same as [single/04-pipeline](../../single/04-pipeline/) but split into modular layout.

```bash
rakitsu run examples/modular/04-pipeline/config.yaml "Write a blog post about WebAssembly"
```

## Structure

```
config.yaml          # Settings, provider, orchestrator with pipeline steps
agents/              # Auto-discovered agents
  researcher.md
  drafter.md
  editor.md
  translator-es.md
  translator-jp.md
  translator-fr.md
tools/
  write_file.yaml
```
