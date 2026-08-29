// Centralized checkpoint/event definition — single source of truth.
// All UI labels derive from this map. Replace labels for i18n.

export interface CheckpointDef {
  /** Internal event type name (used in breakpoints and API) */
  event: string;
  /** Internal checkpoint name (used in Go Check() calls) */
  checkpoint: string;
  /** Display label for UI */
  label: string;
  /** Short description */
  description: string;
}

/**
 * All breakpoint-able checkpoints.
 * To add a new language, replace `label` and `description` values.
 */
export const CHECKPOINTS: CheckpointDef[] = [
  { event: 'AGENT_START',         checkpoint: 'pre_agent',          label: 'Agent Start',         description: 'Before agent starts' },
  { event: 'THOUGHT_START',       checkpoint: 'pre_thought',        label: 'Thought Start',       description: 'Before LLM call' },
  { event: 'THOUGHT_END',         checkpoint: 'post_thought',       label: 'Thought End',         description: 'After LLM response' },
  { event: 'TOOL_CALL_START',     checkpoint: 'pre_tool',           label: 'Tool Call',           description: 'Before tool execution' },
  { event: 'REFLECTION_START',    checkpoint: 'pre_reflection',     label: 'Reflection',          description: 'Before reflection step' },
  { event: 'GROUND_CHECK_START',  checkpoint: 'pre_ground_check',   label: 'Ground Check',        description: 'Before validation' },
  { event: 'PIPELINE_STEP_START', checkpoint: 'pre_pipeline_step',  label: 'Pipeline Step',       description: 'Before pipeline step' },
];

/** Map event type → checkpoint def */
export const EVENT_TO_CHECKPOINT = new Map(CHECKPOINTS.map(c => [c.event, c]));

/** Map checkpoint name → checkpoint def */
export const CHECKPOINT_TO_DEF = new Map(CHECKPOINTS.map(c => [c.checkpoint, c]));

/** Map DebugTreeNode type → event type name */
export const NODE_TYPE_TO_EVENT: Record<string, string> = {
  agent: 'AGENT_START',
  thought: 'THOUGHT_START',
  iteration: 'THOUGHT_START',
  tool_call: 'TOOL_CALL_START',
  step: 'PIPELINE_STEP_START',
  reflection: 'REFLECTION_START',
  ground_check: 'GROUND_CHECK_START',
};

/** Breakpoint dropdown options derived from the map */
export const BREAKPOINT_OPTIONS = [
  { value: '*', label: '* — any event' },
  ...CHECKPOINTS.map(c => ({ value: c.event, label: `${c.event} — ${c.description}` })),
];
