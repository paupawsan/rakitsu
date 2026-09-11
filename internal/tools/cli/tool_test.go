package cli

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/paupawsan/rakitsu/internal/config"
)

// ============================================================
// Helpers
// ============================================================

func newTool(cmd string, params map[string]config.Parameter, allowedPaths ...[]string) *Tool {
	def := &config.ToolDefinition{
		Name:        "test-tool",
		Description: "test",
		Command:     cmd,
		Parameters:  params,
	}
	if len(allowedPaths) > 0 {
		def.Sandbox = &config.SandboxConfig{
			Type:         "local_restricted",
			AllowedPaths: allowedPaths[0],
		}
	}
	return NewTool(def)
}

func newToolWithUserWhitelist(cmd string, extra []string) *Tool {
	def := &config.ToolDefinition{
		Name:    "test-tool",
		Command: cmd,
	}
	return NewTool(def, extra)
}

// ============================================================
// isCommandAllowed — security whitelist/blocklist
// ============================================================

func TestIsCommandAllowed_BlockedCommand(t *testing.T) {
	tool := newTool("ls", nil)
	for _, cmd := range []string{"rm", "sudo", "kill", "mv", "cp", "chmod"} {
		if tool.isCommandAllowed(cmd) {
			t.Errorf("blocked command %q should not be allowed", cmd)
		}
	}
}

func TestIsCommandAllowed_PathBasedBypass(t *testing.T) {
	// /bin/rm should still be blocked — filepath.Base strips the path
	tool := newTool("ls", nil)
	if tool.isCommandAllowed("/bin/rm") {
		t.Error("/bin/rm should be blocked (base is 'rm')")
	}
	if tool.isCommandAllowed("/usr/bin/sudo") {
		t.Error("/usr/bin/sudo should be blocked")
	}
}

func TestIsCommandAllowed_Whitelisted(t *testing.T) {
	tool := newTool("ls", nil)
	for _, cmd := range []string{"ls", "cat", "grep", "git", "go", "kubectl"} {
		if !tool.isCommandAllowed(cmd) {
			t.Errorf("whitelisted command %q should be allowed", cmd)
		}
	}
}

func TestIsCommandAllowed_UnknownCommand(t *testing.T) {
	tool := newTool("ls", nil)
	if tool.isCommandAllowed("curl") {
		t.Error("unlisted command 'curl' should not be allowed")
	}
	if tool.isCommandAllowed("wget") {
		t.Error("unlisted command 'wget' should not be allowed")
	}
}

// ============================================================
// Execute — security errors via Execute()
// ============================================================

func TestExecute_BlockedCommand_SecurityError(t *testing.T) {
	tool := newTool("rm", nil)
	_, err := tool.Execute(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error for blocked command")
	}
	var se *SecurityError
	if !errors.As(err, &se) {
		t.Errorf("expected *SecurityError, got %T: %v", err, err)
	}
	if se.Command == "" {
		t.Error("SecurityError.Command should be set")
	}
}

func TestExecute_UnlistedCommand_SecurityError(t *testing.T) {
	tool := newTool("curl", nil)
	_, err := tool.Execute(context.Background(), map[string]interface{}{})
	var se *SecurityError
	if !errors.As(err, &se) {
		t.Errorf("expected *SecurityError for unlisted command, got %T: %v", err, err)
	}
}

func TestExecute_UserWhitelist_Allowed(t *testing.T) {
	// "echo" is not in the system whitelist (and not blocked), so this only
	// succeeds if the user whitelist actually grants it — a regression that
	// broke the user-whitelist override would surface here as an error.
	tool := newToolWithUserWhitelist("echo hello", []string{"echo"})
	out, err := tool.Execute(context.Background(), map[string]interface{}{})
	if err != nil {
		t.Fatalf("expected user-whitelisted command to succeed, got error: %v", err)
	}
	if !strings.Contains(out, "hello") {
		t.Errorf("expected 'hello' in output, got %q", out)
	}
}

// ============================================================
// buildCommand — argument substitution
// ============================================================

func TestBuildCommand_Placeholder(t *testing.T) {
	// Build a tool with a placeholder in the command
	def := &config.ToolDefinition{
		Name:    "cat-file",
		Command: "cat {{file}}",
		Parameters: map[string]config.Parameter{
			"file": {Type: "string", Description: "file to read", Required: true},
		},
	}
	tool := NewTool(def)
	cmd, err := tool.buildCommand(map[string]interface{}{"file": "/etc/hostname"})
	if err != nil {
		t.Fatalf("buildCommand failed: %v", err)
	}
	if len(cmd) < 2 || cmd[1] != "/etc/hostname" {
		t.Errorf("expected cmd[1]='/etc/hostname', got %v", cmd)
	}
}

func TestBuildCommand_RequiredParamMissing(t *testing.T) {
	def := &config.ToolDefinition{
		Name:    "tool",
		Command: "ls",
		Parameters: map[string]config.Parameter{
			"dir": {Type: "string", Required: true},
		},
	}
	tool := NewTool(def)
	_, err := tool.buildCommand(map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error for missing required parameter")
	}
}

func TestBuildCommand_OptionalParam_Appended(t *testing.T) {
	def := &config.ToolDefinition{
		Name:    "tool",
		Command: "ls",
		Parameters: map[string]config.Parameter{
			"format": {Type: "string", Required: false},
		},
	}
	tool := NewTool(def)
	cmd, err := tool.buildCommand(map[string]interface{}{"format": "long"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	joined := strings.Join(cmd, " ")
	if !strings.Contains(joined, "format=long") {
		t.Errorf("expected --format=long in command, got %q", joined)
	}
}

// ============================================================
// GetParametersSchema
// ============================================================

func TestGetParametersSchema_Structure(t *testing.T) {
	def := &config.ToolDefinition{
		Name:    "tool",
		Command: "ls",
		Parameters: map[string]config.Parameter{
			"path":   {Type: "string", Description: "dir path", Required: true},
			"format": {Type: "string", Description: "output format", Required: false},
		},
	}
	tool := NewTool(def)
	schema := tool.GetParametersSchema()

	if schema["type"] != "object" {
		t.Errorf("schema type should be 'object', got %v", schema["type"])
	}
	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("schema.properties should be map[string]interface{}")
	}
	if _, ok := props["path"]; !ok {
		t.Error("schema.properties should contain 'path'")
	}
	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("schema.required should be []string")
	}
	found := false
	for _, r := range required {
		if r == "path" {
			found = true
		}
	}
	if !found {
		t.Error("'path' should be in required list")
	}
}

// ============================================================
// Integration — real execution (safe commands only)
// ============================================================

func TestExecute_LS_Success(t *testing.T) {
	tool := newTool("ls", nil)
	out, err := tool.Execute(context.Background(), map[string]interface{}{})
	if err != nil {
		t.Fatalf("ls should succeed: %v", err)
	}
	if out == "" {
		t.Error("ls should produce non-empty output")
	}
}

func TestExecute_ContextCancelled(t *testing.T) {
	tool := newTool("ls", nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	// Command may still succeed (ls is fast) or fail — just verify no panic
	_, _ = tool.Execute(ctx, map[string]interface{}{})
}

// ============================================================
// B20 regression — shell-wrapper command templates
// ============================================================

func TestSplitCommand_SimpleWhitespace(t *testing.T) {
	got := splitCommand("git log --oneline")
	want := []string{"git", "log", "--oneline"}
	if len(got) != len(want) {
		t.Fatalf("expected %d parts, got %d: %v", len(want), len(got), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("part[%d]: expected %q, got %q", i, w, got[i])
		}
	}
}

func TestSplitCommand_ShellWrapper(t *testing.T) {
	// The exact pattern from examples/single/02-single-agent/config.yaml and
	// examples/dogfood/01-weekly-status/config.yaml
	got := splitCommand("sh -c 'git {{args}}'")
	want := []string{"sh", "-c", "git {{args}}"}
	if len(got) != len(want) {
		t.Fatalf("expected %d parts, got %d: %v", len(want), len(got), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("part[%d]: expected %q, got %q", i, w, got[i])
		}
	}
}

func TestSplitCommand_NestedDoubleQuotesInSingle(t *testing.T) {
	// The payload inside single quotes can contain double quotes untouched —
	// this is what lets `git log --since="7 days ago"` survive templating.
	got := splitCommand(`sh -c 'git log --since="7 days ago" --oneline'`)
	want := []string{"sh", "-c", `git log --since="7 days ago" --oneline`}
	if len(got) != len(want) {
		t.Fatalf("expected %d parts, got %d: %v", len(want), len(got), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("part[%d]: expected %q, got %q", i, w, got[i])
		}
	}
}

func TestBuildCommand_ShellWrapperPlaceholder(t *testing.T) {
	// B20 regression: confirm that {{args}} substitution happens on the
	// shell payload slot (not on the split pieces of the payload).
	def := &config.ToolDefinition{
		Name:    "git-shell",
		Command: "sh -c 'git {{args}}'",
		Parameters: map[string]config.Parameter{
			"args": {Type: "string", Required: true},
		},
	}
	tool := NewTool(def)
	cmd, err := tool.buildCommand(map[string]interface{}{
		"args": `log --since="7 days ago" --oneline`,
	})
	if err != nil {
		t.Fatalf("buildCommand failed: %v", err)
	}
	if len(cmd) != 3 {
		t.Fatalf("expected 3 parts (sh, -c, payload), got %d: %v", len(cmd), cmd)
	}
	if cmd[0] != "sh" {
		t.Errorf("cmd[0]: expected %q, got %q", "sh", cmd[0])
	}
	if cmd[1] != "-c" {
		t.Errorf("cmd[1]: expected %q, got %q", "-c", cmd[1])
	}
	expected := `git log --since="7 days ago" --oneline`
	if cmd[2] != expected {
		t.Errorf("cmd[2]: expected %q, got %q", expected, cmd[2])
	}
}

func TestIsCommandAllowed_ShAndBashInWhitelist(t *testing.T) {
	// B20: sh and bash must be in the system whitelist so that
	// `sh -c '...'` command templates work as documented.
	tool := newTool("sh", nil)
	if !tool.isCommandAllowed("sh") {
		t.Error("sh should be in system whitelist (needed for shell-wrapper command templates)")
	}
	if !tool.isCommandAllowed("bash") {
		t.Error("bash should be in system whitelist (needed for shell-wrapper command templates)")
	}
}

func TestBuildCommand_ShellPayloadBlocklistLint(t *testing.T) {
	// B20 mitigation: when the shell payload (after {{args}} substitution)
	// contains a blocked command name as a standalone token, reject it.
	def := &config.ToolDefinition{
		Name:    "git-shell",
		Command: "sh -c 'git {{args}}'",
		Parameters: map[string]config.Parameter{
			"args": {Type: "string", Required: true},
		},
	}
	tool := NewTool(def)
	_, err := tool.buildCommand(map[string]interface{}{
		"args": `log --oneline; rm -rf /tmp/whatever`,
	})
	if err == nil {
		t.Fatal("expected error for shell payload containing blocked command 'rm'")
	}
	if !strings.Contains(err.Error(), "blocked") && !strings.Contains(err.Error(), "rm") {
		t.Errorf("error should mention the blocked command, got: %v", err)
	}
}

func TestBuildCommand_ShellPayloadBlocklistAllowsSubstrings(t *testing.T) {
	// The lint should match standalone command tokens, not substrings inside args.
	// `git log --grep "arm"` contains "rm" as a substring of "arm" but is NOT
	// invoking rm — the lint must not reject this.
	def := &config.ToolDefinition{
		Name:    "git-shell",
		Command: "sh -c 'git {{args}}'",
		Parameters: map[string]config.Parameter{
			"args": {Type: "string", Required: true},
		},
	}
	tool := NewTool(def)
	_, err := tool.buildCommand(map[string]interface{}{
		"args": `log --grep "arm64"`,
	})
	if err != nil {
		t.Errorf("substring 'rm' inside 'arm64' should not trigger blocklist: %v", err)
	}
}

func TestBuildCommand_ShellPayloadBlocklistAllowsSubcommands(t *testing.T) {
	// Regression: git's own subcommands share a name with a blocked
	// standalone command ("init", "mv") -- the lint must only check the
	// first word of each command segment, not every token in the payload.
	// Found live: dogfood scenario 14's bootstrap step ran `git init -q`
	// and got rejected with "shell payload contains blocked command init",
	// even though "init" here is git's subcommand, not the system command.
	def := &config.ToolDefinition{
		Name:    "git-shell",
		Command: "sh -c 'git {{args}}'",
		Parameters: map[string]config.Parameter{
			"args": {Type: "string", Required: true},
		},
	}
	tool := NewTool(def)

	cases := []string{
		"init -q",
		"mv old.txt new.txt",
	}
	for _, args := range cases {
		if _, err := tool.buildCommand(map[string]interface{}{"args": args}); err != nil {
			t.Errorf("git subcommand %q should not trigger blocklist: %v", args, err)
		}
	}
}

func TestBuildCommand_ShellPayloadBlocklistCatchesStandaloneAcrossSeparators(t *testing.T) {
	// The fix for the subcommand false positive must not weaken detection
	// of a genuinely standalone blocked command after any shell separator.
	def := &config.ToolDefinition{
		Name:    "sh-tool",
		Command: "sh -c '{{cmd}}'",
		Parameters: map[string]config.Parameter{
			"cmd": {Type: "string", Required: true},
		},
	}
	tool := NewTool(def)

	cases := map[string]string{
		"leading semicolon":  "echo hi; rm -rf /tmp/x",
		"leading &&":         "echo hi && rm -rf /tmp/x",
		"leading pipe":       "cat file | rm -rf /tmp/x",
		"sole invocation":    "rm -rf /tmp/x",
		"inside a subshell":  "(cd /tmp && rm -rf x)",
		"backtick expansion": "echo `rm -rf /tmp/x`",
	}
	for name, cmd := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := tool.buildCommand(map[string]interface{}{"cmd": cmd})
			if err == nil {
				t.Fatalf("expected blocked-command error for %q, got none", cmd)
			}
			if !strings.Contains(err.Error(), "rm") {
				t.Errorf("error should mention 'rm', got: %v", err)
			}
		})
	}
}

func TestBuildCommand_ShellPayloadBlocklistCatchesReExecWrappers(t *testing.T) {
	// Regression: restricting the lint to a segment's first word (the fix
	// above) stopped it from catching a blocked command hiding behind a
	// wrapper that re-invokes a later word as the actual command to run —
	// unlike `git mv`, where "mv" is git's own subcommand and never
	// executes as a standalone command.
	def := &config.ToolDefinition{
		Name:    "sh-tool",
		Command: "sh -c '{{cmd}}'",
		Parameters: map[string]config.Parameter{
			"cmd": {Type: "string", Required: true},
		},
	}
	tool := NewTool(def)

	cases := map[string]string{
		"xargs":      "xargs rm -rf /tmp/x",
		"find -exec": "find . -exec rm -rf {} +",
		"env":        "env rm -rf /tmp/x",
	}
	for name, cmd := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := tool.buildCommand(map[string]interface{}{"cmd": cmd})
			if err == nil {
				t.Fatalf("expected blocked-command error for %q, got none", cmd)
			}
			if !strings.Contains(err.Error(), "rm") {
				t.Errorf("error should mention 'rm', got: %v", err)
			}
		})
	}
}

// ============================================================
// Stress — concurrent Execute calls
// ============================================================

func TestStress_ConcurrentExecute(t *testing.T) {
	tool := newTool("ls", nil)
	const N = 30
	var wg sync.WaitGroup
	var successes atomic.Int64

	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			_, err := tool.Execute(context.Background(), map[string]interface{}{})
			if err == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := successes.Load(); got != N {
		t.Errorf("expected %d successes, got %d", N, got)
	}
}

func TestStress_ConcurrentSecurityChecks(t *testing.T) {
	tool := newTool("ls", nil)
	const N = 100
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(idx int) {
			defer wg.Done()
			if idx%2 == 0 {
				tool.isCommandAllowed("ls")
			} else {
				tool.isCommandAllowed("rm")
			}
		}(i)
	}
	wg.Wait()
}

// ============================================================
// B15 regression — cli tool output truncation
// ============================================================

func TestTruncateOutput_BelowCap(t *testing.T) {
	in := "short output"
	if got := truncateOutput(in, 100); got != in {
		t.Errorf("output below cap should pass through, got %q", got)
	}
}

func TestTruncateOutput_DisabledWithNegative(t *testing.T) {
	in := strings.Repeat("x", 50000)
	if got := truncateOutput(in, -1); got != in {
		t.Errorf("negative cap should disable truncation")
	}
}

func TestTruncateOutput_HardCut(t *testing.T) {
	in := strings.Repeat("x", 10000) // no newlines in last 200 bytes
	got := truncateOutput(in, 8192)
	if len(got) >= len(in) {
		t.Errorf("truncated output should be shorter than original")
	}
	if !strings.Contains(got, "[truncated") {
		t.Errorf("truncation marker missing: %q", got[len(got)-100:])
	}
	if !strings.Contains(got, "original 10000 bytes") {
		t.Errorf("original size not reported: %q", got[len(got)-100:])
	}
}

func TestTruncateOutput_PrefersLineBoundary(t *testing.T) {
	// Build 8200 bytes with a newline at byte 8100 (within 200 of the 8192 cap)
	head := strings.Repeat("a", 8100)
	tail := "\n" + strings.Repeat("b", 99)
	in := head + tail
	got := truncateOutput(in, 8192)
	// The preserved prefix should end exactly at the newline boundary (8100),
	// not mid-line at 8192.
	prefix := strings.SplitN(got, "\n...", 2)[0]
	if len(prefix) != 8100 {
		t.Errorf("expected line-boundary cut at 8100, got prefix len %d", len(prefix))
	}
}

func TestExecute_OutputTruncatedAtDefault(t *testing.T) {
	// `yes` spews forever — pipe through head via sh -c to produce a deterministic
	// oversized output, then rely on our post-exec truncation to cap it.
	def := &config.ToolDefinition{
		Name:    "spew",
		Command: "sh -c 'yes rakitsu | head -c 50000'",
	}
	tool := NewTool(def)
	out, err := tool.Execute(context.Background(), map[string]interface{}{})
	if err != nil {
		t.Fatalf("spew command should succeed: %v", err)
	}
	if len(out) > DefaultMaxOutputBytes+200 {
		t.Errorf("output not truncated: len=%d, cap=%d", len(out), DefaultMaxOutputBytes)
	}
	if !strings.Contains(out, "[truncated") {
		t.Errorf("truncation marker missing from oversized output")
	}
}

func TestExecute_OutputTruncationRespectsConfigOverride(t *testing.T) {
	def := &config.ToolDefinition{
		Name:    "spew",
		Command: "sh -c 'yes rakitsu | head -c 5000'",
		Sandbox: &config.SandboxConfig{
			Type: "local_restricted",
			ResourceLimits: config.ResourceLimits{
				MaxOutputBytes: 1024,
			},
		},
	}
	tool := NewTool(def)
	out, err := tool.Execute(context.Background(), map[string]interface{}{})
	if err != nil {
		t.Fatalf("spew command should succeed: %v", err)
	}
	if len(out) > 1024+200 {
		t.Errorf("output should be capped at 1024, got len=%d", len(out))
	}
	if !strings.Contains(out, "[truncated") {
		t.Errorf("truncation marker missing")
	}
}

// ============================================================
// Environment scrubbing — a spawned tool subprocess must never be able to
// read back the rakitsu-serve control-plane secrets out of its own env.
// ============================================================

func TestExecute_ScrubsSensitiveServerEnvVars(t *testing.T) {
	t.Setenv("RAKITSU_API_TOKEN", "supersecret-api-token")
	t.Setenv("RAKITSU_SESSION_MSG_TOKEN", "supersecret-msg-token")
	t.Setenv("HARMLESS_TEST_VAR", "should-still-be-visible")

	def := &config.ToolDefinition{
		Name:    "leak-check",
		Command: `sh -c 'echo "API=$RAKITSU_API_TOKEN MSG=$RAKITSU_SESSION_MSG_TOKEN OK=$HARMLESS_TEST_VAR"'`,
		Sandbox: &config.SandboxConfig{Type: "local_restricted"},
	}
	tool := NewTool(def)
	out, err := tool.Execute(context.Background(), map[string]interface{}{})
	if err != nil {
		t.Fatalf("command should succeed: %v", err)
	}
	if strings.Contains(out, "supersecret-api-token") {
		t.Errorf("RAKITSU_API_TOKEN leaked into subprocess env, got: %q", out)
	}
	if strings.Contains(out, "supersecret-msg-token") {
		t.Errorf("RAKITSU_SESSION_MSG_TOKEN leaked into subprocess env, got: %q", out)
	}
	if !strings.Contains(out, "should-still-be-visible") {
		t.Errorf("unrelated env vars should still pass through to the subprocess, got: %q", out)
	}
}

func TestExecute_OutputTruncationDisabledWithNegativeOne(t *testing.T) {
	def := &config.ToolDefinition{
		Name:    "spew",
		Command: "sh -c 'yes rakitsu | head -c 20000'",
		Sandbox: &config.SandboxConfig{
			Type: "local_restricted",
			ResourceLimits: config.ResourceLimits{
				MaxOutputBytes: -1,
			},
		},
	}
	tool := NewTool(def)
	out, err := tool.Execute(context.Background(), map[string]interface{}{})
	if err != nil {
		t.Fatalf("spew command should succeed: %v", err)
	}
	if len(out) < 20000 {
		t.Errorf("output should not be truncated, got len=%d", len(out))
	}
	if strings.Contains(out, "[truncated") {
		t.Errorf("truncation marker should not be present")
	}
}

func TestBuildDockerArgs_HardensContainer(t *testing.T) {
	sandbox := &config.SandboxConfig{
		Type:            "docker",
		MountWorkdir:    true,
		NetworkIsolated: true,
	}
	args := buildDockerArgs(sandbox, []string{"echo", "hi"})

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--read-only") {
		t.Errorf("docker args missing --read-only, got: %v", args)
	}
	if !strings.Contains(joined, "--security-opt no-new-privileges") {
		t.Errorf("docker args missing --security-opt no-new-privileges, got: %v", args)
	}
	if !strings.Contains(joined, "--tmpfs /tmp") {
		t.Errorf("docker args missing a writable /tmp tmpfs (needed since --read-only locks the rest of the root fs), got: %v", args)
	}
}

// TestBuildDockerArgs_DefaultsHardenFurther: a bare docker sandbox config
// used to run as root, with every Linux
// capability, no process-count limit, an open network, and a writable
// workdir bind mount.
func TestBuildDockerArgs_DefaultsHardenFurther(t *testing.T) {
	sandbox := &config.SandboxConfig{Type: "docker", MountWorkdir: true}
	args := buildDockerArgs(sandbox, []string{"echo", "hi"})
	joined := strings.Join(args, " ")

	if !strings.Contains(joined, "--cap-drop ALL") {
		t.Errorf("docker args missing --cap-drop ALL, got: %v", args)
	}
	if !strings.Contains(joined, "--user "+DefaultDockerUser) {
		t.Errorf("docker args missing default non-root --user, got: %v", args)
	}
	if !strings.Contains(joined, "--pids-limit "+strconv.Itoa(DefaultPidsLimit)) {
		t.Errorf("docker args missing default --pids-limit, got: %v", args)
	}
	if !strings.Contains(joined, "--network none") {
		t.Errorf("docker args should default to no network, got: %v", args)
	}
	if !containsArgPair(args, "-v", func(v string) bool { return strings.HasSuffix(v, ":ro") }) {
		t.Errorf("workdir bind mount should default to read-only, got: %v", args)
	}
}

// TestBuildDockerArgs_OverridesRespected checks every new hardening knob
// can be explicitly opted out of, since some workloads (npm/pip install,
// a build step that writes into the workdir) legitimately need to.
func TestBuildDockerArgs_OverridesRespected(t *testing.T) {
	sandbox := &config.SandboxConfig{
		Type:                 "docker",
		MountWorkdir:         true,
		MountWorkdirWritable: true,
		AllowNetwork:         true,
		User:                 "1000:1000",
		ResourceLimits:       config.ResourceLimits{PidsLimit: -1},
	}
	args := buildDockerArgs(sandbox, []string{"echo", "hi"})
	joined := strings.Join(args, " ")

	if strings.Contains(joined, "--network none") {
		t.Errorf("AllowNetwork should skip --network none, got: %v", args)
	}
	if strings.Contains(joined, "--pids-limit") {
		t.Errorf("PidsLimit: -1 should skip --pids-limit, got: %v", args)
	}
	if !strings.Contains(joined, "--user 1000:1000") {
		t.Errorf("User override not applied, got: %v", args)
	}
	if containsArgPair(args, "-v", func(v string) bool { return strings.HasSuffix(v, ":ro") }) {
		t.Errorf("MountWorkdirWritable should mount workdir read-write, got: %v", args)
	}
}

// containsArgPair reports whether args has flag immediately followed by a
// value matching pred.
func containsArgPair(args []string, flag string, pred func(string) bool) bool {
	for i, a := range args {
		if a == flag && i+1 < len(args) && pred(args[i+1]) {
			return true
		}
	}
	return false
}

// ============================================================
// working-dir existence — paupawsan/rakitsu#28
// ============================================================

// TestExecute_MissingSandboxWorkdir_ClearError: when the exec cwd falls back
// to sandbox.AllowedPaths[0] and that directory does not exist, the error
// must name the working directory instead of leaking os/exec's misleading
// "fork/exec <binary>: no such file or directory".
func TestExecute_MissingSandboxWorkdir_ClearError(t *testing.T) {
	tool := newTool("ls", nil, []string{"./does-not-exist-28/"})
	_, err := tool.Execute(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error for missing working directory")
	}
	if strings.Contains(err.Error(), "fork/exec") {
		t.Errorf("raw fork/exec error leaked: %v", err)
	}
	if !strings.Contains(err.Error(), "working directory") {
		t.Errorf("error should name the working directory, got: %v", err)
	}
}

// ============================================================
// allowed_paths — argument path filter
// ============================================================

// allowedFixture returns an allowed directory containing inside.txt, whose
// parent holds outside.txt (the file every escape below tries to reach).
func allowedFixture(t *testing.T) (allowed string, outsideFile string) {
	t.Helper()
	parent := t.TempDir()
	allowed = filepath.Join(parent, "allowed")
	if err := os.MkdirAll(filepath.Join(allowed, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	outsideFile = filepath.Join(parent, "outside.txt")
	if err := os.WriteFile(outsideFile, []byte("marker-outside-483"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(allowed, "inside.txt"), []byte("marker-inside-483"), 0o644); err != nil {
		t.Fatal(err)
	}
	return allowed, outsideFile
}

var pathParam = map[string]config.Parameter{"path": {Type: "string", Required: true}}

func catTool(cmd string, allowed string) *Tool {
	return newTool(cmd, pathParam, []string{allowed})
}

func assertDenied(t *testing.T, tool *Tool, path string) {
	t.Helper()
	out, err := tool.Execute(context.Background(), map[string]interface{}{"path": path})
	var se *SecurityError
	if !errors.As(err, &se) {
		t.Errorf("path %q: want *SecurityError, got err=%v out=%q", path, err, out)
	}
	if strings.Contains(out, "marker-outside-483") {
		t.Errorf("path %q: outside content leaked: %q", path, out)
	}
}

func assertAllowed(t *testing.T, tool *Tool, path, wantMarker string) {
	t.Helper()
	out, err := tool.Execute(context.Background(), map[string]interface{}{"path": path})
	if err != nil {
		t.Fatalf("path %q: unexpected error: %v", path, err)
	}
	if !strings.Contains(out, wantMarker) {
		t.Errorf("path %q: want %q in output, got %q", path, wantMarker, out)
	}
}

func TestExecute_AllowedPaths_ParentTraversalDenied(t *testing.T) {
	allowed, _ := allowedFixture(t)
	assertDenied(t, catTool("cat {{path}}", allowed), "../outside.txt")
}

func TestExecute_AllowedPaths_NestedTraversalDenied(t *testing.T) {
	allowed, _ := allowedFixture(t)
	assertDenied(t, catTool("cat {{path}}", allowed), "sub/../../outside.txt")
}

func TestExecute_AllowedPaths_AbsoluteOutsideDenied(t *testing.T) {
	allowed, outside := allowedFixture(t)
	assertDenied(t, catTool("cat {{path}}", allowed), outside)
}

// Relative arguments resolve against the directory the command actually
// runs in (allowed_paths[0]), not the rakitsu process cwd — the old check
// used filepath.Abs, which wrongly rejected "./inside.txt" whenever the
// process cwd was elsewhere and never saw bare "inside.txt" at all.
func TestExecute_AllowedPaths_RelativeResolvesAgainstSandboxDir(t *testing.T) {
	allowed, _ := allowedFixture(t)
	tool := catTool("cat {{path}}", allowed)
	assertAllowed(t, tool, "inside.txt", "marker-inside-483")
	assertAllowed(t, tool, "./inside.txt", "marker-inside-483")
	assertAllowed(t, tool, "sub/../inside.txt", "marker-inside-483")
}

func TestExecute_AllowedPaths_SymlinkEscapeDenied(t *testing.T) {
	allowed, outside := allowedFixture(t)
	if err := os.Symlink(outside, filepath.Join(allowed, "link.txt")); err != nil {
		t.Skipf("symlink not supported here: %v", err)
	}
	assertDenied(t, catTool("cat {{path}}", allowed), "link.txt")
}

// Shell-wrapped templates carry the whole payload as one argv element; the
// filter tokenizes it so a traversal inside the payload is still caught.
func TestExecute_AllowedPaths_ShellPayloadTraversalDenied(t *testing.T) {
	allowed, _ := allowedFixture(t)
	tool := catTool("sh -c 'cat {{path}}'", allowed)
	assertDenied(t, tool, "../outside.txt")
	assertAllowed(t, tool, "inside.txt", "marker-inside-483")
}

func TestExecute_AllowedPaths_ShellTildeExpansionDenied(t *testing.T) {
	allowed, outside := allowedFixture(t)
	t.Setenv("HOME", filepath.Dir(outside))
	assertDenied(t, catTool("sh -c 'cat {{path}}'", allowed), "~/outside.txt")
}

// The shell expands a glob before the command sees it, so a match that is
// a symlink out of the fence must be caught the same as if it had been
// named directly. Globs over in-fence files still work.
func TestExecute_AllowedPaths_ShellGlobExpandingToSymlinkEscapeDenied(t *testing.T) {
	allowed, outside := allowedFixture(t)
	if err := os.Symlink(outside, filepath.Join(allowed, "link.txt")); err != nil {
		t.Skipf("symlink not supported here: %v", err)
	}
	tool := catTool("sh -c 'cat {{path}}'", allowed)
	assertDenied(t, tool, "*")
	assertDenied(t, tool, "*.txt")
	assertAllowed(t, tool, "in*.txt", "marker-inside-483")
}

// The shell concatenates adjacent quoted fragments: the fragments ".." and
// "/outside.txt" reach cat as ../outside.txt. Stripping only the outer
// quotes would leave a harmless-looking component with embedded quotes.
func TestExecute_AllowedPaths_ShellQuotedFragmentTraversalDenied(t *testing.T) {
	allowed, _ := allowedFixture(t)
	tool := catTool("sh -c 'cat {{path}}'", allowed)
	assertDenied(t, tool, `'..''/outside.txt'`)
	assertDenied(t, tool, `".."'/outside.txt'`)
}

// A not-yet-existing name under a symlinked directory is judged by where
// the symlink points, so `touch linkdir/new` cannot land outside the fence.
func TestExecute_AllowedPaths_SymlinkDirNonexistentChildDenied(t *testing.T) {
	allowed, outside := allowedFixture(t)
	if err := os.Symlink(filepath.Dir(outside), filepath.Join(allowed, "linkdir")); err != nil {
		t.Skipf("symlink not supported here: %v", err)
	}
	assertDenied(t, catTool("cat {{path}}", allowed), "linkdir/does-not-exist-483.txt")
}

// allowed_paths: ["/"] means the whole filesystem; the root must not trim
// to an empty prefix that no path matches.
func TestExecute_AllowedPaths_FilesystemRootAllowsAll(t *testing.T) {
	allowed, _ := allowedFixture(t)
	assertAllowed(t, catTool("cat {{path}}", "/"), filepath.Join(allowed, "inside.txt"), "marker-inside-483")
}

// An explicit working_dir inside allowed_paths is what relative arguments
// resolve against; "../inside.txt" from allowed/sub lands inside the fence.
func TestExecute_AllowedPaths_ExplicitWorkingDir(t *testing.T) {
	allowed, _ := allowedFixture(t)
	def := &config.ToolDefinition{
		Name:       "test-tool",
		Command:    "cat {{path}}",
		Parameters: pathParam,
		WorkingDir: filepath.Join(allowed, "sub"),
		Sandbox:    &config.SandboxConfig{Type: "local_restricted", AllowedPaths: []string{allowed}},
	}
	tool := NewTool(def)
	assertAllowed(t, tool, "../inside.txt", "marker-inside-483")
	assertDenied(t, tool, "../../outside.txt")
}

func TestExecute_NoAllowedPaths_NoPathFilter(t *testing.T) {
	tool := newTool("cat {{path}}", pathParam)
	_, err := tool.Execute(context.Background(), map[string]interface{}{"path": "../definitely-missing-483.txt"})
	var se *SecurityError
	if errors.As(err, &se) {
		t.Errorf("without allowed_paths there is no path filter, got %v", err)
	}
}

// Documents the trust boundary rather than a bug: allowed_paths is an
// ARGUMENT filter. An allowed interpreter opens whatever its code names, so
// the filter cannot contain it — real containment needs sandbox: docker
// (docs/SECURITY.md). If this test ever starts failing because the read is
// blocked, the docs and this comment must change together.
func TestExecute_AllowedPaths_IsArgumentFilterNotContainment(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	allowed, _ := allowedFixture(t)
	def := &config.ToolDefinition{
		Name:       "test-tool",
		Command:    "python3 -c {{code}}",
		Parameters: map[string]config.Parameter{"code": {Type: "string", Required: true}},
		Sandbox:    &config.SandboxConfig{Type: "local_restricted", AllowedPaths: []string{allowed}},
	}
	out, err := NewTool(def).Execute(context.Background(), map[string]interface{}{
		"code": "print(open('../outside.txt').read())",
	})
	if err != nil || !strings.Contains(out, "marker-outside-483") {
		t.Fatalf("expected the interpreter to read past the argument filter (documented limitation); err=%v out=%q", err, out)
	}
}
