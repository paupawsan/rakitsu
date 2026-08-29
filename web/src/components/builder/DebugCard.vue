<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted } from 'vue';
import type { Node } from '@vue-flow/core';
import type { DebugPausedPayload } from '../../types';

const props = defineProps<{
  node: Node | null;
  pausedCheckpoint: string | null;
  pausedContext: DebugPausedPayload | null;
}>();

const emit = defineEmits<{
  close: [];
  resume: [];
  step: [];
  stop: [];
}>();

// Draggable
const posX = ref(40);
const posY = ref(80);
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

function fmtTokens(n: number): string {
  if (n >= 1000) return (n / 1000).toFixed(1) + 'k';
  return String(n);
}

function checkpointLabel(cp: string | null): string {
  const labels: Record<string, string> = {
    pre_agent: 'Before Agent',
    pre_thought: 'Before LLM call',
    post_thought: 'After LLM response',
    pre_tool: 'Before Tool',
    pre_reflection: 'Before Reflection',
    pre_ground_check: 'Before Ground Check',
    pre_pipeline_step: 'Before Pipeline Step',
  };
  return cp ? (labels[cp] ?? cp) : '';
}
</script>

<template>
  <div
    v-if="node"
    class="debug-card"
    :style="{ left: posX + 'px', top: posY + 'px' }"
  >
    <!-- Drag handle / header -->
    <div class="dc-header" @mousedown="onMouseDown">
      <span class="dc-pause-badge">PAUSED</span>
      <span class="dc-name">{{ data.name as string }}</span>
      <span v-if="pausedCheckpoint" class="dc-checkpoint">{{ checkpointLabel(pausedCheckpoint) }}</span>
      <button class="dc-close" @click="emit('close')">✕</button>
    </div>

    <div class="dc-body">
      <!-- Pause context rows -->
      <div v-if="pausedContext?.iteration" class="dc-row">
        <span class="dc-label">Iteration</span>
        <span class="dc-val">{{ pausedContext.iteration }}</span>
      </div>
      <div v-if="pausedContext?.history_length" class="dc-row">
        <span class="dc-label">History</span>
        <span class="dc-val">{{ pausedContext.history_length }} msgs</span>
      </div>
      <div v-if="pausedContext?.total_tokens_in || data._tokenCount" class="dc-row">
        <span class="dc-label">Tokens</span>
        <span class="dc-val">{{ fmtTokens((pausedContext?.total_tokens_in ?? 0) + (pausedContext?.total_tokens_out ?? 0) || (data._tokenCount as number ?? 0)) }}</span>
      </div>
      <div v-if="pausedContext?.budget_ratio != null" class="dc-row">
        <span class="dc-label">Budget</span>
        <span class="dc-val">{{ Math.round(pausedContext.budget_ratio * 100) }}%</span>
      </div>
      <div v-if="pausedContext?.last_thought || data._lastThought" class="dc-thought">
        <span class="dc-label">Last thought</span>
        <p class="dc-thought-text">"{{ pausedContext?.last_thought ?? data._lastThought }}"</p>
      </div>
      <div v-if="pausedContext?.reason" class="dc-row dc-reason-row">
        <span class="dc-label">Reason</span>
        <span class="dc-val dc-reason">{{ pausedContext.reason }}</span>
      </div>
    </div>

    <!-- Actions -->
    <div class="dc-actions">
      <button class="dc-btn dc-btn--resume" @click="emit('resume')">RESUME</button>
      <button class="dc-btn dc-btn--step" @click="emit('step')">STEP</button>
      <button class="dc-btn dc-btn--stop" @click="emit('stop')">STOP</button>
    </div>
  </div>
</template>

<style scoped>
.debug-card {
  position: absolute; /* was fixed — escaped tab container causing ghost overlay */
  z-index: 1600;
  width: 280px;
  background: var(--surface-1);
  border: 1px solid var(--status-paused);
  border-radius: var(--node-radius);
  box-shadow: 0 8px 32px rgba(0,0,0,0.5), 0 0 0 1px rgba(245,158,11,0.3);
  color: var(--text-primary);
  font-size: 12px;
  user-select: none;
}

.dc-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  background: rgba(245,158,11,0.12);
  border-radius: var(--node-radius) var(--node-radius) 0 0;
  cursor: grab;
  border-bottom: 1px solid rgba(245,158,11,0.3);
}

.dc-header:active { cursor: grabbing; }

.dc-pause-badge {
  font-size: 9px;
  font-family: var(--font-mono);
  background: rgba(245,158,11,0.2);
  padding: 1px 5px;
  border-radius: 2px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--status-paused);
}

.dc-name {
  font-weight: 600;
  font-size: 13px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  flex-shrink: 0;
}

.dc-checkpoint {
  flex: 1;
  font-size: 10px;
  font-family: var(--font-mono);
  color: var(--status-paused);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  text-align: right;
}

.dc-close {
  background: none;
  border: none;
  color: var(--text-secondary);
  cursor: pointer;
  font-size: 12px;
  padding: 0 2px;
  line-height: 1;
  flex-shrink: 0;
}

.dc-close:hover { color: var(--text-primary); }

.dc-body {
  padding: 10px 12px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.dc-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.dc-label {
  font-size: 10px;
  font-family: var(--font-mono);
  color: var(--text-muted);
  flex-shrink: 0;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.dc-val {
  font-size: 12px;
  font-family: var(--font-mono);
  color: var(--text-primary);
}

.dc-reason-row { margin-top: 2px; }

.dc-reason {
  font-size: 11px;
  color: var(--status-paused);
  text-align: right;
}

.dc-thought {
  display: flex;
  flex-direction: column;
  gap: 3px;
  margin-top: 2px;
}

.dc-thought-text {
  font-size: 11px;
  color: var(--text-secondary);
  font-style: italic;
  margin: 0;
  line-height: 1.4;
  word-break: break-word;
}

.dc-actions {
  display: flex;
  gap: 4px;
  padding: 8px 12px;
  border-top: 1px solid rgba(245,158,11,0.2);
}

.dc-btn {
  flex: 1;
  padding: 5px 8px;
  border-radius: var(--node-radius);
  font-size: 10px;
  font-weight: 600;
  font-family: var(--font-mono);
  letter-spacing: 0.05em;
  cursor: pointer;
  border: none;
  transition: all 0.15s;
}

.dc-btn--resume {
  background: var(--status-running);
  color: #000;
}

.dc-btn--resume:hover { background: #16a34a; }

.dc-btn--step {
  background: var(--accent-agent);
  color: #fff;
}

.dc-btn--step:hover { background: #4a7cd8; }

.dc-btn--stop {
  background: var(--status-error);
  color: #fff;
}

.dc-btn--stop:hover { background: #dc2626; }
</style>
