<script setup lang="ts">
import MultiHandles from './MultiHandles.vue';
import { computed } from 'vue';

const props = defineProps<{
  data: {
    label: string;
    nodeType: string;
    status: string;
    active: boolean;
    durationMs?: number;
  };
}>();

const colorMap: Record<string, string> = {
  thought: 'var(--accent-agent)',
  reflection: 'var(--accent-skill)',
  ground_check: 'var(--status-paused)',
  handoff: 'var(--accent-tool)',
  error: 'var(--status-error)',
};

const bgColor = computed(() => colorMap[props.data.nodeType] || 'var(--surface-4)');

function fmtDuration(ms?: number): string {
  if (!ms) return '';
  return ms >= 1000 ? `${(ms / 1000).toFixed(1)}s` : `${ms}ms`;
}
</script>

<template>
  <div
    class="debug-generic-node"
    :class="[data.status, { active: data.active }]"
    :style="{ background: bgColor }"
  >
    <MultiHandles />
    <div class="node-type">{{ data.nodeType.replace('_', ' ') }}</div>
    <div class="node-label">{{ data.label }}</div>
    <div class="node-duration" v-if="data.durationMs">{{ fmtDuration(data.durationMs) }}</div>
  </div>
</template>

<style scoped>
.debug-generic-node {
  padding: 6px 10px;
  border-radius: var(--node-radius);
  color: white;
  font-size: 11px;
  min-width: 120px;
  text-align: center;
  border: 2px solid transparent;
}
.debug-generic-node.running { border-color: var(--status-running); }
.debug-generic-node.error { border-color: var(--status-error); }
.debug-generic-node.success { opacity: 0.85; }
.debug-generic-node.paused {
  background: rgba(245, 158, 11, 0.1) !important;
  border-color: var(--status-paused) !important;
  color: var(--status-paused);
  animation: pause-pulse 1.5s ease-in-out infinite;
}
.debug-generic-node.paused::after {
  content: '\23f8';
  font-size: 12px;
  margin-left: 4px;
  color: var(--status-paused);
}
.debug-generic-node.active {
  animation: generic-pulse 2s infinite;
}
.node-type {
  font-size: 9px;
  text-transform: uppercase;
  opacity: 0.8;
  letter-spacing: 0.5px;
  margin-bottom: 2px;
  font-family: var(--font-mono);
}
.node-label {
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 160px;
}
.node-duration {
  font-size: 9px;
  opacity: 0.8;
  margin-top: 2px;
  font-family: var(--font-mono);
}

@keyframes pause-pulse {
  0%, 100% { background: rgba(245, 158, 11, 0.08); }
  50% { background: rgba(245, 158, 11, 0.18); }
}
@keyframes generic-pulse {
  0%, 100% { box-shadow: 0 0 0 2px rgba(34, 197, 94, 0.5); }
  50% { box-shadow: 0 0 0 4px rgba(34, 197, 94, 0.3), 0 0 12px rgba(34, 197, 94, 0.2); }
}
</style>
