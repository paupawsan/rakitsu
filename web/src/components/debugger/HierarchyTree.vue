<script setup lang="ts">
import { ref, watch, nextTick } from 'vue';
import type { DebugTreeNode } from '../../types';
import TreeNode from './TreeNode.vue';

const props = defineProps<{
  roots: DebugTreeNode[];
  activeIds: Set<string>;
  selectedId?: string;
  renderKey?: number;
  visibleUpTo?: number;
  totalEvents?: number;
  isHistory?: boolean;
  isDebugMode?: boolean;
  isConnected?: boolean;
  isPreview?: boolean;
  breakpointedAgents?: Set<string>;
}>();

const emit = defineEmits<{
  select: [node: DebugTreeNode];
  'update:visibleUpTo': [value: number];
  'set-breakpoint': [node: DebugTreeNode];
  'run-to-node': [node: DebugTreeNode];
  'debug-from-here': [node: DebugTreeNode];
}>();

const search = ref('');
const treeContainer = ref<HTMLElement>();

// Auto-scroll to bottom when new nodes appear
watch(
  () => props.roots.length,
  async () => {
    await nextTick();
    if (treeContainer.value) {
      treeContainer.value.scrollTop = treeContainer.value.scrollHeight;
    }
  },
);

function matchesSearch(node: DebugTreeNode): boolean {
  if (!search.value) return true;
  const q = search.value.toLowerCase();
  if (node.label.toLowerCase().includes(q)) return true;
  if (node.agentName.toLowerCase().includes(q)) return true;
  return node.children.some(matchesSearch);
}

</script>

<template>
  <div class="hierarchy-tree">
    <div class="tree-search">
      <input
        v-model="search"
        type="text"
        placeholder="Filter by agent or label..."
        class="search-input"
      />
    </div>
    <div class="tree-content" ref="treeContainer">
      <template v-if="roots.length > 0">
        <template v-for="root in roots" :key="root.id">
          <TreeNode
            v-if="matchesSearch(root)"
            :node="root"
            :depth="0"
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
        </template>
      </template>
      <div v-else class="empty-state">
        Waiting for events...
      </div>
    </div>

    <!-- Playback controls are in DebugView (shared across all visualization styles) -->
  </div>
</template>

<style scoped>
.hierarchy-tree {
  display: flex;
  flex-direction: column;
  height: 100%;
}
.tree-search {
  padding: 8px;
  border-bottom: 1px solid var(--border-subtle);
}
.search-input {
  width: 100%;
  padding: 6px 10px;
  border: 1px solid var(--input-border);
  border-radius: var(--node-radius);
  font-size: 12px;
  outline: none;
  background: var(--input-bg);
  color: var(--input-text);
}
.search-input::placeholder { color: var(--input-placeholder); }
.search-input:focus { border-color: var(--border-focus); }
.tree-content {
  flex: 1;
  overflow-y: auto;
  padding: 4px 0 40px 0;
}
.empty-state {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  color: var(--text-muted);
  font-size: 14px;
}
</style>
