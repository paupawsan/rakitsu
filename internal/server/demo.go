package server

import (
	"regexp"
	"strings"
)

// DemoYAML is the embedded demo configuration for the first-run welcome
// experience. It uses Ollama as default provider (no API key required) and
// cli tools that echo realistic JSON so the demo works without external APIs.
// DemoYAMLTemplate uses %s for provider name — filled by the /api/demo endpoint.
const DemoYAMLTemplate = `name: "Market Analysis Demo"
version: "1.0"
description: "Demo agent analyzing stock data — showcases the Rakitsu debugger"

settings:
  default_provider: {{PROVIDER}}
  providers:
    openai:
      type: openai
      api_key: "${OPENAI_API_KEY}"
    ollama:
      type: ollama
      base_url: http://localhost:11434/v1
    litellm:
      type: litellm
      api_key: "${LITELLM_API_KEY}"
      base_url: "${LITELLM_BASE_URL}"
    anthropic:
      type: anthropic
      api_key: "${ANTHROPIC_API_KEY}"
    gemini:
      type: gemini
      api_key: "${GEMINI_API_KEY}"
  defaults:
    model: {{MODEL}}
    temperature: 0.7
    max_tokens: 1024
  execution:
    max_iterations: 5
    timeout_seconds: 60

tools:
  - name: get_price
    type: cli
    description: "Get current price data for a stock ticker"
    command: "echo '{\"ticker\": \"{{ticker}}\", \"price\": 182.52, \"change\": \"+1.3%\", \"volume\": \"62.4M\"}'"
    parameters:
      ticker:
        type: string
        description: "Stock ticker symbol (e.g. AAPL)"
        required: true

  - name: get_news
    type: cli
    description: "Get recent news headlines for a topic"
    command: "echo '[{\"headline\": \"Tech stocks rally on AI spending surge\", \"source\": \"Reuters\"}, {\"headline\": \"Fed signals rate pause through Q2\", \"source\": \"Bloomberg\"}]'"
    parameters:
      topic:
        type: string
        description: "News topic to search"
        required: true

agents:
  - name: MarketAnalyst
    role: worker
    system_prompt: |
      You are a market analyst. Use your tools to gather price data and news,
      then provide a brief market outlook. Be concise — 2-3 paragraphs max.
    tools: [get_price, get_news]
    settings:
      max_iterations: 5
`

// DemoQuery is the default query for the demo run.
const DemoQuery = "Analyze AAPL and give me a brief market outlook"

// DemoProviders lists the providers available in the demo, with default models.
var DemoProviders = []struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	Model string `json:"model"`
}{
	{"ollama", "Ollama (local, free)", "llama3.2"},
	{"litellm", "LiteLLM (proxy)", "gpt-4o-mini"},
	{"openai", "OpenAI", "gpt-4o-mini"},
	{"anthropic", "Anthropic", "claude-sonnet-4-20250514"},
	{"gemini", "Google Gemini", "gemini-2.0-flash"},
}

// DemoYAML returns the demo config YAML for the given provider and model.
// Callers MUST validate both against demoIdentifierPattern first — this
// function does a raw, unescaped substitution into a YAML template, so an
// unvalidated value containing a newline or YAML-structural character can
// break out of the `default_provider: {{PROVIDER}}` / `model: {{MODEL}}`
// scalars and inject arbitrary sibling YAML (round-7 PR #9 review finding).
func DemoYAML(provider, model string) string {
	r := strings.NewReplacer("{{PROVIDER}}", provider, "{{MODEL}}", model)
	return r.Replace(DemoYAMLTemplate)
}

// demoIdentifierPattern is the boundary validation for /api/demo's
// provider/model query params: real provider and model names are always
// simple identifiers, so a strict allowlist closes the YAML-injection
// vector at the point untrusted input enters, regardless of what a future
// caller does with the (still unescaped) DemoYAML template.
var demoIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]+$`)

// validDemoIdentifier reports whether s is safe to substitute into
// DemoYAMLTemplate.
func validDemoIdentifier(s string) bool {
	return s != "" && demoIdentifierPattern.MatchString(s)
}

// DemoBreakpoints returns pre-placed breakpoints for the demo run.
func DemoBreakpoints() []BreakpointEntry {
	return []BreakpointEntry{
		{EventType: "pre_thought", AgentName: "MarketAnalyst"},
	}
}
