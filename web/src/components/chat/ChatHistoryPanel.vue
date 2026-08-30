<script setup lang="ts">
import { computed } from 'vue';
import type { ChatBlock } from '../../types/chat';

// Read-only counterpart to ChatPanel.vue. Renders a pre-computed transcript
// for a past chat session — no WebSocket, no input. The parent is expected
// to reconstruct `blocks` from the session's event log (see
// composables/reconstructChat.ts).
const props = defineProps<{
  blocks: ChatBlock[];
  title?: string;
}>();

const empty = computed(() => props.blocks.length === 0);
</script>

<template>
  <div class="chat-panel history">
    <header class="chat-header">
      <div class="title">
        <span class="agent-name">{{ title || 'chat' }}</span>
        <span class="model-tag">history</span>
      </div>
      <div class="status">
        <span class="led off"></span>
        <span>read-only</span>
      </div>
    </header>

    <div class="messages">
      <div
        v-for="(block, i) in blocks"
        :key="i"
        :class="['block', `block-${block.kind}`]"
      >
        <div class="block-body">
          <pre v-if="block.kind === 'user'" class="user-text">{{ block.text }}</pre>
          <div
            v-else-if="block.kind === 'assistant'"
            class="assistant-text"
          >
            {{ block.text }}<span v-if="block.interrupted" class="interrupted"> (interrupted)</span>
          </div>
          <div v-else class="system-text">{{ block.text }}</div>
        </div>
      </div>
      <div v-if="empty" class="empty">No chat transcript for this session.</div>
    </div>
  </div>
</template>

<style scoped>
.chat-panel.history {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--color-surface-1, #1e1e1e);
  color: var(--color-text, #ddd);
  font-size: 13px;
}
.chat-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 12px;
  border-bottom: 1px solid var(--color-border, #333);
}
.title {
  display: flex;
  gap: 8px;
  align-items: baseline;
}
.agent-name {
  font-weight: 600;
}
.model-tag {
  padding: 1px 6px;
  border-radius: 3px;
  background: rgba(255, 255, 255, 0.08);
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  opacity: 0.7;
}
.status {
  display: flex;
  gap: 6px;
  align-items: center;
  font-size: 11px;
  opacity: 0.7;
}
.led {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #666;
}
.messages {
  flex: 1;
  overflow-y: auto;
  padding: 10px 12px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.block {
  max-width: 92%;
}
.block-user {
  align-self: flex-end;
}
.block-assistant,
.block-system {
  align-self: flex-start;
}
.block-body {
  padding: 8px 10px;
  border-radius: 6px;
  white-space: pre-wrap;
  word-wrap: break-word;
}
.block-user .block-body {
  background: var(--color-accent-bg, #1a3a5c);
}
.block-assistant .block-body {
  background: var(--color-surface-2, #2a2a2a);
}
.block-system .block-body {
  background: transparent;
  border: 1px dashed var(--color-border, #333);
  font-size: 12px;
  opacity: 0.75;
  font-style: italic;
}
.user-text {
  margin: 0;
  font-family: inherit;
  white-space: pre-wrap;
}
.interrupted {
  color: #f44336;
  font-size: 11px;
  margin-left: 4px;
}
.empty {
  padding: 20px;
  text-align: center;
  opacity: 0.5;
  font-style: italic;
}
</style>
