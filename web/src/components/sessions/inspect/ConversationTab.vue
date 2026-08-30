<script setup lang="ts">
import { computed, onMounted, watch } from 'vue';
import { useSessionHistory } from '../../../composables/useSessionHistory';
import { reconstructChatBlocksFromEvents } from '../../../composables/reconstructChat';
import ChatHistoryPanel from '../../chat/ChatHistoryPanel.vue';

// ConversationTab — read-only chat replay for a past session. The whole
// pipeline (fetch meta+events -> reconstruct blocks -> render) lives in
// useSessionHistory + reconstructChat + ChatHistoryPanel already; this tab
// just wires them together for the new Sessions tab. When the user picks
// a different session in the parent, :key on InspectViewer remounts us,
// so onMounted is enough to drive the initial load.

const props = defineProps<{
  sessionId: string;
}>();

const history = useSessionHistory();

const blocks = computed(() =>
  reconstructChatBlocksFromEvents(history.sessionEvents.value),
);

const title = computed(() => history.selectedSession.value?.name ?? '');
const loading = computed(() => history.loading.value);
const err = computed(() => history.error.value);
const isEmpty = computed(
  () => !loading.value && !err.value && blocks.value.length === 0,
);

async function load(id: string) {
  await history.loadSession(id);
}

onMounted(() => {
  load(props.sessionId);
});

// Defensive: if the parent forgot to re-key on session change, react to
// prop changes too.
watch(
  () => props.sessionId,
  (id) => {
    if (id) load(id);
  },
);
</script>

<template>
  <div class="conversation-tab">
    <div v-if="loading" class="centered">Loading session…</div>
    <div v-else-if="err" class="centered err">{{ err }}</div>
    <div v-else-if="isEmpty" class="centered empty">
      No conversation reconstructed from this session's events.
      <div class="empty-hint">
        Older sessions without CHAT_TURN_START events may render as turns with
        placeholder user prompts (the text was never emitted).
      </div>
    </div>
    <ChatHistoryPanel
      v-else
      :blocks="blocks"
      :title="title"
    />
  </div>
</template>

<style scoped>
.conversation-tab {
  height: 100%;
  display: flex;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
}

.centered {
  padding: 24px 16px;
  text-align: center;
  color: var(--text-muted);
  font-size: 13px;
}
.centered.err {
  color: var(--status-error, #ef4444);
}

.empty-hint {
  margin-top: 8px;
  font-size: 12px;
  line-height: 1.5;
  color: var(--text-muted);
  max-width: 460px;
  margin-left: auto;
  margin-right: auto;
}
</style>
