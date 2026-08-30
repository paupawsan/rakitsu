<script setup lang="ts">
import { computed, watch, nextTick, onActivated } from 'vue';
import { VueFlow, useVueFlow } from '@vue-flow/core';
import { Background } from '@vue-flow/background';
import { Controls } from '@vue-flow/controls';
import { MiniMap } from '@vue-flow/minimap';
import type { Node, Edge } from '@vue-flow/core';
import DebugPipelineNode from './debug-nodes/DebugPipelineNode.vue';
import DebugAgentNode from './debug-nodes/DebugAgentNode.vue';
import DebugToolNode from './debug-nodes/DebugToolNode.vue';
import DebugStepNode from './debug-nodes/DebugStepNode.vue';
import DebugGenericNode from './debug-nodes/DebugGenericNode.vue';
import ExecIterationNode from './debug-nodes/ExecIterationNode.vue';
import ExecThoughtNode from './debug-nodes/ExecThoughtNode.vue';
import RakitsuExecEdge from '../edges/RakitsuExecEdge.vue';
import WallpaperHost from '../WallpaperHost.vue';
import { useWallpaperPrefs } from '../../composables/useWallpaperPrefs';

const { enabled: wallpaperEnabled } = useWallpaperPrefs();

const props = defineProps<{
  nodes: Node[];
  edges: Edge[];
  visibleUpTo: number;
  totalEvents: number;
  isHistory: boolean;
}>();

const emit = defineEmits<{
  nodeClick: [nodeId: string];
  'update:visibleUpTo': [value: number];
}>();

const { onNodeClick, setNodes, setEdges, fitView, dimensions } = useVueFlow();

// Only render MiniMap/Background when VueFlow has measured real container dimensions (prevents NaN SVG errors)
const miniMapReady = computed(() => dimensions.value.width > 0 && dimensions.value.height > 0);

// Re-fit when the tab becomes active again (DOM reinserted by keep-alive).
onActivated(async () => {
  if (props.nodes.length > 0) {
    await nextTick();
    fitView({ padding: 0.1 });
  }
});

onNodeClick(({ node }) => {
  emit('nodeClick', node.id);
});

// VueFlow manages its own internal state — prop changes alone don't trigger re-renders.
// Explicitly push updates via setNodes/setEdges when props change.
watch(() => props.nodes, (newNodes) => {
  setNodes(newNodes);
}, { deep: true });

watch(() => props.edges, (newEdges) => {
  setEdges(newEdges);
}, { deep: true });

const nodeTypes = {
  'debug-pipeline': DebugPipelineNode,
  'debug-agent': DebugAgentNode,
  'debug-tool': DebugToolNode,
  'debug-step': DebugStepNode,
  'debug-generic': DebugGenericNode,
  'exec-iteration': ExecIterationNode,
  'exec-thought': ExecThoughtNode,
} as any;

const edgeTypes = {
  'rakitsu-exec': RakitsuExecEdge,
} as any;
</script>

<template>
  <div class="execution-graph" :class="{ 'has-wallpaper': wallpaperEnabled }">
    <WallpaperHost />
    <VueFlow
      :nodes="nodes"
      :edges="edges"
      :node-types="nodeTypes"
      :edge-types="edgeTypes"
      :fit-view-on-init="true"
      :default-edge-options="{ type: 'rakitsu-exec' }"
      class="graph-canvas"
    >
      <Background v-if="miniMapReady" variant="dots" :gap="16" :size="1" :color="'rgba(255,255,255,0.06)'" />
      <Controls />
      <MiniMap v-if="miniMapReady" />
    </VueFlow>
    <!-- Playback controls are in DebugView (shared across all visualization styles) -->
  </div>
</template>

<style scoped>
.execution-graph {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--canvas-bg);
  position: relative;
}
.execution-graph.has-wallpaper {
  background: transparent;
}
.execution-graph.has-wallpaper :deep(.vue-flow__background) {
  background: transparent;
}
.execution-graph.has-wallpaper :deep(.vue-flow__pane) {
  background: transparent;
}
.graph-canvas {
  flex: 1;
}
.timeline-slider {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 16px;
  background: var(--surface-2);
  border-top: 1px solid var(--border-subtle);
}
.slider {
  flex: 1;
  accent-color: var(--accent-agent);
}
.slider-label {
  font-size: 12px;
  color: var(--text-muted);
  white-space: nowrap;
}
</style>

<style>
@import '@vue-flow/core/dist/style.css';
@import '@vue-flow/core/dist/theme-default.css';
@import '@vue-flow/controls/dist/style.css';
@import '@vue-flow/minimap/dist/style.css';
</style>
