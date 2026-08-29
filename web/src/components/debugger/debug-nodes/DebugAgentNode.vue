<script setup lang="ts">
import MultiHandles from './MultiHandles.vue';

defineProps<{
  data: {
    label: string;
    model: string;
    status: string;
    active: boolean;
    tokens?: { total_tokens: number };
  };
}>();
</script>

<template>
  <div class="debug-agent-node" :class="[data.status, { active: data.active }]">
    <MultiHandles />
    <div class="node-header">{{ data.label }}</div>
    <div class="node-model" v-if="data.model">{{ data.model }}</div>
    <div class="node-tokens" v-if="data.tokens">
      {{ data.tokens.total_tokens.toLocaleString() }} tokens
    </div>
  </div>
</template>

<style scoped>
.debug-agent-node {
  padding: 8px 12px;
  border-radius: var(--node-radius);
  background: var(--surface-3);
  border-left: 4px solid var(--accent-agent);
  border-top: 1px solid var(--node-border);
  border-right: 1px solid var(--node-border);
  border-bottom: 1px solid var(--node-border);
  color: var(--text-primary);
  font-size: 12px;
  min-width: 140px;
  text-align: center;
}
.debug-agent-node.pending { border-style: dashed; border-color: var(--text-muted); background: var(--surface-2); color: var(--text-muted); }
.debug-agent-node.running { border-color: var(--status-running); border-left-color: var(--status-running); }
.debug-agent-node.error { border-color: var(--status-error); border-left-color: var(--status-error); }
.debug-agent-node.success { opacity: 0.85; }
.debug-agent-node.salvaged {
  /* Track A: Phase 6.2 Mitigation A — agent terminated with raw reasoning
     instead of a committed answer. Visually distinct from both success
     (green) and error (red): amber border + ⚠ glyph so the operator
     immediately sees this iteration didn't actually answer. */
  border-color: var(--status-paused);
  border-left-color: var(--status-paused);
  background: rgba(245, 158, 11, 0.06);
}
.debug-agent-node.salvaged::after {
  content: '\26a0';
  font-size: 11px;
  margin-left: 4px;
  color: var(--status-paused);
}
.debug-agent-node.paused {
  background: rgba(245, 158, 11, 0.1) !important;
  border-color: var(--status-paused) !important;
  color: var(--status-paused);
  animation: pause-pulse 1.5s ease-in-out infinite;
}
.debug-agent-node.paused::after {
  content: '\23f8';
  font-size: 12px;
  margin-left: 4px;
  color: var(--status-paused);
}
.debug-agent-node.active {
  animation: node-pulse 2s infinite;
}
.node-header { font-weight: 600; margin-bottom: 2px; color: var(--text-primary); }
.node-model { font-size: 10px; color: var(--text-secondary); font-family: var(--font-mono); }
.node-tokens { font-size: 10px; margin-top: 4px; color: var(--text-secondary); font-family: var(--font-mono); }

@keyframes pause-pulse {
  0%, 100% { background: rgba(245, 158, 11, 0.08); }
  50% { background: rgba(245, 158, 11, 0.18); }
}
@keyframes node-pulse {
  0%, 100% { box-shadow: 0 0 0 2px rgba(34, 197, 94, 0.5); }
  50% { box-shadow: 0 0 0 4px rgba(34, 197, 94, 0.3), 0 0 12px rgba(34, 197, 94, 0.2); }
}
</style>
