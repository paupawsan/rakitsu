<script setup lang="ts">
import { ref, onMounted } from 'vue';
import SessionList from './SessionList.vue';
import InspectViewer from './InspectViewer.vue';
import { useSessionHistory } from '../../composables/useSessionHistory';
import type { ChatSessionMeta } from '../../types';

// SessionsView — top-level Sessions tab. Owns past-session browsing,
// replacing the dual-purpose role the Debugger used to play. Layout: card
// list on the left, Inspect viewer on the right.
//
// Resume / Clone / Re-run action emits — InspectViewer fires them,
// SessionsView forwards them up to App.vue which handles tab switching +
// workspace state. Fork emit yields a new live ChatSessionMeta, so
// App.vue routes it through the same handler as Resume.

const history = useSessionHistory();
const selectedId = ref<string | null>(null);

const emit = defineEmits<{
  resume: [meta: ChatSessionMeta];
  clone: [data: { yaml: string; projectId?: string }];
  rerun: [newSessionId: string];
  fork: [meta: ChatSessionMeta];
  debug: [sessionId: string];
}>();

onMounted(() => {
  history.fetchSessions();
});

function selectSession(id: string) {
  selectedId.value = id;
}

function refresh() {
  history.fetchSessions();
}
</script>

<template>
  <div class="sessions-view">
    <SessionList
      :sessions="history.sessions.value"
      :selected-id="selectedId"
      :loading="history.loading.value"
      :error="history.error.value"
      @select="selectSession"
      @refresh="refresh"
    />
    <section class="main">
      <InspectViewer
        v-if="selectedId"
        :key="selectedId"
        :session-id="selectedId"
        @resume="(meta) => emit('resume', meta)"
        @clone="(data) => emit('clone', data)"
        @rerun="(id) => emit('rerun', id)"
        @fork="(meta) => emit('fork', meta)"
        @debug="(id) => emit('debug', id)"
      />
      <div v-else class="placeholder">
        <h3>No session selected</h3>
        <p>Pick a session from the list to inspect its conversation, events, branch tree, and config — then Debug, Clone, Resume, Fork, or Re-run it from the header actions.</p>
      </div>
    </section>
  </div>
</template>

<style scoped>
.sessions-view {
  display: grid;
  grid-template-columns: 320px 1fr;
  height: 100%;
  background: var(--surface-0);
}

.main {
  overflow: hidden;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.placeholder {
  height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 32px;
  color: var(--text-muted);
  text-align: center;
}
.placeholder h3 {
  margin: 0 0 8px;
  font-size: 16px;
  color: var(--text-primary);
}
.placeholder p {
  margin: 0;
  font-size: 13px;
  line-height: 1.5;
  max-width: 420px;
}
</style>
