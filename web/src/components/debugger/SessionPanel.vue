<script setup lang="ts">
import { ref, computed, onUnmounted } from 'vue';
import { useSessionFilter, FILTER_FIELDS } from '../../composables/useSessionFilter';
import type { SessionMeta } from '../../types';
import RunLauncher from './RunLauncher.vue';
import ComboBox from '../builder/ComboBox.vue';

export interface HubSession {
  id: string;
  name: string;
  query: string;
  config_path: string;
  agents: string[];
  start_time: string;
  status: string;
  debug: boolean;
  /** "chat" marks an interactive TUI session (cross-session messaging target). */
  mode?: string;
  /** Mirrors the remote session's settings.session_msg.enabled. */
  accepts_messages?: boolean;
  /** Session kind: 'chat' for interactive, 'one-shot' for single-turn. */
  kind?: 'chat' | 'one-shot';
}

const props = defineProps<{
  dataMode: 'live' | 'history';
  hubSessions: HubSession[];
  hubLoading: boolean;
  attachedSession: HubSession | null;
  sessions: SessionMeta[];
  historyLoading: boolean;
  historyError: string | null;
  selectedSessionId: string | null;
  isOpen: boolean;
  isHubMode: boolean;
  baseUrl: string;
}>();

const emit = defineEmits<{
  'select-hub-session': [session: HubSession];
  'attach-debugger': [session: HubSession];
  'detach-debugger': [session: HubSession];
  'select-history-session': [id: string];
  'inspect-in-builder': [session: SessionMeta];
  'rerun': [id: string];
  'refresh-hub': [];
  'run-started': [debug: boolean];
  'delete-sessions': [ids: string[]];
  'toggle': [];
}>();

const { chips, inputField, inputValue, inputRegex, addChip, removeChip, clearAll, filteredList } = useSessionFilter();

const filteredHubSessions = computed(() => filteredList(props.hubSessions));
const filteredHistorySessions = computed(() => filteredList(props.sessions));

const fieldMeta = computed(() => FILTER_FIELDS.find(f => f.key === inputField.value) ?? FILTER_FIELDS[0]!);

// Multi-select state
const selectMode = ref(false);
const selectedIds = ref<Set<string>>(new Set());

const selectedCount = computed(() => selectedIds.value.size);

function toggleSelectMode() {
  selectMode.value = !selectMode.value;
  if (!selectMode.value) selectedIds.value = new Set();
}

function toggleSelect(id: string) {
  const next = new Set(selectedIds.value);
  if (next.has(id)) next.delete(id);
  else next.add(id);
  selectedIds.value = next;
}

function selectAll() {
  const list = props.dataMode === 'live' ? filteredHubSessions.value : filteredHistorySessions.value;
  selectedIds.value = new Set(list.map(s => s.id));
}

function selectNone() {
  selectedIds.value = new Set();
}

function deleteSelected() {
  if (selectedIds.value.size === 0) return;
  const count = selectedIds.value.size;
  if (!confirm(`Delete ${count} session${count > 1 ? 's' : ''}? This cannot be undone.`)) return;
  emit('delete-sessions', [...selectedIds.value]);
  selectedIds.value = new Set();
  selectMode.value = false;
}

function handleKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && inputValue.value.trim()) {
    e.preventDefault();
    addChip();
  }
}

// Cross-session messaging: send affordance on chat-mode hub sessions
// (interactive TUIs registered with mode:"chat"). POSTs to the hub's
// /api/sessions/{id}/message endpoint.
const sendTarget = ref<string | null>(null);
const sendText = ref('');
const sendBusy = ref(false);
const sendError = ref<string | null>(null);

function toggleSend(id: string) {
  sendError.value = null;
  sendText.value = '';
  sendTarget.value = sendTarget.value === id ? null : id;
}

async function sendToSession() {
  const id = sendTarget.value;
  const text = sendText.value.trim();
  if (!id || !text || sendBusy.value) return;
  sendBusy.value = true;
  sendError.value = null;
  try {
    const res = await fetch(`${props.baseUrl}/api/sessions/${encodeURIComponent(id)}/message`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ text, from_session_id: 'web-ui', from_name: 'Web UI' }),
    });
    const body = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(body.error || body.status || res.statusText);
    sendTarget.value = null;
    sendText.value = '';
  } catch (e: unknown) {
    sendError.value = (e as Error).message;
  } finally {
    sendBusy.value = false;
  }
}

function handleCardClick(session: HubSession | SessionMeta, type: 'hub' | 'history') {
  if (selectMode.value) {
    toggleSelect(session.id);
    return;
  }
  if (type === 'hub') emit('select-hub-session', session as HubSession);
  else emit('select-history-session', session.id);
}

// Resize drag
const panelWidth = ref(320);
const isResizing = ref(false);

function onResizeStart(e: MouseEvent) {
  e.preventDefault();
  isResizing.value = true;
  const startX = e.clientX;
  const startW = panelWidth.value;

  function onMove(ev: MouseEvent) {
    const delta = startX - ev.clientX;
    panelWidth.value = Math.max(200, Math.min(600, startW + delta));
  }
  function onUp() {
    isResizing.value = false;
    document.removeEventListener('mousemove', onMove);
    document.removeEventListener('mouseup', onUp);
  }
  document.addEventListener('mousemove', onMove);
  document.addEventListener('mouseup', onUp);
}

onUnmounted(() => { isResizing.value = false; });
</script>

<template>
  <div
    class="session-panel"
    :class="{ collapsed: !isOpen, resizing: isResizing }"
    :style="isOpen ? { width: panelWidth + 'px', minWidth: panelWidth + 'px' } : undefined"
  >
    <div class="resize-handle" @mousedown="onResizeStart"></div>
    <div class="panel-content" :style="{ width: panelWidth + 'px' }">
      <!-- Panel header with close button -->
      <div class="panel-header">
        <span class="panel-title">Sessions</span>
        <button class="panel-close-btn" @click="emit('toggle')" title="Close panel">&times;</button>
      </div>
      <!-- Filter section -->
      <div class="filter-section">
        <!-- Active filter chips -->
        <div v-if="chips.length > 0" class="chip-row">
          <span
            v-for="(chip, i) in chips"
            :key="i"
            class="filter-chip"
            :class="{ regex: chip.isRegex }"
          >
            <span class="chip-field">{{ chip.field }}:</span>
            <span class="chip-value">{{ chip.value }}</span>
            <button class="chip-remove" @click="removeChip(i)">x</button>
          </span>
          <button class="chip-clear" @click="clearAll()">Clear all</button>
        </div>

        <!-- Input row: field selector + value + regex toggle + add -->
        <div class="input-row">
          <ComboBox
            :model-value="inputField"
            :options="FILTER_FIELDS.map(f => ({ value: f.key, label: f.label }))"
            placeholder="Field"
            :allow-custom="false"
            @update:model-value="inputField = $event as any"
          />
          <input
            v-model="inputValue"
            type="text"
            class="filter-input"
            :placeholder="fieldMeta.placeholder"
            @keydown="handleKeydown"
          />
          <button
            class="regex-btn"
            :class="{ active: inputRegex }"
            title="Regex mode"
            @click="inputRegex = !inputRegex"
          >.*</button>
          <button
            class="add-btn"
            :disabled="!inputValue.trim()"
            title="Pin filter"
            @click="addChip()"
          >+</button>
        </div>

        <!-- Select mode toolbar -->
        <div class="select-toolbar">
          <button
            class="select-toggle-btn"
            :class="{ active: selectMode }"
            @click="toggleSelectMode"
          >{{ selectMode ? 'Cancel' : 'Select' }}</button>
          <template v-if="selectMode">
            <button class="select-action-btn" @click="selectAll">All</button>
            <button class="select-action-btn" @click="selectNone">None</button>
            <span v-if="selectedCount > 0" class="select-count">{{ selectedCount }}</span>
            <button
              class="delete-btn"
              :disabled="selectedCount === 0"
              @click="deleteSelected"
            >Delete</button>
          </template>
        </div>
      </div>

      <!-- Live mode (v-show keeps DOM stable, prevents flicker) -->
      <div v-show="dataMode === 'live'" class="mode-container">
        <div v-show="hubLoading && hubSessions.length === 0" class="centered">Loading...</div>
        <div v-show="hubSessions.length === 0 && !isHubMode && !(hubLoading && hubSessions.length === 0)" class="launcher-section">
          <RunLauncher :base-url="baseUrl" @run-started="(debug: boolean) => emit('run-started', debug)" />
        </div>
        <div v-show="hubSessions.length > 0 || isHubMode" class="session-list">
          <div
            v-for="s in filteredHubSessions"
            :key="s.id"
            class="session-card"
            :class="{
              attached: attachedSession?.id === s.id,
              'select-mode': selectMode,
              checked: selectMode && selectedIds.has(s.id),
            }"
            @click="handleCardClick(s, 'hub')"
          >
            <div class="session-card-header">
              <input
                v-if="selectMode"
                type="checkbox"
                class="card-checkbox"
                :checked="selectedIds.has(s.id)"
                @click.stop="toggleSelect(s.id)"
              />
              <span class="session-card-name">{{ s.name }}</span>
              <div class="session-card-badges">
                <span v-if="attachedSession?.id === s.id" class="badge attached">Attached</span>
                <span class="badge" :class="s.status">{{ s.status }}</span>
              </div>
            </div>
            <div class="session-card-meta">{{ s.query }}</div>
            <div v-if="!selectMode" class="session-card-footer">
              <span class="session-card-agents">{{ s.agents?.join(', ') }}</span>
              <button
                v-if="s.mode === 'chat' && s.status === 'running'"
                class="action-btn send"
                :title="s.accepts_messages ? 'Send a message into this session' : 'Session has not opted in (settings.session_msg.enabled)'"
                :disabled="!s.accepts_messages"
                @click.stop="toggleSend(s.id)"
              >Send</button>
              <button
                v-if="attachedSession?.id !== s.id"
                class="action-btn attach"
                @click.stop="emit('attach-debugger', s)"
              >Attach</button>
              <button
                v-else
                class="action-btn detach"
                @click.stop="emit('detach-debugger', s)"
              >Detach</button>
            </div>
            <div v-if="sendTarget === s.id" class="session-send-box" @click.stop>
              <input
                v-model="sendText"
                placeholder="Message this session…"
                :disabled="sendBusy"
                @keyup.enter="sendToSession"
              />
              <button class="action-btn send" :disabled="sendBusy || !sendText.trim()" @click="sendToSession">
                {{ sendBusy ? '…' : 'Send' }}
              </button>
            </div>
            <div v-if="sendTarget === s.id && sendError" class="send-error">{{ sendError }}</div>
          </div>
          <div v-if="filteredHubSessions.length === 0 && hubSessions.length > 0" class="centered">
            No matching sessions
          </div>
          <div class="panel-actions">
            <button class="action-btn refresh" @click="emit('refresh-hub')">Refresh</button>
          </div>
        </div>
      </div>

      <!-- History mode (v-show keeps DOM stable, prevents flicker) -->
      <div v-show="dataMode === 'history'" class="mode-container">
        <div v-show="historyLoading" class="centered">Loading...</div>
        <div v-show="!historyLoading && !!historyError" class="centered error">{{ historyError }}</div>
        <div v-show="!historyLoading && !historyError && sessions.length === 0" class="centered">No sessions found.</div>
        <div v-show="!historyLoading && !historyError && sessions.length > 0" class="session-list">
          <div
            v-for="s in filteredHistorySessions"
            :key="s.id"
            class="session-card"
            :class="{
              selected: selectedSessionId === s.id,
              'select-mode': selectMode,
              checked: selectMode && selectedIds.has(s.id),
            }"
            @click="handleCardClick(s, 'history')"
          >
            <div class="session-card-header">
              <input
                v-if="selectMode"
                type="checkbox"
                class="card-checkbox"
                :checked="selectedIds.has(s.id)"
                @click.stop="toggleSelect(s.id)"
              />
              <span class="session-card-name">{{ s.name }}</span>
              <span class="badge" :class="s.status">{{ s.status }}</span>
            </div>
            <div class="session-card-meta">
              {{ new Date(s.start_time).toLocaleString() }}
            </div>
            <div class="session-card-query">{{ s.query }}</div>
            <div v-if="!selectMode" class="session-card-footer">
              <button
                v-if="s.config_yaml || s.config_path"
                class="action-btn inspect"
                @click.stop="emit('inspect-in-builder', s)"
              >Inspect</button>
              <button
                class="action-btn rerun"
                @click.stop="emit('rerun', s.id)"
              >Re-run</button>
            </div>
          </div>
          <div v-if="filteredHistorySessions.length === 0 && sessions.length > 0" class="centered">
            No matching sessions
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.session-panel {
  position: relative;
  border-left: 1px solid var(--border-subtle);
  background: var(--surface-2);
  display: flex;
  flex-direction: column;
  overflow: hidden;
  transition: width 0.2s ease, min-width 0.2s ease, opacity 0.2s ease;
  flex-shrink: 0;
}

.session-panel.resizing {
  transition: none;
  user-select: none;
}

.session-panel.collapsed {
  width: 0 !important;
  min-width: 0 !important;
  opacity: 0;
  border-left: none;
  pointer-events: none;
}

.resize-handle {
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  width: 4px;
  cursor: col-resize;
  z-index: 10;
}
.resize-handle:hover,
.session-panel.resizing .resize-handle {
  background: var(--accent-agent);
}

.panel-content {
  display: flex;
  flex-direction: column;
  height: 100%;
  overflow: hidden;
}

.panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 10px;
  background: var(--surface-3);
  border-bottom: 1px solid var(--border-subtle);
  flex-shrink: 0;
}
.panel-title {
  font-family: var(--font-mono);
  font-size: 11px;
  font-weight: 600;
  color: var(--text-primary);
  text-transform: uppercase;
  letter-spacing: 0.04em;
}
.panel-close-btn {
  background: none;
  border: none;
  color: var(--text-secondary);
  font-size: 16px;
  cursor: pointer;
  padding: 0 4px;
  line-height: 1;
}
.panel-close-btn:hover { color: var(--text-primary); }

/* Filter section */
.filter-section {
  padding: 8px;
  border-bottom: 1px solid var(--border-subtle);
  flex-shrink: 0;
}

.chip-row {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-bottom: 6px;
  align-items: center;
}

.filter-chip {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  padding: 2px 6px;
  background: rgba(91, 141, 239, 0.15);
  border-radius: 3px;
  font-size: 10px;
  max-width: 100%;
}
.filter-chip.regex {
  background: rgba(245, 158, 11, 0.15);
}
.chip-field {
  color: var(--accent-agent);
  font-weight: 600;
}
.chip-value {
  color: var(--text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 140px;
}
.chip-remove {
  border: none;
  background: none;
  color: var(--text-muted);
  cursor: pointer;
  font-size: 10px;
  padding: 0 2px;
  line-height: 1;
}
.chip-remove:hover { color: var(--status-error); }

.chip-clear {
  border: none;
  background: none;
  color: var(--status-error);
  cursor: pointer;
  font-size: 10px;
  padding: 0 4px;
}
.chip-clear:hover { text-decoration: underline; }

.input-row {
  display: flex;
  gap: 3px;
  align-items: stretch;
}

.field-select {
  padding: 4px 2px;
  border: 1px solid var(--input-border);
  border-radius: 3px;
  font-size: 10px;
  background: var(--input-bg);
  color: var(--input-text);
  outline: none;
  cursor: pointer;
  width: 60px;
  flex-shrink: 0;
}
.field-select:focus { border-color: var(--border-focus); }

.filter-input {
  flex: 1;
  min-width: 0;
  padding: 4px 6px;
  border: 1px solid var(--input-border);
  border-radius: 3px;
  font-size: 11px;
  outline: none;
  background: var(--input-bg);
  color: var(--input-text);
}
.filter-input::placeholder { color: var(--input-placeholder); }
.filter-input:focus { border-color: var(--border-focus); }

.regex-btn, .add-btn {
  padding: 4px 6px;
  border: 1px solid var(--border-default);
  border-radius: 3px;
  background: var(--surface-3);
  font-size: 10px;
  font-weight: 700;
  cursor: pointer;
  color: var(--text-muted);
  flex-shrink: 0;
}
.regex-btn.active { background: var(--accent-agent); color: white; border-color: var(--accent-agent); }
.add-btn { color: var(--status-running); border-color: var(--status-running); }
.add-btn:hover:not(:disabled) { background: var(--status-running); color: white; }
.add-btn:disabled { opacity: 0.4; cursor: default; }

/* Select toolbar */
.select-toolbar {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-top: 6px;
}

.select-toggle-btn {
  padding: 3px 8px;
  border: 1px solid var(--border-default);
  border-radius: 3px;
  background: var(--surface-3);
  font-size: 10px;
  cursor: pointer;
  color: var(--text-secondary);
}
.select-toggle-btn.active {
  background: var(--accent-agent);
  color: white;
  border-color: var(--accent-agent);
}

.select-action-btn {
  padding: 3px 6px;
  border: 1px solid var(--border-default);
  border-radius: 3px;
  background: var(--surface-3);
  font-size: 10px;
  cursor: pointer;
  color: var(--text-secondary);
}
.select-action-btn:hover { background: var(--surface-4); }

.select-count {
  font-size: 10px;
  color: var(--accent-agent);
  font-weight: 600;
  margin-left: auto;
}

.delete-btn {
  padding: 3px 8px;
  border: 1px solid var(--status-error);
  border-radius: 3px;
  background: var(--surface-3);
  font-size: 10px;
  font-weight: 600;
  cursor: pointer;
  color: var(--status-error);
}
.delete-btn:hover:not(:disabled) { background: var(--status-error); color: white; }
.delete-btn:disabled { opacity: 0.4; cursor: default; }

.mode-container {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  min-height: 0;
}

/* Session list */
.session-list {
  flex: 1;
  overflow-y: auto;
  padding: 8px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-height: 0;
}

.launcher-section {
  flex: 1;
  overflow-y: auto;
  min-height: 0;
}

.centered {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px 12px;
  color: var(--text-muted);
  font-size: 12px;
  text-align: center;
}
.centered.error { color: var(--status-error); }

/* Session cards */
.session-card {
  padding: 10px 12px;
  background: var(--surface-3);
  border: 1px solid var(--border-default);
  border-radius: var(--node-radius);
  cursor: pointer;
  transition: border-color 0.2s, box-shadow 0.2s;
}
.session-card:hover {
  border-color: var(--accent-agent);
  box-shadow: 0 1px 4px rgba(91, 141, 239, 0.15);
}
.session-card.attached {
  border-color: var(--accent-skill);
  box-shadow: 0 0 0 1px var(--accent-skill);
}
.session-card.selected {
  border-color: var(--accent-agent);
  background: rgba(91, 141, 239, 0.1);
}
.session-card.select-mode {
  cursor: default;
}
.session-card.checked {
  border-color: var(--status-error);
  background: rgba(239, 68, 68, 0.1);
}

.session-card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 4px;
  gap: 4px;
}

.card-checkbox {
  width: 14px;
  height: 14px;
  flex-shrink: 0;
  cursor: pointer;
  accent-color: var(--status-error);
}

.session-card-name {
  font-weight: 600;
  font-size: 12px;
  color: var(--text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  flex: 1;
}

.session-card-badges {
  display: flex;
  gap: 4px;
  flex-shrink: 0;
}

.badge {
  font-size: 10px;
  padding: 1px 5px;
  border-radius: 3px;
  font-weight: 500;
  white-space: nowrap;
}
.badge.completed, .badge.success { background: rgba(34, 197, 94, 0.15); color: var(--status-success); }
.badge.running { background: rgba(91, 141, 239, 0.15); color: var(--accent-agent); }
.badge.error { background: rgba(239, 68, 68, 0.15); color: var(--status-error); }
.badge.timeout { background: rgba(245, 158, 11, 0.15); color: var(--status-paused); }
.badge.stale { background: var(--surface-4); color: var(--text-muted); font-style: italic; }
.badge.attached { background: rgba(192, 132, 252, 0.15); color: var(--accent-skill); }

.session-card-meta {
  font-size: 11px;
  color: var(--text-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.session-card-query {
  font-size: 11px;
  color: var(--text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  margin-top: 2px;
}

.session-card-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 6px;
  gap: 4px;
}

.session-card-agents {
  font-size: 10px;
  color: var(--text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  flex: 1;
}

/* Action buttons */
.action-btn {
  padding: 3px 8px;
  border-radius: 3px;
  font-size: 10px;
  cursor: pointer;
  white-space: nowrap;
  border: 1px solid;
  background: var(--surface-3);
}

.action-btn.attach { color: var(--accent-agent); border-color: var(--accent-agent); }
.action-btn.attach:hover { background: var(--accent-agent); color: white; }

.action-btn.detach { color: var(--status-error); border-color: var(--status-error); }
.action-btn.detach:hover { background: var(--status-error); color: white; }

.action-btn.inspect { color: var(--accent-agent); border-color: var(--accent-agent); }
.action-btn.inspect:hover { background: var(--accent-agent); color: white; }

.action-btn.rerun { color: var(--status-running); border-color: var(--status-running); }
.action-btn.rerun:hover { background: var(--status-running); color: white; }

.action-btn.refresh { color: var(--text-secondary); border-color: var(--border-default); }
.action-btn.refresh:hover { background: var(--surface-4); }

.action-btn.send { color: var(--status-running); border-color: var(--status-running); }
.action-btn.send:hover:not(:disabled) { background: var(--status-running); color: white; }
.action-btn.send:disabled { opacity: 0.4; cursor: default; }

.session-send-box {
  display: flex;
  gap: 4px;
  margin-top: 6px;
}
.session-send-box input {
  flex: 1;
  min-width: 0;
  font-size: 11px;
  padding: 3px 6px;
  background: var(--surface-1);
  color: var(--text-primary);
  border: 1px solid var(--border-default);
  border-radius: 3px;
}

.send-error {
  margin-top: 4px;
  font-size: 10px;
  color: var(--status-error);
}

.panel-actions {
  display: flex;
  justify-content: center;
  padding: 8px 0;
}
</style>
