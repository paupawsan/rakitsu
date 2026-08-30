# Rakitsu Web Frontend

Vue 3 + TypeScript + Vite frontend for Rakitsu. Provides a visual agent builder, real-time execution inspector, and interactive debugger — all embedded into the Go binary at build time.

## Quick Start

```bash
cd web
npm install
npm run dev          # Dev server at http://localhost:5173
```

Production build (called by `make build-embedded`):
```bash
npm run build        # Output to web/dist/, embedded via go:embed
```

## Architecture

```
web/src/
├── components/
│   ├── builder/           # Visual Builder — drag-and-drop agent flow design
│   │   ├── VisualBuilder.vue    # Vue Flow canvas + toolbar
│   │   ├── NodeEditor.vue       # Edit agent/tool/skill properties
│   │   └── SettingsPanel.vue    # Global settings editor (API keys, defaults)
│   ├── nodes/             # Vue Flow node components
│   │   ├── AgentNode.vue        # Agent node with provider/model badges
│   │   ├── ToolNode.vue         # Tool node (CLI/FS type)
│   │   ├── SkillNode.vue        # Skill node
│   │   └── OrchestratorNode.vue # Orchestrator with strategy badge
│   ├── inspector/         # Run Inspector — real-time event stream
│   │   ├── RunInspector.vue     # Event list with type filters
│   │   └── EventItem.vue        # Renders each of 24 event types
│   └── debugger/          # Debugger — breakpoints, stepping, param overrides
│       ├── DebugView.vue        # Main debug layout
│       ├── HierarchyTree.vue    # Agent hierarchy sidebar
│       ├── ExecutionGraph.vue   # Live execution flow graph
│       ├── DetailPanel.vue      # Selected node details + param editor
│       └── debug-nodes/         # Debug-specific node renderers
├── composables/           # Composition API hooks
│   ├── useEventStream.ts        # SSE connection + event parsing
│   ├── useAgentRun.ts           # Agent execution lifecycle
│   ├── useDebugControl.ts       # Debug pause/resume/step/breakpoints
│   ├── useDebugTree.ts          # Build debug hierarchy from events
│   ├── useModularConfig.ts      # Modular config import/export
│   ├── useSessionHistory.ts     # Session list + replay
│   └── useYamlExport.ts         # Canvas → YAML serialization
├── types/                 # TypeScript types mirroring Go structs
│   ├── config.ts                # Config, AgentConfig, ToolConfig, etc.
│   └── events.ts                # EventType union, payload interfaces
└── App.vue                # Tab layout: Builder | Inspector | Debugger
```

## Key Features

- **Visual Builder**: Design agent flows on a Vue Flow canvas. Add agents, tools, skills, orchestrator. Configure via node editor. Export as YAML.
- **Run Inspector**: Stream 24 typed SSE events in real-time. Filter by event type. View thoughts, tool calls, reflections, ground-checks, token usage.
- **Debugger**: Attach to running agents via hub. Set breakpoints (checkpoint, agent, tool, iteration). Step in/out of delegations. Override model parameters mid-run.
- **Session History**: Browse recorded sessions. Replay with full event reconstruction.

## Settings Panel

The Settings panel (gear icon in Visual Builder) edits global `settings:` fields that are written into the exported YAML. Sections:

| Section | What it configures |
|---------|-------------------|
| **API Keys** | `settings.api_keys` — API key per provider (`openai`, `anthropic`, `gemini`). Use `${ENV_VAR}` syntax. |
| **Providers** | `settings.providers` — Named provider instances (LiteLLM, Ollama, vLLM). `type` + `api_key` + `base_url`. |
| **Defaults** | `settings.defaults` — Default model, temperature, max_tokens applied to all agents. |
| **Execution** | `settings.execution` — Max iterations, timeout, retry attempts, token/cost budgets. |
| **Logging** | `settings.logging` — Log level (`debug`/`info`/`warn`/`error`) and optional log file path. |
| **Hub Connection** | `settings.hub_url` — SSE hub URL for `rakitsu run` to stream events to. Equivalent to `--hub` CLI flag; CLI flag wins when both are set. |

> **Note:** Server bind host and port (`rakitsu serve --host`, `rakitsu serve --port`, `rakitsu ui --port`) are CLI-only flags and cannot be configured via YAML.

## Hub vs UI vs Debug Port

Three ways to monitor a run — choose based on your workflow:

| Command | When to use |
|---------|------------|
| `rakitsu serve` + `rakitsu run` | Multi-run monitoring. Hub at `:9100` shows all active runs as cards. Click to inspect any run live. |
| `rakitsu ui` | Full Visual Builder + Inspector + Debugger in one process. Suitable for single-machine development. |
| `rakitsu run --debug-port 9100` | Standalone debug server for one run. Attach the web UI debugger to it via "Attach Debugger". |

The `--hub` flag (or `settings.hub_url` in YAML) tells `rakitsu run` where to push events. `rakitsu serve` is the hub receiver.

## Types

TypeScript types in `web/src/types/` mirror Go structs from `internal/config/config.go` and `internal/telemetry/events.go`. These are manually synchronized — both compilers validate their own side, but the JSON serialization boundary requires manual alignment. When adding new fields to Go structs, update the corresponding TS interfaces in `web/src/types/config.ts` or `events.ts`.

Full config schema: [docs/configuration.md](../docs/configuration.md)

## Dev Notes

- Vue 3 `<script setup lang="ts">` with Composition API
- Scoped CSS per component
- Vue Flow for canvas rendering with custom node components
- SSE via native `EventSource` API
- Built output is embedded into Go binary via `internal/webui/embed.go`
