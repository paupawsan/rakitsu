// Package agent provides the agent execution engine with ReAct loop.
package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/paupawsan/rakitsu/internal/config"
	"github.com/paupawsan/rakitsu/internal/debug"
	"github.com/paupawsan/rakitsu/internal/llm"
	"github.com/paupawsan/rakitsu/internal/telemetry"
	"github.com/paupawsan/rakitsu/internal/tools"
)

// Orchestrator manages multiple agents and routes tasks between them
type Orchestrator struct {
	name           string
	strategy       string
	model          string
	systemPrompt   string
	agentNames     []string
	routingConfig  *config.RoutingConfig
	handoffConfig  *config.HandoffConfig
	pipelineConfig *config.PipelineConfig

	llmProvider  llm.LLMProvider
	eventBus     *telemetry.EventBus
	agents       map[string]Runner
	toolRegistry *tools.ToolRegistry
	debugCtrl    *debug.DebugController
	// steering, when non-nil, is forwarded to the synthetic supervisor Agent
	// built per Run — the orchestrator itself has no ReAct loop, only the
	// supervisor does.
	steering func() []llm.Message

	// Checkpoint support (pipeline mode only)
	checkpoint       *CheckpointData      // non-nil = resume mode; steps here are skipped
	checkpointWriter func(CheckpointData) // called after each successful step; nil = no-op

	// Budget enforcement
	rootGuard    *CompositeGuard // global CompositeGuard shared with all worker agents; nil = no enforcement
	pricing      config.PricingConfig
	pricingKnown bool

	// Fallback state: capture the most recent successful worker result
	// per Run so that if the synthetic supervisor returns empty (which
	// happens because Hierarchical supervisors only have delegate_to_X
	// tools and no respond_to_user path), the orchestrator returns the
	// last worker output instead of "".
	lastWorkerMu     sync.RWMutex
	lastWorkerResult string

	// Per-Run, per-worker unproductive-outcome tracker. When a worker
	// returns unproductively (LastRunUnproductive()=true —
	// salvaged_no_progress, max_iter+empty, or success+empty), the entry
	// in unproductiveWorkers flips. Subsequent delegations to the same
	// worker in this Run increment postUnproductiveRedelegations[w]; once
	// that count reaches workerUnproductiveCap (default 1), the
	// DelegationTool returns a [DELEGATION BLOCKED] message instead of
	// calling the worker and emits WORKER_REDELEGATION_BLOCKED. Cleared at
	// the top of each Run by clearLastWorkerResult(). Covers both the
	// salvage-only subset and the broader max_iter+empty / success+empty
	// supersets.
	salvageMu                     sync.Mutex
	unproductiveWorkers           map[string]bool
	postUnproductiveRedelegations map[string]int
}

// workerUnproductiveCap is the maximum number of delegations to the same
// worker allowed AFTER that worker has returned unproductively (salvaged,
// max_iter+empty, or success+empty) in the current supervisor turn.
// 1 = one retry is allowed, the next is blocked.
const workerUnproductiveCap = 1

// SalvageReporter is implemented by Runner-typed workers (currently *Agent)
// that can report whether their most recent Run terminated in a salvaged-
// no-progress state. Strict subset of OutcomeReporter — the orchestrator
// prefers OutcomeReporter when available and falls back to SalvageReporter
// for the salvage-only signal.
type SalvageReporter interface {
	LastRunSalvaged() bool
}

// OutcomeReporter is implemented by Runner-typed workers (currently *Agent)
// that can report whether their most recent Run terminated unproductively —
// salvaged_no_progress, max_iterations+empty, or success+empty. Strict
// superset of SalvageReporter's signal; DelegationTool checks this first
// and falls back to SalvageReporter otherwise.
type OutcomeReporter interface {
	LastRunUnproductive() bool
}

// recordWorkerResult is called by DelegationTool when a worker returns a
// non-empty result. The most recent value is used as a fallback if the
// supervisor's own final answer is empty.
func (o *Orchestrator) recordWorkerResult(result string) {
	if strings.TrimSpace(result) == "" {
		return
	}
	o.lastWorkerMu.Lock()
	o.lastWorkerResult = result
	o.lastWorkerMu.Unlock()
}

func (o *Orchestrator) getLastWorkerResult() string {
	o.lastWorkerMu.RLock()
	defer o.lastWorkerMu.RUnlock()
	return o.lastWorkerResult
}

func (o *Orchestrator) clearLastWorkerResult() {
	o.lastWorkerMu.Lock()
	o.lastWorkerResult = ""
	o.lastWorkerMu.Unlock()

	// Clear per-Run unproductive tracking so each new
	// supervisor turn starts with a clean slate.
	o.salvageMu.Lock()
	o.unproductiveWorkers = nil
	o.postUnproductiveRedelegations = nil
	o.salvageMu.Unlock()
}

// isRedelegationBlocked reports whether a further delegation to the named
// worker should be refused in this Run. Returns true only when the worker
// has previously returned unproductively AND postUnproductiveRedelegations[w]
// has reached workerUnproductiveCap. "Unproductive" covers the strict
// superset of salvaged plus max_iter+empty and success+empty.
func (o *Orchestrator) isRedelegationBlocked(worker string) bool {
	o.salvageMu.Lock()
	defer o.salvageMu.Unlock()
	if !o.unproductiveWorkers[worker] {
		return false
	}
	return o.postUnproductiveRedelegations[worker] >= workerUnproductiveCap
}

// postUnproductiveCount returns how many post-unproductive delegations have
// already happened to the named worker in this Run. Used for telemetry
// payloads (the legacy JSON field `post_salvage_count` carries this value
// since salvaged is the original / strict-subset shape).
func (o *Orchestrator) postUnproductiveCount(worker string) int {
	o.salvageMu.Lock()
	defer o.salvageMu.Unlock()
	return o.postUnproductiveRedelegations[worker]
}

// recordWorkerOutcome updates the per-worker unproductive-outcome tracker
// AFTER a successful delegate-to-worker call. unproductive indicates whether
// the worker terminated without a usable answer on this call (salvaged,
// max_iter+empty, or success+empty). The bookkeeping:
//   - if unproductiveWorkers[w] was already true: this call was a post-
//     unproductive re-delegation, so increment postUnproductiveRedelegations[w].
//   - if unproductive: mark unproductiveWorkers[w] = true for future calls.
//
// The order (read before, set after) ensures the FIRST unproductive return
// from a worker doesn't count itself as a post-unproductive attempt.
func (o *Orchestrator) recordWorkerOutcome(worker string, unproductive bool) {
	o.salvageMu.Lock()
	defer o.salvageMu.Unlock()
	if o.unproductiveWorkers == nil {
		o.unproductiveWorkers = make(map[string]bool)
		o.postUnproductiveRedelegations = make(map[string]int)
	}
	if o.unproductiveWorkers[worker] {
		o.postUnproductiveRedelegations[worker]++
	}
	if unproductive {
		o.unproductiveWorkers[worker] = true
	}
}

// SetDebugController attaches a debug controller for pipeline-level breakpoints.
func (o *Orchestrator) SetDebugController(dc *debug.DebugController) {
	o.debugCtrl = dc
}

// SetSteering forwards the steering drain hook to the synthetic supervisor
// built per Run — the run is steered at its top-level loop only; workers
// never drain (spec §6).
func (o *Orchestrator) SetSteering(f func() []llm.Message) { o.steering = f }

// SetCheckpoint loads a prior checkpoint for resume. Steps present in the
// checkpoint will be skipped and their saved outputs injected into context.
func (o *Orchestrator) SetCheckpoint(cp *CheckpointData) {
	o.checkpoint = cp
}

// SetCheckpointWriter registers a callback invoked after each successful
// pipeline step. The callback receives updated CheckpointData to persist.
func (o *Orchestrator) SetCheckpointWriter(fn func(CheckpointData)) {
	o.checkpointWriter = fn
}

// SetRootGuard attaches the shared budget guard from the caller so the
// supervisor agent's token usage is counted against the global budget.
func (o *Orchestrator) SetRootGuard(g *CompositeGuard, pricing config.PricingConfig, known bool) {
	o.rootGuard = g
	o.pricing = pricing
	o.pricingKnown = known
}

// NewOrchestrator creates a new orchestrator from configuration
// GetName returns the orchestrator's name.
func (o *Orchestrator) GetName() string { return o.name }

// GetRole returns RoleSupervisor for orchestrators.
func (o *Orchestrator) GetRole() AgentRole { return RoleSupervisor }

// GetTools returns the delegation tool names.
func (o *Orchestrator) GetTools() []string { return o.getDelegationToolNames() }

// NewOrchestrator creates a new orchestrator from configuration.
// The agents map may contain both *Agent and *Orchestrator values (nested orchestration).
func NewOrchestrator(
	orchConfig *config.OrchestratorConfig,
	llmProvider llm.LLMProvider,
	eventBus *telemetry.EventBus,
	agents map[string]Runner,
) *Orchestrator {
	return &Orchestrator{
		name:           orchConfig.Name,
		strategy:       orchConfig.Strategy,
		model:          orchConfig.Model,
		systemPrompt:   orchConfig.SystemPrompt,
		agentNames:     orchConfig.Agents,
		routingConfig:  orchConfig.Routing,
		handoffConfig:  orchConfig.Handoff,
		pipelineConfig: orchConfig.Pipeline,
		llmProvider:    llmProvider,
		eventBus:       eventBus,
		agents:         agents,
		toolRegistry:   tools.NewToolRegistry(),
	}
}

// AgentNames returns the names of sub-agents this orchestrator delegates to,
// in declaration order. Used by the chat layer's `/model` command to enumerate
// swappable workers. Excludes the orchestrator itself.
func (o *Orchestrator) AgentNames() []string {
	out := make([]string, 0, len(o.agentNames))
	out = append(out, o.agentNames...)
	return out
}

// GetAgent returns the sub-agent runner by name (case-sensitive) and a bool
// indicating presence. Used by the chat layer's `/model` command to reach a
// specific worker for runtime model swap. Returns (nil, false) for unknown
// names; returns the orchestrator itself if name matches o.name (so callers
// can target the supervisor with the same API — though Orchestrator's own LLM
// is NOT swappable in this release; type-assert to *Agent first).
func (o *Orchestrator) GetAgent(name string) (Runner, bool) {
	if r, ok := o.agents[name]; ok {
		return r, true
	}
	return nil, false
}

// Run executes the orchestrator with the given query.
// Routes to the appropriate strategy: Pipeline (deterministic) or ReAct (LLM-driven).
func (o *Orchestrator) Run(ctx context.Context, query string) (string, error) {
	switch o.strategy {
	case "Pipeline":
		return o.runPipeline(ctx, query)
	default: // "ReAct", "PlanAndExecute", "Hierarchical"
		return o.runReAct(ctx, query)
	}
}

// runReAct executes the orchestrator using LLM-driven delegation via tool calls.
func (o *Orchestrator) runReAct(ctx context.Context, query string) (string, error) {
	// Register delegation tools for each worker agent
	o.registerDelegationTools()

	// Clear any prior-Run worker result so this Run's fallback only
	// considers workers invoked in this Run.
	o.clearLastWorkerResult()

	// Create a supervisor agent that uses delegation tools
	supervisor := &Agent{
		name:          o.name,
		role:          RoleSupervisor,
		model:         o.model,
		systemPrompt:  o.buildSystemPrompt(),
		tools:         o.getDelegationToolNames(),
		llmProvider:   o.llmProvider,
		toolRegistry:  o.toolRegistry,
		eventBus:      o.eventBus,
		maxIterations: 15, // Higher limit for orchestrator
		contextMon:    NewContextMonitor(config.ContextConfig{}),
		steering:      o.steering,
	}

	// Wire budget guard: supervisor counts against the global budget
	if o.rootGuard != nil {
		supTG := NewTokenGuard(0, 0) // no per-supervisor cap; root enforces global limit
		supervisor.SetTokenGuard(supTG)
		supervisor.SetGuard(NewCompositeGuard(o.rootGuard, supTG))
		supervisor.SetPricing(o.pricing, o.pricingKnown)
	}

	// Run the supervisor
	result, err := supervisor.Run(ctx, query)
	if err != nil {
		return "", err
	}

	// Fallback: synthetic Hierarchical supervisors have only
	// delegate_to_X tools — no path to emit text directly. When the
	// supervisor finishes without producing a final answer (e.g. it
	// delegated successfully and then returned empty thinking), the
	// orchestrator would return "" and the upstream chat agent would
	// retry invoke_config indefinitely. Falling back to the most recent
	// worker output is strictly better than empty: the user gets the
	// answer the worker produced, even if synthesis was missed.
	if strings.TrimSpace(result) == "" {
		if last := o.getLastWorkerResult(); last != "" {
			return last, nil
		}
	}
	return result, nil
}

// registerDelegationTools creates virtual tools for delegating to worker agents
func (o *Orchestrator) registerDelegationTools() {
	for _, agentName := range o.agentNames {
		agent, exists := o.agents[agentName]
		if !exists {
			continue
		}

		// Create a delegation tool for this agent
		tool := &DelegationTool{
			agent:      agent,
			eventBus:   o.eventBus,
			handoffCfg: o.handoffConfig,
			fromAgent:  o.name,
			debugCtrl:  o.debugCtrl,
			parent:     o,
		}

		o.toolRegistry.RegisterTool(tool)
	}
}

// getDelegationToolNames returns the names of all delegation tools
func (o *Orchestrator) getDelegationToolNames() []string {
	names := make([]string, len(o.agentNames))
	for i, agentName := range o.agentNames {
		names[i] = "delegate_to_" + sanitizeToolName(agentName)
	}
	return names
}

// buildSystemPrompt builds the system prompt for the supervisor
func (o *Orchestrator) buildSystemPrompt() string {
	var sb strings.Builder

	sb.WriteString(o.systemPrompt)
	sb.WriteString("\n\n## Available Agents\n\n")

	for _, agentName := range o.agentNames {
		agent, exists := o.agents[agentName]
		if !exists {
			continue
		}

		sb.WriteString(fmt.Sprintf("- **%s**: ", agentName))
		sb.WriteString(fmt.Sprintf("Role: %s, Tools: %v\n", agent.GetRole(), agent.GetTools()))
	}

	sb.WriteString("\n## Delegation Tools\n\n")
	sb.WriteString("You have access to delegation tools to assign tasks to worker agents:\n")

	for _, agentName := range o.agentNames {
		toolName := "delegate_to_" + sanitizeToolName(agentName)
		sb.WriteString(fmt.Sprintf("- `%s`: Delegate a task to %s\n", toolName, agentName))
	}

	sb.WriteString("\n## Instructions\n\n")
	sb.WriteString("1. Analyze the user's request\n")
	sb.WriteString("2. Break down complex tasks into subtasks\n")
	sb.WriteString("3. Delegate subtasks to the most appropriate agent\n")
	sb.WriteString("4. Wait for agent responses before proceeding\n")
	sb.WriteString("5. Synthesize results and provide a comprehensive answer\n")

	return sb.String()
}

// sanitizeToolName converts an agent name to a valid tool name
func sanitizeToolName(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, " ", "_"))
}

// ============================================================
// DELEGATION TOOL
// ============================================================

// DelegationTool is a virtual tool that delegates tasks to worker agents or sub-orchestrators
type DelegationTool struct {
	agent      Runner
	eventBus   *telemetry.EventBus
	handoffCfg *config.HandoffConfig
	fromAgent  string
	debugCtrl  *debug.DebugController
	parent     *Orchestrator // back-pointer for worker-result capture; may be nil
}

// GetName returns the tool name
func (t *DelegationTool) GetName() string {
	return "delegate_to_" + sanitizeToolName(t.agent.GetName())
}

// GetDescription returns the tool description
func (t *DelegationTool) GetDescription() string {
	return fmt.Sprintf("Delegate a task to the %s agent. Use this when the task requires %s's expertise.",
		t.agent.GetName(), t.agent.GetName())
}

// GetParametersSchema returns the JSON Schema for parameters
func (t *DelegationTool) GetParametersSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"task": map[string]interface{}{
				"type":        "string",
				"description": "The task to delegate to the agent",
			},
			"context": map[string]interface{}{
				"type":        "string",
				"description": "Additional context to help the agent understand the task",
			},
		},
		"required": []string{"task"},
	}
}

// Execute delegates the task to the worker agent
func (t *DelegationTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	task, ok := args["task"].(string)
	if !ok {
		return "", fmt.Errorf("task argument required")
	}

	// Get optional context
	var additionalContext string
	if ctxVal, ok := args["context"].(string); ok {
		additionalContext = ctxVal
	}

	// Build the full task with context
	fullTask := task
	if additionalContext != "" {
		fullTask = fmt.Sprintf("%s\n\nContext: %s", task, additionalContext)
	}

	workerName := t.agent.GetName()

	// Refuse the delegation when the worker has already
	// returned unproductively (salvaged, max_iter+empty, or success+empty)
	// in this supervisor turn AND the post-unproductive cap has been
	// reached. Returns a [DELEGATION BLOCKED] tool output (not an error)
	// so the supervisor's ReAct loop continues normally and the LLM is
	// informed about why this delegation didn't happen — letting it
	// choose a different worker or commit a final answer instead.
	if t.parent != nil && t.parent.isRedelegationBlocked(workerName) {
		count := t.parent.postUnproductiveCount(workerName)
		t.eventBus.Emit(t.fromAgent, telemetry.EventWorkerRedelegationBlocked, telemetry.WorkerRedelegationBlockedPayload{
			FromAgent:        t.fromAgent,
			BlockedWorker:    workerName,
			Reason:           "post-unproductive cap reached",
			PostSalvageCount: count,
		})
		return fmt.Sprintf(
			"[DELEGATION BLOCKED] %s previously returned an unproductive result (salvaged, max_iterations with empty output, or success with empty output) in this turn and has been re-tried %d time(s). Further delegations to %s are blocked. Pick a different worker, or stop and respond to the user directly with what you already know.",
			workerName, count, workerName,
		), nil
	}

	// Debug breakpoint: before agent delegation (ReAct mode)
	if t.debugCtrl != nil {
		if err := t.debugCtrl.Check(ctx, "pre_agent", workerName, 0, nil); err != nil {
			return "", err
		}
	}

	// Emit handoff event
	t.eventBus.Emit(t.fromAgent, telemetry.EventAgentHandoff, telemetry.AgentHandoffPayload{
		FromAgent:   t.fromAgent,
		ToAgent:     workerName,
		Task:        task,
		ContextSize: len(fullTask),
	})

	// Execute the worker agent
	result, err := t.agent.Run(ctx, fullTask)
	if err != nil {
		return "", fmt.Errorf("agent %s failed: %w", workerName, err)
	}

	// Capture this worker's result on the parent orchestrator so it
	// can fall back to the latest worker output if the supervisor's own
	// final answer is empty.
	if t.parent != nil {
		t.parent.recordWorkerResult(result)

		// Read the worker's outcome (via OutcomeReporter,
		// the broader signal — falling back to SalvageReporter for the
		// salvage-only subset on workers that don't implement
		// OutcomeReporter yet) and update the per-worker tracker. This is
		// what makes the NEXT delegate_to_<worker> call in this turn
		// either pass through or hit the gate above.
		unproductive := false
		if r, ok := t.agent.(OutcomeReporter); ok {
			unproductive = r.LastRunUnproductive()
		} else if r, ok := t.agent.(SalvageReporter); ok {
			unproductive = r.LastRunSalvaged()
		}
		t.parent.recordWorkerOutcome(workerName, unproductive)
	}

	// Emit message event with the result
	t.eventBus.Emit(workerName, telemetry.EventAgentMessage, telemetry.AgentMessagePayload{
		FromAgent: workerName,
		ToAgent:   t.fromAgent,
		Message:   result,
		Type:      "result",
	})

	return result, nil
}
