package main

import (
	"bufio"
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
  3. Enter API key (or detect from environment)
  4. Generate project in ./rakitsu-project/
  5. Start the web UI

Example:
  rakitsu quickstart`,
	RunE: runQuickstart,
}

func init() {
	rootCmd.AddCommand(quickstartCmd)
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
	choice := readLine(reader)
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
	providers := []struct {
		name    string
		id      string
		envVar  string
		needKey bool
	}{
		{"OpenAI", "openai", "OPENAI_API_KEY", true},
		{"Anthropic", "anthropic", "ANTHROPIC_API_KEY", true},
		{"Google Gemini", "gemini", "GEMINI_API_KEY", true},
		{"Ollama (local)", "ollama", "", false},
		{"LiteLLM (proxy)", "litellm", "", false},
	}

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
	pChoice := readLine(reader)
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
			apiKey = strings.TrimSpace(readLine(reader))
			if apiKey == "" {
				fmt.Printf("  Warning: no API key provided. Set $%s before running.\n\n", prov.envVar)
			}
		}
	}

	// Step 4: Generate project
	projectDir := "rakitsu-project"
	fmt.Printf("  Project directory [%s]: ", projectDir)
	dirChoice := readLine(reader)
	if dirChoice != "" {
		projectDir = strings.TrimSpace(dirChoice)
	}

	absDir, _ := filepath.Abs(projectDir)
	if err := os.MkdirAll(absDir, 0755); err != nil {
		return fmt.Errorf("cannot create directory: %w", err)
	}

	// Step 4b: Single file or modular?
	fmt.Print("  Project structure — modular (agents/, tools/ dirs) or single YAML? [M/s]: ")
	structChoice := strings.ToLower(strings.TrimSpace(readLine(reader)))
	useModular := structChoice != "s" && structChoice != "single"

	// Generate config using scaffold
	preset, ok := scaffold.Presets[tmpl.id]
	if !ok {
		return fmt.Errorf("unknown template: %s", tmpl.id)
	}
	model := scaffold.ProviderDefaults[prov.id]
	if model == "" {
		model = "gpt-4o-mini"
	}
	data := scaffold.TemplateData{
		Provider:  prov.id,
		Model:     model,
		APIKeyEnv: scaffold.APIKeyEnvVar(prov.id),
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
		confirm := strings.ToLower(strings.TrimSpace(readLine(reader)))
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
	startChoice := strings.ToLower(strings.TrimSpace(readLine(reader)))
	if startChoice != "n" && startChoice != "no" {
		fmt.Println()
		fmt.Println("  Starting Rakitsu web UI...")
		fmt.Println("  Open http://localhost:9100 in your browser")
		fmt.Println()

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

func readLine(reader *bufio.Reader) string {
	line, _ := reader.ReadString('\n')
	return strings.TrimRight(line, "\r\n")
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
