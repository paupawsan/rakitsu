<script setup lang="ts">
import MultiHandles from './MultiHandles.vue';

defineProps<{
  data: {
    label: string;
    status: string;
    active: boolean;
  };
}>();
</script>

<template>
  <div class="debug-pipeline-node" :class="[data.status, { active: data.active }]">
    <MultiHandles />
    <div class="pipeline-header">{{ data.label }}</div>
  </div>
</template>

<style scoped>
.debug-pipeline-node {
  position: relative;
  padding: 0;
  border-radius: var(--node-radius);
  background: var(--surface-2);
  border: 2px solid var(--node-border);
  min-width: 200px;
}
.debug-pipeline-node.pending { border-style: dashed; border-color: var(--text-muted); }
.debug-pipeline-node.running { border-color: var(--status-running); }
.debug-pipeline-node.success { border-color: var(--status-running); }
.debug-pipeline-node.error { border-color: var(--status-error); }
.debug-pipeline-node.paused {
  background: rgba(245, 158, 11, 0.1) !important;
  border-color: var(--status-paused) !important;
  animation: pause-pulse 1.5s ease-in-out infinite;
}
.debug-pipeline-node.paused .pipeline-header { color: var(--status-paused); background: rgba(245, 158, 11, 0.15); }
.debug-pipeline-node.paused::after {
  content: '\23f8';
  font-size: 14px;
  position: absolute;
  top: 8px;
  right: 10px;
  color: var(--status-paused);
}
.debug-pipeline-node.active {
  animation: pipeline-pulse 2s infinite;
}
.pipeline-header {
  padding: 8px 14px;
  font-size: 13px;
  font-weight: 700;
  color: var(--text-primary);
  border-bottom: 1px solid var(--border-subtle);
  background: var(--surface-3);
  border-radius: calc(var(--node-radius) - 2px) calc(var(--node-radius) - 2px) 0 0;
}

@keyframes pause-pulse {
  0%, 100% { background: rgba(245, 158, 11, 0.08); }
  50% { background: rgba(245, 158, 11, 0.18); }
}
@keyframes pipeline-pulse {
  0%, 100% { box-shadow: 0 0 0 2px rgba(34, 197, 94, 0.3); }
  50% { box-shadow: 0 0 0 4px rgba(34, 197, 94, 0.2), 0 0 16px rgba(34, 197, 94, 0.1); }
}
</style>
