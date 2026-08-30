<script setup lang="ts">
import { computed, onMounted, ref, toRef } from 'vue';
import { useSessionList, type RuntimeSessionMeta } from '../../composables/useSessionList';

// Session picker. Lists every live runtime session (chat +
// one-shot) and surfaces a live-status dot. Selection is read-only — it
// emits `select` but doesn't change attach routing yet.

const props = defineProps<{ modelValue?: string | null }>();
const emit = defineEmits<{
  'update:modelValue': [id: string | null];
  select: [session: RuntimeSessionMeta | null];
}>();

const { sessions, start, refresh } = useSessionList({ pollMs: 2000 });
const open = ref(false);
const root = ref<HTMLElement | null>(null);

onMounted(() => {
  start();
});

function onDocClick(e: MouseEvent) {
  if (!root.value) return;
  if (!root.value.contains(e.target as Node)) open.value = false;
}

function toggle() {
  open.value = !open.value;
  if (open.value) {
    refresh();
    document.addEventListener('click', onDocClick);
  } else {
    document.removeEventListener('click', onDocClick);
  }
}

function pick(s: RuntimeSessionMeta | null) {
  emit('update:modelValue', s?.id ?? null);
  emit('select', s);
  open.value = false;
  document.removeEventListener('click', onDocClick);
}

// Stop an entry directly from the dropdown. One-shot runs go through
// /api/run/stop (the runner is single-run so no id is needed); chat sessions
// through /api/chat/{id}/stop. If the LLM call ignores ctx cancellation the
// goroutine may keep living, but the backend now unregisters synchronously
// so the picker clears immediately.
async function stopSession(s: RuntimeSessionMeta) {
  try {
    if (s.mode === 'oneshot') {
      await fetch('/api/run/stop', { method: 'POST' });
    } else {
      await fetch(`/api/chat/${encodeURIComponent(s.id)}/stop`, { method: 'POST' });
    }
    if (modelValueRef.value === s.id) {
      emit('update:modelValue', null);
      emit('select', null);
    }
    refresh();
  } catch (e) {
    console.error('[SessionPicker] stop failed', e);
  }
}

const modelValueRef = toRef(props, 'modelValue');

const selected = computed(() => sessions.value.find((s) => s.id === props.modelValue) ?? null);

const counts = computed(() => {
  const chat = sessions.value.filter((s) => s.mode === 'chat').length;
  const oneshot = sessions.value.filter((s) => s.mode === 'oneshot').length;
  return { chat, oneshot, total: chat + oneshot };
});

function label(s: RuntimeSessionMeta): string {
  return `[${s.mode}] ${s.name || s.config_id} · ${s.id.slice(0, 8)}`;
}
</script>

<template>
  <div ref="root" class="session-picker">
    <button class="picker-trigger" :class="{ active: open }" @click="toggle" :title="selected ? `Selected: ${selected.id}` : 'Pick a live session'">
      <span v-if="selected" class="dot" :class="selected.status" />
      <span class="label">
        <template v-if="selected">{{ label(selected) }}</template>
        <template v-else-if="counts.total === 0">No active sessions</template>
        <template v-else>{{ counts.total }} active <span class="subcount">({{ counts.chat }} chat, {{ counts.oneshot }} run)</span></template>
      </span>
      <span class="caret">▾</span>
    </button>

    <div v-if="open" class="picker-menu" role="listbox">
      <div v-if="sessions.length === 0" class="empty">No active sessions</div>
      <div
        v-for="s in sessions"
        :key="s.id"
        class="picker-item"
        :class="{ selected: s.id === modelValue }"
      >
        <button class="picker-item-main" @click="pick(s)">
          <span class="dot" :class="s.status" />
          <span class="mode-chip" :class="s.mode">{{ s.mode }}</span>
          <span class="item-label">{{ s.name || s.config_id }}</span>
          <span class="item-id">{{ s.id.slice(0, 8) }}</span>
        </button>
        <button
          class="stop-btn"
          title="Stop this session"
          @click.stop="stopSession(s)"
        >Stop</button>
      </div>
      <button v-if="modelValue" class="picker-item clear" @click="pick(null)">
        Clear selection
      </button>
    </div>
  </div>
</template>

<style scoped>
.session-picker {
  position: relative;
  display: inline-flex;
}

.picker-trigger {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px;
  background: var(--color-surface-2, #2a2a2a);
  border: 1px solid var(--color-border, #3a3a3a);
  border-radius: 4px;
  color: var(--color-text, #ddd);
  font-size: 12px;
  cursor: pointer;
  max-width: 260px;
}
.picker-trigger:hover,
.picker-trigger.active {
  background: var(--color-surface-3, #333);
}
.picker-trigger .label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 80px;
}
.picker-trigger .subcount {
  opacity: 0.6;
  font-size: 11px;
}
.picker-trigger .caret {
  opacity: 0.6;
  font-size: 10px;
}

.dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #888;
  flex-shrink: 0;
}
.dot.running {
  background: #4caf50;
  box-shadow: 0 0 4px rgba(76, 175, 80, 0.6);
}
.dot.paused {
  background: #ff9800;
}
.dot.stopped {
  background: #666;
}
.dot.errored {
  background: #f44336;
}

.picker-menu {
  position: absolute;
  top: calc(100% + 4px);
  left: 0;
  min-width: 280px;
  max-height: 320px;
  overflow-y: auto;
  background: var(--color-surface-2, #2a2a2a);
  border: 1px solid var(--color-border, #3a3a3a);
  border-radius: 4px;
  z-index: 1000;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.4);
}

.empty {
  padding: 10px 12px;
  color: var(--color-text-muted, #888);
  font-size: 12px;
  font-style: italic;
}

.picker-item {
  display: flex;
  align-items: center;
  width: 100%;
  color: var(--color-text, #ddd);
  font-size: 12px;
}
.picker-item:hover {
  background: var(--color-surface-3, #333);
}
.picker-item.selected {
  background: var(--color-accent-bg, #1a3a5c);
}
.picker-item-main {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 12px;
  background: none;
  border: none;
  color: inherit;
  font-size: inherit;
  text-align: left;
  cursor: pointer;
}
.stop-btn {
  padding: 3px 10px;
  margin-right: 8px;
  background: transparent;
  border: 1px solid var(--color-border, #3a3a3a);
  border-radius: 3px;
  color: var(--color-text-muted, #bbb);
  font-size: 11px;
  cursor: pointer;
}
.stop-btn:hover {
  background: #c62828;
  border-color: #c62828;
  color: #fff;
}
button.picker-item.clear {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  padding: 6px 12px;
  background: none;
  border: none;
  border-top: 1px solid var(--color-border, #3a3a3a);
  color: var(--color-text-muted, #888);
  font-size: 12px;
  cursor: pointer;
}
button.picker-item.clear:hover {
  background: var(--color-surface-3, #333);
}

.mode-chip {
  padding: 1px 6px;
  border-radius: 3px;
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  font-weight: 600;
}
.mode-chip.chat {
  background: rgba(76, 175, 80, 0.15);
  color: #8ed98e;
}
.mode-chip.oneshot {
  background: rgba(33, 150, 243, 0.15);
  color: #8ac5f0;
}

.item-label {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.item-id {
  font-family: var(--font-mono, monospace);
  font-size: 10px;
  opacity: 0.5;
}
</style>
