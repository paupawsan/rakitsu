import { ref, computed, watch, type Ref } from 'vue';
import type { DebugTreeNode, SessionMeta, AgentEvent } from '../types';

const MAX_DISPLAY = 20;

export interface RunSnapshot {
  id: string;           // session ID from backend
  sessionId: string;    // same — for clarity
  timestamp: string;
  query: string;
  debug: boolean;
  treeRoots: DebugTreeNode[];  // loaded on demand, empty until selected
  loaded: boolean;              // whether treeRoots has been fetched
  summary: {
    totalIterations: number;
    totalToolCalls: number;
    totalTokens: number;
    durationMs: number;
    status: 'success' | 'error';
    agentSummaries: { name: string; iterations: number; tools: number; tokens: number }[];
  };
}

function sessionToSnapshot(s: SessionMeta): RunSnapshot {
  return {
    id: s.id,
    sessionId: s.id,
    timestamp: s.start_time,
    query: s.query,
    debug: false,  // backend doesn't store this yet
    treeRoots: [],
    loaded: false,
    summary: {
      totalIterations: 0,
      totalToolCalls: 0,
      totalTokens: s.total_tokens,
      durationMs: s.duration_ms,
      status: (s.status === 'error' || s.status === 'timeout') ? 'error' : 'success',
      agentSummaries: (s.agents ?? []).map(name => ({ name, iterations: 0, tools: 0, tokens: 0 })),
    },
  };
}

function deepCloneTree(nodes: DebugTreeNode[]): DebugTreeNode[] {
  return nodes.map(n => ({
    ...n,
    startEvent: { ...n.startEvent },
    endEvent: n.endEvent ? { ...n.endEvent } : undefined,
    tokens: n.tokens ? { ...n.tokens } : undefined,
    details: n.details ? { ...n.details } : undefined,
    children: deepCloneTree(n.children),
  }));
}

function summarizeTree(roots: DebugTreeNode[]): RunSnapshot['summary'] {
  let totalIterations = 0;
  let totalToolCalls = 0;
  let totalTokens = 0;
  let durationMs = 0;
  let status: 'success' | 'error' = 'success';
  const agentMap = new Map<string, { iterations: number; tools: number; tokens: number }>();

  function walk(node: DebugTreeNode) {
    if (node.type === 'iteration') totalIterations++;
    if (node.type === 'tool_call') totalToolCalls++;
    if (node.tokens?.total_tokens) totalTokens += node.tokens.total_tokens;
    if (node.durationMs && node.type === 'pipeline') durationMs = Math.max(durationMs, node.durationMs);
    if (node.status === 'error') status = 'error';

    if (node.type === 'agent') {
      const entry = agentMap.get(node.agentName) ?? { iterations: 0, tools: 0, tokens: 0 };
      for (const c of node.children) {
        if (c.type === 'iteration') entry.iterations++;
        for (const gc of c.children) {
          if (gc.type === 'tool_call') entry.tools++;
        }
      }
      if (node.tokens?.total_tokens) entry.tokens += node.tokens.total_tokens;
      if (node.durationMs) durationMs = Math.max(durationMs, node.durationMs);
      agentMap.set(node.agentName, entry);
    }

    for (const c of node.children) walk(c);
  }

  for (const r of roots) walk(r);

  return {
    totalIterations,
    totalToolCalls,
    totalTokens,
    durationMs,
    status,
    agentSummaries: [...agentMap.entries()].map(([name, s]) => ({ name, ...s })),
  };
}

/**
 * Run snapshots backed by backend sessions.
 * Fetches sessions for the project on init and project change.
 * Tree data loaded on demand when a snapshot is selected.
 */
export function useRunSnapshots(projectId: Ref<string>) {
  const snapshots = ref<RunSnapshot[]>([]);
  const selectedSnapshotId = ref<string | null>(null);
  const hiddenIds = ref<Set<string>>(new Set());  // "cleared" sessions — hidden until reload

  const visibleSnapshots = computed(() =>
    snapshots.value.filter(s => !hiddenIds.value.has(s.id))
  );

  const selectedSnapshot = computed(() =>
    snapshots.value.find(s => s.id === selectedSnapshotId.value) ?? null
  );

  /** Fetch sessions for current project from backend */
  async function fetchForProject(pid?: string) {
    const id = pid ?? projectId.value;
    if (!id) return;
    try {
      const params = `?project_id=${encodeURIComponent(id)}`;
      const res = await fetch(`/api/sessions${params}`);
      if (!res.ok) return;
      const sessions: SessionMeta[] = (await res.json()) ?? [];
      // Only show completed sessions, most recent last, cap at MAX_DISPLAY
      const completed = sessions
        .filter(s => s.status === 'success' || s.status === 'error' || s.status === 'timeout')
        .reverse()
        .slice(-MAX_DISPLAY);
      // Preserve any already-loaded tree data
      const existing = new Map(snapshots.value.map(s => [s.id, s]));
      snapshots.value = completed.map(s => existing.get(s.id) ?? sessionToSnapshot(s));
      hiddenIds.value = new Set();
    } catch { /* ignore fetch errors */ }
  }

  /** Load tree data for a snapshot by fetching session events */
  async function loadTreeForSnapshot(snapId: string): Promise<DebugTreeNode[]> {
    try {
      const res = await fetch(`/api/sessions/${snapId}/events`);
      if (!res.ok) return [];
      const events: AgentEvent[] = (await res.json()) ?? [];
      // Build tree using the same logic as useDebugTree, but standalone
      const { buildTreeFromEvents } = await import('./debugTreeBuilder');
      return buildTreeFromEvents(events);
    } catch {
      return [];
    }
  }

  // Fetch on init and when project changes
  fetchForProject();
  watch(projectId, (newId) => {
    selectedSnapshotId.value = null;
    fetchForProject(newId);
  });

  /**
   * Capture: called after a run completes. Creates snapshot from live tree data
   * and refreshes from backend to pick up the new session.
   */
  function capture(treeRoots: DebugTreeNode[], query: string, debug: boolean): RunSnapshot {
    const cloned = deepCloneTree(treeRoots);
    const snap: RunSnapshot = {
      id: `pending_${Date.now()}`,
      sessionId: '',
      timestamp: new Date().toISOString(),
      query,
      debug,
      treeRoots: cloned,
      loaded: true,
      summary: summarizeTree(cloned),
    };
    snapshots.value = [...snapshots.value, snap];
    // Refresh from backend after a short delay to pick up the real session
    setTimeout(() => fetchForProject(), 1000);
    return snap;
  }

  function select(id: string | null) {
    selectedSnapshotId.value = id;
  }

  /** Hide from view (not a backend delete) */
  function remove(id: string) {
    hiddenIds.value = new Set([...hiddenIds.value, id]);
    if (selectedSnapshotId.value === id) selectedSnapshotId.value = null;
  }

  /** Hide all from view (not a backend delete). Reload restores them. */
  function clear() {
    hiddenIds.value = new Set(snapshots.value.map(s => s.id));
    selectedSnapshotId.value = null;
  }

  return {
    snapshots: visibleSnapshots,
    selectedSnapshotId,
    selectedSnapshot,
    capture,
    select,
    remove,
    clear,
    loadTreeForSnapshot,
    fetchForProject,
  };
}
