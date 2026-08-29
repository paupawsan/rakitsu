package scaffold

import (
	"fmt"
	"strings"
)

// Render executes the preset templates with data and returns a map of
// relative file path → rendered content.
//
// When dir is false, a single entry is returned keyed as "<preset-id>.yaml".
// When dir is true, one entry per DirFiles template is returned.
func Render(preset Preset, data TemplateData, dir bool) (map[string]string, error) {
	if dir {
		return renderDir(preset, data)
	}
	return renderSingle(preset, data)
}

func renderSingle(preset Preset, data TemplateData) (map[string]string, error) {
	content, err := renderTemplate(preset.SingleFile, data)
	if err != nil {
		return nil, fmt.Errorf("rendering %s single file: %w", preset.ID, err)
	}
	return map[string]string{preset.ID + ".yaml": content}, nil
}

func renderDir(preset Preset, data TemplateData) (map[string]string, error) {
	result := make(map[string]string, len(preset.DirFiles))
	for relPath, tmpl := range preset.DirFiles {
		content, err := renderTemplate(tmpl, data)
		if err != nil {
			return nil, fmt.Errorf("rendering %s dir file %q: %w", preset.ID, relPath, err)
		}
		result[relPath] = content
	}
	return result, nil
}

func renderTemplate(tmpl string, data TemplateData) (string, error) {
	// Provider/Model come straight from --provider/--model flags; APIKeyEnv
	// is derived from --provider via APIKeyEnvVar's fixed 4-way switch, not
	// its own flag, so today it's always one of a few hardcoded safe
	// strings. All three are spliced into YAML below. Rejecting an embedded
	// newline stops a crafted value from opening a new YAML line (the most
	// direct way to inject a structurally distinct key or document) — it
	// does not guarantee full YAML safety against every structural
	// character (a `"` or `:` can still corrupt the single line it lands
	// on). Real-world risk from the CLI is low (Provider/Model come from the
	// same person's own invocation; APIKeyEnv isn't user-supplied at all
	// today), but Render/TemplateData are exported — a direct caller of this
	// package isn't bound by that. APIKeyEnv gets two more checks below:
	// unlike Provider/Model, it's always spliced inside a double-quoted
	// scalar (`api_key: "${...}"`), where an embedded `"` would break the
	// quoting on that line, and a `\` is also meaningful there (YAML's
	// double-quoted scalars support backslash escapes, so e.g. `\q` isn't a
	// valid one). Provider/Model still only reject `\n`/`\r`, not the fuller
	// set of YAML-structural characters (`"`/`\`/`:`/`#`) — left as-is since
	// they're bare (unquoted) scalars where the practical risk is lower, and
	// closing that gap too is tracked as a follow-up rather than done here.
	if strings.ContainsAny(data.Provider, "\n\r") {
		return "", fmt.Errorf("provider must not contain a newline")
	}
	if strings.ContainsAny(data.Model, "\n\r") {
		return "", fmt.Errorf("model must not contain a newline")
	}
	if strings.ContainsAny(data.APIKeyEnv, "\n\r") {
		return "", fmt.Errorf("api key env var must not contain a newline")
	}
	if strings.Contains(data.APIKeyEnv, `"`) {
		return "", fmt.Errorf(`api key env var must not contain a '"'`)
	}
	if strings.Contains(data.APIKeyEnv, `\`) {
		return "", fmt.Errorf(`api key env var must not contain a '\'`)
	}

	r := strings.NewReplacer(
		"{{.Provider}}", data.Provider,
		"{{.Model}}", data.Model,
		"{{.APIKeyEnv}}", data.APIKeyEnv,
	)
	result := r.Replace(tmpl)

	// Remove api_key lines with empty env var (e.g. Ollama needs no key)
	var lines []string
	for _, line := range strings.Split(result, "\n") {
		if strings.Contains(line, `api_key: "${}"`) {
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n"), nil
}

// DefaultModel returns the default model for a provider, falling back to
// the provider's entry in ProviderDefaults or "gpt-4o-mini".
func DefaultModel(provider string) string {
	if m, ok := ProviderDefaults[provider]; ok {
		return m
	}
	return "gpt-4o-mini"
}

// ListTable returns a formatted table of all presets for --list output.
func ListTable() string {
	var sb strings.Builder
	sb.WriteString("Available use-cases:\n\n")
	sb.WriteString(fmt.Sprintf("  %-18s %-14s %s\n", "USE-CASE", "TYPE", "DESCRIPTION"))
	sb.WriteString("  " + strings.Repeat("─", 70) + "\n")
	for _, id := range PresetsOrdered {
		p := Presets[id]
		sb.WriteString(fmt.Sprintf("  %-18s %-14s %s\n", p.ID, p.Type, p.Description))
	}
	sb.WriteString("\nExamples:\n")
	sb.WriteString("  rakitsu scaffold llm-chat\n")
	sb.WriteString("  rakitsu scaffold code-review --provider anthropic --model claude-sonnet-4-6\n")
	sb.WriteString("  rakitsu scaffold dev-team --dir -o ./my-dev-team\n")
	return sb.String()
}
