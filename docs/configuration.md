# Rakitsu Configuration Reference

Rakitsu uses YAML configuration files to define agents, tools, skills, orchestrators, and workflows.

## Table of Contents

- [File Format](#file-format)
- [File References](#file-references)
- [Directory Auto-Discovery](#directory-auto-discovery)
- [Root Config](#root-config)
- [Settings](#settings)
- [Tools](#tools)
- [Skills](#skills)
- [Agents](#agents)
- [Orchestrator](#orchestrator)
- [Workflows](#workflows)
- [Complete Example](#complete-example)

## File Format

Configuration files are YAML with environment variable expansion. Use `${VAR_NAME}` to reference environment variables and `${VAR:-default}` for defaults.

```yaml
settings:
  api_keys:
    openai: "${OPENAI_API_KEY}"
    gemini: "${GEMINI_API_KEY:-}"
```

## File References

Text fields (system prompts, prompt templates) can be loaded from external files instead of being inlined.

### Explicit `file:` Prefix

Prefix the value with `file:` to load from a file. Paths are relative to the config file's directory.

```yaml
agents:
  - name: "Code Reviewer"
    system_prompt: "file:prompts/code-reviewer.md"

skills:
  - name: "security_audit"
    prompt_template: "file:skills/security-audit.md"
```

If the file doesn't exist, an error is returned.

### Auto-Detection

Values ending in `.md`, `.txt`, or `.prompt` are automatically resolved as file references if:
1. The value is a single line (no newlines)
2. The file exists at the resolved path

```yaml
# Auto-detected — loads prompts/code-reviewer.md if it exists:
system_prompt: prompts/code-reviewer.md

# NOT auto-detected — multi-line content is always treated as inline:
system_prompt: |
  You are a senior code reviewer...
```

If the file doesn't exist, the value is kept as a literal string (no error).

### Supported Fields

| Field | Context |
|-------|---------|
| `system_prompt` | Agents, Orchestrator |
| `prompt_template` | Skills |
| `prompt` | ReflectionConfig, GroundCheckConfig, SynthesisConfig |

## Directory Auto-Discovery

Place definition files in conventional directories next to the config file to auto-discover them.

```
project/
├── config.yaml           # Main config
├── agents/               # Auto-discovered agent definitions
│   ├── code-reviewer.yaml
│   └── security-auditor.md
├── skills/               # Auto-discovered skill definitions
│   ├── code-review.yaml
│   └── security-audit.md
├── tools/                # Auto-discovered tool definitions
│   └── list-files.yaml
└── prompts/              # Referenced by file: or auto-detect
    └── reviewer-prompt.md
```

### Supported File Formats

**YAML files** (`.yaml`, `.yml`) — same structure as inline definitions:

```yaml
# agents/code-reviewer.yaml
name: "Code Reviewer"
role: "worker"
provider: "gemini"
model: "gemini-2.5-flash"
system_prompt: prompts/code-reviewer.md
tools:
  - read_file
  - search_files
settings:
  max_iterations: 8
```

**Markdown files** (`.md`) — YAML front matter + body as prompt:

```markdown
---
name: "Code Reviewer"
role: "worker"
provider: "gemini"
model: "gemini-2.5-flash"
tools:
  - read_file
  - search_files
settings:
  max_iterations: 8
---

You are a senior code reviewer. Your job is to read source code files
and identify bugs, logic errors, missing error handling, and code quality issues.
```

For agents, the markdown body becomes `system_prompt`. For skills, it becomes `prompt_template`.

### Precedence

Inline definitions in the main config file take precedence. If an agent named "Code Reviewer" is defined both inline and in `agents/code-reviewer.yaml`, the inline one is used.

## Root Config

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Project/system name |
| `project_id` | string | Optional project identifier (optional) |
| `version` | string | Configuration version |
| `description` | string | Description of the system |
| `interactive` | bool | Run as a chat TUI instead of a one-shot run (optional) |
| `interactive_overlay` | bool | Wrap the root runner in a chat-host meta-agent; defaults to `true` for non-conversational configs (optional) |
| `force_delegation` | bool | ChatHost must call `invoke_config` every turn — removes the "answer directly" path and discovery tools; use for weak models that would otherwise skip delegation (optional) |
| `settings` | Settings | Global settings |
| `tools` | ToolDefinition[] | Tool definitions |
| `skills` | SkillDefinition[] | Skill definitions |
| `agents` | AgentDefinition[] | Agent definitions |
| `orchestrator` | OrchestratorConfig | Multi-agent orchestrator (optional) |
| `orchestrators` | OrchestratorConfig[] | Multiple named orchestrators (optional) |
| `workflows` | WorkflowDefinition[] | Pre-configured workflows (optional) |

## Settings

```yaml
settings:
  default_provider: "openai"
  api_keys:
    openai: "${OPENAI_API_KEY}"
    anthropic: "${ANTHROPIC_API_KEY}"
    gemini: "${GEMINI_API_KEY}"
  base_urls:
    ollama: "http://localhost:11434/v1"
  credentials_files:
    gemini: "${GOOGLE_APPLICATION_CREDENTIALS}"
  allowed_commands:
    - "golangci-lint"
    - "terraform"
    - "jq"
  defaults:
    model: "gpt-4o-mini"
    temperature: 0.7
    max_tokens: 4096
  execution:
    max_iterations: 10
    timeout_seconds: 300
    retry_attempts: 3
  logging:
    level: "info"
    file: "rakitsu.log"
  server:
    host: "localhost"
    port: 8080
    websocket_path: "/ws"
```

### Settings Fields

| Field | Type | Description |
|-------|------|-------------|
| `default_provider` | string | Default LLM provider: `openai`, `anthropic`, `gemini`, `codex`, `ollama` |
| `api_keys` | map[string]string | Provider API keys (supports `${ENV_VAR}`) |
| `base_urls` | map[string]string | Custom endpoint URLs per provider |
| `credentials_files` | map[string]string | Service account JSON paths per provider |
| `locations` | map[string]string | Cloud region per provider (e.g., `gemini: "global"` for Vertex AI) |
| `projects` | map[string]string | Cloud project ID per provider (e.g., `gemini: "my-gcp-project"` for Vertex AI) |
| `providers` | map[string]ProviderDefinition | Named provider instances (see below) |
| `allowed_commands` | []string | User-defined commands allowed for CLI tools (extends the built-in allowlist). A denylist (`rm`, `sudo`, `kill`, …) blocks those exact names, but this is **not** a security boundary — allowed interpreters like `python3`/`bash` can still run arbitrary code. Only add commands you trust; see [SECURITY.md](SECURITY.md). |

### Named Providers

Named provider instances allow defining reusable provider configurations. Agents reference them by name via the `provider` field.

```yaml
settings:
  providers:
    litellm:
      type: "openai"
      api_key: "${LITELLM_API_KEY}"
      base_url: "https://my-proxy.example.com/v1"
    vertex:
      type: "gemini"
      credentials_file: "./keys/sa.json"
      location: "us-central1"
      project: "my-project"
```

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Provider type: `openai`, `anthropic`, `gemini`, `codex`, `ollama` |
| `api_key` | string | API key (supports `${ENV_VAR}`); not used by `codex` |
| `base_url` | string | Custom endpoint URL |
| `credentials_file` | string | Service account JSON path (Gemini) or `auth.json` path (`codex`, default `~/.codex/auth.json`) |
| `location` | string | Cloud region (Vertex AI) |
| `project` | string | Cloud project ID (Vertex AI) |

### Defaults

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `model` | string | `gpt-4o-mini` | Default model name |
| `temperature` | float | 0.7 | Sampling temperature (0.0-2.0) |
| `max_tokens` | int | 4096 | Maximum response tokens |

### Execution

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `max_iterations` | int | 10 | Max ReAct loop iterations |
| `timeout_seconds` | int | 300 | Execution timeout |
| `retry_attempts` | int | 3 | Retry count on failure |
| `max_total_tokens` | int | 0 | Hard token budget across all agents (0 = unlimited) |
| `max_cost` | float | 0 | Hard cost budget in USD across all agents (0 = unlimited) |

When a budget is exceeded, the run terminates with status `budget_exceeded`.

### Pricing

Per-model pricing overrides for cost tracking. Keys are model names, values specify cost per million tokens.

```yaml
settings:
  pricing:
    gpt-4o:
      input: 2.50
      output: 10.00
    gemini-2.5-flash:
      input: 0.15
      output: 0.60
```

| Field | Type | Description |
|-------|------|-------------|
| `input` | float | Cost per 1M input tokens (USD) |
| `output` | float | Cost per 1M output tokens (USD) |

### Logging

| Field | Type | Description |
|-------|------|-------------|
| `level` | string | Log level: `debug`, `info`, `warn`, `error` |
| `file` | string | Log file path |

### Server

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `host` | string | `localhost` | Server bind address |
| `port` | int | 8080 | Server port |
| `websocket_path` | string | `/ws` | WebSocket endpoint path |

## Tools

Tools are capabilities that agents can invoke during execution.

```yaml
tools:
  - name: "list_files"
    type: "cli"
    description: "List files and directories"
    command: "ls -la"
    sandbox:
      type: "local_restricted"
      allowed_paths: ["./"]
      resource_limits:
        timeout_sec: 10

  - name: "read_file"
    type: "fs"
    description: "Read file contents"
    operation: "read"
    allowed_paths: ["./"]
    parameters:
      path:
        type: "string"
        description: "Path to the file"
        required: true

  - name: "external_tools"
    type: "mcp_server"
    description: "Tools from a remote MCP server"
    url: "http://localhost:9200/mcp"     # http transport
    transport: "http"                    # "stdio" or "http"
    # args: ["--flag"]                   # stdio transport: subprocess args

  - name: "researcher"
    type: "a2a"
    description: "Delegate to the Researcher agent on another rakitsu process"
    url: "http://research-host:9100/a2a"
    agent: "Researcher"                  # remote agent name to delegate to
```

### ToolDefinition Fields

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | **Required.** Unique tool name |
| `type` | string | **Required.** Tool type: `cli`, `fs`, `mcp_server`, `a2a` |
| `description` | string | What the tool does (shown to LLM) |
| `command` | string | Shell command (for `cli` type) |
| `operation` | string | Operation name (for `fs` type): `read`, `write`, `search`, `list` |
| `method` | string | HTTP method (for future `http` type) |
| `executable` | string | Path to executable |
| `script` | string | Script content |
| `parameters` | map[string]Parameter | Parameter definitions |
| `timeout_seconds` | int | Execution timeout |
| `env` | map[string]string | Environment variables |
| `working_dir` | string | Working directory |
| `allowed_paths` | []string | Allowed filesystem paths. **Set this explicitly for `fs` tools** — the default (`["."]`) is the whole launch directory, which includes `.env`/`.git`/anything else sitting next to the config unless you fence it off. |
| `args` | []string | Subprocess args (for `mcp_server` type, stdio transport) |
| `url` | string | Server URL (for `mcp_server` type, http transport; also used by `a2a` type) |
| `transport` | string | `stdio` or `http` (for `mcp_server` type) |
| `agent` | string | Remote agent name to delegate to (for `a2a` type) |
| `allowed_exit_codes` | []int | Acceptable exit codes |
| `sandbox` | SandboxConfig | Security sandbox configuration |

### Parameter

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Parameter type: `string`, `number`, `boolean`, `array` |
| `description` | string | Parameter description (shown to LLM) |
| `required` | bool | Whether the parameter is required |
| `default` | any | Default value |
| `enum` | []string | Allowed values |

### SandboxConfig

> **Read [SECURITY.md](SECURITY.md) first.** The `local_restricted` command
> allowlist is a guard rail against mistakes, **not** a security boundary — it
> permits `python3`/`node`/`bash`/`find`, which can run arbitrary code, and
> `allowed_paths` only filters the paths that appear as command *arguments*.
> Only run configs you trust; use `type: docker` for untrusted work, and never
> expose `rakitsu serve` to the network without `RAKITSU_API_TOKEN`.

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Sandbox type: `local_restricted`, `docker` |
| `image` | string | Docker image (for `docker` type) |
| `mount_workdir` | bool | Mount working directory into container, read-only by default (see `mount_workdir_writable`) |
| `mount_workdir_writable` | bool | `docker`: mount the working directory read-write instead of read-only. Needed if the command must write into the workdir (e.g. a formatter that rewrites files in place); leave off for anything that only reads it. |
| `allowed_paths` | []string | `local_restricted`: the command runs in `allowed_paths[0]` (unless `working_dir` is set) and any argument token that resolves outside every listed directory (`../x`, absolute paths, symlinks out, `~/x` in shell payloads) is refused. An argument filter, not containment — see SECURITY.md. |
| `network_isolated` | bool | Deprecated alias for the default (no network). Kept for backward compatibility; use `allow_network` to opt back in. |
| `allow_network` | bool | `docker`: give the container network access. Default is none — most `cli` tools (linters, formatters, interpreters) don't need it, and a container that can't reach the network can't exfiltrate or phone home even if the command turns out hostile. |
| `user` | string | `docker`: `--user` the container runs as, `"uid:gid"`. Defaults to `65534:65534` (nobody:nogroup) — a container escape or compromised tool doesn't come out as root. |
| `resource_limits` | ResourceLimits | Resource constraints |

### ResourceLimits

| Field | Type | Description |
|-------|------|-------------|
| `cpu_limit` | string | CPU limit (e.g., `"0.5"`) |
| `memory_limit` | string | Memory limit (e.g., `"256m"`) |
| `timeout_sec` | int | Execution timeout in seconds |
| `max_output_bytes` | int | Caps captured stdout/stderr for `cli` tools, and file size for `fs` `read` operations. `0` applies a conservative default (8KB for `cli`, 10MB for `fs`); `-1` disables the cap. |
| `pids_limit` | int | `docker`: caps the container's process/thread count (`--pids-limit`), a floor against a fork bomb or runaway subprocess spawn. `0` applies a default of 128; `-1` disables the cap. |

## Skills

Skills are reusable prompt templates with associated tools.

```yaml
skills:
  - name: "code_review"
    description: "Perform a structured code review"
    tools:
      - "read_file"
      - "search_files"
    prompt_template: |
      Perform a thorough code review. Focus on:
      1. Bugs and logic errors
      2. Error handling gaps
      3. Security vulnerabilities
```

### SkillDefinition Fields

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | **Required.** Unique skill name |
| `description` | string | What the skill does |
| `tools` | []string | Tool names available to this skill |
| `prompt_template` | string | Prompt template (supports [file references](#file-references)) |

## Agents

Agents are LLM-powered workers that execute tasks using the ReAct loop.

```yaml
agents:
  - name: "Code Reviewer"
    role: "worker"
    provider: "gemini"
    model: "gemini-2.5-flash"
    system_prompt: |
      You are a senior code reviewer...
    tools:
      - "read_file"
      - "search_files"
    skills:
      - "code_review"
    settings:
      max_iterations: 8
      reflection:
        enabled: true
        mode: "after_tool"
        frequency: "on_error"
      ground_check:
        enabled: true
        confidence_threshold: 0.7
```

### AgentDefinition Fields

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | **Required.** Unique agent name |
| `role` | string | Agent role: `worker`, `supervisor` |
| `provider` | string | LLM provider (overrides `settings.default_provider`) |
| `model` | string | Model name (overrides `settings.defaults.model`) |
| `model_config` | ModelConfig | Per-agent model parameters |
| `system_prompt` | string | System prompt (supports [file references](#file-references)) |
| `tools` | []string | Tool names this agent can use |
| `skills` | []string | Skill names this agent can use |
| `tools_inline` | ToolDefinition[] | Agent-specific inline tool definitions |
| `settings` | AgentSettings | Agent behavior settings |

### ModelConfig

Per-agent model parameter overrides.

| Field | Type | Description |
|-------|------|-------------|
| `temperature` | float | Sampling temperature (0.0-2.0) |
| `max_tokens` | int | Maximum response tokens |
| `top_p` | float | Nucleus sampling parameter |
| `frequency_penalty` | float | Frequency penalty (-2.0 to 2.0) |
| `presence_penalty` | float | Presence penalty (-2.0 to 2.0) |
| `timeout_sec` | int | Per-request LLM timeout in seconds |
| `no_stream_tools` | bool | Disable streaming when tools are present. Required for some providers (vLLM, Qwen via LiteLLM) that don't support streaming with tool definitions. |

### AgentSettings

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `max_iterations` | int | 10 | Max ReAct loop iterations |
| `max_total_tokens` | int | 0 | Per-agent token budget (0 = unlimited). Overrides global `execution.max_total_tokens` for this agent. |
| `max_cost` | float | 0 | Per-agent cost budget in USD (0 = unlimited). Overrides global `execution.max_cost` for this agent. |
| `verbose` | bool | false | Enable verbose output |
| `timeout` | int | 300 | Agent timeout in seconds |
| `reflection` | ReflectionConfig | disabled | Self-reflection configuration |
| `ground_check` | GroundCheckConfig | disabled | Ground-check validation |
| `context` | ContextConfig | `strategy: "full"` | Context management strategy |

### ContextConfig

Controls how the agent manages conversation context across iterations. Without context management, the full message history grows unboundedly and may exceed provider token limits on long runs.

`context` is a field of `AgentSettings` (per-agent), not of the top-level `settings` block:

```yaml
agents:
  - name: "MyAgent"
    settings:
      context:
        strategy: "step_log"
        keep_recent: 3
        max_tool_output: 8000
        fence_outputs: true
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `strategy` | string | `full` | Context strategy: `full`, `sliding_window`, `step_log` |
| `window_size` | int | 0 | Number of recent turns to keep (for `sliding_window`). Snaps to turn boundaries. |
| `keep_recent` | int | 3 | Number of recent steps to include verbatim (for `step_log`). Older steps get one-line summaries. |
| `max_tool_output` | int | 0 | Max characters per tool output (0 = unlimited). Longer outputs are truncated. |
| `fence_outputs` | bool | false | Wrap tool outputs in `<tool_output>` delimiters. Helps the LLM distinguish data from instructions. |

**Strategies:**

| Strategy | Behavior | When to use |
|----------|----------|-------------|
| `full` | Keep entire history (current default). Optional truncation + fencing. | Short runs (< 5 iterations), small tool outputs |
| `sliding_window` | Keep last N turns, discard older ones. | Medium runs where only recent context matters |
| `step_log` | Query + one-line summaries of old steps + last N steps verbatim. Summaries are programmatic (not LLM-generated). | Long-running code agents (10+ iterations, large tool outputs) |

**Security:** When `fence_outputs: true`, tool outputs are wrapped in delimiters and the system prompt instructs the LLM to treat fenced content as untrusted data. This reduces prompt injection risk from tool outputs (e.g., a file containing "ignore all previous instructions").

### ReflectionConfig

Controls agent self-reflection — the agent reviews its own reasoning and tool outputs.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | false | Enable reflection |
| `mode` | string | `after_tool` | When to reflect: `after_tool`, `before_answer`, `both` |
| `frequency` | string | `always` | How often: `always`, `on_error`, `every_n` |
| `every_n` | int | 1 | Reflect every N iterations (when `frequency: "every_n"`) |
| `prompt` | string | (built-in) | Custom reflection prompt (supports [file references](#file-references)) |

**Recommended settings by role:**
- **Worker agents**: `frequency: "on_error"` — skip reflection when tools succeed
- **Orchestrator**: `mode: "before_answer"` — reflect before synthesizing final report
- **High-stakes agents**: `frequency: "always"`, `mode: "both"` — maximum quality

### GroundCheckConfig

Validates the agent's final answer for factual accuracy and grounding.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | false | Enable ground-check |
| `confidence_threshold` | float | 0.7 | Minimum confidence score (0.0-1.0) |
| `prompt` | string | (built-in) | Custom validation prompt (supports [file references](#file-references)) |
| `max_retries` | int | 1 | Retry count on low confidence |

**Note:** Ground-check adds an extra LLM call per agent completion. For multi-agent setups, consider enabling it only on the orchestrator or critical agents.

## Orchestrator

The orchestrator coordinates multiple agents using delegation tools.

```yaml
orchestrator:
  name: "Tech Lead"
  strategy: "ReAct"
  provider: "gemini"
  model: "gemini-2.5-flash"
  model_config:
    temperature: 0.3
  system_prompt: |
    You are the Tech Lead coordinating a team...
  agents:
    - "Code Reviewer"
    - "Security Auditor"
    - "DevOps Analyst"
  routing:
    auto_delegate_tools: true
  handoff:
    include_context: true
    max_context_length: 4000
    allow_cross_agent_calls: true
```

### OrchestratorConfig Fields

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | **Required.** Orchestrator name |
| `strategy` | string | Strategy: `ReAct`, `PlanAndExecute`, `Hierarchical`, `Pipeline` |
| `provider` | string | LLM provider |
| `model` | string | Model name |
| `model_config` | ModelConfig | Model parameter overrides |
| `system_prompt` | string | System prompt (supports [file references](#file-references)) |
| `agents` | []string | Names of worker agents to coordinate |
| `routing` | RoutingConfig | Task routing configuration |
| `handoff` | HandoffConfig | Agent handoff behavior |
| `pipeline` | PipelineConfig | Deterministic pipeline (when strategy is `Pipeline`) |

### RoutingConfig

| Field | Type | Description |
|-------|------|-------------|
| `auto_delegate_tools` | bool | Auto-create delegation tools for each agent |
| `rules` | RoutingRule[] | Conditional routing rules |

### RoutingRule

| Field | Type | Description |
|-------|------|-------------|
| `condition` | string | Routing condition expression |
| `delegate_to` | string | Target agent name |

### HandoffConfig

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `include_context` | bool | false | Include conversation context in handoff |
| `max_context_length` | int | 4000 | Maximum context size (characters) |
| `allow_cross_agent_calls` | bool | false | Allow agents to call other agents |

### PipelineConfig

When `strategy: "Pipeline"`, the orchestrator runs agents in a deterministic sequence instead of using LLM-driven delegation. Each step's output feeds into the next step as context.

```yaml
orchestrator:
  strategy: "Pipeline"
  agents: ["Planner", "Developer", "Verifier", "Reviewer", "Security"]
  pipeline:
    steps:
      - name: "plan"
        agent: "Planner"

      - name: "implement"
        agent: "Developer"

      - name: "verify"
        agent: "Verifier"

      - name: "review"
        type: "parallel"
        steps:
          - name: "code_review"
            agent: "Reviewer"
          - name: "security_check"
            agent: "Security"
```

### PipelineStep

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | **Required.** Step name (used in events and logging) |
| `type` | string | Step type: `sequential` (default), `parallel`, `loop` |
| `agent` | string | Agent to execute this step (for leaf steps) |
| `task` | string | Optional task override (defaults to the CLI query + previous step output) |
| `steps` | PipelineStep[] | Nested sub-steps (for `parallel` and `loop` types) |
| `max_iterations` | int | Maximum loop iterations (for `loop` type) |
| `condition_agent` | string | Agent that evaluates the loop condition (for `loop` type) |
| `condition_prompt` | string | Prompt for the condition agent (for `loop` type) |

**Step types:**

| Type | Behavior |
|------|----------|
| `sequential` | Run steps one after another. Each step receives the previous step's output. Default when `type` is omitted. |
| `parallel` | Run all nested `steps` concurrently. Results are merged for the next step. |
| `loop` | Repeat nested steps until `condition_agent` returns "done" or `max_iterations` is reached. |

## Workflows

Workflows define pre-configured sequences of agent tasks.

```yaml
workflows:
  - name: "full_review"
    description: "Complete code review pipeline"
    steps:
      - agent: "Code Reviewer"
        task: "Review code for bugs and quality issues"
      - agent: "Security Auditor"
        task: "Audit for security vulnerabilities"
        depends_on: ["Code Reviewer"]
      - agent: "DevOps Analyst"
        task: "Check deployment and infrastructure"
    final_synthesis:
      agent: "Tech Lead"
      prompt: "Synthesize all findings into a prioritized report"
```

### WorkflowDefinition Fields

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Workflow name |
| `description` | string | Workflow description |
| `steps` | WorkflowStep[] | Ordered execution steps |
| `final_synthesis` | SynthesisConfig | Final synthesis step (optional) |

### WorkflowStep

| Field | Type | Description |
|-------|------|-------------|
| `agent` | string | Agent name to execute this step |
| `task` | string | Task description |
| `depends_on` | []string | Agent names this step depends on |
| `condition` | string | Condition expression for execution |

### SynthesisConfig

| Field | Type | Description |
|-------|------|-------------|
| `agent` | string | Agent to perform synthesis |
| `prompt` | string | Synthesis prompt (supports [file references](#file-references)) |

## Complete Example

A multi-agent setup using file references and all major features:

```yaml
name: "DevOps Review Team"
version: "1.0.0"
description: "Multi-agent code review system"

settings:
  default_provider: "gemini"
  api_keys:
    gemini: "${GEMINI_API_KEY}"
  defaults:
    model: "gemini-2.5-flash"
    temperature: 0.5
    max_tokens: 4096

tools:
  - name: "read_file"
    type: "fs"
    description: "Read file contents"
    operation: "read"
    allowed_paths: ["./"]
    parameters:
      path:
        type: "string"
        description: "Path to the file"
        required: true

skills:
  - name: "code_review"
    description: "Structured code review"
    tools: ["read_file"]
    # Load prompt from file:
    prompt_template: "file:skills/code-review.md"

agents:
  - name: "Code Reviewer"
    role: "worker"
    # Load system prompt from markdown file:
    system_prompt: prompts/code-reviewer.md
    tools: ["read_file"]
    skills: ["code_review"]
    settings:
      max_iterations: 8
      reflection:
        enabled: true
        mode: "after_tool"
        frequency: "on_error"

  - name: "Security Auditor"
    role: "worker"
    system_prompt: "file:prompts/security-auditor.md"
    tools: ["read_file"]

orchestrator:
  name: "Tech Lead"
  strategy: "ReAct"
  system_prompt: "file:prompts/tech-lead.md"
  agents:
    - "Code Reviewer"
    - "Security Auditor"
  handoff:
    include_context: true
    max_context_length: 4000
```

With directory auto-discovery, you can also define agents in separate files:

```
project/
├── config.yaml
├── agents/
│   └── code-reviewer.md    # Auto-discovered
├── skills/
│   └── code-review.md      # Auto-discovered
└── prompts/
    └── tech-lead.md         # Referenced by file:
```
