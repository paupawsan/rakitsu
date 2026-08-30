<script setup lang="ts">
// TreeTab — Sessions tab Inspect viewer Tree sub-tab.
//
// Fetches the persisted .chat.json snapshot via GET /api/chat/{id}/tree
// (falls back to disk when the session is no longer live) and renders it
// through BranchTreeView in static mode. No WebSocket: this is forensic,
// not live. If the session predates the .chat.json envelope or never
// produced one (CLI run with no chat turns), the endpoint 404s and we
// surface a friendly empty state.

import { onMounted, ref, watch } from 'vue';
import type { TreeSnapshot } from '../../../types/chat';
import BranchTreeView from '../../chat/BranchTreeView.vue';

const props = defineProps<{
  sessionId: string;
}>();

const tree = ref<TreeSnapshot | null>(null);
const loading = ref(false);
const error = ref<string | null>(null);
const notFound = ref(false);

async function load() {
  loading.value = true;
  error.value = null;
  notFound.value = false;
  tree.value = null;
  try {
    const res = await fetch(`/api/chat/${encodeURIComponent(props.sessionId)}/tree`);
    if (res.status === 404) {
      notFound.value = true;
      return;
    }
    if (!res.ok) {
      throw new Error(`HTTP ${res.status}`);
    }
    tree.value = (await res.json()) as TreeSnapshot;
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
  } finally {
    loading.value = false;
  }
}

onMounted(load);
watch(() => props.sessionId, load);
</script>

<template>
  <div class="tree-tab">
    <div v-if="loading" class="centered">Loading tree…</div>
    <div v-else-if="error" class="centered err">Error: {{ error }}</div>
    <div v-else-if="notFound" class="centered empty">
      <strong>No branching turn history on disk for this session.</strong>
      <div class="empty-hint">
        Persisted <code>.chat.json</code> snapshots are produced by chat
        (interactive) sessions starting 2026-05-20. Older sessions
        and CLI <code>rakitsu run</code> invocations with no chat turns don't
        have one.
      </div>
    </div>
    <BranchTreeView
      v-else
      :session-id="sessionId"
      :tree="tree"
      class="viewer"
    />
  </div>
</template>

<style scoped>
.tree-tab {
  height: 100%;
  display: flex;
  flex-direction: column;
  min-height: 0;
}
.viewer {
  flex: 1;
  min-height: 0;
}

.centered {
  padding: 32px 16px;
  text-align: center;
  color: var(--text-muted);
  font-size: 13px;
  line-height: 1.5;
}
.centered.err {
  color: var(--status-error, #ef4444);
}
.centered.empty strong {
  color: var(--text-primary);
}
.empty-hint {
  margin-top: 12px;
  max-width: 460px;
  margin-left: auto;
  margin-right: auto;
  font-size: 12px;
  color: var(--text-muted);
}
.empty-hint code {
  font-family: var(--font-mono);
  font-size: 11px;
  background: var(--surface-3);
  padding: 1px 4px;
  border-radius: 2px;
}
</style>
