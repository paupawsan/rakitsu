<script setup lang="ts">
// ConfigTab — Sessions tab Inspect viewer Config sub-tab.
//
// Renders the YAML config that ran this session, read-only. Source:
// SessionMeta.config_yaml (populated by the server when the session
// started). Older sessions persisted only config_path and won't have
// inline YAML here — surface that as an empty state instead of fetching
// from disk; the Clone action is the path that needs the YAML,
// and it'll handle the fallback there.

import { computed, onMounted, ref, watch } from 'vue';
import { useSessionHistory } from '../../../composables/useSessionHistory';

const props = defineProps<{
  sessionId: string;
}>();

const history = useSessionHistory();
const copied = ref(false);
let copiedTimer: ReturnType<typeof setTimeout> | undefined;

async function load(id: string) {
  copied.value = false;
  await history.loadSession(id);
}

onMounted(() => load(props.sessionId));
watch(() => props.sessionId, (id) => { if (id) load(id); });

const yaml = computed(() => history.selectedSession.value?.config_yaml ?? '');
const path = computed(() => history.selectedSession.value?.config_path ?? '');
const hasYaml = computed(() => yaml.value.length > 0);

async function copyYaml() {
  if (!yaml.value) return;
  try {
    await navigator.clipboard.writeText(yaml.value);
    copied.value = true;
    if (copiedTimer) clearTimeout(copiedTimer);
    copiedTimer = setTimeout(() => { copied.value = false; }, 1500);
  } catch {
    /* clipboard unavailable */
  }
}

function lineCount(s: string): number {
  if (!s) return 0;
  return s.replace(/\n+$/, '').split('\n').length;
}
</script>

<template>
  <div class="config-tab">
    <div v-if="history.loading.value" class="centered">Loading config…</div>
    <div v-else-if="history.error.value" class="centered err">{{ history.error.value }}</div>
    <div v-else-if="!hasYaml" class="centered empty">
      <strong>No inline YAML on this session.</strong>
      <div class="empty-hint">
        <template v-if="path">
          Server recorded only the config path:
          <code>{{ path }}</code>.
          The Clone action will resolve this on-demand.
        </template>
        <template v-else>
          Neither inline YAML nor config_path is present — this can happen for
          very old sessions or sessions where the config wasn't captured.
        </template>
      </div>
    </div>
    <template v-else>
      <header class="config-header">
        <span class="title">Config</span>
        <span class="lines">{{ lineCount(yaml) }} lines</span>
        <span v-if="path" class="path" :title="path">{{ path }}</span>
        <button class="copy-btn" :class="{ copied }" @click="copyYaml">
          {{ copied ? 'Copied' : 'Copy YAML' }}
        </button>
      </header>
      <pre class="yaml">{{ yaml }}</pre>
    </template>
  </div>
</template>

<style scoped>
.config-tab {
  height: 100%;
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.config-header {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 16px;
  border-bottom: 1px solid var(--border-subtle);
  background: var(--surface-1);
  flex-shrink: 0;
  font-size: 12px;
}
.title {
  font-weight: 600;
  color: var(--text-primary);
}
.lines {
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--text-muted);
}
.path {
  flex: 1;
  font-family: var(--font-mono);
  font-size: 10px;
  color: var(--text-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  text-align: right;
}
.copy-btn {
  background: var(--surface-3);
  border: 1px solid var(--border-subtle);
  color: var(--text-secondary);
  font-family: var(--font-mono);
  font-size: 11px;
  padding: 4px 10px;
  border-radius: 3px;
  cursor: pointer;
}
.copy-btn:hover {
  color: var(--text-primary);
  background: var(--surface-4);
}
.copy-btn.copied {
  color: var(--status-running);
}

.yaml {
  flex: 1;
  margin: 0;
  padding: 12px 16px;
  overflow: auto;
  background: var(--surface-0);
  color: var(--text-primary);
  font-family: var(--font-mono);
  font-size: 12px;
  line-height: 1.5;
  white-space: pre;
  word-break: normal;
}

.centered {
  padding: 32px 16px;
  text-align: center;
  color: var(--text-muted);
  font-size: 13px;
  line-height: 1.5;
}
.centered.err { color: var(--status-error, #ef4444); }
.centered.empty strong { color: var(--text-primary); }

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
  word-break: break-all;
}
</style>
