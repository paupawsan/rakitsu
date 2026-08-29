<script setup lang="ts">
import { computed, ref } from 'vue';
import type { SessionMeta } from '../../types';

// SessionList — past-session browser pane for SessionsView. Receives the
// session array from the parent (which owns the useSessionHistory composable)
// and emits select/refresh events. No actions are rendered on rows: per the
// PLAN doc, all per-session actions (Clone / Resume / Fork / Re-run) move
// inside the Inspect viewer (D-5). The row is just a row.

const props = defineProps<{
  sessions: SessionMeta[];
  selectedId: string | null;
  loading: boolean;
  error: string | null;
}>();

const emit = defineEmits<{
  select: [id: string];
  refresh: [];
}>();

const filter = ref('');

const sorted = computed(() =>
  [...props.sessions].sort((a, b) => {
    const ta = a.start_time ? new Date(a.start_time).getTime() : 0;
    const tb = b.start_time ? new Date(b.start_time).getTime() : 0;
    return tb - ta;
  }),
);

const filtered = computed(() => {
  const q = filter.value.trim().toLowerCase();
  if (!q) return sorted.value;
  return sorted.value.filter((s) => {
    const hay = `${s.name} ${s.query} ${s.agents?.join(' ') ?? ''} ${s.status}`.toLowerCase();
    return hay.includes(q);
  });
});

function formatTime(t: string | undefined): string {
  if (!t) return '';
  try {
    return new Date(t).toLocaleString();
  } catch {
    return t;
  }
}
</script>

<template>
  <aside class="session-list">
    <div class="list-header">
      <h3>Sessions</h3>
      <button class="btn-refresh" :disabled="loading" title="Refresh" @click="emit('refresh')">↻</button>
    </div>

    <div class="filter">
      <input
        v-model="filter"
        type="text"
        placeholder="Filter by name, query, agent, status…"
        spellcheck="false"
      />
    </div>

    <div v-if="loading" class="centered">Loading…</div>
    <div v-else-if="error" class="centered err">{{ error }}</div>
    <div v-else-if="sessions.length === 0" class="centered empty">
      No sessions yet.
      <div class="empty-hint">
        Past runs from CLI <code>rakitsu run</code> and chat sessions land here.
      </div>
    </div>
    <div v-else-if="filtered.length === 0" class="centered empty">
      No sessions match "{{ filter }}".
    </div>
    <ul v-else class="cards">
      <li
        v-for="s in filtered"
        :key="s.id"
        class="card"
        :class="{ active: s.id === selectedId }"
        @click="emit('select', s.id)"
      >
        <div class="card-row">
          <span class="card-name">{{ s.name || s.id.slice(0, 8) }}</span>
          <span class="badge" :class="s.status">{{ s.status }}</span>
        </div>
        <div class="card-time">
          {{ formatTime(s.start_time) }}
          <span class="card-id" :title="s.id">· {{ s.id.slice(0, 8) }}</span>
        </div>
        <div v-if="s.query" class="card-query">{{ s.query }}</div>
        <div v-if="s.agents && s.agents.length > 0" class="card-agents">
          {{ s.agents.join(' · ') }}
        </div>
      </li>
    </ul>
  </aside>
</template>

<style scoped>
.session-list {
  display: flex;
  flex-direction: column;
  border-right: 1px solid var(--border-subtle);
  background: var(--surface-1);
  overflow: hidden;
}

.list-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  border-bottom: 1px solid var(--border-subtle);
}
.list-header h3 {
  margin: 0;
  font-size: 12px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: var(--text-secondary);
}

.btn-refresh {
  background: transparent;
  border: 1px solid var(--border-subtle);
  color: var(--text-secondary);
  font-size: 12px;
  padding: 2px 8px;
  border-radius: 3px;
  cursor: pointer;
  line-height: 1;
}
.btn-refresh:hover:not(:disabled) {
  color: var(--text-primary);
  background: var(--surface-2);
}
.btn-refresh:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.filter {
  padding: 8px 12px;
  border-bottom: 1px solid var(--border-subtle);
}
.filter input {
  width: 100%;
  padding: 6px 8px;
  background: var(--surface-0);
  color: var(--text-primary);
  border: 1px solid var(--border-subtle);
  border-radius: 3px;
  font-family: var(--font-sans);
  font-size: 12px;
}
.filter input:focus {
  outline: none;
  border-color: var(--accent-agent);
}

.cards {
  flex: 1;
  overflow-y: auto;
  list-style: none;
  margin: 0;
  padding: 8px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.card {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 10px 12px;
  border-radius: 4px;
  border: 1px solid transparent;
  cursor: pointer;
  background: var(--surface-2);
}
.card:hover {
  background: var(--surface-3);
}
.card.active {
  background: var(--surface-3);
  border-color: var(--accent-agent);
}

.card-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
}
.card-name {
  font-size: 13px;
  font-weight: 500;
  color: var(--text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  flex: 1;
  min-width: 0;
}
.card-time {
  font-family: var(--font-mono);
  font-size: 10px;
  color: var(--text-muted);
  display: flex;
  align-items: baseline;
  gap: 4px;
}
.card-id {
  opacity: 0.7;
}
.card-query {
  font-size: 12px;
  color: var(--text-secondary);
  line-height: 1.4;
  overflow: hidden;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}
.card-agents {
  font-family: var(--font-mono);
  font-size: 10px;
  color: var(--text-muted);
}

.badge {
  font-family: var(--font-mono);
  font-size: 9px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  padding: 1px 5px;
  border-radius: 2px;
  flex-shrink: 0;
}
.badge.running { background: var(--status-running); color: var(--surface-0); }
.badge.success { background: var(--status-running); color: var(--surface-0); }
.badge.error   { background: var(--status-error, #ef4444); color: white; }
.badge.timeout { background: var(--status-paused, #f59e0b); color: var(--surface-0); }

.centered {
  padding: 24px 16px;
  text-align: center;
  color: var(--text-muted);
  font-size: 12px;
}
.centered.err { color: var(--status-error, #ef4444); }

.empty-hint {
  margin-top: 8px;
  font-size: 11px;
  line-height: 1.5;
}
.empty-hint code {
  font-family: var(--font-mono);
  font-size: 10px;
  background: var(--surface-3);
  padding: 1px 4px;
  border-radius: 2px;
}
</style>
