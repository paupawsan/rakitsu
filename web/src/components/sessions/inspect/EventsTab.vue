<script setup lang="ts">
// EventsTab — Sessions tab Inspect viewer Events sub-tab.
//
// Renders a simple, scrollable timeline of telemetry events for the
// selected session. Forensic by design: each row shows timestamp,
// event_type, agent_name, and a one-line summary derived from the
// payload. Filter input narrows by substring across the row text.
//
// Heavy events (long tool outputs, full reasoning streams) are not
// expanded here — clicking a row toggles a JSON view of the raw event
// for deep dives. That keeps the timeline scannable while still being
// useful when something looks wrong.

import { computed, onMounted, ref, watch } from 'vue';
import { useSessionHistory } from '../../../composables/useSessionHistory';
import type { AgentEvent } from '../../../types';

const props = defineProps<{
  sessionId: string;
}>();

const history = useSessionHistory();
const filter = ref('');
const expandedIdx = ref<number | null>(null);

async function load(id: string) {
  expandedIdx.value = null;
  await history.loadSession(id);
}

onMounted(() => load(props.sessionId));
watch(() => props.sessionId, (id) => { if (id) load(id); });

// One-line summary derived from event_type + payload heuristics. Kept
// permissive: payload shapes drift over time, so we read defensively.
function summaryOf(e: AgentEvent): string {
  const p = (e.payload ?? {}) as Record<string, unknown>;
  const pick = (...keys: string[]): string => {
    for (const k of keys) {
      const v = p[k];
      if (typeof v === 'string' && v.length > 0) return v;
    }
    return '';
  };
  switch (e.event_type) {
    case 'TOOL_CALL_START':
      return `${pick('tool_name')} ${pick('arguments', 'args') || ''}`.trim();
    case 'TOOL_CALL_END':
      return `${pick('tool_name')} → ${pick('status', 'error') || 'ok'}`;
    case 'AGENT_START':
    case 'AGENT_END':
      return pick('role', 'provider', 'model') || '';
    case 'PIPELINE_START':
    case 'PIPELINE_END':
      return pick('status') || '';
    case 'TOKEN_CHUNK':
    case 'REASONING_CHUNK':
      return `+${(pick('text') || '').slice(0, 40)}`;
    case 'EXECUTION_COMPLETE':
    case 'SESSION_END':
      return `${pick('status')} · ${pick('final_answer').slice(0, 80)}`;
    case 'ERROR':
      return pick('error', 'message') || 'error';
    default:
      return pick('text', 'message', 'final_answer', 'status').slice(0, 80);
  }
}

function timeOf(e: AgentEvent): string {
  if (!e.timestamp) return '';
  try {
    const d = new Date(e.timestamp);
    return d.toLocaleTimeString();
  } catch {
    return e.timestamp;
  }
}

const events = computed<AgentEvent[]>(() => history.sessionEvents.value);

const filtered = computed(() => {
  const q = filter.value.trim().toLowerCase();
  if (!q) return events.value;
  return events.value.filter((e) => {
    const hay = `${e.event_type ?? ''} ${e.agent_name ?? ''} ${summaryOf(e)}`.toLowerCase();
    return hay.includes(q);
  });
});

function toggle(idx: number) {
  expandedIdx.value = expandedIdx.value === idx ? null : idx;
}

function rawJson(e: AgentEvent): string {
  return JSON.stringify(e, null, 2);
}
</script>

<template>
  <div class="events-tab">
    <div class="filter-bar">
      <input
        v-model="filter"
        type="text"
        placeholder="Filter by type, agent, or summary…"
        spellcheck="false"
      />
      <span class="count" v-if="events.length > 0">
        {{ filtered.length }} / {{ events.length }} events
      </span>
    </div>

    <div v-if="history.loading.value" class="centered">Loading events…</div>
    <div v-else-if="history.error.value" class="centered err">{{ history.error.value }}</div>
    <div v-else-if="events.length === 0" class="centered empty">No events for this session.</div>
    <div v-else-if="filtered.length === 0" class="centered empty">No events match "{{ filter }}".</div>
    <ul v-else class="timeline">
      <li
        v-for="(e, i) in filtered"
        :key="i"
        class="row"
        :class="{ expanded: expandedIdx === i, [`type-${e.event_type ?? ''}`]: true }"
        @click="toggle(i)"
      >
        <span class="time">{{ timeOf(e) }}</span>
        <span class="type">{{ e.event_type }}</span>
        <span class="agent">{{ e.agent_name || '—' }}</span>
        <span class="summary">{{ summaryOf(e) }}</span>
        <pre v-if="expandedIdx === i" class="raw">{{ rawJson(e) }}</pre>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.events-tab {
  height: 100%;
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.filter-bar {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 16px;
  border-bottom: 1px solid var(--border-subtle);
  background: var(--surface-1);
  flex-shrink: 0;
}
.filter-bar input {
  flex: 1;
  padding: 6px 8px;
  background: var(--surface-0);
  color: var(--text-primary);
  border: 1px solid var(--border-subtle);
  border-radius: 3px;
  font-family: var(--font-sans);
  font-size: 12px;
}
.filter-bar input:focus {
  outline: none;
  border-color: var(--accent-agent);
}
.count {
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--text-muted);
}

.timeline {
  flex: 1;
  overflow-y: auto;
  margin: 0;
  padding: 8px 0;
  list-style: none;
}

.row {
  display: grid;
  grid-template-columns: 80px 180px 120px 1fr;
  gap: 12px;
  padding: 4px 16px;
  font-family: var(--font-mono);
  font-size: 11px;
  line-height: 1.5;
  cursor: pointer;
  border-left: 3px solid transparent;
}
.row:hover {
  background: var(--surface-1);
}
.row.expanded {
  border-left-color: var(--accent-agent);
  background: var(--surface-1);
}

.time { color: var(--text-muted); }
.type {
  color: var(--accent-agent);
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.agent {
  color: var(--text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.summary {
  color: var(--text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.raw {
  grid-column: 1 / -1;
  margin: 6px 0 10px;
  padding: 8px 10px;
  background: var(--surface-0);
  color: var(--text-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: 3px;
  font-size: 10px;
  line-height: 1.45;
  max-height: 320px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-word;
}

.row.type-ERROR .type { color: var(--status-error, #ef4444); }
.row.type-SESSION_END .type,
.row.type-EXECUTION_COMPLETE .type { color: var(--status-running); }

.centered {
  padding: 32px 16px;
  text-align: center;
  color: var(--text-muted);
  font-size: 13px;
}
.centered.err { color: var(--status-error, #ef4444); }
</style>
