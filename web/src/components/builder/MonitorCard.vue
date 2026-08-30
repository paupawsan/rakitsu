<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted } from 'vue';
import type { Node } from '@vue-flow/core';

const props = defineProps<{
  node: Node | null;
}>();

const emit = defineEmits<{
  close: [];
}>();

// Draggable card position
const posX = ref(40);
const posY = ref(40);
let dragging = false;
let dragOffX = 0;
let dragOffY = 0;

function onMouseDown(e: MouseEvent) {
  dragging = true;
  dragOffX = e.clientX - posX.value;
  dragOffY = e.clientY - posY.value;
  e.preventDefault();
}

function onMouseMove(e: MouseEvent) {
  if (!dragging) return;
  posX.value = e.clientX - dragOffX;
  posY.value = e.clientY - dragOffY;
}

function onMouseUp() { dragging = false; }

onMounted(() => {
  window.addEventListener('mousemove', onMouseMove);
  window.addEventListener('mouseup', onMouseUp);
});

onUnmounted(() => {
  window.removeEventListener('mousemove', onMouseMove);
  window.removeEventListener('mouseup', onMouseUp);
});

const data = computed(() => props.node?.data as Record<string, unknown> ?? {});

const STATUS_ICONS: Record<string, string> = {
  running: '◉', paused: '⏸', success: '✓', error: '✗',
};

function fmtTokens(n: number): string {
  if (n >= 1000) return (n / 1000).toFixed(1) + 'k';
  return String(n);
}
</script>

<template>
  <div
    v-if="node"
    class="monitor-card"
    :style="{ left: posX + 'px', top: posY + 'px' }"
  >
    <!-- Drag handle / header -->
    <div class="mc-header" @mousedown="onMouseDown">
      <span class="mc-type-badge">{{ node.type === 'orchestrator' ? 'ORCH' : node.type === 'tool' ? 'TOOL' : 'AGT' }}</span>
      <span class="mc-name">{{ data.name as string }}</span>
      <span v-if="data._runStatus" class="mc-status-icon" :class="data._runStatus as string">
        {{ STATUS_ICONS[data._runStatus as string] ?? '○' }}
      </span>
      <button class="mc-close" @click="emit('close')">✕</button>
    </div>

    <div class="mc-body">
      <!-- Status row -->
      <div class="mc-row">
        <span class="mc-label">Status</span>
        <span class="mc-val mc-status" :class="data._runStatus as string">
          {{ (data._runStatus as string) ?? 'pending' }}
        </span>
      </div>

      <!-- Iteration -->
      <div v-if="data._iterationCount" class="mc-row">
        <span class="mc-label">Iteration</span>
        <span class="mc-val">{{ data._iterationCount }}</span>
      </div>

      <!-- Tokens -->
      <div v-if="data._tokenCount" class="mc-row">
        <span class="mc-label">Tokens</span>
        <span class="mc-val">{{ fmtTokens(data._tokenCount as number) }}</span>
      </div>

      <!-- Last thought -->
      <div v-if="data._lastThought" class="mc-thought">
        <span class="mc-label">Last thought</span>
        <p class="mc-thought-text">"{{ data._lastThought }}"</p>
      </div>

      <!-- Design fields summary -->
      <div v-if="node.type === 'agent'" class="mc-row mc-meta">
        <span class="mc-label">Provider</span>
        <span class="mc-val">{{ (data.provider as string) || 'openai' }} / {{ data.model }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.monitor-card {
  position: absolute; /* was fixed — escaped tab container causing ghost overlay */
  z-index: 1500;
  width: 260px;
  background: var(--surface-1);
  border: 1px solid var(--border-default);
  border-radius: var(--node-radius);
  box-shadow: 0 8px 32px rgba(0,0,0,0.5);
  color: var(--text-primary);
  font-size: 12px;
  user-select: none;
}

.mc-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  background: var(--surface-3);
  border-radius: var(--node-radius) var(--node-radius) 0 0;
  cursor: grab;
  border-bottom: 1px solid var(--border-default);
}

.mc-header:active { cursor: grabbing; }

.mc-type-badge {
  font-size: 9px;
  font-family: var(--font-mono);
  background: rgba(255, 255, 255, 0.08);
  padding: 1px 5px;
  border-radius: 2px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--text-secondary);
}

.mc-name {
  flex: 1;
  font-weight: 600;
  font-size: 13px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mc-status-icon { font-size: 14px; font-weight: 700; }
.mc-status-icon.running { color: var(--status-running); }
.mc-status-icon.paused  { color: var(--status-paused); }
.mc-status-icon.success { color: var(--status-running); }
.mc-status-icon.error   { color: var(--status-error); }

.mc-close {
  background: none;
  border: none;
  color: var(--text-secondary);
  cursor: pointer;
  font-size: 12px;
  padding: 0 2px;
  line-height: 1;
}

.mc-close:hover { color: var(--text-primary); }

.mc-body { padding: 10px 12px; display: flex; flex-direction: column; gap: 6px; }

.mc-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.mc-label {
  font-size: 10px;
  font-family: var(--font-mono);
  color: var(--text-muted);
  flex-shrink: 0;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.mc-val { font-size: 12px; font-family: var(--font-mono); color: var(--text-primary); }

.mc-status { font-weight: 600; text-transform: uppercase; letter-spacing: 0.04em; }
.mc-status.running { color: var(--status-running); }
.mc-status.paused  { color: var(--status-paused); }
.mc-status.success { color: var(--status-running); }
.mc-status.error   { color: var(--status-error); }

.mc-thought { display: flex; flex-direction: column; gap: 3px; }

.mc-thought-text {
  font-size: 11px;
  color: var(--text-secondary);
  font-style: italic;
  margin: 0;
  line-height: 1.4;
  word-break: break-word;
}

.mc-meta { opacity: 0.7; }
</style>
