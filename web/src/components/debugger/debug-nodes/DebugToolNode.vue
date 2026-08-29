<script setup lang="ts">
import MultiHandles from './MultiHandles.vue';

defineProps<{
  data: {
    label: string;
    status: string;
    active: boolean;
    durationMs?: number;
  };
}>();

function fmtDuration(ms?: number): string {
  if (!ms) return '';
  return ms >= 1000 ? `${(ms / 1000).toFixed(1)}s` : `${ms}ms`;
}
</script>

<template>
  <div class="debug-tool-node" :class="[data.status, { active: data.active }]">
    <MultiHandles />
    <div class="node-name">{{ data.label }}</div>
    <div class="node-meta">
      <span v-if="data.durationMs">{{ fmtDuration(data.durationMs) }}</span>
      <span class="status-icon">{{ data.status === 'error' ? 'x' : data.status === 'success' ? 'ok' : '...' }}</span>
    </div>
  </div>
</template>

<style scoped>
.debug-tool-node {
  padding: 6px 10px;
  border-radius: var(--node-radius);
  background: var(--surface-3);
  border-left: 4px solid var(--accent-tool);
  border-top: 1px solid var(--node-border);
  border-right: 1px solid var(--node-border);
  border-bottom: 1px solid var(--node-border);
  color: var(--text-primary);
  font-size: 11px;
  min-width: 100px;
  text-align: center;
}
.debug-tool-node.running { border-color: var(--status-running); border-left-color: var(--status-running); }
.debug-tool-node.error { border-color: var(--status-error); border-left-color: var(--status-error); }
.debug-tool-node.paused {
  background: rgba(245, 158, 11, 0.1) !important;
  border-color: var(--status-paused) !important;
  color: var(--status-paused);
  animation: pause-pulse 1.5s ease-in-out infinite;
}
.debug-tool-node.paused::after {
  content: '\23f8';
  font-size: 12px;
  margin-left: 4px;
  color: var(--status-paused);
}
.debug-tool-node.active {
  animation: tool-pulse 2s infinite;
}
.node-name { font-weight: 600; color: var(--text-primary); }
.node-meta { font-size: 10px; color: var(--text-secondary); display: flex; gap: 6px; justify-content: center; margin-top: 2px; font-family: var(--font-mono); }
.status-icon { font-weight: 700; }

@keyframes pause-pulse {
  0%, 100% { background: rgba(245, 158, 11, 0.08); }
  50% { background: rgba(245, 158, 11, 0.18); }
}
@keyframes tool-pulse {
  0%, 100% { box-shadow: 0 0 0 2px rgba(34, 197, 94, 0.5); }
  50% { box-shadow: 0 0 0 4px rgba(34, 197, 94, 0.3), 0 0 12px rgba(34, 197, 94, 0.2); }
}
</style>
