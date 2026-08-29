// Package nemoclaw generates NemoClaw deployment files from Rakitsu configs.
package nemoclaw

import (
	"fmt"
	"strings"
)

// OpenShellTarget generates shell fragments for NVIDIA OpenShell / NemoClaw orchestration.
//
// Uses only verified commands from the NemoClaw CLI source (github.com/NVIDIA/NemoClaw):
//   - openshell sandbox upload <name> <src> <dest>       — file transfer into sandbox
//   - openshell sandbox download <name> <src> <dest>     — file transfer from sandbox
//   - openshell sandbox exec --name <name> -- <cmd>      — run a command non-interactively
//   - openshell sandbox connect <name>                   — interactive shell only, no
//     command passthrough (confirmed against a live sandbox: `connect <name> -- <cmd>`
//     rejects the trailing command instead of running it)
//   - openclaw agent --agent main --local -m "msg"       — run agent (in-sandbox)
//   - nemoclaw onboard --non-interactive                 — create sandbox (setup only)
//   - nemoclaw list                                      — verify sandboxes exist
//   - nemoclaw <name> connect                            — interactive SSH via NemoClaw
type OpenShellTarget struct{}

func (t *OpenShellTarget) Name() string { return "openshell" }

func (t *OpenShellTarget) RunAgentCase() string {
	// File-based orchestration: upload input → exec openclaw agent → download output.
	// Uses openshell sandbox commands (verified against NemoClaw source).
	//
	// Remote paths are derived from the step-scoped local input/output
	// filenames (run_agent's caller already names these input-<step>.txt /
	// output-<step>.txt) rather than fixed literals — two steps sharing one
	// agent's sandbox in the same parallel level would otherwise race on
	// the same two remote files. Each openshell invocation's own stdout is
	// redirected to /dev/null, matching the rest of the generated script's
	// discipline of keeping stdout reserved for the final payload — the
	// actual result is pulled back via the download step, not this output.
	return `      remote_in="/sandbox/$(basename "$input_file")"
      remote_out="/sandbox/$(basename "$output_file")"
      openshell sandbox upload "agent-${name}" "$input_file" "$remote_in" >/dev/null
      openshell sandbox exec --name "agent-${name}" -- \
        sh -c "openclaw agent --agent main --local -m \"\$(cat '$remote_in')\" > '$remote_out'" >/dev/null
      openshell sandbox download "agent-${name}" "$remote_out" "$output_file" >/dev/null`
}

func (t *OpenShellTarget) SetupCommands(agentIDs []string) string {
	var b strings.Builder
	b.WriteString("    # Prerequisites: sandboxes must be pre-created via nemoclaw onboard.\n")
	b.WriteString("    # To create sandboxes non-interactively:\n")
	for _, id := range agentIDs {
		b.WriteString(fmt.Sprintf("    #   NEMOCLAW_SANDBOX_NAME=agent-%s nemoclaw onboard --non-interactive\n", id))
	}
	b.WriteString("    printf \"${BLUE}[orchestrate]${NC} Verifying sandboxes...\\n\" >&2\n")
	b.WriteString("    nemoclaw list >&2")
	return b.String()
}

func (t *OpenShellTarget) TeardownCommands() string { return "" }

func (t *OpenShellTarget) DelegateCase() string {
	// Delegate via file transfer: stdin → temp file → upload → exec → download → stdout.
	//
	// Remote paths are derived from the unique local temp file (mktemp)
	// rather than fixed literals, so concurrent delegate.sh invocations
	// against the same agent's sandbox don't race on the same two remote
	// files. See RunAgentCase's comment for the matching stdout-redirect
	// rationale.
	return `  DELEGATE_TMP="$(mktemp)"
  trap 'rm -f "$DELEGATE_TMP" "${DELEGATE_TMP}.out"' EXIT
  cat > "$DELEGATE_TMP"
  remote_in="/sandbox/$(basename "$DELEGATE_TMP").in"
  remote_out="/sandbox/$(basename "$DELEGATE_TMP").out"
  openshell sandbox upload "agent-${AGENT}" "$DELEGATE_TMP" "$remote_in" >/dev/null
  openshell sandbox exec --name "agent-${AGENT}" -- \
    sh -c "openclaw agent --agent main --local -m \"\$(cat '$remote_in')\" > '$remote_out'" >/dev/null
  openshell sandbox download "agent-${AGENT}" "$remote_out" "${DELEGATE_TMP}.out" >/dev/null
  cat "${DELEGATE_TMP}.out"`
}
