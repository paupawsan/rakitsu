<script setup lang="ts">
import { ref } from 'vue';
import type { DebugTreeNode } from '../../types';

const props = defineProps<{
  node: DebugTreeNode;
  depth: number;
  activeIds: Set<string>;
  selectedId?: string;
  renderKey?: number;
  isDebugMode?: boolean;
  isConnected?: boolean;
  isPreview?: boolean;
  isHistory?: boolean;
  breakpointedAgents?: Set<string>;
}>();

const emit = defineEmits<{
  select: [node: DebugTreeNode];
  'set-breakpoint': [node: DebugTreeNode];
  'run-to-node': [node: DebugTreeNode];
  'debug-from-here': [node: DebugTreeNode];
}>();

const expanded = ref(true);
const showActions = ref(false);
const isActive = () => props.activeIds.has(props.node.id);
const isSelected = () => props.selectedId === props.node.id;
const isPaused = () => props.node.status === 'paused';
const hasBreakpoint = () => props.breakpointedAgents?.has(props.node.agentName) || props.breakpointedAgents?.has(props.node.label);

// Breakpoint-able node types
const canBreakpoint = () => ['agent', 'thought', 'tool_call', 'step', 'iteration', 'reflection', 'ground_check'].includes(props.node.type);

function handleContextMenu(e: MouseEvent) {
  if (!canBreakpoint()) return;
  e.preventDefault();
  showActions.value = !showActions.value;
}


const typeColors: Record<string, string> = {
  pipeline: '#667eea',
  step: '#f093fb',
  agent: '#667eea',
  iteration: 'var(--text-secondary)',
  thought: '#00bcd4',
  tool_call: '#11998e',
  reflection: '#9c27b0',
  ground_check: '#ff9800',
  handoff: '#2196f3',
  message: '#43a047',
  error: '#f44336',
};

const typeIcons: Record<string, string> = {
  pipeline: 'P',
  step: 'S',
  agent: 'A',
  iteration: '#',
  thought: 'T',
  tool_call: 'W',
  reflection: 'R',
  ground_check: 'G',
  handoff: 'H',
  message: 'M',
  error: '!',
};

function fmtDuration(ms?: number): string {
  if (!ms) return '';
  return ms >= 1000 ? `${(ms / 1000).toFixed(1)}s` : `${ms}ms`;
}

function fmtTokens(t?: { total_tokens: number }): string {
  if (!t) return '';
  const n = t.total_tokens;
  return n >= 1000 ? `${(n / 1000).toFixed(1)}K` : `${n}`;
}

// Live status — what is the node currently doing?
function liveStatus(): string {
  const n = props.node;
  if (n.status !== 'running' && n.status !== 'paused') return '';

  // Agent node: describe current activity based on last active child
  if (n.type === 'agent' || n.type === 'iteration') {
    const activeChild = [...n.children].reverse().find((c: DebugTreeNode) => c.status === 'running');
    if (activeChild?.type === 'thought') {
      // Prefer the visible answer if it's started flowing; fall back to the
      // reasoning stream so the operator sees *something* during the long
      // reasoning_content phase of reasoning models.
      if (activeChild.streamingText) {
        return `Thinking: ${activeChild.streamingText.slice(-80).replace(/\n/g, ' ')}`;
      }
      if (activeChild.streamingReasoning) {
        return `Reasoning: ${activeChild.streamingReasoning.slice(-80).replace(/\n/g, ' ')}`;
      }
      return 'Thinking...';
    }
    if (activeChild?.type === 'tool_call') {
      const args = activeChild.details?.arguments;
      const argPreview = args ? ` (${JSON.stringify(args).slice(0, 60)})` : '';
      return `Calling: ${activeChild.label}${argPreview}`;
    }
    if (activeChild?.type === 'reflection') return 'Reflecting...';
    if (activeChild?.type === 'ground_check') return 'Validating...';
    if (n.iteration !== undefined) return `Iteration ${n.iteration}`;
    return 'Working...';
  }

  // Thought node: prefer committed answer, fall back to reasoning, then waiting.
  if (n.type === 'thought') {
    if (n.streamingText) return n.streamingText.slice(-100).replace(/\n/g, ' ');
    if (n.streamingReasoning) return `(reasoning) ${n.streamingReasoning.slice(-90).replace(/\n/g, ' ')}`;
    return 'Waiting for response...';
  }

  // Tool call: show arguments
  if (n.type === 'tool_call') {
    const args = n.details?.arguments;
    return args ? JSON.stringify(args).slice(0, 120) : 'Executing...';
  }

  // Step: show which agent is running
  if (n.type === 'step') {
    const activeChild = [...n.children].reverse().find((c: DebugTreeNode) => c.status === 'running');
    if (activeChild) return `Running: ${activeChild.label}`;
    return 'Starting...';
  }

  return '';
}
</script>

<template>
  <div class="tree-node-wrapper">
    <div
      class="tree-node"
      :class="{
        active: isActive(),
        selected: isSelected(),
        paused: isPaused(),
        running: isActive() && !isPaused(),
        [node.status]: true,
      }"
      :style="{ paddingLeft: depth * 20 + 'px', borderLeftColor: typeColors[node.type] || 'var(--text-secondary)' }"
      @click="showActions = false; emit('select', node)"
      @contextmenu="handleContextMenu"
    >
      <button
        v-if="node.children.length > 0"
        class="toggle"
        @click.stop="expanded = !expanded"
      >{{ expanded ? '\u25BC' : '\u25B6' }}</button>
      <span v-else class="toggle-spacer"></span>

      <span v-if="hasBreakpoint()" class="bp-dot" title="Breakpoint set"></span>
      <span class="status-dot" :class="node.status"></span>
      <span class="type-badge" :style="{ background: isPaused() ? '#ff9800' : typeColors[node.type] || 'var(--text-secondary)' }">
        {{ isPaused() ? '\u23f8' : typeIcons[node.type] || '?' }}
      </span>

      <span class="node-label">{{ node.label }}</span>

      <span class="node-meta" v-if="isPaused()">
        <span class="paused-badge">PAUSED</span>
      </span>
      <span class="node-meta" v-else-if="isActive()">
        <span class="spinner"></span>
        <span v-if="node.iteration !== undefined" class="iter-badge">iter {{ node.iteration }}</span>
      </span>
      <span class="node-meta" v-else>
        <span v-if="node.durationMs" class="duration">{{ fmtDuration(node.durationMs) }}</span>
        <span v-if="node.tokens" class="tokens">{{ fmtTokens(node.tokens) }} tok</span>
      </span>

      <!-- Inline action buttons on selected breakpoint-able nodes -->
      <span v-if="isSelected() && canBreakpoint()" class="node-actions" @click.stop>
        <button
          v-if="isConnected || isPreview"
          class="action-btn bp-btn"
          title="Set breakpoint here"
          @click="emit('set-breakpoint', node)"
        >BP</button>
        <button
          v-if="isConnected"
          class="action-btn run-to-btn"
          title="Run to here"
          @click="emit('run-to-node', node)"
        >RT</button>
        <button
          v-if="isHistory"
          class="action-btn debug-from-btn"
          title="Debug from here"
          @click="emit('debug-from-here', node)"
        >DF</button>
      </span>
    </div>

    <!-- Live status preview for active/paused nodes -->
    <div
      v-if="(isActive() || isPaused()) && liveStatus()"
      class="live-status"
      :style="{ paddingLeft: (depth * 20 + 30) + 'px' }"
    >
      <span class="status-text">{{ liveStatus() }}</span>
    </div>

    <!-- Context menu overlay -->
    <div v-if="showActions && canBreakpoint()" class="context-menu" :style="{ left: depth * 20 + 30 + 'px' }">
      <button v-if="isConnected || isPreview" @click="emit('set-breakpoint', node); showActions = false">
        Set Breakpoint
      </button>
      <button v-if="isConnected" @click="emit('run-to-node', node); showActions = false">
        Run to Here
      </button>
      <button v-if="isHistory" @click="emit('debug-from-here', node); showActions = false">
        Debug from Here
      </button>
    </div>

    <div v-if="expanded && node.children.length > 0" class="children">
      <TreeNode
        v-for="child in node.children"
        :key="child.id"
        :node="child"
        :depth="depth + 1"
        :render-key="renderKey"
        :active-ids="activeIds"
        :selected-id="selectedId"
        :is-debug-mode="isDebugMode"
        :is-connected="isConnected"
        :is-preview="isPreview"
        :is-history="isHistory"
        :breakpointed-agents="breakpointedAgents"
        @select="emit('select', $event)"
        @set-breakpoint="emit('set-breakpoint', $event)"
        @run-to-node="emit('run-to-node', $event)"
        @debug-from-here="emit('debug-from-here', $event)"
      />
    </div>
  </div>
</template>

<style scoped>
.tree-node {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 8px;
  cursor: pointer;
  border-left: 3px solid transparent;
  font-size: 13px;
  transition: background 0.15s;
  color: var(--text-primary);
}
.tree-node:hover { background: rgba(91, 141, 239, 0.08); }
.tree-node.selected { background: rgba(91, 141, 239, 0.15); }
.tree-node.active { border-left-width: 3px; }
.tree-node.error .node-label { color: var(--status-error); }
.tree-node.running {
  background: rgba(34, 197, 94, 0.08);
  border-left-color: var(--status-running);
}
.tree-node.salvaged {
  background: rgba(245, 158, 11, 0.08);
  border-left-color: var(--status-paused);
}
.tree-node.salvaged .node-label { color: var(--status-paused); }
.tree-node.salvaged .node-label::after {
  content: ' \26a0';
  font-size: 11px;
  color: var(--status-paused);
}
/* Iteration badge */
.iter-badge {
  background: var(--accent-agent);
  color: white;
  font-size: 9px;
  font-weight: 600;
  padding: 0 4px;
  border-radius: 3px;
}

/* Live status preview line */
.live-status {
  font-size: 11px;
  color: var(--text-secondary);
  padding: 1px 8px 3px;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
  max-width: 100%;
  border-left: 3px solid transparent;
}
.live-status .status-text {
  font-family: var(--font-mono);
  font-size: 10px;
  color: var(--text-muted);
  background: rgba(255, 255, 255, 0.03);
  padding: 1px 6px;
  border-radius: 3px;
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* Breakpoint dot — red gutter marker like IDE */
.status-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  flex-shrink: 0;
}
.status-dot.success { background: var(--status-success); }
.status-dot.error { background: var(--status-error); }
.status-dot.running { background: var(--accent-agent); animation: pulse 1.5s infinite; }
.status-dot.paused { background: var(--status-paused); }
.status-dot.pending { background: var(--text-muted); }
.status-dot.salvaged { background: var(--status-paused); }
@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}

.bp-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--status-error);
  flex-shrink: 0;
  box-shadow: 0 0 4px rgba(239, 68, 68, 0.5);
}
.tree-node.paused {
  background: rgba(245, 158, 11, 0.15) !important;
  border-left-color: var(--status-paused) !important;
  animation: pause-pulse 1.5s ease-in-out infinite;
}
.tree-node.paused .node-label { color: #f59e0b; font-weight: 600; }
.paused-badge {
  background: var(--status-paused);
  color: white;
  font-size: 9px;
  font-weight: 700;
  padding: 1px 5px;
  border-radius: 3px;
  letter-spacing: 0.5px;
}
@keyframes pause-pulse {
  0%, 100% { background: rgba(245, 158, 11, 0.1); }
  50% { background: rgba(245, 158, 11, 0.22); }
}

/* Inline action buttons */
.node-actions {
  display: flex;
  gap: 3px;
  margin-left: 4px;
  flex-shrink: 0;
}
.action-btn {
  background: none;
  border: 1px solid var(--border-default);
  border-radius: 3px;
  font-size: 9px;
  font-weight: 700;
  padding: 1px 4px;
  cursor: pointer;
  color: var(--text-muted);
  transition: all 0.15s;
}
.action-btn:hover { background: var(--surface-4); }
.bp-btn { color: var(--status-error); border-color: var(--status-error); }
.bp-btn:hover { background: rgba(239, 68, 68, 0.1); }
.run-to-btn { color: var(--accent-agent); border-color: var(--accent-agent); }
.run-to-btn:hover { background: rgba(91, 141, 239, 0.1); }
.debug-from-btn { color: var(--status-paused); border-color: var(--status-paused); }
.debug-from-btn:hover { background: rgba(245, 158, 11, 0.1); }

/* Context menu */
.context-menu {
  position: absolute;
  z-index: 100;
  background: var(--surface-3);
  border: 1px solid var(--border-default);
  border-radius: var(--node-radius);
  box-shadow: 0 4px 12px rgba(0,0,0,0.4);
  padding: 4px 0;
  min-width: 150px;
}
.context-menu button {
  display: block;
  width: 100%;
  background: none;
  border: none;
  padding: 6px 12px;
  font-size: 12px;
  text-align: left;
  cursor: pointer;
  color: var(--text-primary);
}
.context-menu button:hover {
  background: rgba(91, 141, 239, 0.1);
}

.toggle {
  background: none;
  border: none;
  cursor: pointer;
  font-size: 10px;
  width: 16px;
  padding: 0;
  color: var(--text-muted);
}
.toggle-spacer { width: 16px; display: inline-block; }

.type-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border-radius: 4px;
  color: white;
  font-size: 10px;
  font-weight: 700;
  flex-shrink: 0;
}

.node-label {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-primary);
}

.node-meta {
  display: flex;
  gap: 8px;
  font-size: 11px;
  color: var(--text-secondary);
  flex-shrink: 0;
}

.spinner {
  width: 10px;
  height: 10px;
  border: 2px solid var(--status-running);
  border-top-color: transparent;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}
</style>
