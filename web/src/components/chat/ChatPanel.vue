<script setup lang="ts">
import { computed, nextTick, onMounted, onBeforeUnmount, ref, watch } from 'vue';
import { useChatSession } from '../../composables/useChatSession';
import { renderMarkdown } from '../../composables/useMarkdown';

interface Props {
  sessionId: string;
  // When true, the WS stream includes raw telemetry events for the debugger
  // chat pane. The web chat tab leaves this off.
  forwardEvents?: boolean;
}

const props = withDefaults(defineProps<Props>(), { forwardEvents: false });

// One ChatPanel instance is scoped to one sessionId. The parent passes
// :key="sessionId" to force remount when switching sessions — which is
// cheap now because the composable is module-scoped: remount simply
// re-attaches to the existing state in the registry, no state is lost.
const chat = useChatSession(props.sessionId);
const input = ref('');
const messagesEl = ref<HTMLElement | null>(null);
const textareaEl = ref<HTMLTextAreaElement | null>(null);

// Input history recall (Up/Down). historyIdx === -1 means not navigating;
// the first step back saves the unsent draft so stepping forward restores it.
const historyIdx = ref(-1);
const historyDraft = ref('');

// Follow-mode scroll: pin to bottom while the user is at (or near) the
// bottom of the conversation; once they scroll up to re-read something,
// new agent tokens stop yanking the viewport down. They re-enter follow
// mode by scrolling back to the bottom themselves, or by sending a new
// turn (sending implies wanting to watch the answer).
const followBottom = ref(true);
const BOTTOM_SLACK = 64; // px tolerance — streaming wraps shouldn't unstick

function isAtBottom(el: HTMLElement): boolean {
  return el.scrollHeight - el.scrollTop - el.clientHeight <= BOTTOM_SLACK;
}

function onMessagesScroll() {
  const el = messagesEl.value;
  if (!el) return;
  followBottom.value = isAtBottom(el);
}

function scrollToBottomIfFollowing() {
  if (!followBottom.value) return;
  const el = messagesEl.value;
  if (!el) return;
  el.scrollTop = el.scrollHeight;
}

function pinToBottom() {
  followBottom.value = true;
  nextTick().then(() => {
    const el = messagesEl.value;
    if (el) el.scrollTop = el.scrollHeight;
  });
}

onMounted(() => {
  chat.connect({ forwardEvents: props.forwardEvents });
  nextTick().then(() => {
    const el = messagesEl.value;
    if (el) {
      el.scrollTop = el.scrollHeight;
      el.addEventListener('scroll', onMessagesScroll, { passive: true });
    }
  });
});

onBeforeUnmount(() => {
  messagesEl.value?.removeEventListener('scroll', onMessagesScroll);
});

// Deep watch on blocks: token streaming mutates an existing block's text
// in place, so the previous .length watch missed the steady stream and
// only fired when a new block appeared (causing visible "lag and snap").
watch(
  () => chat.blocks.value,
  async () => {
    await nextTick();
    scrollToBottomIfFollowing();
  },
  { deep: true },
);

const placeholder = computed(() => {
  if (chat.pendingInput.value) return 'Answer the agent\u2019s question\u2026';
  if (chat.isGenerating.value) return 'Enter interrupts and replaces the current turn\u2026';
  return 'Type a message\u2026  (Enter to send, Shift+Enter for newline)';
});

function handleSubmit(event: Event) {
  event.preventDefault();
  historyIdx.value = -1; // exit history navigation on send
  const text = input.value.trim();
  if (!text) return;
  // Answer a pending user_input before submitting a fresh turn.
  if (chat.pendingInput.value) {
    chat.respondUserInput(text);
    input.value = '';
    return;
  }
  // Fluid-chat: interrupt any live gen then submit.
  if (chat.isGenerating.value) {
    chat.interrupt();
  }
  chat.submit(text);
  input.value = '';
  // Sending implies the user wants to watch the answer — re-enter follow
  // mode even if they had previously scrolled up to read history.
  pinToBottom();
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) {
    handleSubmit(event);
    return;
  }
  // Up/Down recall input history. ArrowUp only hijacks when navigating
  // already or the caret is at the very start, so multi-line editing still
  // works. ArrowDown only acts while navigating.
  const el = textareaEl.value;
  if (event.key === 'ArrowUp' && !event.isComposing) {
    if (historyIdx.value !== -1 || (el && el.selectionStart === 0)) {
      event.preventDefault();
      recallPrev();
    }
  } else if (event.key === 'ArrowDown' && !event.isComposing) {
    if (historyIdx.value !== -1) {
      event.preventDefault();
      recallNext();
    }
  }
}

function recallPrev() {
  const hist = chat.inputHistory.value;
  if (hist.length === 0) return;
  if (historyIdx.value === -1) {
    historyDraft.value = input.value;
    historyIdx.value = hist.length;
  }
  if (historyIdx.value > 0) {
    historyIdx.value--;
    input.value = hist[historyIdx.value] ?? '';
    moveCaretToEnd();
  }
}

function recallNext() {
  if (historyIdx.value === -1) return;
  const hist = chat.inputHistory.value;
  historyIdx.value++;
  if (historyIdx.value >= hist.length) {
    historyIdx.value = -1;
    input.value = historyDraft.value;
  } else {
    input.value = hist[historyIdx.value] ?? '';
  }
  moveCaretToEnd();
}

function moveCaretToEnd() {
  nextTick(() => {
    const el = textareaEl.value;
    if (el) {
      el.focus();
      el.selectionStart = el.selectionEnd = el.value.length;
    }
  });
}

function stopGeneration() {
  chat.interrupt();
}

// truncate shortens long tool-argument strings for the block header.
function truncate(s: string | undefined, n: number): string {
  if (!s) return '';
  return s.length > n ? s.slice(0, n - 3) + '...' : s;
}

function lineCount(s: string | undefined): number {
  if (!s) return 0;
  return s.replace(/\n+$/, '').split('\n').length;
}

// fmtSubTokens renders a subagent's token count the way the TUI does:
// compact "3.0K tok" above 1000, plain "N tok" below.
function fmtSubTokens(n: number | undefined): string {
  const tokens = n ?? 0;
  return tokens >= 1000 ? `${(tokens / 1000).toFixed(1)}K tok` : `${tokens} tok`;
}

// --- Branch ops UI ---
//
// Backend + composable methods (editTurn / regenerate / switchBranch) wire
// three inline affordances:
//   - User blocks get a ✎ edit button on hover. Clicking swaps the bubble
//     for an inline textarea; Submit calls editTurn(turnId, newText) which
//     creates a sibling branch at the same level and switches to it; the
//     server's transcript_resync drives the visible refresh.
//   - The last assistant block gets a ⟳ regenerate button on hover.
//     Clicking calls regenerate(turnId) which creates a sibling of the
//     same user turn (same UserText) — semantically "try again".
//   - The ‹n/m› chip on a user block becomes clickable: chevrons call
//     switchBranch(turnId, ±1) to flip the active sibling.
//
// All three call WS messages; the server fans out the resync. Disabled
// while a turn is generating — letting the user interrupt or wait first
// avoids racing with the live stream.

const editingIdx = ref<number | null>(null);
const editingText = ref<string>('');
const editingTextareaEl = ref<HTMLTextAreaElement | null>(null);

function startEdit(idx: number, currentText: string) {
  if (chat.isGenerating.value) return;
  editingIdx.value = idx;
  editingText.value = currentText;
  // Focus + select-all once the textarea mounts.
  nextTick(() => {
    const el = editingTextareaEl.value;
    if (el) {
      el.focus();
      el.select();
    }
  });
}

function cancelEdit() {
  editingIdx.value = null;
  editingText.value = '';
}

function submitEdit(turnId: string | undefined) {
  if (!turnId) return cancelEdit();
  const text = editingText.value.trim();
  if (!text) return cancelEdit();
  chat.editTurn(turnId, text);
  cancelEdit();
}

function handleEditKeydown(event: KeyboardEvent, turnId: string | undefined) {
  if (event.key === 'Escape') {
    event.preventDefault();
    cancelEdit();
    return;
  }
  if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
    event.preventDefault();
    submitEdit(turnId);
  }
}

function regenerateAt(turnId: string | undefined) {
  if (!turnId || chat.isGenerating.value) return;
  chat.regenerate(turnId);
}

function switchAt(turnId: string | undefined, dir: -1 | 1) {
  if (!turnId || chat.isGenerating.value) return;
  chat.switchBranch(turnId, dir);
}

// Index of the most recent assistant block. Used to show ⟳ on only the
// last one — earlier assistants in the path are part of finished turns
// the user can't re-roll without first walking back via edit.
const lastAssistantIdx = computed(() => {
  const blocks = chat.blocks.value;
  for (let i = blocks.length - 1; i >= 0; i--) {
    if (blocks[i]?.kind === 'assistant') return i;
  }
  return -1;
});

// --- Copy ---

// copiedIdx marks the block whose Copy button was just clicked, so the
// button can show "Copied" feedback; cleared after a short delay.
const copiedIdx = ref<number | null>(null);
let copiedTimer: ReturnType<typeof setTimeout> | undefined;

async function copyBlock(text: string, idx: number) {
  try {
    await navigator.clipboard.writeText(text);
    copiedIdx.value = idx;
    if (copiedTimer) clearTimeout(copiedTimer);
    copiedTimer = setTimeout(() => {
      copiedIdx.value = null;
    }, 1500);
  } catch {
    /* clipboard unavailable or permission denied — ignore */
  }
}

// Delegated copy for code blocks inside rendered Markdown. The `.md-copy`
// buttons are plain HTML injected by useMarkdown's fence rule, so they have
// no Vue handler — catch the click here and copy the sibling <code>.
async function handleContentClick(event: MouseEvent) {
  const target = event.target as HTMLElement | null;
  const btn = target?.closest('.md-copy') as HTMLElement | null;
  if (!btn) return;
  const code = btn.parentElement?.querySelector('code');
  if (!code) return;
  try {
    await navigator.clipboard.writeText(code.textContent ?? '');
    const original = btn.textContent;
    btn.textContent = 'Copied';
    btn.classList.add('copied');
    setTimeout(() => {
      btn.textContent = original;
      btn.classList.remove('copied');
    }, 1500);
  } catch {
    /* ignore */
  }
}
</script>

<template>
  <div class="chat-panel">
    <header class="chat-header">
      <div class="title">
        <span class="agent-name">{{ chat.meta.value?.agent_name || 'chat' }}</span>
        <span v-if="chat.meta.value?.model" class="model-tag">{{ chat.meta.value.model }}</span>
      </div>
      <div class="status">
        <span :class="['led', chat.isConnected.value ? 'on' : 'off']"></span>
        <span v-if="chat.isGenerating.value" class="generating">generating…</span>
        <span v-else-if="chat.isConnected.value">connected</span>
        <span v-else>disconnected</span>
      </div>
    </header>

    <div ref="messagesEl" class="messages">
      <div
        v-for="(block, i) in chat.blocks.value"
        :key="i"
        :class="['block', `block-${block.kind}`]"
        :style="(block.kind === 'tool' || block.kind === 'reasoning' || block.kind === 'subagent') && block.depth
          ? { marginLeft: block.depth * 18 + 'px' }
          : undefined"
      >
        <!-- Tool-call block: collapsible header + output (or error) body.
             Failed calls (toolErr set) get a red left border and a truncated
             error in the header; clicking expands to the full error so the
             user can read the whole "tool not found: …" string instead of
             a wrapped overflow next to the args. -->
        <div
          v-if="block.kind === 'tool'"
          class="tool-block"
          :class="{ failed: !!block.toolErr }"
        >
          <div
            class="tool-header"
            :class="{ clickable: block.toolDone && (!!block.output || !!block.toolErr) }"
            @click="block.toolDone && (block.output || block.toolErr) && chat.toggleCollapse(i)"
          >
            <span v-if="block.toolDone && (block.output || block.toolErr)" class="glyph">{{ block.collapsed ? '▸' : '▾' }}</span>
            <span class="tool-name">
              <span v-if="block.depth && block.agentName" class="tool-agent">{{ block.agentName }} → </span>{{ block.toolName }}
            </span>
            <code v-if="block.toolArgs" class="tool-args">{{ truncate(block.toolArgs, 80) }}</code>
            <span v-if="!block.toolDone" class="tool-status running">⏳ running</span>
            <span v-else-if="block.toolErr" class="tool-status err">✗ {{ truncate(block.toolErr, 80) }}</span>
            <span v-else class="tool-status ok">✓ {{ block.duration }}ms</span>
          </div>
          <div v-if="block.toolDone && (block.output || block.toolErr) && block.collapsed" class="tool-summary">
            <template v-if="block.output">{{ lineCount(block.output) }} line{{ lineCount(block.output) === 1 ? '' : 's' }}</template>
            <template v-else>error · click to expand</template>
          </div>
          <div v-else-if="block.toolDone && (block.output || block.toolErr)" class="tool-output-wrap">
            <button
              type="button"
              class="tool-copy"
              :class="{ copied: copiedIdx === i }"
              :title="copiedIdx === i ? 'Copied' : (block.output ? 'Copy output' : 'Copy error')"
              @click="copyBlock(block.output || block.toolErr || '', i)"
            >{{ copiedIdx === i ? 'Copied' : 'Copy' }}</button>
            <pre class="tool-output" :class="{ 'tool-output-err': !block.output && !!block.toolErr }">{{ block.output || block.toolErr }}</pre>
          </div>
        </div>

        <!-- Subagent block: a runtime-spawned child's lifecycle (spawn_agent).
             Pulsing dot while running; ✓/✗ summary once done. Distinct glyph
             set from tool blocks so subagents read as agents at a glance. -->
        <div
          v-else-if="block.kind === 'subagent'"
          class="subagent-block"
          :class="{ failed: block.subDone && block.subStatus && block.subStatus !== 'success' }"
        >
          <span v-if="!block.subDone" class="subagent-dot running"></span>
          <span v-else-if="block.subStatus && block.subStatus !== 'success'" class="subagent-glyph err">✗</span>
          <span v-else class="subagent-glyph ok">✓</span>
          <span class="subagent-name">{{ block.agentName }}</span>
          <span v-if="!block.subDone" class="subagent-status">· {{ block.subModel }} · working…</span>
          <span v-else-if="block.subStatus && block.subStatus !== 'success'" class="subagent-status err">· {{ block.subStatus }}</span>
          <span v-else class="subagent-status">· {{ fmtSubTokens(block.subTokens) }} · {{ ((block.duration ?? 0) / 1000).toFixed(1) }}s</span>
        </div>

        <!-- Reasoning block: collapsible chain-of-thought. -->
        <div v-else-if="block.kind === 'reasoning'" class="reasoning-block">
          <div class="reasoning-header clickable" @click="chat.toggleCollapse(i)">
            <span class="glyph">{{ block.collapsed ? '▸' : '▾' }}</span>
            <span class="reasoning-label">
              💭 {{ block.collapsed ? 'thought' : 'thinking' }}<span v-if="block.depth && block.agentName"> · {{ block.agentName }}</span>
            </span>
            <span v-if="block.collapsed" class="reasoning-summary">
              {{ lineCount(block.text) }} line{{ lineCount(block.text) === 1 ? '' : 's' }}
            </span>
          </div>
          <div v-if="!block.collapsed" class="reasoning-text">{{ block.text }}</div>
        </div>

        <!-- User / assistant / system block. -->
        <div v-else class="block-body">
          <template v-if="block.pendingInputId">
            <div class="pending-question">
              <strong>Agent asks:</strong> {{ block.pendingQuestion }}
            </div>
            <div v-if="block.pendingDefault" class="pending-default">
              Default: <code>{{ block.pendingDefault }}</code>
            </div>
          </template>
          <template v-else>
            <template v-if="block.kind === 'user'">
              <!-- Inline editor swaps in when this user block is being edited. -->
              <template v-if="editingIdx === i">
                <textarea
                  ref="editingTextareaEl"
                  v-model="editingText"
                  class="user-edit"
                  rows="3"
                  spellcheck="false"
                  @keydown="(e: KeyboardEvent) => handleEditKeydown(e, block.turnId)"
                ></textarea>
                <div class="user-edit-actions">
                  <span class="hint">⌘/Ctrl+Enter to submit · Esc to cancel</span>
                  <button class="btn-cancel" type="button" @click="cancelEdit">Cancel</button>
                  <button
                    class="btn-submit"
                    type="button"
                    :disabled="!editingText.trim()"
                    @click="submitEdit(block.turnId)"
                  >Submit edit</button>
                </div>
              </template>
              <template v-else>
                <pre class="user-text">{{ block.text }}</pre>
                <button
                  v-if="block.turnId && !chat.isGenerating.value"
                  type="button"
                  class="msg-edit"
                  title="Edit this message (creates a new branch)"
                  @click="startEdit(i, block.text)"
                >✎</button>
                <!-- Chip is interactive. Chevrons switch the active sibling at this turn level. -->
                <span
                  v-if="(block.branchCount ?? 0) > 1"
                  class="branch-chip"
                  :title="`branch ${(block.branchIndex ?? 0) + 1} of ${block.branchCount} — click ‹ › to switch`"
                >
                  <button
                    class="chip-nav"
                    type="button"
                    :disabled="chat.isGenerating.value"
                    title="Previous branch"
                    @click.stop="switchAt(block.turnId, -1)"
                  >‹</button>
                  <span class="chip-count">{{ (block.branchIndex ?? 0) + 1 }}/{{ block.branchCount }}</span>
                  <button
                    class="chip-nav"
                    type="button"
                    :disabled="chat.isGenerating.value"
                    title="Next branch"
                    @click.stop="switchAt(block.turnId, 1)"
                  >›</button>
                </span>
              </template>
            </template>
            <div v-else class="assistant-text">
              <div class="md-body" @click="handleContentClick" v-html="renderMarkdown(block.text)"></div>
              <span v-if="block.interrupted" class="interrupted">(interrupted)</span>
              <!-- ⟳ regenerate on the last assistant block only. Creates a sibling of the user turn with the same UserText. -->
              <button
                v-if="i === lastAssistantIdx && block.turnId && !chat.isGenerating.value"
                type="button"
                class="msg-regen"
                title="Regenerate this answer (creates a new branch)"
                @click="regenerateAt(block.turnId)"
              >⟳</button>
              <button
                v-if="block.text"
                type="button"
                class="msg-copy"
                :class="{ copied: copiedIdx === i }"
                :title="copiedIdx === i ? 'Copied' : 'Copy message'"
                @click="copyBlock(block.text, i)"
              >{{ copiedIdx === i ? 'Copied' : 'Copy' }}</button>
            </div>
          </template>
        </div>
      </div>
      <div v-if="chat.blocks.value.length === 0" class="empty">
        Start the conversation.
      </div>
    </div>

    <footer class="chat-input">
      <form @submit="handleSubmit">
        <textarea
          ref="textareaEl"
          v-model="input"
          :placeholder="placeholder"
          rows="2"
          @keydown="handleKeydown"
        ></textarea>
        <div class="actions">
          <button
            v-if="chat.isGenerating.value"
            type="button"
            class="btn-stop"
            @click="stopGeneration"
          >
            Stop
          </button>
          <button type="submit" class="btn-send" :disabled="!input.trim()">
            {{ chat.pendingInput.value ? 'Answer' : 'Send' }}
          </button>
        </div>
      </form>
      <div v-if="chat.error.value" class="error">{{ chat.error.value }}</div>
    </footer>
  </div>
</template>

<style scoped>
.chat-panel {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  height: 100%;
  background: var(--surface-0);
  color: var(--text-primary);
  font-family: var(--font-sans);
}

.chat-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 16px;
  background: var(--surface-1);
  border-bottom: 1px solid var(--border-subtle);
  font-size: 12px;
}

.title {
  display: flex;
  gap: 8px;
  align-items: baseline;
}

.agent-name {
  font-weight: 600;
  color: var(--text-primary);
}

.model-tag {
  font-family: var(--font-mono);
  font-size: 10px;
  color: var(--text-muted);
  padding: 1px 6px;
  background: var(--surface-3);
  border-radius: 2px;
}

.status {
  display: flex;
  gap: 6px;
  align-items: center;
  font-family: var(--font-mono);
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--text-secondary);
}

.led {
  width: var(--led-size);
  height: var(--led-size);
  border-radius: 50%;
}
.led.on  { background: var(--status-running); box-shadow: 0 0 4px rgba(34, 197, 94, 0.5); }
.led.off { background: var(--text-muted); }

.generating {
  color: var(--status-running);
}

.messages {
  flex: 1;
  overflow-y: auto;
  padding: 16px 20px;
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.empty {
  color: var(--text-muted);
  text-align: center;
  margin-top: 32px;
  font-style: italic;
  font-size: 13px;
}

.block {
  display: flex;
  font-size: 14px;
  line-height: 1.5;
}

.block-user {
  position: sticky;
  top: 0;
  z-index: 2;
  background: var(--surface-0);
  padding-bottom: 6px;
  margin-bottom: -6px;
}

.block-user .block-body {
  border-left: 3px solid var(--accent-agent);
  padding: 4px 12px;
  background: var(--surface-1);
  border-radius: 0 4px 4px 0;
  max-width: 85%;
  margin-left: auto;
}

.block-assistant .block-body {
  max-width: 85%;
  padding: 2px 4px;
}

.block-system .block-body {
  background: var(--surface-2);
  padding: 8px 12px;
  border-radius: 4px;
  font-size: 13px;
  color: var(--text-secondary);
  border: 1px solid var(--border-subtle);
  max-width: 100%;
}

/* --- Tool-call blocks --- */
.tool-block,
.reasoning-block {
  flex: 1;
  min-width: 0;
}

.tool-header {
  display: flex;
  align-items: baseline;
  gap: 6px;
  flex-wrap: wrap;
  font-family: var(--font-mono);
  font-size: 12px;
  padding: 3px 8px;
  border-left: 2px solid var(--accent-tool, var(--accent-agent));
  background: var(--surface-1);
  border-radius: 0 3px 3px 0;
}
.tool-block.failed > .tool-header {
  border-left-color: var(--status-error, #ef4444);
}
.tool-output-err {
  color: var(--status-error, #ef4444);
}

.clickable {
  cursor: pointer;
}
.clickable:hover {
  background: var(--surface-2);
}

.glyph {
  color: var(--text-muted);
  width: 0.9em;
  flex: none;
}

.tool-name {
  font-weight: 600;
  color: var(--text-primary);
}
.tool-agent {
  color: var(--text-muted);
  font-weight: 400;
}

.tool-args {
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--text-secondary);
  background: var(--surface-2);
  padding: 0 4px;
  border-radius: 2px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 100%;
}

.tool-status {
  font-size: 11px;
  margin-left: auto;
  white-space: nowrap;
}
.tool-status.running { color: var(--status-running); }
.tool-status.ok      { color: var(--text-muted); }
.tool-status.err     { color: var(--status-error, #ef4444); }

/* --- Subagent (spawn_agent) blocks --- */
.subagent-block {
  display: flex;
  align-items: baseline;
  gap: 6px;
  flex-wrap: wrap;
  font-family: var(--font-mono);
  font-size: 12px;
  padding: 3px 8px;
  border-left: 2px solid var(--accent-agent);
  background: var(--surface-1);
  border-radius: 0 3px 3px 0;
}
.subagent-block.failed {
  border-left-color: var(--status-error, #ef4444);
}
.subagent-name {
  font-weight: 600;
  color: var(--text-primary);
}
.subagent-status {
  font-size: 11px;
  color: var(--text-muted);
}
.subagent-status.err {
  color: var(--status-error, #ef4444);
}
.subagent-glyph.ok  { color: var(--text-muted); }
.subagent-glyph.err { color: var(--status-error, #ef4444); }
.subagent-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--status-running);
  animation: subagent-pulse 1.2s ease-in-out infinite;
}
@keyframes subagent-pulse {
  0%, 100% { opacity: 0.35; }
  50%      { opacity: 1; }
}

.tool-summary {
  font-size: 11px;
  color: var(--text-muted);
  font-style: italic;
  padding: 2px 8px 2px 20px;
}

.tool-output-wrap {
  position: relative;
}

.tool-output {
  margin: 2px 0 0;
  padding: 6px 10px;
  background: var(--surface-2);
  border-radius: 3px;
  font-family: var(--font-mono);
  font-size: 12px;
  line-height: 1.45;
  color: var(--text-secondary);
  white-space: pre-wrap;
  word-break: break-word;
  max-height: 280px;
  overflow-y: auto;
}

/* --- Reasoning blocks --- */
.reasoning-header {
  display: flex;
  align-items: baseline;
  gap: 6px;
  font-size: 12px;
  color: var(--text-muted);
  padding: 2px 8px;
}
.reasoning-label {
  font-style: italic;
}
.reasoning-summary {
  font-size: 11px;
  margin-left: auto;
}
.reasoning-text {
  margin-top: 2px;
  padding: 4px 10px;
  border-left: 2px solid var(--border-subtle);
  font-size: 12px;
  line-height: 1.5;
  color: var(--text-muted);
  white-space: pre-wrap;
  word-break: break-word;
  font-style: italic;
}

.user-text {
  margin: 0;
  font-family: var(--font-sans);
  white-space: pre-wrap;
  word-break: break-word;
}

/* Chip becomes an interactive switcher. The container is still
   informational; chevron buttons inside do the work. */
.branch-chip {
  display: inline-flex;
  align-items: center;
  gap: 0.25em;
  margin-top: 0.25em;
  padding: 0 0.2em;
  border: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.18));
  border-radius: 6px;
  font-family: var(--font-mono);
  font-size: 0.75em;
  line-height: 1.4;
  opacity: 0.85;
  user-select: none;
}
.chip-count {
  padding: 0 0.1em;
}
.chip-nav {
  background: transparent;
  border: none;
  color: var(--text-secondary);
  font-family: var(--font-mono);
  font-size: 1.1em;
  line-height: 1;
  padding: 0 0.25em;
  cursor: pointer;
}
.chip-nav:hover:not(:disabled) {
  color: var(--text-primary);
}
.chip-nav:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

/* Inline edit + regenerate hover buttons. Same pattern as
   .msg-copy: absolutely positioned, opacity 0 → 1 on parent hover. */
.msg-edit {
  position: absolute;
  top: 50%;
  left: -28px;
  transform: translateY(-50%);
  background: var(--surface-3);
  border: 1px solid var(--border-subtle);
  border-radius: 50%;
  width: 22px;
  height: 22px;
  font-size: 11px;
  color: var(--text-secondary);
  cursor: pointer;
  opacity: 0;
  transition: opacity 0.12s, color 0.12s;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
}
.block-user .block-body {
  position: relative;
}
.block-user .block-body:hover .msg-edit {
  opacity: 1;
}
.msg-edit:hover {
  color: var(--accent-agent);
}

.msg-regen {
  position: absolute;
  top: 0;
  right: 56px; /* leave room for .msg-copy at right: 0 */
  background: var(--surface-3);
  border: 1px solid var(--border-subtle);
  border-radius: 3px;
  padding: 2px 6px;
  font-size: 11px;
  font-family: var(--font-mono);
  color: var(--text-secondary);
  cursor: pointer;
  opacity: 0;
  transition: opacity 0.12s, color 0.12s;
  z-index: 1;
}
.assistant-text:hover .msg-regen {
  opacity: 1;
}
.msg-regen:hover {
  color: var(--accent-agent);
}

/* Inline edit textarea + Submit/Cancel row. Sits in place of
   the user bubble while editingIdx === i. */
.user-edit {
  width: 100%;
  min-height: 56px;
  padding: 6px 8px;
  background: var(--surface-0);
  color: var(--text-primary);
  border: 1px solid var(--accent-agent);
  border-radius: 4px;
  font-family: var(--font-sans);
  font-size: 14px;
  line-height: 1.4;
  resize: vertical;
}
.user-edit:focus {
  outline: none;
}
.user-edit-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 4px;
  font-size: 11px;
}
.user-edit-actions .hint {
  flex: 1;
  font-family: var(--font-mono);
  color: var(--text-muted);
}
.user-edit-actions button {
  padding: 4px 10px;
  font-family: var(--font-sans);
  font-size: 12px;
  border: 1px solid var(--border-subtle);
  border-radius: 3px;
  cursor: pointer;
}
.btn-cancel {
  background: transparent;
  color: var(--text-secondary);
}
.btn-cancel:hover {
  color: var(--text-primary);
  background: var(--surface-2);
}
.btn-submit {
  background: var(--accent-agent);
  color: var(--surface-0);
  border-color: var(--accent-agent);
  font-weight: 600;
}
.btn-submit:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.assistant-text {
  position: relative;
  word-break: break-word;
}

.interrupted {
  color: var(--text-muted);
  font-style: italic;
  font-size: 12px;
}

/* --- Rendered Markdown (assistant messages) --- */
.md-body :deep(p) {
  margin: 0 0 8px;
}
.md-body :deep(p:last-child) {
  margin-bottom: 0;
}
.md-body :deep(ul),
.md-body :deep(ol) {
  margin: 0 0 8px;
  padding-left: 22px;
}
.md-body :deep(li) {
  margin: 2px 0;
}
.md-body :deep(h1),
.md-body :deep(h2),
.md-body :deep(h3),
.md-body :deep(h4) {
  margin: 12px 0 6px;
  line-height: 1.3;
}
.md-body :deep(h1) { font-size: 1.3em; }
.md-body :deep(h2) { font-size: 1.2em; }
.md-body :deep(h3) { font-size: 1.1em; }
.md-body :deep(h4) { font-size: 1em; }
.md-body :deep(a) {
  color: var(--accent-agent);
}
.md-body :deep(code) {
  font-family: var(--font-mono);
  font-size: 0.88em;
  background: var(--surface-2);
  padding: 1px 4px;
  border-radius: 3px;
}
.md-body :deep(blockquote) {
  margin: 8px 0;
  padding-left: 10px;
  border-left: 3px solid var(--border-subtle);
  color: var(--text-secondary);
}
.md-body :deep(hr) {
  border: none;
  border-top: 1px solid var(--border-subtle);
  margin: 10px 0;
}
.md-body :deep(table) {
  border-collapse: collapse;
  margin: 8px 0;
  font-size: 13px;
}
.md-body :deep(th),
.md-body :deep(td) {
  border: 1px solid var(--border-subtle);
  padding: 4px 8px;
}

/* Fenced code blocks — wrapped by useMarkdown's fence rule. */
.md-body :deep(.md-code) {
  position: relative;
  margin: 8px 0;
}
.md-body :deep(.md-code pre) {
  margin: 0;
  padding: 10px 12px;
  background: var(--surface-2);
  border-radius: 4px;
  overflow-x: auto;
}
.md-body :deep(.md-code pre code) {
  background: none;
  padding: 0;
  font-size: 12px;
  line-height: 1.5;
}

/* --- Copy buttons --- */
.msg-copy,
.tool-copy,
.md-body :deep(.md-copy) {
  position: absolute;
  top: 6px;
  right: 6px;
  z-index: 1;
  font-family: var(--font-mono);
  font-size: 10px;
  padding: 2px 6px;
  background: var(--surface-3);
  color: var(--text-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: 3px;
  cursor: pointer;
  opacity: 0;
  transition: opacity 0.12s;
}
.msg-copy {
  top: 0;
  right: 0;
}
.assistant-text:hover .msg-copy,
.tool-output-wrap:hover .tool-copy,
.md-body :deep(.md-code:hover .md-copy) {
  opacity: 1;
}
.msg-copy:hover,
.tool-copy:hover,
.md-body :deep(.md-copy:hover) {
  color: var(--text-primary);
}
.msg-copy.copied,
.tool-copy.copied,
.md-body :deep(.md-copy.copied) {
  color: var(--status-running);
  opacity: 1;
}

.pending-question {
  color: var(--text-primary);
}
.pending-default code {
  font-family: var(--font-mono);
  font-size: 12px;
  color: var(--text-secondary);
}

.chat-input {
  border-top: 1px solid var(--border-subtle);
  padding: 10px 16px;
  background: var(--surface-1);
}

.chat-input form {
  display: flex;
  gap: 10px;
  align-items: flex-end;
}

.chat-input textarea {
  flex: 1;
  font-family: var(--font-sans);
  font-size: 14px;
  padding: 8px 10px;
  background: var(--surface-0);
  color: var(--text-primary);
  border: 1px solid var(--border-subtle);
  border-radius: 4px;
  resize: vertical;
  min-height: 36px;
  max-height: 160px;
  line-height: 1.4;
}

.chat-input textarea:focus {
  outline: none;
  border-color: var(--accent-agent);
}

.actions {
  display: flex;
  gap: 6px;
}

.chat-input button {
  font-family: var(--font-sans);
  font-size: 13px;
  font-weight: 500;
  padding: 8px 16px;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  transition: opacity 0.15s;
}

.btn-send {
  background: var(--accent-agent);
  color: var(--surface-0);
}
.btn-send:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.btn-stop {
  background: var(--surface-3);
  color: var(--text-primary);
}

.error {
  margin-top: 6px;
  font-size: 12px;
  color: var(--status-error, #ef4444);
}
</style>
