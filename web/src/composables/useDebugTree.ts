import { ref, computed, watch, type Ref } from 'vue';
import type { AgentEvent, DebugTreeNode, TokenUsage } from '../types';

let nodeCounter = 0;
function nextId(): string {
  return `dbg_${++nodeCounter}`;
}

function addTokens(target: TokenUsage | undefined, source: TokenUsage | undefined): TokenUsage {
  const t = target ?? { input_tokens: 0, output_tokens: 0, total_tokens: 0 };
  if (!source) return t;
  return {
    input_tokens: t.input_tokens + source.input_tokens,
    output_tokens: t.output_tokens + source.output_tokens,
    total_tokens: t.total_tokens + source.total_tokens,
  };
}

// Step info from pipeline payload for pre-rendering
interface StepInfo {
  name: string;
  type: string;
  agents?: string[];
  steps?: StepInfo[];
}

export function useDebugTree(events: Ref<AgentEvent[]>) {
  const treeRoots = ref<DebugTreeNode[]>([]);
  const activeNodeIds = ref<Set<string>>(new Set());
  const selectedNode = ref<DebugTreeNode | null>(null);
  const visibleUpTo = ref<number>(-1); // -1 = live (all events)
  const renderKey = ref(0); // Increments after every event batch — forces component re-renders

  // Throttled renderKey bump for TOKEN_CHUNK streaming.
  // Bumping on every chunk is too expensive; 200ms throttle gives smooth
  // live text updates without pegging the render loop.
  let streamRenderTimer: ReturnType<typeof setTimeout> | null = null;
  function requestStreamRender() {
    if (streamRenderTimer) return;
    streamRenderTimer = setTimeout(() => {
      streamRenderTimer = null;
      renderKey.value++;
    }, 200);
  }

  // Internal state
  let processedCount = 0;
  const pipelineStack: DebugTreeNode[] = [];
  const agentStacks = new Map<string, DebugTreeNode[]>();
  const openToolCalls = new Map<string, DebugTreeNode>();
  const nodeMap = new Map<string, DebugTreeNode>();
  // Chat mode: one CHAT_TURN_START container per turn. Agent/pipeline roots
  // created while a turn is active are appended to the turn node instead of
  // spilling out as siblings. Stack depth stays at 1 (turns don't nest) but
  // using an array keeps the bookkeeping consistent with pipelineStack.
  const chatTurnStack: DebugTreeNode[] = [];

  // Closure capturing nodeMap
  function createNode(
    type: DebugTreeNode['type'],
    label: string,
    agentName: string,
    event: AgentEvent,
  ): DebugTreeNode {
    const node: DebugTreeNode = {
      id: nextId(),
      type,
      label,
      agentName,
      status: 'running',
      startEvent: event,
      children: [],
    };
    nodeMap.set(node.id, node);
    return node;
  }

  function findNode(id: string): DebugTreeNode | null {
    return nodeMap.get(id) ?? null;
  }

  function getAgentStack(name: string): DebugTreeNode[] {
    if (!agentStacks.has(name)) agentStacks.set(name, []);
    return agentStacks.get(name)!;
  }

  function currentParent(agentName: string): DebugTreeNode | null {
    const stack = getAgentStack(agentName);
    return stack.length > 0 ? stack[stack.length - 1]! : null;
  }

  function pipelineTop(): DebugTreeNode | null {
    return pipelineStack.length > 0 ? pipelineStack[pipelineStack.length - 1]! : null;
  }

  function chatTurnTop(): DebugTreeNode | null {
    return chatTurnStack.length > 0 ? chatTurnStack[chatTurnStack.length - 1]! : null;
  }

  // Attach a new root node. Prefers the active chat-turn container when one
  // is open — that's how every turn's events end up under a single "Turn N"
  // node in the tree instead of spraying across treeRoots.
  function appendRoot(node: DebugTreeNode) {
    const turn = chatTurnTop();
    if (turn) {
      turn.children.push(node);
    } else {
      treeRoots.value.push(node);
    }
  }

  // Pre-render a step (and its sub-steps/agents) as pending nodes
  function preRenderStep(stepDef: StepInfo, parent: DebugTreeNode, event: AgentEvent) {
    const stepType = stepDef.type || 'sequential';
    const label = `${stepDef.name || 'step'} [${stepType}]`;
    const stepNode = createNode('step', label, '', event);
    stepNode.status = 'pending';
    stepNode.details = { stepName: stepDef.name, stepType, agents: stepDef.agents || [] };
    parent.children.push(stepNode);

    // Pre-render agents within this step
    const agents = stepDef.agents || [];
    for (const agentName of agents) {
      const agentNode = createNode('agent', agentName, agentName, event);
      agentNode.status = 'pending';
      stepNode.children.push(agentNode);
    }

    // Pre-render nested sub-steps (for parallel steps)
    if (stepDef.steps) {
      for (const sub of stepDef.steps) {
        preRenderStep(sub, stepNode, event);
      }
    }
  }

  function markActive(node: DebugTreeNode) {
    activeNodeIds.value.add(node.id);
  }

  function markInactive(node: DebugTreeNode) {
    activeNodeIds.value.delete(node.id);
  }

  function completeNode(
    node: DebugTreeNode,
    event: AgentEvent,
    status: 'success' | 'error' | 'salvaged' = 'success',
  ) {
    node.endEvent = event;
    node.status = status;
    if (event.duration_ms) node.durationMs = event.duration_ms;
    if (event.token_usage) node.tokens = addTokens(node.tokens, event.token_usage);
    markInactive(node);
  }

  function processEvent(event: AgentEvent) {
    const p = event.payload;
    const agent = event.agent_name;

    switch (event.event_type) {
      case 'PIPELINE_START': {
        const query = (p.query as string) || '';
        const stepCount = (p.step_count as number) || 0;
        const parentStep = pipelineTop();

        // Sub-pipeline: nest under the current step (e.g., BackendPipeline inside "backend" step)
        // Root pipeline: add to treeRoots
        const isSubPipeline = parentStep != null;
        const label = isSubPipeline
          ? `${agent || 'Sub-Pipeline'} (${stepCount} steps)`
          : `RUN: ${query.length > 40 ? query.slice(0, 40) + '...' : query}${stepCount ? ` (${stepCount} steps)` : ''}`;

        const node = createNode('pipeline', label, agent, event);
        if (isSubPipeline) {
          parentStep.children.push(node);
        } else {
          appendRoot(node);
        }
        pipelineStack.push(node);
        markActive(node);

        // Pre-render step nodes from payload for immediate visibility
        const stepDefs = (p.steps as StepInfo[]) || [];
        for (const stepDef of stepDefs) {
          preRenderStep(stepDef, node, event);
        }
        break;
      }
      case 'PIPELINE_END': {
        const node = pipelineStack.pop();
        if (node) completeNode(node, event, (p.status as string) === 'error' ? 'error' : 'success');
        break;
      }
      case 'PIPELINE_STEP_START': {
        const stepName = (p.step_name as string) || '';
        const stepType = (p.step_type as string) || 'sequential';
        const parent = pipelineTop();

        // Try to match a pre-rendered pending step node
        let node = parent?.children.find(
          (c) => c.type === 'step' && c.status === 'pending' && (c.details?.stepName as string) === stepName
        ) ?? null;

        if (node) {
          // Activate the pending node
          node.status = 'running';
          node.startEvent = event;
        } else {
          // No pre-rendered node — create one
          const label = `${stepName || 'step'} [${stepType}]`;
          node = createNode('step', label, '', event);
          if (parent) parent.children.push(node);
          else appendRoot(node);
        }
        pipelineStack.push(node);
        markActive(node);
        break;
      }
      case 'PIPELINE_STEP_END': {
        const node = pipelineStack.pop();
        if (node) completeNode(node, event, (p.status as string) === 'error' ? 'error' : 'success');
        break;
      }
      case 'AGENT_START': {
        const role = (p.role as string) || '';
        const agentLabel = role === 'supervisor' ? `Orchestrator: ${agent}` : agent;
        // Runtime-spawned children carry an explicit parent link — attach
        // under the spawning agent's node instead of relying on event order
        // (order-based nesting breaks for parallel spawns).
        const explicitParent = (p.parent_agent as string) || event.parent_id || '';
        const parentStack = explicitParent ? agentStacks.get(explicitParent) : undefined;
        const spawnParent = parentStack && parentStack.length > 0 ? parentStack[parentStack.length - 1] : null;
        const parent = spawnParent ?? pipelineTop();

        // Try to match a pre-rendered pending agent node
        let node = parent?.children.find(
          (c) => c.type === 'agent' && c.status === 'pending' && c.agentName === agent
        ) ?? null;

        if (node) {
          node.status = 'running';
          node.label = agentLabel;
          node.startEvent = event;
        } else {
          node = createNode('agent', agentLabel, agent, event);
          if (parent) parent.children.push(node);
          else appendRoot(node);
        }
        getAgentStack(agent).push(node);
        markActive(node);
        const iter = createNode('iteration', 'Iteration 0', agent, event);
        iter.iteration = 0;
        iter.status = 'running';
        node.children.push(iter);
        break;
      }
      case 'AGENT_END': {
        const stack = getAgentStack(agent);
        const node = stack.pop();
        if (node) {
          // Surface "salvaged_no_progress" as its own visual status, distinct
          // from success/error, so debugger viewers can tell at a glance that
          // the model produced reasoning-only output rather than a committed
          // answer.
          const rawStatus = p.status as string;
          let status: DebugTreeNode['status'];
          if (rawStatus === 'error') status = 'error';
          else if (rawStatus === 'salvaged_no_progress') status = 'salvaged';
          else status = 'success';
          completeNode(node, event, status);
          if (event.token_usage) node.tokens = event.token_usage;
          const lastIter = node.children[node.children.length - 1];
          if (lastIter && lastIter.type === 'iteration' && lastIter.status === 'running') {
            lastIter.status = status;
          }
        }
        break;
      }
      case 'THOUGHT_START': {
        const parent = currentParent(agent);
        if (!parent) break;
        const iterNum = (p.iteration as number) ?? event.iteration ?? 0;
        let iterNode = parent.children.find(
          (c) => c.type === 'iteration' && c.iteration === iterNum,
        );
        if (!iterNode) {
          const prevIter = parent.children[parent.children.length - 1];
          if (prevIter && prevIter.type === 'iteration' && prevIter.status === 'running') {
            prevIter.status = 'success';
          }
          iterNode = createNode('iteration', `Iteration ${iterNum}`, agent, event);
          iterNode.iteration = iterNum;
          parent.children.push(iterNode);
        }
        const thought = createNode('thought', 'Thinking...', agent, event);
        iterNode.children.push(thought);
        markActive(thought);
        break;
      }
      case 'THOUGHT_END': {
        const parent = currentParent(agent);
        if (!parent) break;
        const iterNum = (p.iteration as number) ?? event.iteration ?? 0;
        const iterNode = parent.children.find(
          (c) => c.type === 'iteration' && c.iteration === iterNum,
        );
        if (!iterNode) break;
        const thought = [...iterNode.children].reverse().find(
          (c) => c.type === 'thought' && c.status === 'running',
        );
        if (thought) {
          const reasoning = (p.reasoning as string) || '';
          thought.label = reasoning.length > 60 ? reasoning.slice(0, 60) + '...' : reasoning || 'Thought';
          completeNode(thought, event);
        }
        break;
      }
      case 'TOOL_CALL_START': {
        const parent = currentParent(agent);
        if (!parent) break;
        const toolId = (p.tool_call_id as string) || '';
        const toolName = (p.tool_name as string) || 'tool';
        const node = createNode('tool_call', toolName, agent, event);
        const iterNode = parent.children[parent.children.length - 1];
        if (iterNode && iterNode.type === 'iteration') {
          iterNode.children.push(node);
        } else {
          parent.children.push(node);
        }
        if (toolId) openToolCalls.set(toolId, node);
        markActive(node);
        break;
      }
      case 'TOOL_CALL_END': {
        const toolId = (p.tool_call_id as string) || '';
        const node = toolId ? openToolCalls.get(toolId) : null;
        if (node) {
          const hasError = !!(p.error as string);
          completeNode(node, event, hasError ? 'error' : 'success');
          openToolCalls.delete(toolId);
        }
        break;
      }
      case 'REFLECTION_START': {
        const parent = currentParent(agent);
        if (!parent) break;
        const node = createNode('reflection', 'Reflecting...', agent, event);
        const iterNode = parent.children[parent.children.length - 1];
        if (iterNode && iterNode.type === 'iteration') {
          iterNode.children.push(node);
        } else {
          parent.children.push(node);
        }
        markActive(node);
        break;
      }
      case 'REFLECTION_END': {
        const parent = currentParent(agent);
        if (!parent) break;
        const iterNode = parent.children[parent.children.length - 1];
        const container = iterNode?.type === 'iteration' ? iterNode : parent;
        const node = [...container.children].reverse().find(
          (c) => c.type === 'reflection' && c.status === 'running',
        );
        if (node) {
          node.label = `Reflection (${(p.mode as string) || 'after_tool'})`;
          completeNode(node, event);
        }
        break;
      }
      case 'GROUND_CHECK_START': {
        const parent = currentParent(agent);
        if (!parent) break;
        const node = createNode('ground_check', 'Ground check...', agent, event);
        const iterNode = parent.children[parent.children.length - 1];
        if (iterNode && iterNode.type === 'iteration') {
          iterNode.children.push(node);
        } else {
          parent.children.push(node);
        }
        markActive(node);
        break;
      }
      case 'GROUND_CHECK_END': {
        const parent = currentParent(agent);
        if (!parent) break;
        const iterNode = parent.children[parent.children.length - 1];
        const container = iterNode?.type === 'iteration' ? iterNode : parent;
        const node = [...container.children].reverse().find(
          (c) => c.type === 'ground_check' && c.status === 'running',
        );
        if (node) {
          const valid = p.is_valid as boolean;
          node.label = `Ground check: ${valid ? 'valid' : 'invalid'}`;
          completeNode(node, event, valid ? 'success' : 'error');
        }
        break;
      }
      case 'AGENT_HANDOFF': {
        const parent = currentParent(agent) || pipelineTop();
        if (!parent) break;
        const to = (p.to_agent as string) || '?';
        const task = (p.task as string) || '';
        const node = createNode('handoff', `Handoff to ${to}`, agent, event);
        node.status = 'success';
        if (task) node.details = { task: task.substring(0, 200) };
        parent.children.push(node);
        break;
      }
      case 'AGENT_MESSAGE': {
        const parent = currentParent(agent) || pipelineTop();
        if (!parent) break;
        const from = (p.from_agent as string) || '?';
        const msg = (p.message as string) || '';
        const node = createNode('message', `Result from ${from}`, agent, event);
        node.status = 'success';
        if (msg) node.details = { message: msg.substring(0, 500) };
        parent.children.push(node);
        break;
      }
      case 'ERROR': {
        const node = createNode('error', (p.message as string) || 'Error', agent, event);
        node.status = 'error';
        const parent = currentParent(agent) || pipelineTop();
        if (parent) parent.children.push(node);
        else appendRoot(node);
        break;
      }
      case 'CHAT_TURN_START': {
        const turn = (p.turn as number) || chatTurnStack.length + 1;
        const text = (p.text as string) || '';
        const preview = text.length > 60 ? text.slice(0, 60) + '…' : text;
        const label = `Turn ${turn}${preview ? ': ' + preview : ''}`;
        const node = createNode('chat_turn', label, agent, event);
        node.iteration = turn;
        node.details = { turn, text };
        treeRoots.value.push(node); // chat turns always live at the top
        chatTurnStack.push(node);
        markActive(node);
        break;
      }
      case 'CHAT_TURN_END': {
        const node = chatTurnStack.pop();
        if (node) {
          const interrupted = (p.interrupted as boolean) || false;
          completeNode(node, event, interrupted ? 'error' : 'success');
          node.details = {
            ...(node.details || {}),
            final: p.final,
            interrupted,
            error: p.error,
          };
        }
        break;
      }
      case 'TOKEN_USAGE': {
        // Accumulate token usage on the agent node
        if (event.token_usage) {
          const parent = currentParent(agent);
          if (parent) {
            parent.tokens = addTokens(parent.tokens, event.token_usage);
          }
        }
        break;
      }
      case 'TOKEN_CHUNK': {
        // Accumulate streaming text on the active thought node.
        // node is a plain object (not reactive), so mutating
        // streamingText won't trigger Vue re-renders. We use a
        // throttled renderKey bump (200ms) to force periodic updates
        // without pegging the render loop on every chunk.
        const parent = currentParent(agent);
        if (!parent) break;
        const iterNode = parent.children[parent.children.length - 1];
        if (!iterNode || iterNode.type !== 'iteration') break;
        const thought = [...iterNode.children].reverse().find(
          (c) => c.type === 'thought' && c.status === 'running',
        );
        if (thought) {
          thought.streamingText = (thought.streamingText || '') + ((p.text as string) || '');
          requestStreamRender();
        }
        break;
      }
      case 'REASONING_CHUNK': {
        // Same accumulation pattern as TOKEN_CHUNK, but the live
        // reasoning_content lands in a separate streamingReasoning lane.
        // Reasoning models emit thousands per iteration; the existing
        // throttled renderKey bump keeps the render loop sane.
        const parent = currentParent(agent);
        if (!parent) break;
        const iterNode = parent.children[parent.children.length - 1];
        if (!iterNode || iterNode.type !== 'iteration') break;
        const thought = [...iterNode.children].reverse().find(
          (c) => c.type === 'thought' && c.status === 'running',
        );
        if (thought) {
          thought.streamingReasoning = (thought.streamingReasoning || '') + ((p.text as string) || '');
          requestStreamRender();
        }
        break;
      }
      case 'DEBUG_PAUSED': {
        // Mark the current agent node as paused + store inspection context
        const parent = currentParent(agent);
        if (parent) {
          parent.status = 'paused';
          parent.details = { ...parent.details, pauseContext: p };
        }
        break;
      }
      case 'DEBUG_RESUMED': {
        // Restore agent node to running
        const parent = currentParent(agent);
        if (parent && parent.status === 'paused') parent.status = 'running';
        break;
      }
    }
  }

  // Flat list of all nodes (for timeline/mind-map layouts)
  const flatNodes = computed<DebugTreeNode[]>(() => {
    const result: DebugTreeNode[] = [];
    function walk(node: DebugTreeNode) {
      result.push(node);
      node.children.forEach(walk);
    }
    treeRoots.value.forEach(walk);
    return result;
  });

  const tokenTotals = computed<TokenUsage>(() => {
    let totals: TokenUsage = { input_tokens: 0, output_tokens: 0, total_tokens: 0 };
    function walk(node: DebugTreeNode) {
      if (node.type === 'agent' && node.tokens) {
        totals = addTokens(totals, node.tokens);
      }
      node.children.forEach(walk);
    }
    treeRoots.value.forEach(walk);
    return totals;
  });

  // Incremental processing — triggerRef forces computed recomputation
  // since processEvent mutates nodes via plain references (stacks/maps).
  // Watch both length AND array identity (first element id) to detect full replacements.
  let lastArrayId = '';
  watch(
    () => {
      const arr = events.value ?? [];
      const id = arr.length > 0 ? (arr[0]?.id ?? '') : '';
      return `${arr.length}:${id}`;
    },
    () => {
      const arr = events.value ?? [];
      const currentId = arr.length > 0 ? (arr[0]?.id ?? '') : '';

      // Detect array replacement (different first event = new dataset)
      if (currentId !== lastArrayId && processedCount > 0) {
        // Full reset + reprocess — array was swapped (e.g., Live↔History switch)
        treeRoots.value = [];
        activeNodeIds.value = new Set();
        pipelineStack.length = 0;
        agentStacks.clear();
        openToolCalls.clear();
        nodeMap.clear();
        processedCount = 0;
      }
      lastArrayId = currentId;

      const limit = visibleUpTo.value >= 0 ? visibleUpTo.value + 1 : arr.length;
      const prevCount = processedCount;
      while (processedCount < limit) {
        const ev = arr[processedCount];
        if (ev) processEvent(ev);
        processedCount++;
      }
      if (processedCount > prevCount) {
        treeRoots.value = [...treeRoots.value];
        renderKey.value++;
      }
    },
  );

  // Time-travel: rebuild when visibleUpTo changes
  watch(visibleUpTo, (newVal, oldVal) => {
    if (oldVal === undefined) return;
    const target = newVal >= 0 ? newVal + 1 : (events.value ?? []).length;

    // Save selected node ID before potential reset
    const selectedId = selectedNode.value?.id ?? null;

    if (target < processedCount) {
      // Moving backward — full reset + reprocess
      treeRoots.value = [];
      activeNodeIds.value = new Set();
      pipelineStack.length = 0;
      agentStacks.clear();
      openToolCalls.clear();
      nodeMap.clear();
      processedCount = 0;
    }

    // Process forward (catches both backward-after-reset and forward slider moves)
    const prevCount = processedCount;
    while (processedCount < target) {
      const ev = events.value[processedCount];
      if (ev) processEvent(ev);
      processedCount++;
    }
    // Shallow-copy + renderKey to force re-render (forward scrubbing mutates nodes in-place)
    if (processedCount > prevCount || target < (oldVal >= 0 ? oldVal + 1 : processedCount)) {
      treeRoots.value = [...treeRoots.value];
      renderKey.value++;
    }
    // Re-find selected node after rebuild (backward scrub creates new objects)
    if (selectedId) {
      const found = nodeMap.get(selectedId);
      selectedNode.value = found ?? null;
    }
  });

  function reset() {
    treeRoots.value = [];
    activeNodeIds.value = new Set();
    selectedNode.value = null;
    visibleUpTo.value = -1;
    processedCount = 0;
    pipelineStack.length = 0;
    agentStacks.clear();
    openToolCalls.clear();
    nodeMap.clear();
    nodeCounter = 0;
  }

  const activeAgents = computed(() =>
    [...activeNodeIds.value]
      .map((id) => nodeMap.get(id))
      .filter((n): n is DebugTreeNode => n?.type === 'agent'),
  );

  return {
    treeRoots,
    activeNodeIds,
    activeAgents,
    flatNodes,
    selectedNode,
    tokenTotals,
    renderKey,
    visibleUpTo,
    findNode,
    reset,
  };
}
