import { ref, watch, type Ref } from 'vue';
import type { AgentEvent, DebugPausedPayload } from '../types';

export type DebugState = 'running' | 'paused' | 'stepping';
export type DebugMode = 'pipeline' | 'agent' | 'tool' | 'custom';

export interface Breakpoint {
  event_type: string;
  agent_name: string;
}

export interface ParamOverride {
  temperature?: number;
  max_tokens?: number;
  model?: string;
  top_p?: number;
  system_prompt?: string;
  max_iterations?: number;
  tools?: string[];
  sticky: boolean;
}

const BP_STORAGE_KEY = 'rakitsu-debug-breakpoints';

function loadPersistedBreakpoints(): Breakpoint[] {
  try {
    const raw = localStorage.getItem(BP_STORAGE_KEY);
    return raw ? JSON.parse(raw) : [];
  } catch {
    return [];
  }
}

function persistBreakpoints(bps: Breakpoint[]) {
  try {
    localStorage.setItem(BP_STORAGE_KEY, JSON.stringify(bps));
  } catch {
    // localStorage may be unavailable
  }
}

// ── Singleton shared state ──────────────────────────────────────────────────
const debugState = ref<DebugState>('running');
const breakpoints = ref<Breakpoint[]>(loadPersistedBreakpoints());
const isPaused = ref(false);
const pausedAgent = ref<string | null>(null);
const pausedCheckpoint = ref<string | null>(null);
const pausedContext = ref<DebugPausedPayload | null>(null);
const debugMode = ref<DebugMode>('custom');
const toolFilter = ref<string[]>([]);

// Pending user_input wait — populated from USER_INPUT_PENDING events on the
// SSE stream. When non-null, the agent is blocked and the BreakpointBar
// renders a prompt input so the operator can unblock it.
export interface PendingUserInput {
  sessionId: string;
  requestId: string;
  agentName: string;
  question: string;
  default?: string;
}
const pendingUserInput = ref<PendingUserInput | null>(null);

// Auto-persist breakpoints to localStorage
watch(breakpoints, (bps) => persistBreakpoints(bps), { deep: true });

// ── Public composable ───────────────────────────────────────────────────────
// Multi-session debug: `pinnedSessionId` (when provided) is threaded
// through as `session_id` on every debug API call so the server routes to the
// right per-session DebugController. When null/undefined the server falls
// back to the legacy single-controller path.
export function useDebugControl(
  baseUrl: Ref<string>,
  hubSessionId?: Ref<string | null>,
  pinnedSessionId?: Ref<string | null>,
) {

  function apiUrl(path: string): string {
    const base = baseUrl.value.startsWith('http') ? baseUrl.value : `http://${baseUrl.value}`;
    return `${base.replace(/\/+$/, '')}${path}`;
  }

  function pinnedId(): string | null {
    return pinnedSessionId?.value ?? null;
  }

  function withSession<T extends Record<string, unknown>>(body: T): T & { session_id?: string } {
    const sid = pinnedId();
    return sid ? { ...body, session_id: sid } : body;
  }

  function appendSession(path: string): string {
    const sid = pinnedId();
    if (!sid) return path;
    const sep = path.includes('?') ? '&' : '?';
    return `${path}${sep}session_id=${encodeURIComponent(sid)}`;
  }

  /** Send a debug command via hub (queued for CLI polling) */
  async function hubCommand(action: string, data: Record<string, unknown> = {}) {
    await fetch(apiUrl('/api/hub/debug'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ session_id: hubSessionId?.value, action, data }),
    });
  }

  function isHubMode(): boolean {
    return !!(hubSessionId?.value);
  }

  async function fetchState() {
    try {
      const res = await fetch(apiUrl(appendSession('/api/debug/state')));
      const data = await res.json();
      debugState.value = data.state ?? 'running';
      isPaused.value = debugState.value === 'paused';

      // Merge server breakpoints with locally persisted ones
      const serverBps: Breakpoint[] = data.breakpoints ?? [];
      const localBps = loadPersistedBreakpoints();

      for (const local of localBps) {
        const onServer = serverBps.some(
          (s) => s.event_type === local.event_type && s.agent_name === local.agent_name,
        );
        if (!onServer) {
          // Re-apply to server
          if (isHubMode()) {
            hubCommand('set_breakpoint', { event_type: local.event_type, agent_name: local.agent_name }).catch(() => {});
          } else {
            fetch(apiUrl('/api/debug/breakpoints'), {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify(withSession({ action: 'set', event_type: local.event_type, agent_name: local.agent_name })),
            }).catch(() => {});
          }
          serverBps.push(local);
        }
      }

      breakpoints.value = serverBps;
    } catch {
      // Server may not have debug controller active — use local breakpoints
      breakpoints.value = loadPersistedBreakpoints();
    }
  }

  async function setBreakpoint(eventType: string, agentName: string = '*') {
    // Optimistic local update (persisted via watcher)
    const exists = breakpoints.value.some(
      (b) => b.event_type === eventType && b.agent_name === agentName,
    );
    if (!exists) {
      breakpoints.value = [...breakpoints.value, { event_type: eventType, agent_name: agentName }];
    }
    // Sync to server (best-effort)
    try {
      if (isHubMode()) {
        await hubCommand('set_breakpoint', { event_type: eventType, agent_name: agentName });
      } else {
        await fetch(apiUrl('/api/debug/breakpoints'), {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(withSession({ action: 'set', event_type: eventType, agent_name: agentName })),
        });
      }
    } catch {
      // Server unavailable — local state is authoritative
    }
  }

  async function clearBreakpoint(eventType: string, agentName: string = '*') {
    // Optimistic local update
    breakpoints.value = breakpoints.value.filter(
      (b) => !(b.event_type === eventType && b.agent_name === agentName),
    );
    // Sync to server (best-effort)
    try {
      if (isHubMode()) {
        await hubCommand('clear_breakpoint', { event_type: eventType, agent_name: agentName });
      } else {
        await fetch(apiUrl('/api/debug/breakpoints'), {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(withSession({ action: 'clear', event_type: eventType, agent_name: agentName })),
        });
      }
    } catch {
      // Server unavailable — local state is authoritative
    }
  }

  async function clearAllBreakpoints() {
    breakpoints.value = [];
    try {
      if (isHubMode()) {
        await hubCommand('clear_all_breakpoints');
      } else {
        await fetch(apiUrl('/api/debug/breakpoints'), {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(withSession({ action: 'clear-all' })),
        });
      }
    } catch {
      // Server unavailable — local state is authoritative
    }
  }

  type ResumeAction = 'resume' | 'step' | 'step_in' | 'step_out' | 'run_until' | 'stop';

  async function resume(action: ResumeAction = 'resume', extra?: Record<string, unknown>) {
    try {
      const data: Record<string, unknown> = { action, ...extra };
      let ok = false;
      if (isHubMode()) {
        await hubCommand('resume', data);
        ok = true; // hub mode is fire-and-forget via queued commands
      } else {
        const res = await fetch(apiUrl('/api/debug/resume'), {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(withSession(data)),
        });
        ok = res.ok;
      }
      if (ok) {
        isPaused.value = false;
        pausedAgent.value = null;
        pausedCheckpoint.value = null;
        debugState.value = action === 'step' ? 'stepping' : 'running';
      } else {
        console.error('[debug] resume failed — server returned error');
      }
    } catch (e) {
      console.error('[debug] resume error:', e);
    }
  }

  async function stepIn() {
    await resume('step_in');
  }

  async function stepOut() {
    await resume('step_out');
  }

  async function runUntil(condition: { checkpoint?: string; agent_name?: string; tool_name?: string; iteration?: number }) {
    await resume('run_until', condition);
  }

  async function setParams(agent: string, overrides: ParamOverride) {
    try {
      if (isHubMode()) {
        await hubCommand('set_params', { agent, ...overrides });
      } else {
        await fetch(apiUrl('/api/debug/params'), {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(withSession({ agent, ...overrides })),
        });
      }
    } catch {
      // ignore
    }
  }

  async function clearParams(agent: string) {
    try {
      if (isHubMode()) {
        await hubCommand('clear_params', { agent });
      } else {
        await fetch(apiUrl(appendSession(`/api/debug/params?agent=${encodeURIComponent(agent)}`)), {
          method: 'DELETE',
        });
      }
    } catch {
      // ignore
    }
  }

  async function replay(sessionId: string, eventIndex: number) {
    try {
      const res = await fetch(apiUrl('/api/debug/replay'), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ session_id: sessionId, event_index: eventIndex }),
      });
      return await res.json();
    } catch {
      return null;
    }
  }

  async function rerunFromStep(stepName: string, overrides: Record<string, unknown> = {}) {
    try {
      if (isHubMode()) {
        await hubCommand('rerun_from_step', { step_name: stepName, overrides });
      } else {
        await fetch(apiUrl('/api/debug/rerun'), {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(withSession({ step_name: stepName, overrides })),
        });
      }
    } catch {
      // ignore
    }
  }

  async function requestPause() {
    try {
      if (isHubMode()) {
        await hubCommand('pause');
      } else {
        await fetch(apiUrl(appendSession('/api/debug/pause')), { method: 'POST' });
      }
    } catch {
      // ignore
    }
  }

  async function setMode(mode: DebugMode, tools?: string[]) {
    await clearAllBreakpoints();
    debugMode.value = mode;
    switch (mode) {
      case 'pipeline':
        await setBreakpoint('pre_agent');
        await setBreakpoint('pre_step');
        break;
      case 'agent':
        await setBreakpoint('pre_thought');
        await setBreakpoint('pre_tool');
        break;
      case 'tool':
        toolFilter.value = tools || toolFilter.value;
        // For tool mode, set pre_tool breakpoints (tool name filtering is future work)
        await setBreakpoint('pre_tool');
        break;
      case 'custom':
        // No auto-set — user manages breakpoints manually
        break;
    }
  }

  // Handle DEBUG_PAUSED / DEBUG_RESUMED / USER_INPUT_* events from SSE stream
  function handleEvent(event: AgentEvent) {
    if (event.event_type === 'DEBUG_PAUSED') {
      debugState.value = 'paused';
      isPaused.value = true;
      pausedAgent.value = (event.payload.agent_name as string) ?? event.agent_name;
      pausedCheckpoint.value = (event.payload.checkpoint as string) ?? null;
      pausedContext.value = event.payload as unknown as DebugPausedPayload;
    } else if (event.event_type === 'DEBUG_RESUMED') {
      const action = (event.payload.action as string) || 'resume';
      debugState.value = action === 'step' ? 'stepping' : 'running';
      isPaused.value = false;
      pausedAgent.value = null;
      pausedCheckpoint.value = null;
      pausedContext.value = null;
    } else if (event.event_type === 'USER_INPUT_PENDING') {
      pendingUserInput.value = {
        sessionId: event.session_id ?? '',
        requestId: (event.payload.request_id as string) ?? '',
        agentName: (event.payload.agent_name as string) ?? event.agent_name,
        question: (event.payload.question as string) ?? '',
        default: (event.payload.default as string) ?? undefined,
      };
    } else if (event.event_type === 'USER_INPUT_ANSWERED') {
      const rid = (event.payload.request_id as string) ?? '';
      if (pendingUserInput.value && (!rid || pendingUserInput.value.requestId === rid)) {
        pendingUserInput.value = null;
      }
    }
  }

  // Inject an answer to the currently pending user_input wait. Clears the
  // local state optimistically — the USER_INPUT_ANSWERED event will confirm.
  async function answerUserInput(text: string): Promise<boolean> {
    const p = pendingUserInput.value;
    if (!p) return false;
    try {
      const res = await fetch(apiUrl('/api/debug/user_input'), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          session_id: p.sessionId,
          request_id: p.requestId,
          text,
        }),
      });
      if (!res.ok) return false;
      pendingUserInput.value = null;
      return true;
    } catch {
      return false;
    }
  }

  function resetDebugState() {
    isPaused.value = false;
    pausedAgent.value = null;
    pausedCheckpoint.value = null;
    pausedContext.value = null;
    debugState.value = 'running';
  }

  return {
    debugState,
    breakpoints,
    isPaused,
    pausedAgent,
    pausedCheckpoint,
    pausedContext,
    debugMode,
    toolFilter,
    pendingUserInput,
    fetchState,
    setBreakpoint,
    clearBreakpoint,
    clearAllBreakpoints,
    resume,
    stepIn,
    stepOut,
    runUntil,
    requestPause,
    setMode,
    setParams,
    clearParams,
    replay,
    rerunFromStep,
    resetDebugState,
    handleEvent,
    answerUserInput,
  };
}
