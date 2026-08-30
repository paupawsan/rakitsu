import { ref } from 'vue';
import type { SessionMeta, AgentEvent } from '../types';

export function useSessionHistory(baseUrl: string = '') {
  const sessions = ref<SessionMeta[]>([]);
  const loading = ref(false);
  const error = ref<string | null>(null);
  const selectedSession = ref<SessionMeta | null>(null);
  const sessionEvents = ref<AgentEvent[]>([]);

  async function fetchSessions(projectId?: string) {
    loading.value = true;
    error.value = null;
    try {
      const params = projectId ? `?project_id=${encodeURIComponent(projectId)}` : '';
      const res = await fetch(`${baseUrl}/api/sessions${params}`);
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      sessions.value = (await res.json()) ?? [];
    } catch (e) {
      error.value = (e as Error).message;
    } finally {
      loading.value = false;
    }
  }

  async function loadSession(id: string) {
    loading.value = true;
    error.value = null;
    try {
      const [metaRes, eventsRes] = await Promise.all([
        fetch(`${baseUrl}/api/sessions/${id}`),
        fetch(`${baseUrl}/api/sessions/${id}/events`),
      ]);
      if (!metaRes.ok || !eventsRes.ok) throw new Error('Failed to load session');
      selectedSession.value = await metaRes.json();
      sessionEvents.value = (await eventsRes.json()) ?? [];
    } catch (e) {
      error.value = (e as Error).message;
    } finally {
      loading.value = false;
    }
  }

  function clearSelection() {
    selectedSession.value = null;
    sessionEvents.value = [];
  }

  async function rerunSession(id: string, debug = false): Promise<string | null> {
    loading.value = true;
    error.value = null;
    try {
      const debugParam = debug ? '?debug=true' : '';
      const res = await fetch(`${baseUrl}/api/sessions/${id}/rerun${debugParam}`, { method: 'POST' });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        error.value = (data as { error?: string }).error || `HTTP ${res.status}`;
        return null;
      }
      const data = await res.json() as { session_id: string };
      return data.session_id;
    } catch (e) {
      error.value = (e as Error).message;
      return null;
    } finally {
      loading.value = false;
    }
  }

  async function deleteSessions(ids: string[]): Promise<number> {
    try {
      const res = await fetch(`${baseUrl}/api/sessions`, {
        method: 'DELETE',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ids }),
      });
      if (!res.ok) return 0;
      const data = await res.json() as { deleted: number };
      // Remove deleted sessions from local state
      const idSet = new Set(ids);
      sessions.value = sessions.value.filter(s => !idSet.has(s.id));
      if (selectedSession.value && idSet.has(selectedSession.value.id)) {
        clearSelection();
      }
      return data.deleted;
    } catch {
      return 0;
    }
  }

  return {
    sessions,
    loading,
    error,
    selectedSession,
    sessionEvents,
    fetchSessions,
    loadSession,
    clearSelection,
    rerunSession,
    deleteSessions,
  };
}
