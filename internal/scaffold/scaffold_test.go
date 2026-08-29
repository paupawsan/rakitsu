package scaffold

import (
	"context"
	"strings"
	"testing"

	"github.com/paupawsan/rakitsu/internal/config"
	"github.com/paupawsan/rakitsu/internal/tools/cli"
	"gopkg.in/yaml.v3"
)

var testData = TemplateData{
	Provider:  "openai",
	Model:     "gpt-4o-mini",
	APIKeyEnv: "OPENAI_API_KEY",
}

// ============================================================
// Render — YAML-injection guard
// ============================================================

// Regression: --provider/--model are spliced into YAML as both a key and
// scalar values via plain string replacement. A value containing a
// newline could inject arbitrary YAML structure into the generated
// config; renderTemplate must reject it instead of rendering it.

func TestRender_RejectsNewlineInProvider(t *testing.T) {
	data := TemplateData{Provider: "openai\n  evil: true", Model: "gpt-4o-mini", APIKeyEnv: "OPENAI_API_KEY"}
	if _, err := Render(Presets["llm-chat"], data, false); err == nil {
		t.Fatal("expected an error for a Provider value containing a newline")
	}
}

func TestRender_RejectsNewlineInModel(t *testing.T) {
	data := TemplateData{Provider: "openai", Model: "gpt-4o\n  evil: true", APIKeyEnv: "OPENAI_API_KEY"}
	if _, err := Render(Presets["llm-chat"], data, false); err == nil {
		t.Fatal("expected an error for a Model value containing a newline")
	}
}

func TestRender_RejectsNewlineInAPIKeyEnv(t *testing.T) {
	// APIKeyEnv is spliced into the identical quoted-scalar YAML context
	// (api_key: "${...}") as Provider/Model — it needs the same guard.
	data := TemplateData{Provider: "openai", Model: "gpt-4o-mini", APIKeyEnv: "OPENAI_API_KEY\"\n  evil: true"}
	if _, err := Render(Presets["llm-chat"], data, false); err == nil {
		t.Fatal("expected an error for an APIKeyEnv value containing a newline")
	}
}

// Regression: unlike Provider/Model (always spliced bare), APIKeyEnv always
// lands inside a double-quoted YAML scalar (api_key: "${...}") — a literal
// '"' breaks that quoting on the same line even without a newline.
func TestRender_RejectsQuoteInAPIKeyEnv(t *testing.T) {
	data := TemplateData{Provider: "openai", Model: "gpt-4o-mini", APIKeyEnv: `OPENAI_API_KEY"}, evil: true, x: {"`}
	if _, err := Render(Presets["llm-chat"], data, false); err == nil {
		t.Fatal("expected an error for an APIKeyEnv value containing a double-quote")
	}
}

// Regression: a backslash is also meaningful inside the same double-quoted
// YAML scalar (api_key: "${...}") that the '"' check above guards — YAML's
// double-quoted scalars support backslash escapes, so an unescaped one can
// still corrupt the line even without a literal quote character.
func TestRender_RejectsBackslashInAPIKeyEnv(t *testing.T) {
	data := TemplateData{Provider: "openai", Model: "gpt-4o-mini", APIKeyEnv: `OPENAI_API_KEY\q`}
	if _, err := Render(Presets["llm-chat"], data, false); err == nil {
		t.Fatal("expected an error for an APIKeyEnv value containing a backslash")
	}
}

// ============================================================
// Render — single file
// ============================================================

func TestRender_SingleFile_AllPresets_ValidYAML(t *testing.T) {
	for _, id := range PresetsOrdered {
		preset := Presets[id]
		t.Run(id, func(t *testing.T) {
			files, err := Render(preset, testData, false)
			if err != nil {
				t.Fatalf("Render(%s, single) error: %v", id, err)
			}
			if len(files) != 1 {
				t.Fatalf("expected 1 file, got %d", len(files))
			}
			for name, content := range files {
				if !strings.HasSuffix(name, ".yaml") {
					t.Errorf("expected .yaml filename, got %q", name)
				}
				if content == "" {
					t.Errorf("content is empty for %s", name)
				}
				// Validate YAML
				var v interface{}
				if err := yaml.Unmarshal([]byte(content), &v); err != nil {
					t.Errorf("invalid YAML for %s: %v\ncontent:\n%s", name, err, content)
				}
			}
		})
	}
}

// ============================================================
// Render — directory
// ============================================================

func TestRender_Dir_AllPresets_ValidFiles(t *testing.T) {
	for _, id := range PresetsOrdered {
		preset := Presets[id]
		t.Run(id, func(t *testing.T) {
			files, err := Render(preset, testData, true)
			if err != nil {
				t.Fatalf("Render(%s, dir) error: %v", id, err)
			}
			if len(files) == 0 {
				t.Fatalf("expected >0 dir files for %s", id)
			}
			for relPath, content := range files {
				if relPath == "" {
					t.Errorf("empty relPath key")
				}
				// All YAML files must be valid YAML
				if strings.HasSuffix(relPath, ".yaml") || strings.HasSuffix(relPath, ".yml") {
					var v interface{}
					if err := yaml.Unmarshal([]byte(content), &v); err != nil {
						t.Errorf("invalid YAML in dir file %s/%s: %v\ncontent:\n%s", id, relPath, err, content)
					}
				}
			}
		})
	}
}

func TestRender_Dir_HasConfigYAML(t *testing.T) {
	for _, id := range PresetsOrdered {
		preset := Presets[id]
		files, err := Render(preset, testData, true)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if _, ok := files["config.yaml"]; !ok {
			t.Errorf("%s dir mode: missing config.yaml", id)
		}
	}
}

// ============================================================
// Provider / model defaults
// ============================================================

func TestDefaultModel_KnownProviders(t *testing.T) {
	cases := map[string]string{
		"openai":    "gpt-4o-mini",
		"anthropic": "claude-haiku-4-5",
		"gemini":    "gemini-3.1-flash-lite-preview",
		"ollama":    "llama3.2",
	}
	for provider, want := range cases {
		if got := DefaultModel(provider); got != want {
			t.Errorf("DefaultModel(%q) = %q, want %q", provider, got, want)
		}
	}
}

func TestDefaultModel_UnknownProvider_Fallback(t *testing.T) {
	if got := DefaultModel("unknown-llm"); got != "gpt-4o-mini" {
		t.Errorf("DefaultModel(unknown) = %q, want %q", got, "gpt-4o-mini")
	}
}

func TestAPIKeyEnvVar(t *testing.T) {
	cases := map[string]string{
		"openai":    "OPENAI_API_KEY",
		"anthropic": "ANTHROPIC_API_KEY",
		"gemini":    "GEMINI_API_KEY",
		"ollama":    "",
	}
	for provider, want := range cases {
		if got := APIKeyEnvVar(provider); got != want {
			t.Errorf("APIKeyEnvVar(%q) = %q, want %q", provider, got, want)
		}
	}
}

// ============================================================
// Template interpolation
// ============================================================

func TestRender_InterpolatesProvider(t *testing.T) {
	preset := Presets["llm-chat"]
	data := TemplateData{Provider: "anthropic", Model: "claude-haiku-4-5", APIKeyEnv: "ANTHROPIC_API_KEY"}

	files, err := Render(preset, data, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, content := range files {
		if !strings.Contains(content, "anthropic") {
			t.Errorf("rendered content does not contain provider 'anthropic':\n%s", content)
		}
		if !strings.Contains(content, "claude-haiku-4-5") {
			t.Errorf("rendered content does not contain model 'claude-haiku-4-5':\n%s", content)
		}
		if !strings.Contains(content, "ANTHROPIC_API_KEY") {
			t.Errorf("rendered content does not contain API key env var:\n%s", content)
		}
		// Must not contain un-rendered template markers
		if strings.Contains(content, "{{.") {
			t.Errorf("rendered content still contains template markers:\n%s", content)
		}
	}
}

func TestRender_NoTemplateMarkersRemaining(t *testing.T) {
	for _, id := range PresetsOrdered {
		preset := Presets[id]
		for _, dir := range []bool{false, true} {
			files, err := Render(preset, testData, dir)
			if err != nil {
				t.Fatalf("%s dir=%v: %v", id, dir, err)
			}
			for relPath, content := range files {
				if strings.Contains(content, "{{.") {
					t.Errorf("%s/%s (dir=%v): unrendered template marker found:\n%s", id, relPath, dir, content)
				}
			}
		}
	}
}

// ============================================================
// Preset registry
// ============================================================

func TestPresetsOrdered_AllExistInMap(t *testing.T) {
	for _, id := range PresetsOrdered {
		if _, ok := Presets[id]; !ok {
			t.Errorf("PresetsOrdered contains %q but it is not in Presets map", id)
		}
	}
}

func TestPresets_Count(t *testing.T) {
	if len(Presets) != 9 {
		t.Errorf("expected 9 presets, got %d", len(Presets))
	}
}

func TestPresets_AllHaveID(t *testing.T) {
	for id, p := range Presets {
		if p.ID == "" {
			t.Errorf("preset %q has empty ID field", id)
		}
		if p.ID != id {
			t.Errorf("preset key %q != ID field %q", id, p.ID)
		}
		if p.Description == "" {
			t.Errorf("preset %q has empty Description", id)
		}
		if p.SingleFile == "" {
			t.Errorf("preset %q has empty SingleFile template", id)
		}
		// DirFiles are optional for multi-agent presets (single-file is sufficient)
		if len(p.DirFiles) == 0 && p.Type == "single-agent" {
			t.Errorf("preset %q has no DirFiles", id)
		}
	}
}

// ============================================================
// ListTable
// ============================================================

func TestListTable_ContainsAllIDs(t *testing.T) {
	table := ListTable()
	for _, id := range PresetsOrdered {
		if !strings.Contains(table, id) {
			t.Errorf("ListTable output missing preset ID %q", id)
		}
	}
}

// ============================================================
// data-analysis — run_command must actually pass the CLI tool's
// whitelist, not just render as syntactically valid YAML
// ============================================================

// Regression: command: "{{command}}" is a single whitespace-free token, so
// the entire multi-word substituted value (e.g. "awk '{print $1}' data.csv")
// became argv[0], and isCommandAllowed's filepath.Base check matched
// nothing in the whitelist — every real invocation was rejected. This test
// exercises the actual internal/tools/cli.Tool built from the preset's own
// rendered command template, not just a string comparison, so it would
// have failed against the old template.
func TestDataAnalysisPreset_RunCommand_PassesCLIWhitelist(t *testing.T) {
	rendered, err := Render(Presets["data-analysis"], testData, false)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	content := rendered["data-analysis.yaml"]

	var raw struct {
		Tools []struct {
			Name    string `yaml:"name"`
			Command string `yaml:"command"`
		} `yaml:"tools"`
	}
	if err := yaml.Unmarshal([]byte(content), &raw); err != nil {
		t.Fatalf("unmarshal rendered YAML: %v", err)
	}

	var cmdTemplate string
	found := false
	for _, tool := range raw.Tools {
		if tool.Name == "run_command" {
			cmdTemplate = tool.Command
			found = true
		}
	}
	if !found {
		t.Fatal("run_command tool not found in rendered data-analysis preset")
	}

	def := &config.ToolDefinition{Name: "run_command", Type: "cli", Command: cmdTemplate}
	tool := cli.NewTool(def, []string{"awk", "sort", "uniq", "wc", "head", "tail", "cat"})

	out, err := tool.Execute(context.Background(), map[string]interface{}{"command": "echo hello | wc -l"})
	if err != nil {
		t.Fatalf("run_command should execute successfully with the fixed template, got error: %v", err)
	}
	if strings.TrimSpace(out) != "1" {
		t.Errorf("want wc -l output %q, got %q", "1", strings.TrimSpace(out))
	}
}

// ============================================================
// hierarchical — tool grants must back the prompts' promises
// ============================================================

// Regression: all three workers were granted only [read_file] while their
// prompts promised edit/write capability ("handle backend/frontend code",
// "write and run tests") — an impossible-to-fulfill grant. write_file must
// now be present for all three, in both the single-file and dir-file forms.
func TestHierarchicalPreset_WorkersHaveWriteFile(t *testing.T) {
	preset := Presets["hierarchical"]

	rendered, err := Render(preset, testData, false)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	var raw struct {
		Agents []struct {
			Name  string   `yaml:"name"`
			Tools []string `yaml:"tools"`
		} `yaml:"agents"`
	}
	if err := yaml.Unmarshal([]byte(rendered["hierarchical.yaml"]), &raw); err != nil {
		t.Fatalf("unmarshal single-file: %v", err)
	}
	if len(raw.Agents) != 3 {
		t.Fatalf("expected 3 agents, got %d", len(raw.Agents))
	}
	for _, agent := range raw.Agents {
		if !contains(agent.Tools, "write_file") {
			t.Errorf("single-file: agent %q should have write_file to back its prompt's promised capability, got tools %v", agent.Name, agent.Tools)
		}
	}

	dirFiles, err := Render(preset, testData, true)
	if err != nil {
		t.Fatalf("Render(dir): %v", err)
	}
	for _, relPath := range []string{"agents/backend-dev.md", "agents/frontend-dev.md", "agents/qa-engineer.md"} {
		content, ok := dirFiles[relPath]
		if !ok {
			t.Fatalf("missing dir file %s", relPath)
		}
		if !strings.Contains(content, "write_file") {
			t.Errorf("dir-file %s should list write_file among its tools", relPath)
		}
	}
}

// Regression: hierarchical's write_file tool (both forms) was added without
// allowed_paths, unlike every other preset's write_file, which restricts
// writes to ["./"] — a plain oversight, not an intentional broader grant.
func TestHierarchicalPreset_WriteFileHasAllowedPaths(t *testing.T) {
	preset := Presets["hierarchical"]

	rendered, err := Render(preset, testData, false)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	var raw struct {
		Tools []struct {
			Name         string   `yaml:"name"`
			AllowedPaths []string `yaml:"allowed_paths"`
		} `yaml:"tools"`
	}
	if err := yaml.Unmarshal([]byte(rendered["hierarchical.yaml"]), &raw); err != nil {
		t.Fatalf("unmarshal single-file: %v", err)
	}
	found := false
	for _, tool := range raw.Tools {
		if tool.Name != "write_file" {
			continue
		}
		found = true
		if len(tool.AllowedPaths) == 0 {
			t.Error("single-file: write_file should have allowed_paths set")
		}
	}
	if !found {
		t.Fatal("single-file: write_file tool not found")
	}

	dirFiles, err := Render(preset, testData, true)
	if err != nil {
		t.Fatalf("Render(dir): %v", err)
	}
	content, ok := dirFiles["tools/write_file.yaml"]
	if !ok {
		t.Fatal("missing dir file tools/write_file.yaml")
	}
	if !strings.Contains(content, "allowed_paths") {
		t.Error("dir-file tools/write_file.yaml should have allowed_paths set")
	}
}

func contains(ss []string, target string) bool {
	for _, s := range ss {
		if s == target {
			return true
		}
	}
	return false
}

// ============================================================
// Preset name / ID parity
// ============================================================

// Regression: the hierarchical preset's registry ID was "hierarchical" but
// its embedded YAML name: field was "hierarchical-team" — every other
// preset has these match exactly. Checked across all presets, not just
// hierarchical, so a future preset can't reintroduce the same drift.
func TestRender_SingleFile_NameMatchesID(t *testing.T) {
	for _, id := range PresetsOrdered {
		preset := Presets[id]
		rendered, err := Render(preset, testData, false)
		if err != nil {
			t.Fatalf("%s: Render: %v", id, err)
		}
		var raw struct {
			Name string `yaml:"name"`
		}
		if err := yaml.Unmarshal([]byte(rendered[id+".yaml"]), &raw); err != nil {
			t.Fatalf("%s: unmarshal: %v", id, err)
		}
		if raw.Name != preset.ID {
			t.Errorf("preset %q: rendered name %q does not match preset ID", id, raw.Name)
		}
	}
}
