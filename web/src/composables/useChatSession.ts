import { computed, ref, type ComputedRef, type Ref } from 'vue';
import type {
  ChatBlock,
  ChatSessionMeta,
  ClientMsg,
  ServerMsg,
  TranscriptEntry,
  TreeSnapshot,
} from '../types';

// Module-scope session registry: one state object per session id, reactive
// refs shared across every component that mounts with the same id.
//
// Why a registry? Previously this composable was a factory — each ChatPanel
// mount created fresh refs. Switching to another tab unmounted ChatPanel
// and blew away the conversation, even though the server-side session was
// still running. Moving state into module scope means tab switches leave
// the visible conversation intact; new ChatPanel mounts attach to the
// existing state and reuse the WS connection.
//
// The registry also makes multi-session trivial: each session has its own
// State object; ChatView's sidebar can switch between them by swapping the
// sessionId prop on ChatPanel, with no teardown/rebuild of state.

interface SessionState {
  blocks: Ref<ChatBlock[]>;
  isConnected: Ref<boolean>;
  isGenerating: Ref<boolean>;
  error: Ref<string | null>;
  meta: Ref<ChatSessionMeta | null>;
  pendingInput: Ref<ChatBlock | null>;
  inputHistory: Ref<string[]>; // submitted turns, for Up/Down recall
  // Branching turn tree pushed by the server on every mutation.
  // Null until the first tree_snapshot frame arrives; the Tree Viewer
  // subscribes to this. Single-branch sessions ignore it.
  tree: Ref<TreeSnapshot | null>;
  liveGenToken: { value: number }; // intentionally non-reactive
  reasoningSealed: { value: boolean }; // non-reactive; a tool call seals the open reasoning segment
  ws: { value: WebSocket | null };
  forwardEvents: { value: boolean };
  baseUrl: { value: string };
}

const sessions = new Map<string, SessionState>();

function getOrCreate(sessionId: string): SessionState {
  let s = sessions.get(sessionId);
  if (!s) {
    s = {
      blocks: ref<ChatBlock[]>([]),
      isConnected: ref(false),
      isGenerating: ref(false),
      error: ref<string | null>(null),
      meta: ref<ChatSessionMeta | null>(null),
      pendingInput: ref<ChatBlock | null>(null),
      inputHistory: ref<string[]>([]),
      tree: ref<TreeSnapshot | null>(null),
      liveGenToken: { value: 0 },
      reasoningSealed: { value: false },
      ws: { value: null },
      forwardEvents: { value: false },
      baseUrl: { value: '' },
    };
    sessions.set(sessionId, s);
  }
  return s;
}

function wsUrl(baseUrl: string, sessionId: string, events: boolean): string {
  let base = baseUrl;
  if (!base) {
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    base = `${proto}//${window.location.host}`;
  } else {
    base = base.replace(/^http/, 'ws').replace(/\/+$/, '');
  }
  const q = events ? '?events=1' : '';
  return `${base}/ws/chat/${encodeURIComponent(sessionId)}${q}`;
}

function sendOn(ws: WebSocket | null, msg: ClientMsg): boolean {
  if (!ws || ws.readyState !== WebSocket.OPEN) return false;
  ws.send(JSON.stringify(msg));
  return true;
}

function transcriptToBlock(e: TranscriptEntry): ChatBlock {
  // Common branch metadata — carried on every block regardless of kind so
  // the template can decide whether to render `‹n/m›` chips (user blocks
  // with branch_count > 1) or wire edit/regenerate buttons.
  const branch = {
    turnId: e.turn_id,
    branchIndex: e.branch_index,
    branchCount: e.branch_count,
  };
  if (e.kind === 'user_input_request') {
    return {
      kind: 'system',
      text: e.question ?? '',
      pendingInputId: e.request_id,
      pendingQuestion: e.question,
      pendingDefault: e.default,
      ...branch,
    };
  }
  if (e.kind === 'tool') {
    return {
      kind: 'tool',
      text: '',
      toolCallId: e.tool_call_id,
      toolName: e.tool_name,
      toolArgs: e.tool_args,
      toolDone: true,
      toolErr: e.tool_err,
      output: e.output,
      duration: e.duration,
      agentName: e.agent_name,
      depth: e.depth ?? 0,
      collapsed: false,
      ...branch,
    };
  }
  if (e.kind === 'reasoning') {
    // Replayed reasoning is a finished thought — show it collapsed.
    return {
      kind: 'reasoning',
      text: e.text ?? '',
      agentName: e.agent_name,
      depth: e.depth ?? 0,
      collapsed: true,
      ...branch,
    };
  }
  if (e.kind === 'subagent') {
    // Only finished children are ever recorded (see chat_session.go), so
    // replay always renders the done state.
    return {
      kind: 'subagent',
      text: '',
      agentName: e.agent_name,
      depth: e.depth ?? 0,
      subStatus: e.status,
      subTokens: e.tokens,
      subDone: true,
      duration: e.duration,
      ...branch,
    };
  }
  return {
    kind: e.kind,
    text: e.text ?? '',
    interrupted: e.interrupted,
    ...branch,
  } as ChatBlock;
}

// insertBeforeAssistant inserts a block just before the current turn's
// trailing assistant block, so streamed answer text always renders last.
// If the turn has no assistant block yet, the block is appended.
function insertBeforeAssistant(s: SessionState, block: ChatBlock) {
  const blocks = s.blocks.value;
  for (let i = blocks.length - 1; i >= 0; i--) {
    const b = blocks[i];
    if (!b) continue;
    if (b.kind === 'assistant') {
      blocks.splice(i, 0, block);
      return;
    }
    if (b.kind === 'user') break;
  }
  blocks.push(block);
}

// appendReasoning routes a streaming reasoning delta into a reasoning block.
// While a segment is open the delta is appended; once a tool call seals the
// segment, the next delta starts a fresh block so reasoning interleaves with
// tools in execution order.
function appendReasoning(s: SessionState, text: string, agentName: string | undefined, depth: number) {
  if (!s.reasoningSealed.value) {
    const blocks = s.blocks.value;
    for (let i = blocks.length - 1; i >= 0; i--) {
      const b = blocks[i];
      if (!b) continue;
      if (b.kind === 'reasoning') {
        b.text += text;
        return;
      }
      if (b.kind === 'user') break;
    }
  }
  insertBeforeAssistant(s, { kind: 'reasoning', text, agentName, depth, collapsed: false });
  s.reasoningSealed.value = false;
}

function completeToolBlock(s: SessionState, msg: ServerMsg) {
  const blocks = s.blocks.value;
  for (let i = blocks.length - 1; i >= 0; i--) {
    const b = blocks[i];
    if (!b) continue;
    if (b.kind === 'tool' && b.toolCallId === msg.tool_call_id) {
      b.toolDone = true;
      b.toolErr = msg.error;
      b.output = msg.output ?? '';
      b.duration = msg.duration ?? 0;
      return;
    }
  }
}

// completeSubagentBlock marks the matching running subagent block done. A
// spawned child's agent_end always follows its agent_start (the spawn tool
// blocks on the child's Run), so the block always exists by the time this fires.
function completeSubagentBlock(s: SessionState, msg: ServerMsg) {
  const blocks = s.blocks.value;
  for (let i = blocks.length - 1; i >= 0; i--) {
    const b = blocks[i];
    if (!b) continue;
    if (b.kind === 'subagent' && b.agentName === msg.agent_name && !b.subDone) {
      b.subDone = true;
      b.subStatus = msg.status;
      b.subTokens = msg.tokens ?? 0;
      b.duration = msg.duration ?? 0;
      return;
    }
  }
}

// closeRunningSubagents marks every still-running subagent block as done
// with the given status — called when the turn ends (interrupt or error) so
// a spawned child whose agent_end never arrived doesn't spin forever.
function closeRunningSubagents(s: SessionState, status: string) {
  for (const b of s.blocks.value) {
    if (b.kind === 'subagent' && !b.subDone) {
      b.subDone = true;
      b.subStatus = status;
    }
  }
}

// collapseOpenReasoning collapses the most recent reasoning block of the
// current turn — called when a tool call seals it.
function collapseOpenReasoning(s: SessionState) {
  const blocks = s.blocks.value;
  for (let i = blocks.length - 1; i >= 0; i--) {
    const b = blocks[i];
    if (!b) continue;
    if (b.kind === 'reasoning') {
      b.collapsed = true;
      return;
    }
    if (b.kind === 'user') return;
  }
}

// collapseAllReasoning collapses every reasoning block — called at turn end
// so a finished conversation shows one-line thought summaries.
function collapseAllReasoning(s: SessionState) {
  for (const b of s.blocks.value) {
    if (b.kind === 'reasoning') b.collapsed = true;
  }
}

function handleMessage(s: SessionState, msg: ServerMsg) {
  switch (msg.type) {
    case 'attached':
      if (msg.meta) s.meta.value = msg.meta;
      s.reasoningSealed.value = false;
      if (msg.transcript && msg.transcript.length > 0) {
        // Replay: replace blocks with the server's finalized transcript.
        // Any incomplete state (pendingInput) is recomputed from the last
        // user_input_request entry if still open.
        const replayed = msg.transcript.map(transcriptToBlock);
        s.blocks.value = replayed;
        // If the last replayed entry is a pending user_input_request with
        // an id, surface it as the current pending input — users need to
        // answer to unblock the session.
        const lastPending = [...replayed].reverse().find((b) => b.pendingInputId);
        s.pendingInput.value = lastPending ?? null;
      }
      break;

    case 'turn_started':
      s.liveGenToken.value = msg.gen_token ?? 0;
      s.isGenerating.value = true;
      s.reasoningSealed.value = false;
      // Patch the most-recent user block with the server-assigned turn id.
      // submit() and respondUserInput() push the user block optimistically
      // without a turnId (the server hasn't created the turn yet); without
      // this back-fill, only blocks loaded via the initial `attached`
      // transcript ever get turnIds, so the ✎ edit / ⟳ regenerate / Fork
      // affordances stay hidden on every new turn the user sends in-session.
      if (msg.turn_id) {
        const blocks = s.blocks.value;
        for (let i = blocks.length - 1; i >= 0; i--) {
          const b = blocks[i];
          if (b && b.kind === 'user' && !b.turnId) {
            b.turnId = msg.turn_id;
            break;
          }
        }
      }
      s.blocks.value.push({ kind: 'assistant', text: '' });
      break;

    case 'token':
      if ((msg.gen_token ?? 0) !== s.liveGenToken.value) return;
      appendToCurrentAssistant(s, msg.text ?? '');
      break;

    case 'reasoning':
      if ((msg.gen_token ?? 0) !== s.liveGenToken.value) return;
      appendReasoning(s, msg.text ?? '', msg.agent_name, msg.depth ?? 0);
      break;

    case 'tool_start':
      if ((msg.gen_token ?? 0) !== s.liveGenToken.value) return;
      insertBeforeAssistant(s, {
        kind: 'tool',
        text: '',
        toolCallId: msg.tool_call_id,
        toolName: msg.tool_name,
        toolArgs: msg.tool_args,
        toolDone: false,
        agentName: msg.agent_name,
        depth: msg.depth ?? 0,
        collapsed: false,
      });
      // A tool call seals the current reasoning segment.
      s.reasoningSealed.value = true;
      collapseOpenReasoning(s);
      break;

    case 'tool_end':
      if ((msg.gen_token ?? 0) !== s.liveGenToken.value) return;
      completeToolBlock(s, msg);
      break;

    case 'agent_start':
      if ((msg.gen_token ?? 0) !== s.liveGenToken.value) return;
      insertBeforeAssistant(s, {
        kind: 'subagent',
        text: '',
        agentName: msg.agent_name,
        depth: msg.depth ?? 0,
        subModel: msg.model,
        subDone: false,
      });
      break;

    case 'agent_end':
      if ((msg.gen_token ?? 0) !== s.liveGenToken.value) return;
      completeSubagentBlock(s, msg);
      break;

    case 'turn_done':
      if ((msg.gen_token ?? 0) !== s.liveGenToken.value) return;
      s.isGenerating.value = false;
      closeRunningSubagents(s, 'interrupted');
      if (msg.error) {
        markLastAssistantInterrupted(s);
        if (msg.error !== 'context canceled') {
          s.blocks.value.push({ kind: 'system', text: `Error: ${msg.error}` });
        }
      } else if (msg.final) {
        setLastAssistantText(s, msg.final);
      }
      // The turn is done — collapse every reasoning block so the finished
      // conversation shows one-line thought summaries.
      collapseAllReasoning(s);
      break;

    case 'user_input_request': {
      const block: ChatBlock = {
        kind: 'system',
        text: msg.question ?? '',
        pendingInputId: msg.request_id,
        pendingQuestion: msg.question,
        pendingDefault: msg.default,
      };
      s.blocks.value.push(block);
      s.pendingInput.value = block;
      break;
    }

    case 'error':
      s.error.value = msg.message ?? 'unknown error';
      break;

    case 'system':
      // Slash-command reply (e.g. /model ...) — the server handled the input
      // entirely server-side and sent back a system message instead of routing
      // it to the agent. Render as a transient system block, just like the
      // TUI does via BlockSystem. No turn lifecycle (no isGenerating change,
      // no live token coupling).
      if (msg.text) {
        s.blocks.value.push({ kind: 'system', text: msg.text });
      }
      break;

    case 'closed':
      s.isConnected.value = false;
      s.isGenerating.value = false;
      break;

    case 'transcript_resync':
      // Server applied a branch op (edit/regenerate/switch/
      // set_active_path) and the active path changed. Rebuild blocks from
      // the new flat transcript, same shape as `attached` replay.
      s.reasoningSealed.value = false;
      s.isGenerating.value = false;
      if (msg.transcript) {
        const replayed = msg.transcript.map(transcriptToBlock);
        s.blocks.value = replayed;
        const lastPending = [...replayed].reverse().find((b) => b.pendingInputId);
        s.pendingInput.value = lastPending ?? null;
      } else {
        s.blocks.value = [];
        s.pendingInput.value = null;
      }
      break;

    case 'tree_snapshot':
      // Server pushed a fresh tree shape. Single-branch sessions
      // ignore this; multi-branch UIs (inline chips, Tree
      // Viewer) subscribe via composable.tree.
      if (msg.tree) s.tree.value = msg.tree;
      break;
  }
}

function appendToCurrentAssistant(s: SessionState, chunk: string) {
  for (let i = s.blocks.value.length - 1; i >= 0; i--) {
    const b = s.blocks.value[i];
    if (b && b.kind === 'assistant') {
      b.text += chunk;
      return;
    }
  }
  s.blocks.value.push({ kind: 'assistant', text: chunk });
}

function setLastAssistantText(s: SessionState, text: string) {
  for (let i = s.blocks.value.length - 1; i >= 0; i--) {
    const b = s.blocks.value[i];
    if (b && b.kind === 'assistant') {
      b.text = text;
      return;
    }
  }
  s.blocks.value.push({ kind: 'assistant', text });
}

function markLastAssistantInterrupted(s: SessionState) {
  for (let i = s.blocks.value.length - 1; i >= 0; i--) {
    const b = s.blocks.value[i];
    if (b && b.kind === 'assistant') {
      b.interrupted = true;
      return;
    }
  }
}

// Public composable: returns a stable handle scoped to the given sessionId.
// Calling with the same id from a different component reuses the same state.
export function useChatSession(sessionId: string, baseUrl = '') {
  const s = getOrCreate(sessionId);
  if (baseUrl) s.baseUrl.value = baseUrl;

  function connect(opts: { forwardEvents?: boolean } = {}) {
    // Reuse an existing open connection — tab switches and multiple
    // ChatPanel mounts share one WS per session id.
    if (s.ws.value && s.ws.value.readyState === WebSocket.OPEN) return;
    if (s.ws.value && s.ws.value.readyState === WebSocket.CONNECTING) return;
    s.forwardEvents.value = !!opts.forwardEvents;
    const url = wsUrl(s.baseUrl.value, sessionId, s.forwardEvents.value);
    const ws = new WebSocket(url);
    s.ws.value = ws;
    // A stale socket (superseded by a later connect()) can still fire its
    // callbacks after the fact — guard every one so a delayed event from a
    // socket that's no longer current can't corrupt this session's state.
    const isCurrent = () => s.ws.value === ws;
    ws.onopen = () => {
      if (!isCurrent()) return;
      s.isConnected.value = true;
      s.error.value = null;
    };
    ws.onclose = () => {
      if (!isCurrent()) return;
      s.isConnected.value = false;
      s.isGenerating.value = false;
      s.ws.value = null;
    };
    ws.onerror = () => {
      if (!isCurrent()) return;
      s.error.value = 'connection error';
    };
    ws.onmessage = (ev) => {
      if (!isCurrent()) return;
      try {
        const msg = JSON.parse(ev.data) as ServerMsg;
        handleMessage(s, msg);
      } catch {
        /* ignore malformed frames */
      }
    };
  }

  function disconnect() {
    if (s.ws.value) {
      try { s.ws.value.close(); } catch { /* ignore */ }
      s.ws.value = null;
    }
    s.isConnected.value = false;
    s.isGenerating.value = false;
  }

  function submit(text: string) {
    if (!text.trim()) return;
    // Slash commands are dispatched entirely server-side and never reach the
    // agent — don't echo them as a user block in the chat history (matches
    // TUI behavior). The server sends back a `system` message which we render
    // as a system block via the message dispatcher above.
    if (text.trim().startsWith('/')) {
      sendOn(s.ws.value, { type: 'turn', text });
      return;
    }
    // Record for Up/Down recall, skipping consecutive duplicates.
    const hist = s.inputHistory.value;
    if (hist[hist.length - 1] !== text) hist.push(text);
    s.blocks.value.push({ kind: 'user', text });
    sendOn(s.ws.value, { type: 'turn', text });
  }

  function interrupt() {
    sendOn(s.ws.value, { type: 'interrupt' });
  }

  // toggleCollapse flips the collapsed state of a tool/reasoning block.
  function toggleCollapse(index: number) {
    const b = s.blocks.value[index];
    if (b && (b.kind === 'tool' || b.kind === 'reasoning')) {
      b.collapsed = !b.collapsed;
    }
  }

  function respondUserInput(text: string) {
    const p = s.pendingInput.value;
    if (!p || !p.pendingInputId) return;
    sendOn(s.ws.value, { type: 'user_input_response', request_id: p.pendingInputId, text });
    p.text = `Q: ${p.pendingQuestion}\nA: ${text}`;
    p.pendingInputId = undefined;
    s.blocks.value.push({ kind: 'user', text });
    s.pendingInput.value = null;
  }

  function reset() {
    s.blocks.value = [];
    s.pendingInput.value = null;
    s.error.value = null;
    s.liveGenToken.value = 0;
    s.meta.value = null;
    s.tree.value = null;
  }

  // Branch ops — each fires a WS message and lets the server's
  // subsequent transcript_resync + tree_snapshot frames drive the UI
  // update. Returns false when the WS isn't connected; the caller can
  // surface that to the user (or queue and retry).

  function editTurn(turnId: string, text: string): boolean {
    if (!text.trim()) return false;
    return sendOn(s.ws.value, { type: 'edit_turn', turn_id: turnId, text });
  }

  function regenerate(turnId: string): boolean {
    return sendOn(s.ws.value, { type: 'regenerate', turn_id: turnId });
  }

  function switchBranch(turnId: string, dir: -1 | 1): boolean {
    return sendOn(s.ws.value, { type: 'switch_branch', turn_id: turnId, dir });
  }

  function setActivePath(leafTurnId: string): boolean {
    return sendOn(s.ws.value, { type: 'set_active_path', turn_id: leafTurnId });
  }

  function requestTree(): boolean {
    return sendOn(s.ws.value, { type: 'request_tree' });
  }

  // Fork goes through REST (not WS) because it creates a NEW session that
  // the existing WS doesn't know about — the caller switches the active
  // session id after the response lands. mode defaults to 'path'
  // (ChatGPT-style); 'subtree' preserves all sibling branches.
  async function fork(opts: { turnId?: string; mode?: 'path' | 'subtree' } = {}): Promise<ChatSessionMeta> {
    const base = s.baseUrl.value || window.location.origin;
    const url = `${base.replace(/\/+$/, '')}/api/chat/${encodeURIComponent(sessionId)}/fork`;
    const body: Record<string, unknown> = {};
    if (opts.turnId) body.turn_id = opts.turnId;
    if (opts.mode) body.mode = opts.mode;
    const res = await fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (!res.ok) {
      const errText = await res.text().catch(() => `${res.status}`);
      throw new Error(`fork failed: ${errText}`);
    }
    return (await res.json()) as ChatSessionMeta;
  }

  // Caller-facing reactive handle. Expose computeds so callers can use
  // .value freely; the underlying refs are shared across mounts.
  return {
    blocks: computed(() => s.blocks.value) as ComputedRef<ChatBlock[]>,
    isConnected: computed(() => s.isConnected.value),
    isGenerating: computed(() => s.isGenerating.value),
    error: computed(() => s.error.value),
    meta: computed(() => s.meta.value),
    pendingInput: computed(() => s.pendingInput.value),
    inputHistory: computed(() => s.inputHistory.value) as ComputedRef<string[]>,
    tree: computed(() => s.tree.value),
    connect,
    disconnect,
    submit,
    interrupt,
    respondUserInput,
    toggleCollapse,
    reset,
    editTurn,
    regenerate,
    switchBranch,
    setActivePath,
    requestTree,
    fork,
  };
}

// closeChatSession fully removes the session from the registry and closes
// the WS. Call when a session is stopped server-side (or when you really
// mean to discard client state); normal tab switches should NOT call this.
export function closeChatSession(sessionId: string) {
  const s = sessions.get(sessionId);
  if (!s) return;
  if (s.ws.value) {
    try { s.ws.value.close(); } catch { /* ignore */ }
    s.ws.value = null;
  }
  sessions.delete(sessionId);
}
