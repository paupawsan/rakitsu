<script setup lang="ts">
import { computed, watch, onActivated } from 'vue';
import { VueFlow, useVueFlow } from '@vue-flow/core';
import { Background } from '@vue-flow/background';
import { Controls } from '@vue-flow/controls';
import { debugNodeTypes } from '../debug-nodes';
import type { DebugTreeNode } from '../../../types';
import { useTimelineLayout } from '../../../composables/useTimelineLayout';

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

const rootsRef = computed(() => { void props.renderKey; return props.roots; });
const activeRef = computed(() => { void props.renderKey; return props.activeIds; });

const { graphNodes, graphEdges } = useTimelineLayout(rootsRef, activeRef);

const { setNodes, setEdges, fitView } = useVueFlow({ id: 'timeline-flow' });

watch([graphNodes, graphEdges], ([n, e]) => {
  setNodes(n);
  setEdges(e);
}, { immediate: true });

onActivated(() => {
  setTimeout(() => fitView({ padding: 0.2 }), 100);
});

function onNodeClick(event: { node: { id: string } }) {
  emit('node-click', event.node.id);
}
</script>

<template>
  <div class="timeline-style">
    <VueFlow
      :id="'timeline-flow'"
      :node-types="debugNodeTypes"
      :default-edge-options="{ type: 'smoothstep' }"
      :min-zoom="0.1"
      :max-zoom="3"
      fit-view-on-init
      @node-click="onNodeClick"
    >
      <Background />
      <Controls />
    </VueFlow>
    <!-- Playback controls are in DebugView (shared) -->
  </div>
</template>

<style scoped>
.timeline-style {
  display: flex;
  flex-direction: column;
  height: 100%;
}
.timeline-style :deep(.vue-flow) {
  flex: 1;
}
</style>
