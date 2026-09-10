// Package scaffold provides use-case preset templates for rakitsu scaffold.
package scaffold

// Preset is a named use-case configuration template.
// SingleFile contains the single-YAML template string.
// DirFiles maps relative file paths to their template strings for modular output.
// Template variables: {{.Provider}}, {{.Model}}, {{.APIKeyEnv}}, {{.BaseURLEnv}}
type Preset struct {
	ID          string
	Description string
	Type        string // "single-agent", "multi-agent"
	SingleFile  string
	DirFiles    map[string]string
}

// TemplateData holds the variables interpolated into preset templates.
type TemplateData struct {
	Provider   string // e.g. "openai"
	Model      string // e.g. "gpt-4o-mini"
	APIKeyEnv  string // e.g. "OPENAI_API_KEY"
	BaseURLEnv string // e.g. "LITELLM_BASE_URL"; empty when the provider needs no base_url
}

// ProviderDefaults maps provider name to its default model.
var ProviderDefaults = map[string]string{
	"openai":    "gpt-4o-mini",
	"anthropic": "claude-haiku-4-5",
	"gemini":    "gemini-3.1-flash-lite-preview",
	"ollama":    "llama3.2",
	// LiteLLM aliases are user-defined on their own proxy — there's no
	// universal default model name, so this is a placeholder the user is
	// expected to replace (same convention as test/configs/litellm-dev-team).
	"litellm": "your-model-alias",
	// Codex reads the model from ~/.codex/config.toml when none is set; a
	// hardcoded OpenAI model would be rejected by the subscription backend.
	"codex": "",
}

// APIKeyEnvVar returns the canonical env var name for a provider's API key.
func APIKeyEnvVar(provider string) string {
	switch provider {
	case "anthropic":
		return "ANTHROPIC_API_KEY"
	case "gemini":
		return "GEMINI_API_KEY"
	case "ollama", "codex":
		return "" // no API key needed
	case "litellm":
		return "LITELLM_API_KEY"
	default:
		return "OPENAI_API_KEY"
	}
}

// BaseURLEnvVar returns the canonical env var name for a provider's base
// URL, or "" if the provider needs none in the generated config. Ollama
// needs a base_url too, but rakitsu already defaults it to
// http://localhost:11434/v1 at runtime when unset (see createLLMProvider in
// cmd/rakitsu/run.go), so it's deliberately left out here. LiteLLM has no
// such runtime default — its proxy address is always user-specific.
func BaseURLEnvVar(provider string) string {
	switch provider {
	case "litellm":
		return "LITELLM_BASE_URL"
	default:
		return ""
	}
}

// Presets is the registry of all built-in use-case presets.
var Presets = map[string]Preset{
	"llm-chat":      presetLLMChat,
	"code-review":   presetCodeReview,
	"data-analysis": presetDataAnalysis,
	"web-research":  presetWebResearch,
	"dev-team":      presetDevTeam,
	"qa-pipeline":   presetQAPipeline,
	"rag-assistant": presetRAGAssistant,
	"react-team":    presetReActTeam,
	"hierarchical":  presetHierarchical,
}

// PresetsOrdered returns preset IDs in display order.
var PresetsOrdered = []string{
	"llm-chat",
	"code-review",
	"data-analysis",
	"web-research",
	"dev-team",
	"qa-pipeline",
	"rag-assistant",
	"react-team",
	"hierarchical",
}

// ============================================================
// llm-chat — simple conversational assistant
// ============================================================

var presetLLMChat = Preset{
	ID:          "llm-chat",
	Description: "Simple single-agent conversational assistant",
	Type:        "single-agent",
	SingleFile: `name: llm-chat
version: "1.0"
description: Simple conversational LLM assistant

settings:
  default_provider: {{.Provider}}
  providers:
    {{.Provider}}:
      type: {{.Provider}}
      api_key: "${{{.APIKeyEnv}}}"
      base_url: "${{{.BaseURLEnv}}}"
  defaults:
    model: {{.Model}}
    temperature: 0.7
    max_tokens: 2048

agents:
  - name: Assistant
    role: worker
    system_prompt: |
      You are a helpful, friendly assistant. Answer questions clearly and
      concisely. If you are unsure about something, say so honestly.
    settings:
      max_iterations: 15
`,
	DirFiles: map[string]string{
		"config.yaml": `name: llm-chat
version: "1.0"
description: Simple conversational LLM assistant

settings:
  default_provider: {{.Provider}}
  providers:
    {{.Provider}}:
      type: {{.Provider}}
      api_key: "${{{.APIKeyEnv}}}"
      base_url: "${{{.BaseURLEnv}}}"
  defaults:
    model: {{.Model}}
    temperature: 0.7
    max_tokens: 2048
`,
		"agents/assistant.md": `---
name: Assistant
role: worker
settings:
  max_iterations: 15
---
You are a helpful, friendly assistant. Answer questions clearly and
concisely. If you are unsure about something, say so honestly.
`,
	},
}

// ============================================================
// code-review — code reviewer with fs read tools
// ============================================================

var presetCodeReview = Preset{
	ID:          "code-review",
	Description: "Code reviewer with file-system read tools",
	Type:        "single-agent",
	SingleFile: `name: code-review
version: "1.0"
description: Automated code reviewer

settings:
  default_provider: {{.Provider}}
  providers:
    {{.Provider}}:
      type: {{.Provider}}
      api_key: "${{{.APIKeyEnv}}}"
      base_url: "${{{.BaseURLEnv}}}"
  defaults:
    model: {{.Model}}
    temperature: 0.2

tools:
  - name: read_file
    type: fs
    description: Read a source file for review
    operation: read
    allowed_paths: ["./"]
    parameters:
      path:
        type: string
        description: Path to the file to read
        required: true

  - name: list_files
    type: fs
    description: List files in a directory
    operation: list
    allowed_paths: ["./"]
    parameters:
      path:
        type: string
        description: Directory path to list
        required: true

agents:
  - name: CodeReviewer
    role: worker
    tools: [read_file, list_files]
    system_prompt: |
      You are an expert code reviewer. When given a file path or directory,
      read the code and provide structured feedback covering:
      - Correctness and logic errors
      - Security vulnerabilities
      - Performance concerns
      - Code style and readability
      - Suggested improvements

      Be specific, cite line numbers where relevant, and prioritize issues
      by severity (critical / warning / suggestion).
    settings:
      max_iterations: 20
      reflection:
        enabled: true
        mode: before_answer
        frequency: always
`,
	DirFiles: map[string]string{
		"config.yaml": `name: code-review
version: "1.0"
description: Automated code reviewer

settings:
  default_provider: {{.Provider}}
  providers:
    {{.Provider}}:
      type: {{.Provider}}
      api_key: "${{{.APIKeyEnv}}}"
      base_url: "${{{.BaseURLEnv}}}"
  defaults:
    model: {{.Model}}
    temperature: 0.2
`,
		"agents/code-reviewer.md": `---
name: CodeReviewer
role: worker
tools: [read_file, list_files]
settings:
  max_iterations: 20
  reflection:
    enabled: true
    mode: before_answer
    frequency: always
---
You are an expert code reviewer. When given a file path or directory,
read the code and provide structured feedback covering:
- Correctness and logic errors
- Security vulnerabilities
- Performance concerns
- Code style and readability
- Suggested improvements

Be specific, cite line numbers where relevant, and prioritize issues
by severity (critical / warning / suggestion).
`,
		"tools/read-file.yaml": `name: read_file
type: fs
description: Read a source file for review
operation: read
allowed_paths: ["./"]
parameters:
  path:
    type: string
    description: Path to the file to read
    required: true
`,
		"tools/list-files.yaml": `name: list_files
type: fs
description: List files in a directory
operation: list
allowed_paths: ["./"]
parameters:
  path:
    type: string
    description: Directory path to list
    required: true
`,
	},
}

// ============================================================
// data-analysis — data analyst with fs + cli tools
// ============================================================

var presetDataAnalysis = Preset{
	ID:          "data-analysis",
	Description: "Data analyst with file-system and CLI tools",
	Type:        "single-agent",
	SingleFile: `name: data-analysis
version: "1.0"
description: Data analyst agent

settings:
  default_provider: {{.Provider}}
  providers:
    {{.Provider}}:
      type: {{.Provider}}
      api_key: "${{{.APIKeyEnv}}}"
      base_url: "${{{.BaseURLEnv}}}"
  defaults:
    model: {{.Model}}
    temperature: 0.1

tools:
  - name: read_file
    type: fs
    description: Read a data file (CSV, JSON, text)
    operation: read
    allowed_paths: ["./"]
    parameters:
      path:
        type: string
        description: Path to the data file
        required: true

  - name: search_files
    type: fs
    description: Search for patterns in files
    operation: search
    allowed_paths: ["./"]
    parameters:
      path:
        type: string
        description: Directory to search
        required: true
      pattern:
        type: string
        description: Search pattern (regex)
        required: true

  - name: run_command
    type: cli
    description: Run analysis commands (awk, sort, uniq, wc, head, tail)
    # allowed_commands only lints the payload for standalone invocations of
    # blocked commands (see internal/tools/cli's lintShellPayload) — it is
    # not a hard sandbox once the agent's command string reaches a real
    # shell via sh -c. Rakitsu's CLI tool trusts its own config; only give
    # this agent to a task where you trust what it will ask the shell to run.
    command: "sh -c '{{command}}'"
    allowed_commands: [awk, sort, uniq, wc, head, tail, cat]
    parameters:
      command:
        type: string
        description: Shell command to run
        required: true

agents:
  - name: DataAnalyst
    role: worker
    tools: [read_file, search_files, run_command]
    system_prompt: |
      You are a data analyst. When given data files or a directory,
      explore the data, identify patterns, compute statistics, and
      present clear insights. Use shell tools for counting and filtering.
      Always show your work and explain your findings.
    settings:
      max_iterations: 25
      reflection:
        enabled: true
        mode: before_answer
        frequency: always
`,
	DirFiles: map[string]string{
		"config.yaml": `name: data-analysis
version: "1.0"
description: Data analyst agent

settings:
  default_provider: {{.Provider}}
  providers:
    {{.Provider}}:
      type: {{.Provider}}
      api_key: "${{{.APIKeyEnv}}}"
      base_url: "${{{.BaseURLEnv}}}"
  defaults:
    model: {{.Model}}
    temperature: 0.1
`,
		"agents/data-analyst.md": `---
name: DataAnalyst
role: worker
tools: [read_file, search_files, run_command]
settings:
  max_iterations: 25
  reflection:
    enabled: true
    mode: before_answer
    frequency: always
---
You are a data analyst. When given data files or a directory,
explore the data, identify patterns, compute statistics, and
present clear insights. Use shell tools for counting and filtering.
Always show your work and explain your findings.
`,
		"tools/read-file.yaml": `name: read_file
type: fs
description: Read a data file (CSV, JSON, text)
operation: read
allowed_paths: ["./"]
parameters:
  path:
    type: string
    description: Path to the data file
    required: true
`,
		"tools/search-files.yaml": `name: search_files
type: fs
description: Search for patterns in files
operation: search
allowed_paths: ["./"]
parameters:
  path:
    type: string
    description: Directory to search
    required: true
  pattern:
    type: string
    description: Search pattern (regex)
    required: true
`,
		"tools/run-command.yaml": `name: run_command
type: cli
description: Run analysis commands
# allowed_commands only lints the payload for standalone invocations of
# blocked commands (see internal/tools/cli's lintShellPayload) — it is not
# a hard sandbox once the agent's command string reaches a real shell via
# sh -c. Rakitsu's CLI tool trusts its own config; only give this agent to
# a task where you trust what it will ask the shell to run.
command: "sh -c '{{command}}'"
allowed_commands: [awk, sort, uniq, wc, head, tail, cat]
parameters:
  command:
    type: string
    description: Shell command to run
    required: true
`,
	},
}

// ============================================================
// web-research — research agent (prompt-only, no tools)
// ============================================================

var presetWebResearch = Preset{
	ID:          "web-research",
	Description: "Research agent with strong reasoning (no external tools)",
	Type:        "single-agent",
	SingleFile: `name: web-research
version: "1.0"
description: Research agent — deep reasoning from knowledge

settings:
  default_provider: {{.Provider}}
  providers:
    {{.Provider}}:
      type: {{.Provider}}
      api_key: "${{{.APIKeyEnv}}}"
      base_url: "${{{.BaseURLEnv}}}"
  defaults:
    model: {{.Model}}
    temperature: 0.3

agents:
  - name: Researcher
    role: worker
    system_prompt: |
      You are a thorough researcher and analyst. When given a topic or question:
      1. Break it down into sub-questions
      2. Reason through each aspect carefully
      3. Synthesise a comprehensive, well-structured answer
      4. Cite your reasoning, not just conclusions
      5. Acknowledge uncertainty and knowledge limits honestly
    settings:
      max_iterations: 20
      reflection:
        enabled: true
        mode: both
        frequency: always
      ground_check:
        enabled: true
        confidence_threshold: 0.7
        max_retries: 1
`,
	DirFiles: map[string]string{
		"config.yaml": `name: web-research
version: "1.0"
description: Research agent

settings:
  default_provider: {{.Provider}}
  providers:
    {{.Provider}}:
      type: {{.Provider}}
      api_key: "${{{.APIKeyEnv}}}"
      base_url: "${{{.BaseURLEnv}}}"
  defaults:
    model: {{.Model}}
    temperature: 0.3
`,
		"agents/researcher.md": `---
name: Researcher
role: worker
settings:
  max_iterations: 20
  reflection:
    enabled: true
    mode: both
    frequency: always
  ground_check:
    enabled: true
    confidence_threshold: 0.7
    max_retries: 1
---
You are a thorough researcher and analyst. When given a topic or question:
1. Break it down into sub-questions
2. Reason through each aspect carefully
3. Synthesise a comprehensive, well-structured answer
4. Cite your reasoning, not just conclusions
5. Acknowledge uncertainty and knowledge limits honestly
`,
	},
}

// ============================================================
// dev-team — planner → developer → reviewer pipeline
// ============================================================

var presetDevTeam = Preset{
	ID:          "dev-team",
	Description: "Multi-agent dev team: planner → developer → reviewer (pipeline)",
	Type:        "multi-agent",
	SingleFile: `name: dev-team
version: "1.0"
description: Multi-agent software development team

settings:
  default_provider: {{.Provider}}
  providers:
    {{.Provider}}:
      type: {{.Provider}}
      api_key: "${{{.APIKeyEnv}}}"
      base_url: "${{{.BaseURLEnv}}}"
  defaults:
    model: {{.Model}}
    temperature: 0.3

tools:
  - name: read_file
    type: fs
    description: Read a file
    operation: read
    allowed_paths: ["./"]
    parameters:
      path: {type: string, description: File path, required: true}

  - name: write_file
    type: fs
    description: Write or create a file
    operation: write
    allowed_paths: ["./"]
    parameters:
      path: {type: string, description: File path, required: true}
      content: {type: string, description: File content, required: true}

agents:
  - name: Planner
    role: worker
    system_prompt: |
      You are a software architect. Given a task, produce a concise
      implementation plan: what files to create/modify, what each should
      contain, and the order of implementation. Output a numbered list.

  - name: Developer
    role: worker
    tools: [read_file, write_file]
    system_prompt: |
      You are a skilled software developer. Given an implementation plan,
      write clean, well-structured code. Create files as specified.
    settings:
      max_iterations: 30

  - name: Reviewer
    role: worker
    tools: [read_file]
    system_prompt: |
      You are a code reviewer. Read the implemented files and provide
      structured feedback: correctness, security, style, and suggestions.
      Rate overall quality 1-10 and list top 3 improvements.
    settings:
      max_iterations: 15

orchestrator:
  name: TechLead
  strategy: Pipeline
  model: {{.Model}}
  provider: {{.Provider}}
  system_prompt: |
    You are the tech lead. Coordinate the team and summarise the outcome.
  pipeline:
    steps:
      - name: planning
        agent: Planner
        task: "Plan the implementation"
      - name: development
        agent: Developer
        task: "Implement according to the plan"
        depends_on: [planning]
      - name: review
        agent: Reviewer
        task: "Review the implementation"
        depends_on: [development]
`,
	DirFiles: map[string]string{
		"config.yaml": `name: dev-team
version: "1.0"
description: Multi-agent software development team

settings:
  default_provider: {{.Provider}}
  providers:
    {{.Provider}}:
      type: {{.Provider}}
      api_key: "${{{.APIKeyEnv}}}"
      base_url: "${{{.BaseURLEnv}}}"
  defaults:
    model: {{.Model}}
    temperature: 0.3

orchestrator:
  name: TechLead
  strategy: Pipeline
  model: {{.Model}}
  provider: {{.Provider}}
  system_prompt: |
    You are the tech lead. Coordinate the team and summarise the outcome.
  pipeline:
    steps:
      - name: planning
        agent: Planner
        task: "Plan the implementation"
      - name: development
        agent: Developer
        task: "Implement according to the plan"
        depends_on: [planning]
      - name: review
        agent: Reviewer
        task: "Review the implementation"
        depends_on: [development]
`,
		"agents/planner.md": `---
name: Planner
role: worker
---
You are a software architect. Given a task, produce a concise
implementation plan: what files to create/modify, what each should
contain, and the order of implementation. Output a numbered list.
`,
		"agents/developer.md": `---
name: Developer
role: worker
tools: [read_file, write_file]
settings:
  max_iterations: 30
---
You are a skilled software developer. Given an implementation plan,
write clean, well-structured code. Create files as specified.
`,
		"agents/reviewer.md": `---
name: Reviewer
role: worker
tools: [read_file]
settings:
  max_iterations: 15
---
You are a code reviewer. Read the implemented files and provide
structured feedback: correctness, security, style, and suggestions.
Rate overall quality 1-10 and list top 3 improvements.
`,
		"tools/read-file.yaml": `name: read_file
type: fs
description: Read a file
operation: read
allowed_paths: ["./"]
parameters:
  path:
    type: string
    description: File path
    required: true
`,
		"tools/write-file.yaml": `name: write_file
type: fs
description: Write or create a file
operation: write
allowed_paths: ["./"]
parameters:
  path:
    type: string
    description: File path
    required: true
  content:
    type: string
    description: File content
    required: true
`,
	},
}

// ============================================================
// qa-pipeline — test generator + validator
// ============================================================

var presetQAPipeline = Preset{
	ID:          "qa-pipeline",
	Description: "Multi-agent QA: test generator → validator pipeline",
	Type:        "multi-agent",
	SingleFile: `name: qa-pipeline
version: "1.0"
description: Multi-agent QA pipeline

settings:
  default_provider: {{.Provider}}
  providers:
    {{.Provider}}:
      type: {{.Provider}}
      api_key: "${{{.APIKeyEnv}}}"
      base_url: "${{{.BaseURLEnv}}}"
  defaults:
    model: {{.Model}}
    temperature: 0.2

tools:
  - name: read_file
    type: fs
    description: Read source or test files
    operation: read
    allowed_paths: ["./"]
    parameters:
      path: {type: string, description: File path, required: true}

  - name: write_file
    type: fs
    description: Write test files
    operation: write
    allowed_paths: ["./"]
    parameters:
      path: {type: string, description: File path, required: true}
      content: {type: string, description: Content, required: true}

agents:
  - name: TestGenerator
    role: worker
    tools: [read_file, write_file]
    system_prompt: |
      You are a QA engineer. Read the provided source code and generate
      comprehensive tests: unit tests for each function, edge cases, and
      error scenarios. Write tests to disk.
    settings:
      max_iterations: 25

  - name: Validator
    role: worker
    tools: [read_file]
    system_prompt: |
      You are a QA validator. Review the generated tests for coverage
      completeness, test quality, and missing edge cases. Provide a
      pass/fail verdict with a coverage score (0-100).
    settings:
      max_iterations: 15

orchestrator:
  name: QALead
  strategy: Pipeline
  model: {{.Model}}
  provider: {{.Provider}}
  system_prompt: Coordinate test generation and validation.
  pipeline:
    steps:
      - name: generate
        agent: TestGenerator
        task: "Generate tests for the provided source code"
      - name: validate
        agent: Validator
        task: "Validate the generated tests"
        depends_on: [generate]
`,
	DirFiles: map[string]string{
		"config.yaml": `name: qa-pipeline
version: "1.0"
description: Multi-agent QA pipeline

settings:
  default_provider: {{.Provider}}
  providers:
    {{.Provider}}:
      type: {{.Provider}}
      api_key: "${{{.APIKeyEnv}}}"
      base_url: "${{{.BaseURLEnv}}}"
  defaults:
    model: {{.Model}}
    temperature: 0.2

orchestrator:
  name: QALead
  strategy: Pipeline
  model: {{.Model}}
  provider: {{.Provider}}
  system_prompt: Coordinate test generation and validation.
  pipeline:
    steps:
      - name: generate
        agent: TestGenerator
        task: "Generate tests for the provided source code"
      - name: validate
        agent: Validator
        task: "Validate the generated tests"
        depends_on: [generate]
`,
		"agents/test-generator.md": `---
name: TestGenerator
role: worker
tools: [read_file, write_file]
settings:
  max_iterations: 25
---
You are a QA engineer. Read the provided source code and generate
comprehensive tests: unit tests for each function, edge cases, and
error scenarios. Write tests to disk.
`,
		"agents/validator.md": `---
name: Validator
role: worker
tools: [read_file]
settings:
  max_iterations: 15
---
You are a QA validator. Review the generated tests for coverage
completeness, test quality, and missing edge cases. Provide a
pass/fail verdict with a coverage score (0-100).
`,
		"tools/read-file.yaml": `name: read_file
type: fs
description: Read source or test files
operation: read
allowed_paths: ["./"]
parameters:
  path:
    type: string
    description: File path
    required: true
`,
		"tools/write-file.yaml": `name: write_file
type: fs
description: Write test files
operation: write
allowed_paths: ["./"]
parameters:
  path:
    type: string
    description: File path
    required: true
  content:
    type: string
    description: Content
    required: true
`,
	},
}

// ============================================================
// rag-assistant — fs search + summarisation skill
// ============================================================

var presetRAGAssistant = Preset{
	ID:          "rag-assistant",
	Description: "RAG-style assistant: searches files then synthesises answers",
	Type:        "single-agent",
	SingleFile: `name: rag-assistant
version: "1.0"
description: RAG-style assistant — searches a knowledge base and synthesises answers

settings:
  default_provider: {{.Provider}}
  providers:
    {{.Provider}}:
      type: {{.Provider}}
      api_key: "${{{.APIKeyEnv}}}"
      base_url: "${{{.BaseURLEnv}}}"
  defaults:
    model: {{.Model}}
    temperature: 0.3

tools:
  - name: search_files
    type: fs
    description: Search for relevant content in knowledge base files
    operation: search
    allowed_paths: ["./knowledge-base/"]
    working_dir: "./knowledge-base/"
    parameters:
      path:
        type: string
        description: Directory to search
        required: true
      pattern:
        type: string
        description: Search term or regex pattern
        required: true

  - name: read_file
    type: fs
    description: Read a specific knowledge base document
    operation: read
    allowed_paths: ["./knowledge-base/"]
    working_dir: "./knowledge-base/"
    parameters:
      path:
        type: string
        description: File path to read
        required: true

skills:
  - name: summarise
    description: Summarise and synthesise retrieved information
    tools: [search_files, read_file]
    prompt_template: |
      Given the retrieved documents, synthesise a clear, accurate answer.
      Cite the source files for each key fact. If information is missing,
      say so rather than guessing.

agents:
  - name: RAGAssistant
    role: worker
    tools: [search_files, read_file]
    skills: [summarise]
    system_prompt: |
      You are a knowledge assistant. When answering questions:
      1. Search the knowledge base for relevant documents
      2. Read the most relevant files in full
      3. Synthesise a clear, cited answer from the retrieved content
      4. If the knowledge base has no relevant information, say so
    settings:
      max_iterations: 20
      reflection:
        enabled: true
        mode: before_answer
        frequency: always
`,
	DirFiles: map[string]string{
		"config.yaml": `name: rag-assistant
version: "1.0"
description: RAG-style assistant

settings:
  default_provider: {{.Provider}}
  providers:
    {{.Provider}}:
      type: {{.Provider}}
      api_key: "${{{.APIKeyEnv}}}"
      base_url: "${{{.BaseURLEnv}}}"
  defaults:
    model: {{.Model}}
    temperature: 0.3
`,
		"agents/rag-assistant.md": `---
name: RAGAssistant
role: worker
tools: [search_files, read_file]
skills: [summarise]
settings:
  max_iterations: 20
  reflection:
    enabled: true
    mode: before_answer
    frequency: always
---
You are a knowledge assistant. When answering questions:
1. Search the knowledge base for relevant documents
2. Read the most relevant files in full
3. Synthesise a clear, cited answer from the retrieved content
4. If the knowledge base has no relevant information, say so
`,
		"tools/search-files.yaml": `name: search_files
type: fs
description: Search for relevant content in knowledge base files
operation: search
allowed_paths: ["./knowledge-base/"]
working_dir: "./knowledge-base/"
parameters:
  path:
    type: string
    description: Directory to search
    required: true
  pattern:
    type: string
    description: Search term or regex pattern
    required: true
`,
		"tools/read-file.yaml": `name: read_file
type: fs
description: Read a specific knowledge base document
operation: read
allowed_paths: ["./knowledge-base/"]
working_dir: "./knowledge-base/"
parameters:
  path:
    type: string
    description: File path to read
    required: true
`,
		"skills/summarise.yaml": `name: summarise
description: Summarise and synthesise retrieved information
tools: [search_files, read_file]
prompt_template: |
  Given the retrieved documents, synthesise a clear, accurate answer.
  Cite the source files for each key fact. If information is missing,
  say so rather than guessing.
`,
		"knowledge-base/.gitkeep": ``,
	},
}

// ============================================================
// react-team — ReAct supervisor with specialist workers
// ============================================================

var presetReActTeam = Preset{
	ID:          "react-team",
	Description: "ReAct supervisor with specialist workers",
	Type:        "multi-agent",
	SingleFile: `name: react-team
version: "1.0"
description: ReAct-based team with dynamic task delegation

settings:
  default_provider: {{.Provider}}
  providers:
    {{.Provider}}:
      type: {{.Provider}}
      api_key: "${{{.APIKeyEnv}}}"
      base_url: "${{{.BaseURLEnv}}}"
  defaults:
    model: {{.Model}}
    temperature: 0.3
    max_tokens: 4096

tools:
  - name: read_file
    type: fs
    description: Read a file
    operation: read
    parameters:
      path: { type: string, description: "File path", required: true }

  - name: search_files
    type: fs
    description: Search for patterns in files
    operation: search
    parameters:
      pattern: { type: string, description: "Search pattern", required: true }
      path: { type: string, description: "Directory to search", required: true }

agents:
  - name: Researcher
    role: worker
    system_prompt: |
      Research the given topic using available tools. Gather evidence
      and produce a structured brief with citations.
    tools: [read_file, search_files]
    settings:
      max_iterations: 5

  - name: Analyst
    role: worker
    system_prompt: |
      Analyze the research findings. Identify patterns, draw conclusions,
      and provide actionable recommendations.
    settings:
      max_iterations: 3

orchestrator:
  name: Lead
  strategy: ReAct
  provider: {{.Provider}}
  model: {{.Model}}
  system_prompt: |
    You lead a research team. Delegate research tasks to the Researcher
    and analysis tasks to the Analyst. Synthesize their findings.
  agents:
    - Researcher
    - Analyst
  handoff:
    include_context: true
    max_context_length: 4000
`,
	DirFiles: map[string]string{
		"config.yaml": `name: react-team
version: "1.0"
description: ReAct-based team with dynamic task delegation

settings:
  default_provider: {{.Provider}}
  providers:
    {{.Provider}}:
      type: {{.Provider}}
      api_key: "${{{.APIKeyEnv}}}"
      base_url: "${{{.BaseURLEnv}}}"
  defaults:
    model: {{.Model}}
    temperature: 0.3
    max_tokens: 4096

orchestrator:
  name: Lead
  strategy: ReAct
  provider: {{.Provider}}
  model: {{.Model}}
  system_prompt: |
    You lead a research team. Delegate research tasks to the Researcher
    and analysis tasks to the Analyst. Synthesize their findings.
  agents:
    - Researcher
    - Analyst
  handoff:
    include_context: true
    max_context_length: 4000
`,
		"agents/researcher.md": `---
name: Researcher
role: worker
tools:
  - read_file
  - search_files
settings:
  max_iterations: 5
---
Research the given topic using available tools. Gather evidence
and produce a structured brief with citations.
`,
		"agents/analyst.md": `---
name: Analyst
role: worker
settings:
  max_iterations: 3
---
Analyze the research findings. Identify patterns, draw conclusions,
and provide actionable recommendations.
`,
		"tools/read_file.yaml": `name: read_file
type: fs
description: Read a file
operation: read
parameters:
  path: { type: string, description: "File path", required: true }
`,
		"tools/search_files.yaml": `name: search_files
type: fs
description: Search for patterns in files
operation: search
parameters:
  pattern: { type: string, description: "Search pattern", required: true }
  path: { type: string, description: "Directory to search", required: true }
`,
	},
}

// ============================================================
// hierarchical — supervisor with dynamic delegation
// ============================================================

var presetHierarchical = Preset{
	ID:          "hierarchical",
	Description: "Hierarchical supervisor with dynamic delegation",
	Type:        "multi-agent",
	SingleFile: `name: hierarchical
version: "1.0"
description: Hierarchical team with dynamic task assignment

settings:
  default_provider: {{.Provider}}
  providers:
    {{.Provider}}:
      type: {{.Provider}}
      api_key: "${{{.APIKeyEnv}}}"
      base_url: "${{{.BaseURLEnv}}}"
  defaults:
    model: {{.Model}}
    temperature: 0.2
    max_tokens: 4096

tools:
  - name: read_file
    type: fs
    description: Read a file
    operation: read
    parameters:
      path: { type: string, description: "File path", required: true }

  - name: write_file
    type: fs
    description: Write or create a file
    operation: write
    allowed_paths: ["./"]
    parameters:
      path: { type: string, description: "File path", required: true }
      content: { type: string, description: "File content", required: true }

agents:
  - name: BackendDev
    role: worker
    system_prompt: "You handle backend code: APIs, databases, business logic."
    tools: [read_file, write_file]
    settings: { max_iterations: 3 }

  - name: FrontendDev
    role: worker
    system_prompt: "You handle frontend code: components, UI, state management."
    tools: [read_file, write_file]
    settings: { max_iterations: 3 }

  - name: QAEngineer
    role: worker
    system_prompt: "You write tests based on the code you read. Report what you wrote and any gaps you noticed."
    tools: [read_file, write_file]
    settings: { max_iterations: 3 }

orchestrator:
  name: TechLead
  strategy: Hierarchical
  provider: {{.Provider}}
  model: {{.Model}}
  system_prompt: |
    You are the Tech Lead. Assign tasks to the appropriate team member
    based on the code area. Coordinate their work and ensure quality.
  agents:
    - BackendDev
    - FrontendDev
    - QAEngineer
  handoff:
    include_context: true
`,
	DirFiles: map[string]string{
		"config.yaml": `name: hierarchical
version: "1.0"
description: Hierarchical team with dynamic task assignment

settings:
  default_provider: {{.Provider}}
  providers:
    {{.Provider}}:
      type: {{.Provider}}
      api_key: "${{{.APIKeyEnv}}}"
      base_url: "${{{.BaseURLEnv}}}"
  defaults:
    model: {{.Model}}
    temperature: 0.2
    max_tokens: 4096

orchestrator:
  name: TechLead
  strategy: Hierarchical
  provider: {{.Provider}}
  model: {{.Model}}
  system_prompt: |
    You are the Tech Lead. Assign tasks to the appropriate team member
    based on the code area. Coordinate their work and ensure quality.
  agents:
    - BackendDev
    - FrontendDev
    - QAEngineer
  handoff:
    include_context: true
`,
		"agents/backend-dev.md": `---
name: BackendDev
role: worker
tools:
  - read_file
  - write_file
settings:
  max_iterations: 3
---
You handle backend code: APIs, databases, business logic.
`,
		"agents/frontend-dev.md": `---
name: FrontendDev
role: worker
tools:
  - read_file
  - write_file
settings:
  max_iterations: 3
---
You handle frontend code: components, UI, state management.
`,
		"agents/qa-engineer.md": `---
name: QAEngineer
role: worker
tools:
  - read_file
  - write_file
settings:
  max_iterations: 3
---
You write tests based on the code you read. Report what you wrote and any gaps you noticed.
`,
		"tools/read_file.yaml": `name: read_file
type: fs
description: Read a file
operation: read
parameters:
  path: { type: string, description: "File path", required: true }
`,
		"tools/write_file.yaml": `name: write_file
type: fs
description: Write or create a file
operation: write
allowed_paths: ["./"]
parameters:
  path: { type: string, description: "File path", required: true }
  content: { type: string, description: "File content", required: true }
`,
	},
}
