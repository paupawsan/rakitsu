/**
 * Standalone tree builder: converts AgentEvent[] → DebugTreeNode[]
 * Extracted from useDebugTree for use outside reactive context (e.g., loading session history).
 */
import type { AgentEvent, DebugTreeNode, TokenUsage } from '../types';

let nodeCounter = 0;
function nextId(): string {
  return `hist_${++nodeCounter}`;
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

interface StepInfo {
  name: string;
  type: string;
  agents?: string[];
  steps?: StepInfo[];
}

/** Build a DebugTreeNode[] from a flat array of AgentEvents (non-reactive, pure function) */
export function buildTreeFromEvents(events: AgentEvent[]): DebugTreeNode[] {
  const treeRoots: DebugTreeNode[] = [];
  const pipelineStack: DebugTreeNode[] = [];
  const agentStacks = new Map<string, DebugTreeNode[]>();
  const openToolCalls = new Map<string, DebugTreeNode>();
  const nodeMap = new Map<string, DebugTreeNode>();

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

  function preRenderStep(stepDef: StepInfo, parent: DebugTreeNode, event: AgentEvent) {
    const stepType = stepDef.type || 'sequential';
    const label = `${stepDef.name || 'step'} [${stepType}]`;
    const stepNode = createNode('step', label, '', event);
    stepNode.status = 'pending';
    stepNode.details = { stepName: stepDef.name, stepType, agents: stepDef.agents || [] };
    parent.children.push(stepNode);
    for (const agentName of stepDef.agents || []) {
      const agentNode = createNode('agent', agentName, agentName, event);
      agentNode.status = 'pending';
      stepNode.children.push(agentNode);
    }
    if (stepDef.steps) {
      for (const sub of stepDef.steps) preRenderStep(sub, stepNode, event);
    }
  }

  function completeNode(node: DebugTreeNode, event: AgentEvent, status: 'success' | 'error' | 'salvaged' = 'success') {
    node.endEvent = event;
    node.status = status;
    if (event.duration_ms) node.durationMs = event.duration_ms;
    if (event.token_usage) node.tokens = addTokens(node.tokens, event.token_usage);
  }

  for (const event of events) {
    const p = event.payload;
    const agent = event.agent_name;

    switch (event.event_type) {
      case 'PIPELINE_START': {
        const query = (p.query as string) || '';
        const stepCount = (p.step_count as number) || 0;
        const parentStep = pipelineTop();
        const isSubPipeline = parentStep != null;
        const label = isSubPipeline
          ? `${agent || 'Sub-Pipeline'} (${stepCount} steps)`
          : `RUN: ${query.length > 40 ? query.slice(0, 40) + '...' : query}${stepCount ? ` (${stepCount} steps)` : ''}`;
        const node = createNode('pipeline', label, agent, event);
        if (isSubPipeline) parentStep.children.push(node);
        else treeRoots.push(node);
        pipelineStack.push(node);
        const stepDefs = (p.steps as StepInfo[]) || [];
        for (const stepDef of stepDefs) preRenderStep(stepDef, node, event);
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
        let node = parent?.children.find(
          (c) => c.type === 'step' && c.status === 'pending' && (c.details?.stepName as string) === stepName
        ) ?? null;
        if (node) {
          node.status = 'running';
          node.startEvent = event;
        } else {
          const label = `${stepName || 'step'} [${stepType}]`;
          node = createNode('step', label, '', event);
          if (parent) parent.children.push(node);
          else treeRoots.push(node);
        }
        pipelineStack.push(node);
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
        const parent = pipelineTop();
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
          else treeRoots.push(node);
        }
        getAgentStack(agent).push(node);
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
          // Surface "salvaged_no_progress" as a distinct status so the
          // debugger reflects that the model produced reasoning-only output
          // instead of a committed answer, even when SESSION_END.status is
          // "success".
          const rawStatus = p.status as string;
          let status: 'success' | 'error' | 'salvaged';
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
        if (iterNode && iterNode.type === 'iteration') iterNode.children.push(node);
        else parent.children.push(node);
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
        if (iterNode && iterNode.type === 'iteration') iterNode.children.push(node);
        else parent.children.push(node);
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
        else treeRoots.push(node);
        break;
      }
      case 'TOKEN_USAGE': {
        if (event.token_usage) {
          const parent = currentParent(agent);
          if (parent) parent.tokens = addTokens(parent.tokens, event.token_usage);
        }
        break;
      }
      case 'TOKEN_CHUNK': {
        const parent = currentParent(agent);
        if (!parent) break;
        const iterNode = parent.children[parent.children.length - 1];
        if (!iterNode || iterNode.type !== 'iteration') break;
        const thought = [...iterNode.children].reverse().find(
          (c) => c.type === 'thought' && c.status === 'running',
        );
        if (thought) {
          thought.streamingText = (thought.streamingText || '') + ((p.text as string) || '');
        }
        break;
      }
      case 'REASONING_CHUNK': {
        // Mirrors TOKEN_CHUNK: accumulate live reasoning_content into a
        // separate streamingReasoning lane on the currently-running thought
        // so the UI can render chain-of-thought distinct from the committed
        // answer. Reasoning models emit thousands of these per iteration.
        const parent = currentParent(agent);
        if (!parent) break;
        const iterNode = parent.children[parent.children.length - 1];
        if (!iterNode || iterNode.type !== 'iteration') break;
        const thought = [...iterNode.children].reverse().find(
          (c) => c.type === 'thought' && c.status === 'running',
        );
        if (thought) {
          thought.streamingReasoning = (thought.streamingReasoning || '') + ((p.text as string) || '');
        }
        break;
      }
    }
  }

  return treeRoots;
}
