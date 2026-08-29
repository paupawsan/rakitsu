<script setup lang="ts">
import { ref, computed, watch, provide } from 'vue';
import type { DebugTreeNode } from '../../../types';
import ExecutionGraph from '../ExecutionGraph.vue';
import {
  layoutCallTree,
  DEFAULT_SPACING,
} from '../../../composables/useExecutionTreeLayout';

const props = defineProps<{
  roots: DebugTreeNode[];
  activeIds: Set<string>;
  selectedId?: string;
  renderKey?: number;
  visibleUpTo: number;
  totalEvents: number;
  isHistory: boolean;
  visible: boolean;
}>();

const emit = defineEmits<{
  'node-click': [nodeId: string];
  'update:visibleUpTo': [value: number];
}>();

const expandedAgents = ref<Set<string>>(new Set());

function toggleExpand(agentName: string) {
  const s = new Set(expandedAgents.value);
  if (s.has(agentName)) s.delete(agentName);
  else s.add(agentName);
  expandedAgents.value = s;
}

function expandAll() {
  const names = new Set<string>();
  function collect(node: DebugTreeNode) {
    if (node.type === 'agent') names.add(node.agentName);
    for (const c of node.children) collect(c);
  }
  for (const r of props.roots) collect(r);
  expandedAgents.value = names;
}

function collapseAll() {
  expandedAgents.value = new Set();
}

// Auto-expand running/paused agents
watch(() => props.renderKey, () => {
  for (const root of props.roots) {
    autoExpandRunning(root);
  }
});

function autoExpandRunning(node: DebugTreeNode) {
  if (node.type === 'agent' && (node.status === 'running' || node.status === 'paused')) {
    if (!expandedAgents.value.has(node.agentName)) {
      const s = new Set(expandedAgents.value);
      s.add(node.agentName);
      expandedAgents.value = s;
    }
  }
  for (const c of node.children) autoExpandRunning(c);
}

// Provide expand controls for debug nodes that need them
provide('expandedAgents', expandedAgents);
provide('toggleExpand', toggleExpand);

const layoutResult = computed(() => {
  // Touch renderKey to force recomputation on in-place tree mutations
  void props.renderKey;
  if (props.roots.length === 0) return { nodes: [], edges: [] };
  return layoutCallTree(
    props.roots,
    expandedAgents.value,
    40,  // originX — small left margin
    40,  // originY — small top margin
    DEFAULT_SPACING,
  );
});

const graphNodes = computed(() => layoutResult.value.nodes);
const graphEdges = computed(() => layoutResult.value.edges);

function onNodeClick(nodeId: string) {
  // Strip exec_ prefix that layoutCallTree adds, so DebugView can match the tree node
  const rawId = nodeId.startsWith('exec_') ? nodeId.slice(5) : nodeId;

  // Toggle agent expand on click
  for (const root of props.roots) {
    const found = findNode(root, rawId);
    if (found && found.type === 'agent') {
      toggleExpand(found.agentName);
      return;
    }
  }

  emit('node-click', rawId);
}

function findNode(node: DebugTreeNode, id: string): DebugTreeNode | null {
  if (node.id === id) return node;
  for (const c of node.children) {
    const found = findNode(c, id);
    if (found) return found;
  }
  return null;
}
</script>

<template>
  <div class="block-diagram-wrapper">
    <div class="block-toolbar">
      <button class="toolbar-btn" title="Expand all agents" @click="expandAll">
        <span class="toolbar-icon">+</span> Expand All
      </button>
      <button class="toolbar-btn" title="Collapse all agents" @click="collapseAll">
        <span class="toolbar-icon">&minus;</span> Collapse All
      </button>
    </div>
    <ExecutionGraph
      :nodes="graphNodes"
      :edges="graphEdges"
      :visible-up-to="visibleUpTo"
      :total-events="totalEvents"
      :is-history="isHistory"
      :visible="visible"
      @node-click="onNodeClick"
      @update:visible-up-to="emit('update:visibleUpTo', $event)"
    />
  </div>
</template>

<style scoped>
.block-diagram-wrapper {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.block-toolbar {
  display: flex;
  gap: 6px;
  padding: 6px 10px;
  background: var(--surface-2);
  border-bottom: 1px solid var(--border-subtle);
  flex-shrink: 0;
}

.toolbar-btn {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 3px 8px;
  font-size: 11px;
  color: var(--text-muted);
  background: var(--surface-3, var(--surface-1));
  border: 1px solid var(--border-subtle);
  border-radius: 4px;
  cursor: pointer;
  transition: color 0.15s, border-color 0.15s;
}
.toolbar-btn:hover {
  color: var(--text-primary);
  border-color: var(--text-muted);
}

.toolbar-icon {
  font-weight: 700;
  font-size: 13px;
  line-height: 1;
}
</style>
