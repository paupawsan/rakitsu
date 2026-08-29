<script setup lang="ts">
import type { DebugTreeNode } from '../../../types';
import HierarchyTree from '../HierarchyTree.vue';

const props = defineProps<{
  roots: DebugTreeNode[];
  activeIds: Set<string>;
  selectedId?: string;
  renderKey?: number;
  visibleUpTo: number;
  totalEvents: number;
  isHistory: boolean;
  isDebugMode?: boolean;
  isConnected?: boolean;
  isPreview?: boolean;
  breakpointedAgents?: Set<string>;
  visible: boolean;
}>();

const emit = defineEmits<{
  select: [node: DebugTreeNode];
  'update:visibleUpTo': [value: number];
  'set-breakpoint': [node: DebugTreeNode];
  'run-to-node': [node: DebugTreeNode];
  'debug-from-here': [node: DebugTreeNode];
}>();
</script>

<template>
  <HierarchyTree
    :roots="roots"
    :render-key="renderKey"
    :active-ids="activeIds"
    :selected-id="selectedId"
    :visible-up-to="visibleUpTo"
    :total-events="totalEvents"
    :is-history="isHistory"
    :is-debug-mode="isDebugMode"
    :is-connected="isConnected"
    :is-preview="isPreview"
    :breakpointed-agents="breakpointedAgents"
    @select="emit('select', $event)"
    @update:visible-up-to="emit('update:visibleUpTo', $event)"
    @set-breakpoint="emit('set-breakpoint', $event)"
    @run-to-node="emit('run-to-node', $event)"
    @debug-from-here="emit('debug-from-here', $event)"
  />
</template>
