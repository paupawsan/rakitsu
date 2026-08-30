<script setup lang="ts">
import { computed } from 'vue';
import MultiHandles from './MultiHandles.vue';

const props = defineProps<{
  data: {
    label: string;
    status: string;
    active: boolean;
    streamingText?: string;
    streamingReasoning?: string;
    durationMs?: number;
  };
}>();

// Prefer the committed-answer stream; fall back to reasoning so the node
// shows live progress during the long reasoning_content phase instead of
// just the static label.
const displayText = computed(() => {
  const text = props.data.streamingText || props.data.streamingReasoning || props.data.label || '';
  return text.length > 120 ? text.slice(0, 120) + '...' : text;
});

const isReasoningOnly = computed(() =>
  !props.data.streamingText && !!props.data.streamingReasoning,
);

function fmtDuration(ms?: number): string {
  if (!ms) return '';
  return ms >= 1000 ? `${(ms / 1000).toFixed(1)}s` : `${ms}ms`;
}
</script>

<template>
  <div class="exec-thought-node" :class="[data.status, { active: data.active }]">
    <MultiHandles />
    <div class="thought-header">
      <span class="thought-icon">T</span>
      <span v-if="data.durationMs" class="thought-duration">{{ fmtDuration(data.durationMs) }}</span>
    </div>
    <div
      class="thought-text"
      :class="{
        streaming: data.active && (data.streamingText || data.streamingReasoning),
        reasoning: isReasoningOnly,
      }"
    >
      <span v-if="isReasoningOnly" class="reasoning-prefix">…</span>{{ displayText }}
    </div>
  </div>
</template>

<style scoped>
.exec-thought-node {
  padding: 5px 8px;
  border-radius: var(--node-radius);
  background: var(--surface-3);
  border: 1px solid var(--node-border);
  font-size: 10px;
  color: var(--text-secondary);
  max-width: 200px;
  min-width: 120px;
}
.exec-thought-node.running {
  border-color: var(--accent-agent);
}
.exec-thought-node.success {
  border-color: var(--border-subtle);
}
.exec-thought-node.error {
  border-color: var(--status-error);
}
.thought-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 2px;
}
.thought-icon {
  font-family: var(--font-mono);
  font-size: 9px;
  font-weight: 700;
  color: var(--accent-agent);
  background: rgba(91, 141, 239, 0.15);
  padding: 0 4px;
  border-radius: 2px;
}
.thought-duration {
  font-family: var(--font-mono);
  font-size: 9px;
  color: var(--text-muted);
}
.thought-text {
  font-family: var(--font-mono);
  font-size: 9px;
  line-height: 1.3;
  color: var(--text-secondary);
  overflow: hidden;
  word-break: break-word;
}
.thought-text.streaming::after {
  content: '|';
  animation: cursor-blink 0.8s infinite;
  color: var(--accent-agent);
}
.thought-text.reasoning {
  font-style: italic;
  color: var(--text-muted);
}
.reasoning-prefix {
  font-family: var(--font-mono);
  margin-right: 2px;
  opacity: 0.7;
}
@keyframes cursor-blink {
  0%, 50% { opacity: 1; }
  51%, 100% { opacity: 0; }
}

/* VueFlow handles */
:deep(.vue-flow__handle) {
  width: 6px !important;
  height: 6px !important;
  background: var(--node-border) !important;
  border: none !important;
  opacity: 0;
}
</style>
