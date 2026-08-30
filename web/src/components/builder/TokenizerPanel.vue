<script setup lang="ts">
import { ref, computed, watch } from 'vue';
import { useTokenizer, type TokenizeResult } from '../../composables/useTokenizer';
import type { AgentConfig } from '../../types/config';

export interface TokenizerSegment {
  label: string;
  text: string;
  model: string;
}

interface PromptSegment extends TokenizerSegment {
  result: TokenizeResult | null;
  loading: boolean;
}

const props = defineProps<{
  selfUrl: string;
  agents?: AgentConfig[];
  query?: string;
  visible: boolean;
  /** Pre-built segments — when provided, overrides agents+query auto-build */
  initialSegments?: TokenizerSegment[];
}>();

const emit = defineEmits<{
  close: [];
}>();

const baseUrl = computed(() => props.selfUrl);
const { tokenize } = useTokenizer(baseUrl);

const showIds = ref(false);
const segments = ref<PromptSegment[]>([]);
const selectedIdx = ref(0);

// Build segments from initialSegments (if provided) or agents + query
function buildSegments(): PromptSegment[] {
  if (props.initialSegments && props.initialSegments.length > 0) {
    return props.initialSegments
      .filter(s => s.text)
      .map(s => ({ ...s, result: null, loading: false }));
  }

  const segs: PromptSegment[] = [];
  for (const agent of (props.agents ?? [])) {
    if (agent.system_prompt) {
      segs.push({
        label: agent.name || 'Agent',
        text: agent.system_prompt,
        model: agent.model || 'gpt-4',
        result: null,
        loading: false,
      });
    }
  }

  if (props.query) {
    const model = (props.agents ?? [])[0]?.model || 'gpt-4';
    segs.push({ label: 'Query', text: props.query, model, result: null, loading: false });
  }

  return segs;
}

async function tokenizeAll() {
  segments.value = buildSegments();
  selectedIdx.value = 0;

  const promises = segments.value.map(async (seg, i) => {
    segments.value[i]!.loading = true;
    const result = await tokenize(seg.text, seg.model);
    segments.value[i]!.result = result;
    segments.value[i]!.loading = false;
  });

  await Promise.all(promises);
}

// Auto-tokenize when panel becomes visible
watch(() => props.visible, (visible) => {
  if (visible) tokenizeAll();
});

const totalTokens = computed(() =>
  segments.value.reduce((sum, seg) => sum + (seg.result?.total ?? 0), 0)
);

const currentEncoding = computed(() => {
  const first = segments.value.find(s => s.result?.encoding);
  return first?.result?.encoding ?? '';
});

// Format token text for display — make whitespace visible
function displayToken(text: string): string {
  return text
    .replace(/ /g, '\u00B7')     // middle dot for space
    .replace(/\n/g, '\u21B5\n')  // return symbol for newline
    .replace(/\t/g, '\u2192 ');  // arrow for tab
}
</script>

<template>
  <Teleport to="body">
    <div v-if="visible" class="tkp-overlay" @click.self="emit('close')">
      <div class="tkp-panel">
        <!-- Header -->
        <div class="tkp-header">
          <span class="tkp-title">Prompt Tokenizer</span>
          <div class="tkp-header-actions">
            <div class="tkp-toggle">
              <button
                class="tkp-toggle-btn"
                :class="{ active: !showIds }"
                @click="showIds = false"
              >Token text</button>
              <button
                class="tkp-toggle-btn"
                :class="{ active: showIds }"
                @click="showIds = true"
              >Token id</button>
            </div>
            <button class="tkp-close" @click="emit('close')" title="Close">&times;</button>
          </div>
        </div>

        <!-- Body -->
        <div class="tkp-body">
          <!-- Segment sidebar -->
          <div class="tkp-sidebar">
            <div class="tkp-sidebar-title">Segments</div>
            <div
              v-for="(seg, i) in segments"
              :key="i"
              class="tkp-seg-item"
              :class="{ selected: selectedIdx === i }"
              @click="selectedIdx = i"
            >
              <span class="tkp-seg-label">{{ seg.label }}</span>
              <span v-if="seg.loading" class="tkp-seg-count loading">...</span>
              <span v-else-if="seg.result" class="tkp-seg-count">{{ seg.result.total }}</span>
            </div>
          </div>

          <!-- Token display -->
          <div class="tkp-display">
            <template v-if="segments[selectedIdx]">
              <div v-if="segments[selectedIdx]!.loading" class="tkp-loading">
                Tokenizing...
              </div>
              <div v-else-if="segments[selectedIdx]!.result" class="tkp-tokens">
                <span
                  v-for="(tok, j) in segments[selectedIdx]!.result!.tokens"
                  :key="j"
                  class="tkp-token"
                  :class="`tok-${j % 6}`"
                  :title="`ID: ${tok.id}`"
                >{{ showIds ? tok.id : displayToken(tok.text) }}</span>
              </div>
              <div v-else class="tkp-empty">
                No tokens to display
              </div>
            </template>
            <div v-else class="tkp-empty">
              Add agents with system prompts to tokenize
            </div>
          </div>
        </div>

        <!-- Footer -->
        <div class="tkp-footer">
          <span class="tkp-stat">
            Total: <strong>{{ totalTokens }}</strong> tokens
          </span>
          <span v-if="currentEncoding" class="tkp-stat">
            Encoding: <strong>{{ currentEncoding }}</strong>
          </span>
          <button class="tkp-refresh" @click="tokenizeAll" title="Re-tokenize">
            Refresh
          </button>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.tkp-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.tkp-panel {
  background: var(--surface-1);
  border: 1px solid var(--border-default);
  border-radius: 8px;
  width: 780px;
  max-width: 90vw;
  max-height: 80vh;
  display: flex;
  flex-direction: column;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.4);
}

.tkp-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  border-bottom: 1px solid var(--border-subtle);
}

.tkp-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
}

.tkp-header-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.tkp-toggle {
  display: flex;
  background: var(--surface-3);
  border-radius: 4px;
  overflow: hidden;
}

.tkp-toggle-btn {
  padding: 4px 10px;
  border: none;
  background: transparent;
  color: var(--text-secondary);
  font-size: 11px;
  font-family: var(--font-sans);
  cursor: pointer;
  transition: all 0.15s;
}

.tkp-toggle-btn.active {
  background: var(--surface-4);
  color: var(--text-primary);
}

.tkp-close {
  width: 28px;
  height: 28px;
  border: none;
  background: transparent;
  color: var(--text-secondary);
  font-size: 18px;
  cursor: pointer;
  border-radius: 4px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.tkp-close:hover {
  background: var(--surface-3);
  color: var(--text-primary);
}

.tkp-body {
  display: flex;
  flex: 1;
  overflow: hidden;
  min-height: 300px;
}

.tkp-sidebar {
  width: 180px;
  border-right: 1px solid var(--border-subtle);
  padding: 8px 0;
  overflow-y: auto;
  flex-shrink: 0;
}

.tkp-sidebar-title {
  padding: 4px 12px 8px;
  font-size: 10px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--text-muted);
}

.tkp-seg-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 12px;
  cursor: pointer;
  transition: background 0.1s;
}

.tkp-seg-item:hover {
  background: var(--surface-2);
}

.tkp-seg-item.selected {
  background: var(--surface-3);
}

.tkp-seg-label {
  font-size: 12px;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.tkp-seg-count {
  font-size: 11px;
  font-family: var(--font-mono);
  color: var(--text-secondary);
  padding: 1px 6px;
  background: var(--surface-4);
  border-radius: 3px;
  flex-shrink: 0;
  margin-left: 8px;
}

.tkp-seg-count.loading {
  color: var(--text-muted);
}

.tkp-display {
  flex: 1;
  padding: 16px;
  overflow-y: auto;
}

.tkp-loading {
  color: var(--text-muted);
  font-size: 13px;
  font-style: italic;
}

.tkp-empty {
  color: var(--text-muted);
  font-size: 13px;
}

.tkp-tokens {
  line-height: 1.8;
  font-family: var(--font-mono);
  font-size: 13px;
}

.tkp-token {
  display: inline;
  padding: 2px 1px;
  border-radius: 2px;
  white-space: pre-wrap;
  word-break: break-all;
}

/* 6 alternating token colors — dark-theme safe pastels */
.tok-0 { background: rgba(91, 141, 239, 0.22); color: #a8c4f7; }
.tok-1 { background: rgba(45, 212, 160, 0.22); color: #7eebc8; }
.tok-2 { background: rgba(192, 132, 252, 0.22); color: #d4b0fd; }
.tok-3 { background: rgba(244, 114, 182, 0.22); color: #f5a0c7; }
.tok-4 { background: rgba(251, 191, 36, 0.22); color: #f5d576; }
.tok-5 { background: rgba(99, 211, 255, 0.22); color: #8ee0ff; }

.tkp-footer {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 10px 16px;
  border-top: 1px solid var(--border-subtle);
  font-size: 12px;
}

.tkp-stat {
  color: var(--text-secondary);
  font-family: var(--font-mono);
}

.tkp-stat strong {
  color: var(--text-primary);
}

.tkp-refresh {
  margin-left: auto;
  padding: 4px 10px;
  border: 1px solid var(--border-default);
  background: var(--surface-3);
  color: var(--text-secondary);
  font-size: 11px;
  font-family: var(--font-sans);
  border-radius: 4px;
  cursor: pointer;
  transition: all 0.15s;
}

.tkp-refresh:hover {
  background: var(--surface-4);
  color: var(--text-primary);
}
</style>
