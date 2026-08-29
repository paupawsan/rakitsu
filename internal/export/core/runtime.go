// Package core provides the orchestration engine and runtime target interface
// for the export system. The orchestration logic (DAG, parallelism, loops,
// synthesis) is target-agnostic — RuntimeTargets provide platform-specific
// run_agent() implementations.
package core

// RuntimeTarget generates shell script fragments for a specific deployment runtime.
// Orchestration scripts (orchestrate.sh, delegate.sh) are target-agnostic — they handle
// pipeline DAG logic, parallelism, synthesis, and loops. The RuntimeTarget provides
// the platform-specific run_agent() implementation and any setup/teardown.
//
// This interface enables Rakitsu to export to any agent runtime without coupling
// the orchestration engine to a specific platform (NemoClaw, Docker, Manus, etc.).
type RuntimeTarget interface {
	// Name returns the target identifier used as the MODE value in scripts.
	Name() string

	// RunAgentCase returns the bash case branch body for run_agent().
	// The function receives shell variables: $name (agent id), $input_file, $output_file.
	// Must NOT include the case label or ;; terminator — those are added by the caller.
	RunAgentCase() string

	// SetupCommands returns optional bash commands run before orchestration starts.
	// Receives the list of agent IDs that will be used. Return empty string for none.
	SetupCommands(agentIDs []string) string

	// TeardownCommands returns optional bash commands run after orchestration completes.
	// Return empty string for none.
	TeardownCommands() string

	// DelegateCase returns the bash case branch body for delegate.sh (ReAct/Hierarchical).
	// The function receives shell variable: $AGENT (agent id).
	// Task input is available on stdin. Must NOT include the case label or ;; terminator.
	DelegateCase() string
}

// ComposeTarget generates shell fragments for Docker Compose orchestration.
type ComposeTarget struct{}

func (t *ComposeTarget) Name() string { return "compose" }

func (t *ComposeTarget) RunAgentCase() string {
	return `      docker compose exec -T "agent-${name}" openclaw run < "$input_file" > "$output_file"`
}

func (t *ComposeTarget) SetupCommands(agentIDs []string) string {
	return `    docker compose up -d`
}

func (t *ComposeTarget) TeardownCommands() string { return "" }

func (t *ComposeTarget) DelegateCase() string {
	return `  docker compose exec -T "agent-${AGENT}" openclaw run < /dev/stdin`
}
