import { onScopeDispose, ref } from 'vue';

// Runtime session picker composable (Phase 1 of multi-session debug).
//
// Lists live executions — chat sessions and one-shot runs — from the backend
// registry at /api/runtime/sessions. Polls on an interval so the dropdown
// stays fresh without needing push. Read-only: no attach logic here, that
// lands separately.

export type SessionMode = 'oneshot' | 'chat';
export type SessionStatus = 'pending' | 'running' | 'paused' | 'stopped' | 'errored';

export interface RuntimeSessionMeta {
  id: string;
  config_id: string;
  name?: string;
  mode: SessionMode;
  status: SessionStatus;
  started_at?: string;
  ended_at?: string;
  agent?: string;
  model?: string;
}

export interface UseSessionListOptions {
  pollMs?: number; // default 2000
  baseUrl?: string;
}

export function useSessionList(opts: UseSessionListOptions = {}) {
  const pollMs = opts.pollMs ?? 2000;
  const baseUrl = opts.baseUrl ?? '';
  const sessions = ref<RuntimeSessionMeta[]>([]);
  const loading = ref(false);
  const error = ref<string | null>(null);

  let timer: ReturnType<typeof setInterval> | null = null;

  async function refresh() {
    loading.value = true;
    try {
      const res = await fetch(`${baseUrl}/api/runtime/sessions`);
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const list = (await res.json()) as RuntimeSessionMeta[];
      sessions.value = Array.isArray(list) ? list : [];
      error.value = null;
    } catch (e) {
      error.value = (e as Error).message;
    } finally {
      loading.value = false;
    }
  }

  function start() {
    if (timer) return;
    refresh();
    timer = setInterval(refresh, pollMs);
  }

  function stop() {
    if (timer) {
      clearInterval(timer);
      timer = null;
    }
  }

  // Auto-cleanup when the consuming component unmounts.
  onScopeDispose(() => stop());

  return { sessions, loading, error, refresh, start, stop };
}
