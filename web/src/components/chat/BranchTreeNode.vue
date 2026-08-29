<script setup lang="ts">
// BranchTreeNode — Vue Flow node card for a single conversation turn in
// the Tree Viewer. Renders short id + status, truncated user
// text, entry count, and a rollback / agent-retry badge when
// the node's RetryCtx is populated.

import { computed } from 'vue';
import { Handle, Position } from '@vue-flow/core';
import type { BranchTreeNodeData } from '../../composables/useChatTree';

const props = defineProps<{
  data: BranchTreeNodeData;
  selected?: boolean;
}>();

const truncated = computed(() => {
  const t = props.data.userText ?? '';
  return t.length > 60 ? t.slice(0, 57) + '…' : t;
});

const timeLabel = computed(() => {
  try {
    const d = new Date(props.data.created);
    if (Number.isNaN(d.getTime())) return '';
    return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  } catch {
    return '';
  }
});

const statusGlyph = computed(() => {
  switch (props.data.status) {
    case 'complete':    return '✓';
    case 'generating':  return '…';
    case 'interrupted': return '⏸';
    case 'failed':      return '✗';
    default:            return '?';
  }
});
</script>

<template>
  <div
    class="branch-tree-node"
    :class="{ active: data.isActive, selected: !!selected, [`status-${data.status}`]: true }"
  >
    <Handle type="target" :position="Position.Top" />
    <div class="header">
      <span class="short-id" :title="data.id">{{ data.shortId }}</span>
      <span v-if="timeLabel" class="time">{{ timeLabel }}</span>
      <span class="status" :title="data.status">{{ statusGlyph }}</span>
    </div>
    <div class="user-text" :title="data.userText">{{ truncated || '(no user text)' }}</div>
    <div class="footer">
      <span class="entry-count">{{ data.entryCount }} {{ data.entryCount === 1 ? 'entry' : 'entries' }}</span>
      <span
        v-if="data.retrySource"
        class="retry-badge"
        :title="data.retryReason || data.retrySource"
      >↺ {{ data.retrySource }}</span>
    </div>
    <Handle type="source" :position="Position.Bottom" />
  </div>
</template>

<style scoped>
.branch-tree-node {
  box-sizing: border-box;
  width: 280px;
  height: 120px;
  padding: 0.5em 0.7em;
  border-radius: 8px;
  border: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.18));
  background: var(--surface-1, rgba(20, 22, 30, 0.85));
  color: var(--fg, rgba(255, 255, 255, 0.92));
  font-family: var(--font-sans);
  font-size: 0.85em;
  display: flex;
  flex-direction: column;
  gap: 0.35em;
  transition: border-color 120ms ease, box-shadow 120ms ease;
}

.branch-tree-node.active {
  border-color: var(--accent, #4f8cff);
  box-shadow: 0 0 0 1px var(--accent, #4f8cff), 0 4px 14px rgba(79, 140, 255, 0.18);
}

.branch-tree-node.selected {
  border-color: var(--warning, #f5a623);
}

.branch-tree-node.status-failed {
  background: rgba(255, 80, 80, 0.06);
}

.header {
  display: flex;
  align-items: center;
  gap: 0.5em;
  font-family: var(--font-mono);
  font-size: 0.78em;
  opacity: 0.75;
}

.short-id { font-weight: 600; }
.time { margin-left: auto; }
.status { font-size: 1.05em; opacity: 0.9; }

.user-text {
  flex: 1;
  font-size: 0.95em;
  line-height: 1.35;
  overflow: hidden;
  text-overflow: ellipsis;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  word-break: break-word;
}

.footer {
  display: flex;
  align-items: center;
  gap: 0.5em;
  font-family: var(--font-mono);
  font-size: 0.75em;
  opacity: 0.7;
}

.retry-badge {
  margin-left: auto;
  padding: 0 0.4em;
  border-radius: 4px;
  background: rgba(245, 166, 35, 0.18);
  color: var(--warning, #f5a623);
  border: 1px solid rgba(245, 166, 35, 0.35);
}
</style>
