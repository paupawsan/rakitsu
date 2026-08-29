<script setup lang="ts">
import MultiHandles from './MultiHandles.vue';

defineProps<{
  data: {
    label: string;
    nodeType: string;
    status: string;
    active: boolean;
    stepType: string;
    agents: string[];
  };
}>();
</script>

<template>
  <div class="debug-step-node" :class="[data.status, { active: data.active }]">
    <MultiHandles />
    <div class="step-header">
      <span class="step-badge">{{ data.stepType || 'seq' }}</span>
      <span class="step-label">{{ data.label }}</span>
    </div>
  </div>
</template>

<style scoped>
.debug-step-node {
  position: relative;
  padding: 0;
  border-radius: var(--node-radius);
  background: var(--surface-3);
  border: 2px solid var(--accent-skill);
  min-width: 150px;
  min-height: 60px;
}
.debug-step-node.pending { border-style: dashed; border-color: var(--text-muted); background: var(--surface-2); }
.debug-step-node.running { border-color: var(--status-running); }
.debug-step-node.success { border-color: var(--accent-skill); }
.debug-step-node.error { border-color: var(--status-error); }
.debug-step-node.paused {
  background: rgba(245, 158, 11, 0.1) !important;
  border-color: var(--status-paused) !important;
  animation: pause-pulse 1.5s ease-in-out infinite;
}
.debug-step-node.paused .step-header { color: var(--status-paused); }
.debug-step-node.paused .step-badge { background: rgba(245, 158, 11, 0.2); color: var(--status-paused); }
.debug-step-node.paused::after {
  content: '\23f8';
  font-size: 12px;
  position: absolute;
  top: 6px;
  right: 8px;
  color: var(--status-paused);
}
.debug-step-node.active { animation: step-pulse 2s infinite; }
.step-header {
  padding: 6px 10px;
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
  color: var(--accent-skill);
  border-bottom: 1px solid var(--border-subtle);
}
.debug-step-node.pending .step-header { color: var(--text-muted); }
.step-badge {
  background: rgba(192, 132, 252, 0.15); color: var(--accent-skill);
  padding: 1px 6px; border-radius: 4px;
  font-size: 9px; font-weight: 600; text-transform: uppercase;
  font-family: var(--font-mono);
}
.debug-step-node.pending .step-badge { background: var(--surface-4); color: var(--text-muted); }
.step-label { font-weight: 600; color: var(--text-primary); }

@keyframes pause-pulse {
  0%, 100% { background: rgba(245, 158, 11, 0.08); }
  50% { background: rgba(245, 158, 11, 0.18); }
}
@keyframes step-pulse {
  0%, 100% { box-shadow: 0 0 0 2px rgba(34, 197, 94, 0.5); }
  50% { box-shadow: 0 0 0 4px rgba(34, 197, 94, 0.3), 0 0 12px rgba(34, 197, 94, 0.2); }
}
</style>
