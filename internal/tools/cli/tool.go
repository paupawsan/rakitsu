// Package cli provides a CLI tool implementation with security sandboxing.
// It executes shell commands with multiple layers of security.
package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/paupawsan/rakitsu/internal/config"
)

// System-level whitelist (hard-coded for security)
// These are the ONLY commands allowed to be executed
var systemWhitelist = map[string]bool{
	// File operations (safe)
	"ls":   true,
	"cat":  true,
	"grep": true,
	"find": true,
	"head": true,
	"tail": true,
	"wc":   true,
	"sort": true,
	"uniq": true,
	"diff": true,
	"tree": true,

	// Development tools
	"git":     true,
	"npm":     true,
	"node":    true,
	"go":      true,
	"python3": true,
	"pip3":    true,

	// Kubernetes
	"kubectl": true,
	"helm":    true,

	// Docker (limited)
	"docker": true,

	// Shell wrappers — needed for command templates of the form
	// `sh -c 'git {{args}}'`. The shell payload is linted against
	// blockedCommands at buildCommand time (see lintShellPayload).
	// This lint is best-effort, not a security boundary — rakitsu
	// runs as the local user and ultimately trusts its config.
	"sh":   true,
	"bash": true,
}

// Blocked commands (never allowed, even if in whitelist)
// These are explicitly blocked for safety
var blockedCommands = map[string]bool{
	"rm":       true,
	"rmdir":    true,
	"sudo":     true,
	"su":       true,
	"chmod":    true,
	"chown":    true,
	"chgrp":    true,
	"dd":       true,
	"mkfs":     true,
	"fdisk":    true,
	"shutdown": true,
	"reboot":   true,
	"halt":     true,
	"init":     true,
	"kill":     true,
	"killall":  true,
	"pkill":    true,
	"mv":       true, // Can be destructive
	"cp":       true, // Can overwrite files
}

// sensitiveServerEnvVars are rakitsu-serve control-plane secrets that must
// never reach a spawned tool subprocess. exec.Cmd inherits the parent's full
// environment when Env is left nil, which would otherwise hand these tokens
// to any python3/bash/node/etc. tool call an agent makes — letting a run
// started WITH the correct API token read that same token back out of its
// own environment and exfiltrate it. Names must match internal/server's
// apiTokenEnv (auth.go) and sessionMsgTokenEnv (session_message.go);
// duplicated here rather than imported to keep this package independent of
// internal/server.
var sensitiveServerEnvVars = map[string]bool{
	"RAKITSU_API_TOKEN":         true,
	"RAKITSU_SESSION_MSG_TOKEN": true,
}

// scrubbedEnviron returns os.Environ() with sensitiveServerEnvVars removed.
func scrubbedEnviron() []string {
	src := os.Environ()
	out := make([]string, 0, len(src))
	for _, kv := range src {
		name := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			name = kv[:i]
		}
		if sensitiveServerEnvVars[name] {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// DefaultMaxOutputBytes is the default cap on tool output when the config
// does not specify one. Picked to be generous enough for typical `git log`
// and `grep` output but small enough to keep a single tool call from
// blowing out the model's context window. Regression: B15 (2026-04-05),
// where an unbounded `git log` returned 205KB in one call on the weekly
// dogfood run and consumed the entire context.
const DefaultMaxOutputBytes = 8192

// truncateOutput caps s at max bytes. If max <= 0, no truncation is
// applied (caller opted out with -1). When truncation happens, the
// returned string ends with a `... [truncated N bytes, original M]`
// marker so the agent can see data was elided. Truncation prefers a
// clean line boundary near the cap when one exists within the last
// 200 bytes, falling back to a hard byte cut otherwise.
func truncateOutput(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	cut := max
	// Prefer a line boundary within the last 200 bytes of the cap so we
	// don't slice mid-line.
	if lb := strings.LastIndexByte(s[:cut], '\n'); lb >= 0 && cut-lb < 200 {
		cut = lb
	}
	elided := len(s) - cut
	return s[:cut] + fmt.Sprintf("\n... [truncated %d bytes, original %d bytes]", elided, len(s))
}

// Tool implements the Tool interface for CLI commands with security sandboxing
type Tool struct {
	name          string
	description   string
	command       []string // Base command (e.g., ["kubectl", "get"])
	parameters    map[string]config.Parameter
	sandbox       *config.SandboxConfig
	workingDir    string          // Working directory for command execution
	userWhitelist map[string]bool // User-configured allowed commands
}

// NewTool creates a new CLI tool from configuration.
// allowedCommands is an optional list of user-configured commands to allow
// beyond the built-in system whitelist.
// splitCommand parses a command template string into argv parts, respecting
// single-quoted sections so that patterns like `sh -c 'git {{args}}'` produce
// three parts (sh, -c, `git {{args}}`) instead of four.
//
// This is a minimal shell-aware splitter, not a full shell lexer:
//   - Whitespace separates tokens outside quotes.
//   - Single quotes (') delimit a literal section; its contents (including
//     any double quotes and whitespace) are preserved as one token.
//   - Double quotes are NOT treated specially — they pass through as
//     literal characters. This is deliberate: the common pattern is
//     `sh -c 'git log --since="7 days ago"'`, where the double quotes
//     belong to the payload, not the outer template.
//   - Backslash escapes are not supported. Keep command templates simple.
//
// Regression: B20 (2026-04-05) — the previous implementation used
// strings.Fields which whitespace-split `"sh -c 'git {{args}}'"` into
// `["sh", "-c", "'git", "{{args}}'"]`, breaking every shell-wrapped
// command template in the repo.
func splitCommand(s string) []string {
	var parts []string
	var cur strings.Builder
	inSingle := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch == '\'' && !inSingle:
			inSingle = true
		case ch == '\'' && inSingle:
			inSingle = false
		case ch == ' ' || ch == '\t':
			if inSingle {
				cur.WriteByte(ch)
			} else if cur.Len() > 0 {
				parts = append(parts, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteByte(ch)
		}
	}
	if cur.Len() > 0 {
		parts = append(parts, cur.String())
	}
	return parts
}

// lintShellPayload does a best-effort scan of a shell -c payload for
// standalone invocations of commands in blockedCommands. It matches only
// whole tokens (so `--grep "arm64"` doesn't trip on the "rm" substring).
// Returns a non-nil error describing the first blocked command found.
//
// Security note: this is a footgun fence, NOT a security boundary. A
// determined attacker with config-write access can bypass it many ways
// (env vars, command substitution, unusual quoting). It exists to catch
// obvious mistakes, not to defend against hostile configs. Rakitsu
// ultimately runs as the local user with the user's trust.
func lintShellPayload(payload string) error {
	// Tokenize on shell metacharacters and whitespace so that `; rm -rf /`
	// and `git log && rm` both expose "rm" as a standalone token.
	tokenSep := func(r rune) bool {
		switch r {
		case ' ', '\t', '\n', ';', '|', '&', '(', ')', '<', '>', '`':
			return true
		}
		return false
	}
	for _, tok := range strings.FieldsFunc(payload, tokenSep) {
		// Strip leading $() or backticks artifacts — best-effort
		tok = strings.TrimLeft(tok, "$(")
		if blockedCommands[tok] {
			return fmt.Errorf("shell payload contains blocked command %q", tok)
		}
	}
	return nil
}

func NewTool(def *config.ToolDefinition, allowedCommands ...[]string) *Tool {
	// Parse command into parts using shell-aware splitting so that
	// templates like `sh -c 'git {{args}}'` parse to 3 tokens, not 4.
	// See splitCommand() comment for the B20 regression details.
	var cmdParts []string
	if def.Command != "" {
		cmdParts = splitCommand(def.Command)
	}

	// Default sandbox config
	sandbox := def.Sandbox
	if sandbox == nil {
		sandbox = &config.SandboxConfig{
			Type: "local_restricted",
		}
	}

	// Build user whitelist map
	userWL := map[string]bool{}
	if len(allowedCommands) > 0 && allowedCommands[0] != nil {
		for _, cmd := range allowedCommands[0] {
			userWL[cmd] = true
		}
	}

	return &Tool{
		name:          def.Name,
		description:   def.Description,
		command:       cmdParts,
		parameters:    def.Parameters,
		sandbox:       sandbox,
		workingDir:    def.WorkingDir,
		userWhitelist: userWL,
	}
}

// GetName returns the tool's name
func (t *Tool) GetName() string {
	return t.name
}

// GetDescription returns the tool's description
func (t *Tool) GetDescription() string {
	return t.description
}

// GetParametersSchema returns the JSON Schema for parameters
func (t *Tool) GetParametersSchema() map[string]interface{} {
	schema := map[string]interface{}{
		"type":       "object",
		"properties": make(map[string]interface{}),
		"required":   []string{},
	}

	props := schema["properties"].(map[string]interface{})
	var required []string

	for name, param := range t.parameters {
		prop := map[string]interface{}{
			"type":        param.Type,
			"description": param.Description,
		}
		if param.Default != nil {
			prop["default"] = param.Default
		}
		if len(param.Enum) > 0 {
			prop["enum"] = param.Enum
		}
		props[name] = prop

		if param.Required {
			required = append(required, name)
		}
	}

	schema["required"] = required
	return schema
}

// Execute runs the CLI command with security sandboxing
func (t *Tool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	// 1. Build full command with arguments
	fullCmd, err := t.buildCommand(args)
	if err != nil {
		return "", err
	}

	// 2. Validate against whitelist
	if !t.isCommandAllowed(fullCmd[0]) {
		return "", &SecurityError{
			Command: fullCmd[0],
			Reason:  "command not in system whitelist",
		}
	}

	// 3. Execute based on sandbox type
	switch t.sandbox.Type {
	case "docker":
		return t.executeInDocker(ctx, fullCmd)
	case "local_restricted":
		fallthrough
	default:
		return t.executeLocalRestricted(ctx, fullCmd)
	}
}

// buildCommand builds the full command with arguments
func (t *Tool) buildCommand(args map[string]interface{}) ([]string, error) {
	// Start with base command
	cmd := make([]string, len(t.command))
	copy(cmd, t.command)

	// If command has placeholders, substitute them.
	// IMPORTANT: a single command part may contain multiple distinct
	// placeholders (e.g. a shell payload like `sh -c 'grep "{{pattern}}" "{{file}}"'`).
	// Iterate until no more recognized placeholders remain, not just once.
	// See New-B-cli-multi-placeholder in STABILITY-gate.md for the original
	// regression (dogfood-01 search tool, 2026-04-05).
	for i, part := range cmd {
		for strings.Contains(part, "{{") && strings.Contains(part, "}}") {
			start := strings.Index(part, "{{") + 2
			end := strings.Index(part, "}}")
			if end <= start {
				break // malformed; bail to avoid infinite loop
			}
			placeholder := strings.TrimSpace(part[start:end])

			val, ok := args[placeholder]
			if !ok {
				// Unrecognized placeholder — stop so we don't infinite-loop
				// on a template variable the caller forgot to pass.
				break
			}
			part = strings.Replace(part, "{{"+placeholder+"}}", fmt.Sprintf("%v", val), -1)
		}
		cmd[i] = part
	}

	// Add additional arguments
	for name, param := range t.parameters {
		if val, ok := args[name]; ok {
			// Skip if it's a placeholder that was already substituted
			isPlaceholder := false
			for _, part := range t.command {
				if strings.Contains(part, "{{"+name+"}}") {
					isPlaceholder = true
					break
				}
			}
			if !isPlaceholder {
				cmd = append(cmd, fmt.Sprintf("--%s=%v", name, val))
			}
		} else if param.Required {
			return nil, fmt.Errorf("required parameter '%s' not provided", name)
		}
	}

	// B20 mitigation: when the assembled command is a shell wrapper
	// (sh -c '<payload>' or bash -c '<payload>'), lint the post-substitution
	// payload for standalone invocations of blocked commands. This catches
	// obvious cases like `log; rm -rf /` without blocking legitimate uses
	// like `log --grep "arm64"` (where "rm" is only a substring).
	if len(cmd) >= 3 && (cmd[0] == "sh" || cmd[0] == "bash") && cmd[1] == "-c" {
		if err := lintShellPayload(cmd[2]); err != nil {
			return nil, err
		}
	}

	return cmd, nil
}

// isCommandAllowed checks if a command is allowed
func (t *Tool) isCommandAllowed(cmd string) bool {
	// Get base command name
	baseCmd := filepath.Base(cmd)

	// Check blocked list first (always takes precedence)
	if blockedCommands[baseCmd] {
		return false
	}

	// Check system whitelist
	if systemWhitelist[baseCmd] {
		return true
	}

	// Check user-configured whitelist
	return t.userWhitelist[baseCmd]
}

// executeLocalRestricted executes the command locally with restrictions
func (t *Tool) executeLocalRestricted(ctx context.Context, cmd []string) (string, error) {
	// Set environment variables
	if t.sandbox != nil {
		for _, path := range t.sandbox.AllowedPaths {
			// Validate that any path arguments are within allowed paths
			for _, arg := range cmd[1:] {
				if strings.HasPrefix(arg, "/") || strings.HasPrefix(arg, "./") {
					absArg, _ := filepath.Abs(arg)
					absAllowed, _ := filepath.Abs(path)
					if absArg != absAllowed && !strings.HasPrefix(absArg, absAllowed+string(filepath.Separator)) {
						return "", &SecurityError{
							Command: strings.Join(cmd, " "),
							Reason:  fmt.Sprintf("path '%s' is not in allowed paths", arg),
						}
					}
				}
			}
		}
	}

	// Set timeout
	timeout := 30 * time.Second
	if t.sandbox != nil && t.sandbox.ResourceLimits.TimeoutSec > 0 {
		timeout = time.Duration(t.sandbox.ResourceLimits.TimeoutSec) * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	execCmd := exec.CommandContext(ctx, cmd[0], cmd[1:]...)
	execCmd.Env = scrubbedEnviron()

	// Set working directory: explicit workingDir first, then sandbox allowed path
	if t.workingDir != "" {
		execCmd.Dir = t.workingDir
	} else if t.sandbox != nil && len(t.sandbox.AllowedPaths) > 0 {
		execCmd.Dir = t.sandbox.AllowedPaths[0]
	}

	// Run in its own process group so a timeout kills the whole tree, not
	// just the direct child — matters for whitelisted shell wrappers
	// (sh -c/bash -c) that spawn further children of their own.
	setNewProcessGroup(execCmd)
	execCmd.Cancel = func() error { return killProcessGroup(execCmd) }

	// Capture output
	output, err := execCmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("command timed out after %v", timeout)
		}
		return truncateOutput(string(output), t.maxOutputBytes()), fmt.Errorf("command failed: %w", err)
	}

	return truncateOutput(string(output), t.maxOutputBytes()), nil
}

// maxOutputBytes returns the configured cap for this tool invocation, or
// DefaultMaxOutputBytes when the config leaves it unset (0). A negative
// value disables truncation.
func (t *Tool) maxOutputBytes() int {
	if t.sandbox == nil || t.sandbox.ResourceLimits.MaxOutputBytes == 0 {
		return DefaultMaxOutputBytes
	}
	return t.sandbox.ResourceLimits.MaxOutputBytes
}

// buildDockerArgs constructs the "docker run" argument list for cmd under
// sandbox. Split out from executeInDocker so the hardening flags below are
// unit-testable without actually invoking Docker.
func buildDockerArgs(sandbox *config.SandboxConfig, cmd []string) []string {
	dockerCmd := []string{"run", "--rm"}

	// Root filesystem is read-only by default; only the explicit mounts below
	// are writable. --tmpfs gives commands that need scratch space (compilers,
	// package managers, etc.) somewhere to write without punching a hole in
	// the read-only root. --security-opt=no-new-privileges blocks setuid/
	// setgid privilege escalation inside the container.
	dockerCmd = append(dockerCmd, "--read-only", "--tmpfs", "/tmp:rw,noexec,nosuid,size=64m")
	dockerCmd = append(dockerCmd, "--security-opt", "no-new-privileges")

	// Add working directory mount
	if sandbox.MountWorkdir {
		cwd, _ := os.Getwd()
		dockerCmd = append(dockerCmd, "-v", cwd+":/workspace")
		dockerCmd = append(dockerCmd, "-w", "/workspace")
	}

	// Add resource limits
	if sandbox.ResourceLimits.CPULimit != "" {
		dockerCmd = append(dockerCmd, "--cpus", sandbox.ResourceLimits.CPULimit)
	}
	if sandbox.ResourceLimits.MemoryLimit != "" {
		dockerCmd = append(dockerCmd, "--memory", sandbox.ResourceLimits.MemoryLimit)
	}

	// Network isolation
	if sandbox.NetworkIsolated {
		dockerCmd = append(dockerCmd, "--network", "none")
	}

	// Add image
	image := sandbox.Image
	if image == "" {
		image = "alpine:latest"
	}
	dockerCmd = append(dockerCmd, image)

	// Add the actual command
	dockerCmd = append(dockerCmd, cmd...)
	return dockerCmd
}

// executeInDocker executes the command in a Docker container
// This provides stronger isolation but requires Docker to be installed
func (t *Tool) executeInDocker(ctx context.Context, cmd []string) (string, error) {
	dockerArgs := buildDockerArgs(t.sandbox, cmd)

	// Execute. Scrub sensitive env from the local `docker` CLI process itself;
	// the container's own environment is separate and unaffected (docker run
	// does not forward host env unless -e/--env-file is passed, which this
	// command builder does not do).
	execCmd := exec.CommandContext(ctx, "docker", dockerArgs...)
	execCmd.Env = scrubbedEnviron()
	output, err := execCmd.CombinedOutput()
	if err != nil {
		return truncateOutput(string(output), t.maxOutputBytes()), fmt.Errorf("docker execution failed: %w", err)
	}

	return truncateOutput(string(output), t.maxOutputBytes()), nil
}

// SecurityError is returned when a command is blocked by security rules
type SecurityError struct {
	Command string
	Reason  string
}

func (e *SecurityError) Error() string {
	return fmt.Sprintf("security error: command '%s' blocked: %s", e.Command, e.Reason)
}
