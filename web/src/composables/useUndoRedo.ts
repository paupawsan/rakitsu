import { ref, computed, type Ref } from 'vue';
import type { Node, Edge } from '@vue-flow/core';

interface Snapshot {
  nodes: string;
  edges: string;
  settings?: string;
  /** Optional auxiliary state — opaque JSON blobs keyed by name */
  aux?: Record<string, string>;
}

const MAX_STACK = 50;

/**
 * Auxiliary state provider — external state that participates in undo/redo.
 * Register via `registerAux(name, { capture, restore })`.
 */
export interface AuxStateProvider {
  capture: () => string;
  restore: (data: string) => void;
}

export function useUndoRedo(
  nodes: Ref<Node[]>,
  edges: Ref<Edge[]>,
  settings?: Ref<Record<string, unknown>>,
) {
  const undoStack = ref<Snapshot[]>([]);
  const redoStack = ref<Snapshot[]>([]);
  const auxProviders = new Map<string, AuxStateProvider>();

  const canUndo = computed(() => undoStack.value.length > 0);
  const canRedo = computed(() => redoStack.value.length > 0);

  let lastSnapshotTime = 0;

  /** Register auxiliary state that should be captured/restored with undo/redo */
  function registerAux(name: string, provider: AuxStateProvider) {
    auxProviders.set(name, provider);
  }

  /** Capture all auxiliary state */
  function captureAux(): Record<string, string> | undefined {
    if (auxProviders.size === 0) return undefined;
    const aux: Record<string, string> = {};
    for (const [name, p] of auxProviders) {
      aux[name] = p.capture();
    }
    return aux;
  }

  /** Restore all auxiliary state */
  function restoreAux(aux?: Record<string, string>) {
    if (!aux) return;
    for (const [name, data] of Object.entries(aux)) {
      const p = auxProviders.get(name);
      if (p) p.restore(data);
    }
  }

  /** Capture current state as a snapshot. Debounce: skip if < 500ms since last. */
  function saveSnapshot(force = false) {
    const now = Date.now();
    if (!force && now - lastSnapshotTime < 500) return;
    lastSnapshotTime = now;
    const snap: Snapshot = {
      nodes: JSON.stringify(nodes.value.map(n => ({
        id: n.id,
        type: n.type,
        position: { x: n.position.x, y: n.position.y },
        data: n.data,
        parentNode: n.parentNode,
        extent: n.extent,
        expandParent: n.expandParent,
        hidden: n.hidden,
        style: n.style,
        class: n.class,
      }))),
      edges: JSON.stringify(edges.value.map(e => ({
        id: e.id,
        source: e.source,
        target: e.target,
        sourceHandle: e.sourceHandle,
        targetHandle: e.targetHandle,
        label: e.label,
        animated: e.animated,
      }))),
      settings: settings ? JSON.stringify(settings.value) : undefined,
      aux: captureAux(),
    };

    undoStack.value.push(snap);
    if (undoStack.value.length > MAX_STACK) {
      undoStack.value.shift();
    }
    // New action clears redo stack
    redoStack.value = [];
  }

  /** Restore a snapshot */
  function restore(snap: Snapshot) {
    nodes.value = JSON.parse(snap.nodes);
    edges.value = JSON.parse(snap.edges);
    if (settings && snap.settings) {
      settings.value = JSON.parse(snap.settings);
    }
    restoreAux(snap.aux);
  }

  /** Create snapshot of current state */
  function captureCurrentSnapshot(): Snapshot {
    return {
      nodes: JSON.stringify(nodes.value.map(n => ({
        id: n.id, type: n.type, position: { x: n.position.x, y: n.position.y },
        data: n.data, parentNode: n.parentNode, extent: n.extent,
        expandParent: n.expandParent, hidden: n.hidden, style: n.style, class: n.class,
      }))),
      edges: JSON.stringify(edges.value.map(e => ({
        id: e.id, source: e.source, target: e.target,
        sourceHandle: e.sourceHandle, targetHandle: e.targetHandle,
        label: e.label, animated: e.animated,
      }))),
      settings: settings ? JSON.stringify(settings.value) : undefined,
      aux: captureAux(),
    };
  }

  /** Undo: pop undoStack, push current to redoStack */
  function undo() {
    if (undoStack.value.length === 0) return;
    redoStack.value.push(captureCurrentSnapshot());
    restore(undoStack.value.pop()!);
  }

  /** Redo: pop redoStack, push current to undoStack */
  function redo() {
    if (redoStack.value.length === 0) return;
    undoStack.value.push(captureCurrentSnapshot());
    const snap = redoStack.value.pop()!;
    restore(snap);
  }

  /** Clear all history */
  function clearHistory() {
    undoStack.value = [];
    redoStack.value = [];
  }

  return {
    canUndo,
    canRedo,
    saveSnapshot,
    undo,
    redo,
    clearHistory,
    registerAux,
  };
}
