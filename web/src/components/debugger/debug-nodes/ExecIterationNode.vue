<script setup lang="ts">
import MultiHandles from './MultiHandles.vue';

defineProps<{
  data: {
    label: string;
    status: string;
    active: boolean;
    iteration: number;
    hiddenBefore?: number;
  };
}>();
</script>

<template>
  <div class="exec-iter-node" :class="[data.status, { active: data.active }]">
    <MultiHandles />
    <span v-if="data.hiddenBefore" class="hidden-count">+{{ data.hiddenBefore }}</span>
    <span class="iter-label">Iter {{ data.iteration }}</span>
    <span class="status-dot"></span>
  </div>
</template>

<style scoped>
.exec-iter-node {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 3px 10px;
  border-radius: 12px;
  background: var(--surface-3);
  border: 1px solid var(--node-border);
  font-family: var(--font-mono);
  font-size: 10px;
  color: var(--text-secondary);
  white-space: nowrap;
}
.exec-iter-node.running {
  border-color: var(--status-running);
  color: var(--status-running);
}
.exec-iter-node.paused {
  border-color: var(--status-paused);
  color: var(--status-paused);
}
.exec-iter-node.success {
  border-color: var(--border-subtle);
  color: var(--text-muted);
}
.exec-iter-node.error {
  border-color: var(--status-error);
  color: var(--status-error);
}
.exec-iter-node.salvaged {
  /* Track A: iteration terminated with salvaged_no_progress — model
     produced raw reasoning rather than a committed answer. Amber dot +
     border to distinguish from success/error. */
  border-color: var(--status-paused);
  color: var(--status-paused);
}
.iter-label {
  font-weight: 600;
}
.hidden-count {
  font-size: 9px;
  color: var(--text-muted);
  margin-right: 2px;
}
.status-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--text-muted);
  flex-shrink: 0;
}
.running .status-dot { background: var(--status-running); box-shadow: 0 0 4px var(--status-running); }
.paused .status-dot { background: var(--status-paused); box-shadow: 0 0 4px var(--status-paused); }
.success .status-dot { background: var(--status-success, #22c55e); }
.error .status-dot { background: var(--status-error); }
.salvaged .status-dot { background: var(--status-paused); box-shadow: 0 0 4px var(--status-paused); }

/* VueFlow handles */
:deep(.vue-flow__handle) {
  width: 6px !important;
  height: 6px !important;
  background: var(--node-border) !important;
  border: none !important;
  opacity: 0;
}
</style>
