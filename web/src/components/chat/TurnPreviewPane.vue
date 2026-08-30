<script setup lang="ts">
// TurnPreviewPane — fetches a single turn's full entries via
// GET /api/chat/{sessionId}/turn/{turnId} when the user selects a node
// in the Branch Tree Viewer. Read-only; activation/edit are wired
// through useChatSession from the parent component.

import { computed, ref, watch } from 'vue';

// Mirror of internal/turntree.Node[transcriptEntry] — full payload from
// the per-turn REST endpoint. Entries are typed loosely (`Record<string,
// unknown>`) because the kind discriminator drives the render; the
// strict TranscriptEntry shape is wire-checked by useChatSession on
// transcript_resync, not here.
interface TurnPayload {
  id: string;
  parent_id?: string;
  user_text: string;
  entries?: Record<string, unknown>[];
  children?: string[];
  status: string;
  created: string;
  retry_ctx?: { source: string; prior_turn_id?: string; reason: string };
}

const props = defineProps<{
  sessionId: string;
  turnId: string | null;
  baseUrl?: string;
}>();

const loading = ref(false);
const error = ref<string | null>(null);
const payload = ref<TurnPayload | null>(null);

const fetchUrl = computed(() => {
  if (!props.turnId) return null;
  const base = (props.baseUrl || window.location.origin).replace(/\/+$/, '');
  return `${base}/api/chat/${encodeURIComponent(props.sessionId)}/turn/${encodeURIComponent(props.turnId)}`;
});

async function load() {
  if (!fetchUrl.value || !props.turnId) {
    payload.value = null;
    error.value = null;
    return;
  }
  loading.value = true;
  error.value = null;
  try {
    const res = await fetch(fetchUrl.value);
    if (!res.ok) {
      throw new Error(`HTTP ${res.status}`);
    }
    payload.value = (await res.json()) as TurnPayload;
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
    payload.value = null;
  } finally {
    loading.value = false;
  }
}

watch(() => props.turnId, load, { immediate: true });

function entryLabel(e: Record<string, unknown>): string {
  const kind = String(e.kind ?? '');
  switch (kind) {
    case 'assistant': return 'assistant';
    case 'tool':      return `tool · ${String(e.tool_name ?? '?')}`;
    case 'reasoning': return 'reasoning';
    case 'system':    return 'system';
    case 'user_input_request': return 'user_input_request';
    default:          return kind || '?';
  }
}

function entryBody(e: Record<string, unknown>): string {
  const text = e.text;
  if (typeof text === 'string' && text) return text;
  const output = e.output;
  if (typeof output === 'string' && output) return output;
  return '';
}

</script>

<template>
  <aside class="turn-preview-pane" :aria-busy="loading">
    <div v-if="!turnId" class="empty">Select a turn to preview.</div>
    <div v-else-if="loading" class="empty">Loading turn…</div>
    <div v-else-if="error" class="error">Failed to load turn: {{ error }}</div>
    <template v-else-if="payload">
      <div class="header">
        <span class="id" :title="payload.id">{{ payload.id.slice(0, 8) }}</span>
        <span class="status">{{ payload.status }}</span>
      </div>
      <div class="user-block">
        <div class="label">User</div>
        <pre class="text">{{ payload.user_text || '(no user text)' }}</pre>
      </div>
      <div v-if="payload.retry_ctx" class="retry-ctx">
        <strong>↺ {{ payload.retry_ctx.source }}</strong>
        <span v-if="payload.retry_ctx.prior_turn_id"> · from {{ payload.retry_ctx.prior_turn_id.slice(0, 8) }}</span>
        <div class="reason">{{ payload.retry_ctx.reason }}</div>
      </div>
      <div v-for="(e, i) in (payload.entries || [])" :key="i" class="entry">
        <div class="label">{{ entryLabel(e) }}</div>
        <pre v-if="entryBody(e)" class="text">{{ entryBody(e) }}</pre>
      </div>
      <div v-if="(payload.entries || []).length === 0" class="empty">No entries yet.</div>
    </template>
  </aside>
</template>

<style scoped>
.turn-preview-pane {
  display: flex;
  flex-direction: column;
  gap: 0.7em;
  padding: 0.8em;
  background: var(--surface-1, rgba(20, 22, 30, 0.85));
  border-left: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.18));
  overflow-y: auto;
  font-family: var(--font-sans);
  font-size: 0.88em;
  color: var(--fg, rgba(255, 255, 255, 0.92));
}

.empty, .error {
  opacity: 0.7;
  font-style: italic;
}
.error { color: var(--error, #ff7575); }

.header {
  display: flex;
  align-items: center;
  gap: 0.6em;
  font-family: var(--font-mono);
  font-size: 0.85em;
  opacity: 0.85;
}
.header .status { margin-left: auto; }

.user-block, .entry {
  display: flex;
  flex-direction: column;
  gap: 0.25em;
}

.label {
  font-family: var(--font-mono);
  font-size: 0.75em;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  opacity: 0.6;
}

.text {
  margin: 0;
  padding: 0.4em 0.6em;
  background: rgba(255, 255, 255, 0.04);
  border-radius: 4px;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: var(--font-mono);
  font-size: 0.85em;
}

.retry-ctx {
  padding: 0.5em 0.7em;
  border-radius: 6px;
  background: rgba(245, 166, 35, 0.12);
  border: 1px solid rgba(245, 166, 35, 0.35);
  color: var(--warning, #f5a623);
  font-size: 0.85em;
}
.retry-ctx .reason {
  margin-top: 0.3em;
  opacity: 0.9;
  font-style: italic;
}
</style>
