<script setup lang="ts">
import { computed } from 'vue';

const props = defineProps<{
  text: string;
  totalTokens?: number;
}>();

interface TokenSpan {
  text: string;
  estimatedTokens: number;
}

const spans = computed<TokenSpan[]>(() => {
  if (!props.text) return [];
  // Split by whitespace, keeping delimiters
  const parts = props.text.split(/(\s+)/);
  return parts.map((part) => {
    if (/^\s+$/.test(part)) {
      return { text: part, estimatedTokens: 0 };
    }
    // Rough estimate: ~0.75 tokens per word, longer words cost more
    const est = Math.max(1, Math.round(part.length / 4));
    return { text: part, estimatedTokens: est };
  });
});

function tokenColor(tokens: number): string {
  if (tokens <= 0) return 'transparent';
  if (tokens <= 1) return 'rgba(34, 197, 94, 0.15)';
  if (tokens <= 3) return 'rgba(245, 158, 11, 0.2)';
  return 'rgba(245, 158, 11, 0.3)';
}
</script>

<template>
  <div class="token-highlight">
    <span
      v-for="(span, i) in spans"
      :key="i"
      :style="{ background: tokenColor(span.estimatedTokens) }"
      :title="span.estimatedTokens > 0 ? `~${span.estimatedTokens} token(s)` : ''"
      class="token-span"
    >{{ span.text }}</span>
    <div v-if="totalTokens" class="token-hint">
      Actual: {{ totalTokens.toLocaleString() }} tokens
    </div>
  </div>
</template>

<style scoped>
.token-highlight {
  font-family: var(--font-mono);
  font-size: 11px;
  white-space: pre-wrap;
  word-break: break-all;
  line-height: 1.6;
  padding: 8px;
  background: var(--surface-1);
  border: 1px solid var(--border-subtle);
  border-radius: var(--node-radius);
  max-height: 300px;
  overflow-y: auto;
  color: var(--text-primary);
}
.token-span {
  border-radius: 2px;
}
.token-hint {
  margin-top: 4px;
  font-size: 10px;
  color: var(--text-secondary);
  font-family: var(--font-sans);
}
</style>
