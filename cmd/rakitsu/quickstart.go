package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/paupawsan/rakitsu/internal/scaffold"
	"github.com/spf13/cobra"
)

var quickstartCmd = &cobra.Command{
	Use:   "quickstart",
	Short: "Interactive wizard to create and run your first agent project",
	Long: `Creates a new rakitsu project with an interactive wizard.
No YAML knowledge required — pick a template, enter your API key, and run.

Steps:
  1. Pick a template (Chat Bot, Code Reviewer, Research Team, etc.)
  2. Pick a provider (OpenAI, Anthropic, Ollama, LiteLLM)
  3. Enter API key / base URL (or detect from environment)
  4. Generate project in ./rakitsu-project/
  5. Start the web UI

Example:
  rakitsu quickstart`,
	RunE: runQuickstart,
}

func init() {
	rootCmd.AddCommand(quickstartCmd)
}

// quickstartProvider describes one provider choice in the wizard: its
// display name, the scaffold provider id, and what credentials it needs
// guidance for. envVar/needKey drive the Step 3 API-key check/prompt/warn
// flow; baseURLEnvVar/needBaseURL drive the analogous step for providers
// with no fixed endpoint (LiteLLM). Ollama also needs a base_url in
// principle, but rakitsu already defaults it to http://localhost:11434/v1
// at runtime (createLLMProvider) when unset, so it's left without
// guidance here — there's nothing to warn the user about for the common
// local case.
type quickstartProvider struct {
	name          string
	id            string
	envVar        string
	needKey       bool
	baseURLEnvVar string
	needBaseURL   bool
}

var quickstartProviders = []quickstartProvider{
	{name: "OpenAI", id: "openai", envVar: "OPENAI_API_KEY", needKey: true},
	{name: "Anthropic", id: "anthropic", envVar: "ANTHROPIC_API_KEY", needKey: true},
	{name: "Google Gemini", id: "gemini", envVar: "GEMINI_API_KEY", needKey: true},
	{name: "Ollama (local)", id: "ollama"},
	{name: "LiteLLM (proxy)", id: "litellm", envVar: "LITELLM_API_KEY", needKey: true, baseURLEnvVar: "LITELLM_BASE_URL", needBaseURL: true},
	{name: "Codex (ChatGPT subscription, needs `codex login`)", id: "codex"},
}

func runQuickstart(cmd *cobra.Command, args []string) error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println()
	fmt.Println("  Welcome to Rakitsu — The Agent IDE")
	fmt.Println("  =====================================")
	fmt.Println()

	// Step 1: Pick template
	templates := []struct {
		name string
		id   string
		desc string
	}{
		{"Chat Bot", "llm-chat", "Simple conversational assistant"},
		{"Code Reviewer", "code-review", "Multi-agent code review pipeline"},
		{"Research Team", "web-research", "Research agent with reasoning"},
		{"Dev Team", "dev-team", "Planner → Developer → Reviewer pipeline"},
		{"Data Analysis", "data-analysis", "Data analyst with CLI + file tools"},
		{"QA Pipeline", "qa-pipeline", "Test generator → Validator pipeline"},
		{"RAG Assistant", "rag-assistant", "Search knowledge base + synthesise"},
	}

	fmt.Println("  Pick a template:")
	fmt.Println()
	for i, t := range templates {
		fmt.Printf("    %d. %-18s %s\n", i+1, t.name, t.desc)
	}
	fmt.Println()
	fmt.Print("  Enter number [1]: ")
	choice, err := mustReadLine(reader)
	if err != nil {
		return err
	}
	idx := 0
	if choice != "" {
		n, err := strconv.Atoi(strings.TrimSpace(choice))
		if err != nil || n < 1 || n > len(templates) {
			return fmt.Errorf("invalid choice: %s", choice)
		}
		idx = n - 1
	}
	tmpl := templates[idx]
	fmt.Printf("  → %s\n\n", tmpl.name)

	// Step 2: Pick provider
	providers := quickstartProviders

	fmt.Println("  Pick a provider:")
	fmt.Println()
	for i, p := range providers {
		status := ""
		if p.envVar != "" {
			if os.Getenv(p.envVar) != "" {
				status = " (key detected)"
			}
		}
		fmt.Printf("    %d. %s%s\n", i+1, p.name, status)
	}
	fmt.Println()
	fmt.Print("  Enter number [1]: ")
	pChoice, err := mustReadLine(reader)
	if err != nil {
		return err
	}
	pIdx := 0
	if pChoice != "" {
		n, err := strconv.Atoi(strings.TrimSpace(pChoice))
		if err != nil || n < 1 || n > len(providers) {
			return fmt.Errorf("invalid choice: %s", pChoice)
		}
		pIdx = n - 1
	}
	prov := providers[pIdx]
	fmt.Printf("  → %s\n\n", prov.name)

	// Step 3: API key
	apiKey := ""
	if prov.needKey {
		existing := os.Getenv(prov.envVar)
		if existing != "" {
			fmt.Printf("  API key detected from $%s\n\n", prov.envVar)
			apiKey = existing
		} else {
			fmt.Printf("  Enter %s API key: ", prov.name)
			line, err := mustReadLine(reader)
			if err != nil {
				return err
			}
			apiKey = strings.TrimSpace(line)
			if apiKey == "" {
				fmt.Printf("  Warning: no API key provided. Set $%s before running.\n\n", prov.envVar)
			}
		}
	}

	// Step 3b: Base URL — providers with no fixed endpoint (LiteLLM) need
	// one before the generated config will reach anything. Same
	// detect/prompt/warn shape as the API key step above; the value is
	// never written into the generated config, only used for this check
	// (the config always references the env var by name, same as the key).
	baseURL := ""
	if prov.needBaseURL {
		existing := os.Getenv(prov.baseURLEnvVar)
		if existing != "" {
			fmt.Printf("  Base URL detected from $%s\n\n", prov.baseURLEnvVar)
			baseURL = existing
		} else {
			fmt.Printf("  Enter %s base URL (e.g. https://your-proxy-host/v1): ", prov.name)
			line, err := mustReadLine(reader)
			if err != nil {
				return err
			}
			baseURL = strings.TrimSpace(line)
			if baseURL == "" {
				fmt.Printf("  Warning: no base URL provided. Set $%s before running.\n\n", prov.baseURLEnvVar)
			}
		}
	}

	// Step 4: Generate project
	projectDir := "rakitsu-project"
	fmt.Printf("  Project directory [%s]: ", projectDir)
	dirChoice, err := mustReadLine(reader)
	if err != nil {
		return err
	}
	if dirChoice != "" {
		projectDir = strings.TrimSpace(dirChoice)
	}

	absDir, _ := filepath.Abs(projectDir)
	if err := os.MkdirAll(absDir, 0755); err != nil {
		return fmt.Errorf("cannot create directory: %w", err)
	}

	// Step 4b: Single file or modular?
	fmt.Print("  Project structure — modular (agents/, tools/ dirs) or single YAML? [M/s]: ")
	structLine, err := mustReadLine(reader)
	if err != nil {
		return err
	}
	structChoice := strings.ToLower(strings.TrimSpace(structLine))
	useModular := structChoice != "s" && structChoice != "single"

	// Generate config using scaffold
	preset, ok := scaffold.Presets[tmpl.id]
	if !ok {
		return fmt.Errorf("unknown template: %s", tmpl.id)
	}
	model := scaffold.DefaultModel(prov.id)
	data := scaffold.TemplateData{
		Provider:   prov.id,
		Model:      model,
		APIKeyEnv:  scaffold.APIKeyEnvVar(prov.id),
		BaseURLEnv: scaffold.BaseURLEnvVar(prov.id),
	}
	files, err := scaffold.Render(preset, data, useModular)
	if err != nil {
		return fmt.Errorf("scaffold error: %w", err)
	}

	// Step 4c: refuse to silently clobber an already-scaffolded project.
	// Re-running quickstart into an existing project dir would otherwise
	// overwrite already-edited config/agent files with no warning.
	existing := existingQuickstartFiles(files, absDir, useModular)
	if len(existing) > 0 {
		fmt.Printf("\n  %d file(s) already exist in %s and would be overwritten:\n", len(existing), absDir)
		for _, p := range existing {
			fmt.Printf("    %s\n", p)
		}
		fmt.Print("  Overwrite? [y/N]: ")
		confirmLine, err := mustReadLine(reader)
		if err != nil {
			return err
		}
		confirm := strings.ToLower(strings.TrimSpace(confirmLine))
		if confirm != "y" && confirm != "yes" {
			return fmt.Errorf("aborted: refusing to overwrite existing project files")
		}
	}

	// Write files
	var configPath string
	if useModular {
		for relPath, content := range files {
			fullPath := filepath.Join(absDir, relPath)
			if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
				return fmt.Errorf("cannot create directory: %w", err)
			}
			if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
				return fmt.Errorf("cannot write %s: %w", relPath, err)
			}
		}
		configPath = filepath.Join(absDir, "config.yaml")
		fmt.Println()
		fmt.Printf("  Project created at: %s\n", absDir)
		fmt.Printf("  Files: %d (%s)\n\n", len(files), "modular layout")
	} else {
		var yaml string
		for _, content := range files {
			yaml = content
			break
		}
		configPath = filepath.Join(absDir, "config.yaml")
		if err := os.WriteFile(configPath, []byte(yaml), 0644); err != nil {
			return fmt.Errorf("cannot write config: %w", err)
		}
		fmt.Println()
		fmt.Printf("  Project created at: %s\n", absDir)
		fmt.Printf("  Config: %s\n\n", configPath)
	}

	// Step 5: Start serve
	fmt.Print("  Start web UI now? [Y/n]: ")
	startLine, err := mustReadLine(reader)
	if err != nil {
		return err
	}
	startChoice := strings.ToLower(strings.TrimSpace(startLine))
	if startChoice != "n" && startChoice != "no" {
		fmt.Println()
		fmt.Println("  Starting Rakitsu web UI...")
		fmt.Println("  Open http://localhost:9100 in your browser")
		fmt.Println()

		// serve's ConfigStore only scans ".", "./examples", "./configs"
		// relative to the process's working directory, and generated
		// configs use allowed_paths like "./" (resolved against cwd too).
		// Without this chdir, serve keeps running from wherever quickstart
		// was invoked, so the project just created in absDir is invisible
		// in the web UI's config list.
		if err := os.Chdir(absDir); err != nil {
			return fmt.Errorf("cannot switch to project directory: %w", err)
		}

		// Try to open browser
		go openBrowser("http://localhost:9100")

		// Run serve with the project directory. serveCmd is defined with
		// Run (not RunE) — invoke that directly; calling the nil RunE
		// segfaults.
		serveCmd.Flags().Set("port", "9100") //nolint:errcheck
		os.Args = []string{"rakitsu", "serve"}
		serveCmd.Run(serveCmd, []string{})
		return nil
	}

	fmt.Println()
	fmt.Println("  To start later, run:")
	fmt.Printf("    cd %s && rakitsu serve\n", projectDir)
	fmt.Println()
	return nil
}

// readLine reads one line from stdin, trimming the trailing line ending.
// ok is false only when stdin was already at EOF with nothing left to read
// — a real terminal never produces that (Enter always terminates a line),
// so it means quickstart is being driven non-interactively (piped/
// redirected/closed stdin) and the caller must abort rather than silently
// substituting every remaining prompt's bracketed default.
func readLine(reader *bufio.Reader) (line string, ok bool) {
	s, err := reader.ReadString('\n')
	if s == "" && err != nil {
		return "", false
	}
	return strings.TrimRight(s, "\r\n"), true
}

// errNonInteractive is returned by mustReadLine when stdin runs out
// mid-wizard. Its bracketed-default prompts (Overwrite? [y/N], Start web
// UI now? [Y/n]) must never be silently answered by an absent terminal.
var errNonInteractive = errors.New("quickstart needs an interactive terminal — stdin closed with more input expected; run it from a real shell, or use 'rakitsu scaffold <use-case>' for non-interactive project generation")

// mustReadLine wraps readLine for call sites that cannot proceed on EOF.
func mustReadLine(reader *bufio.Reader) (string, error) {
	line, ok := readLine(reader)
	if !ok {
		return "", errNonInteractive
	}
	return line, nil
}

// existingQuickstartFiles returns the absolute paths, under absDir, of any
// rendered scaffold file that already exists on disk. Used to warn before
// a quickstart re-run silently overwrites an already-scaffolded project.
func existingQuickstartFiles(files map[string]string, absDir string, useModular bool) []string {
	var existing []string
	if useModular {
		for relPath := range files {
			if _, err := os.Stat(filepath.Join(absDir, relPath)); err == nil {
				existing = append(existing, filepath.Join(absDir, relPath))
			}
		}
	} else if _, err := os.Stat(filepath.Join(absDir, "config.yaml")); err == nil {
		existing = append(existing, filepath.Join(absDir, "config.yaml"))
	}
	return existing
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return
	}
	cmd.Run()
}
