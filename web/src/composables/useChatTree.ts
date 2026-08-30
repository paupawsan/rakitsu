// useChatTree — converts a session's TreeSnapshot (pushed from the server
// on every branch op) into Vue Flow nodes/edges with a dagre layout, and
// owns the local "selected turn" state the Tree Viewer's preview pane
// reads. Adapter, not state owner: the canonical tree lives in
// useChatSession; this composable only reshapes it for the canvas.

import { computed, ref, type ComputedRef, type Ref } from 'vue';
import dagre from '@dagrejs/dagre';
import type { Edge, Node } from '@vue-flow/core';
import type { TreeSnapshot } from '../types';

const NODE_W = 280;
const NODE_H = 120;

interface BranchTreeNodeData {
  id: string;
  shortId: string;
  userText: string;
  status: 'generating' | 'complete' | 'interrupted' | 'failed';
  created: string;
  entryCount: number;
  retrySource?: string;
  retryReason?: string;
  isActive: boolean;
}

interface LayoutResult {
  nodes: Node<BranchTreeNodeData>[];
  edges: Edge[];
}

function shortenId(id: string): string {
  // UUIDs are long; render the first segment so cards stay readable.
  if (id.length <= 8) return id;
  const dash = id.indexOf('-');
  if (dash > 0 && dash <= 8) return id.slice(0, dash);
  return id.slice(0, 8);
}

function buildLayout(snap: TreeSnapshot | null): LayoutResult {
  if (!snap || snap.nodes.length === 0) {
    return { nodes: [], edges: [] };
  }

  const activeSet = new Set(snap.active_path);
  const g = new dagre.graphlib.Graph();
  g.setGraph({ rankdir: 'TB', ranksep: 70, nodesep: 50, marginx: 40, marginy: 40 });
  g.setDefaultEdgeLabel(() => ({}));

  const nodes: Node<BranchTreeNodeData>[] = [];
  const edges: Edge[] = [];

  for (const n of snap.nodes) {
    g.setNode(n.id, { width: NODE_W, height: NODE_H });
    nodes.push({
      id: n.id,
      type: 'branch-tree',
      position: { x: 0, y: 0 }, // dagre fills this in
      data: {
        id: n.id,
        shortId: shortenId(n.id),
        userText: n.user_text ?? '',
        status: n.status,
        created: n.created,
        entryCount: n.entry_count,
        retrySource: n.retry_ctx?.source,
        retryReason: n.retry_ctx?.reason,
        isActive: activeSet.has(n.id),
      },
      style: { width: `${NODE_W}px`, height: `${NODE_H}px` },
    });

    if (n.parent_id) {
      // Active-path edges glow + animate; dead-branch edges stay subtle.
      const onActivePath = activeSet.has(n.parent_id) && activeSet.has(n.id);
      edges.push({
        id: `e-${n.parent_id}-${n.id}`,
        source: n.parent_id,
        target: n.id,
        type: 'default',
        animated: onActivePath,
        style: onActivePath
          ? { stroke: 'var(--accent, #4f8cff)', strokeWidth: 2 }
          : { stroke: 'var(--border-subtle, rgba(255,255,255,0.18))', strokeWidth: 1 },
      });
      g.setEdge(n.parent_id, n.id);
    }
  }

  dagre.layout(g);
  for (const node of nodes) {
    const pos = g.node(node.id);
    if (pos) {
      node.position = { x: pos.x - NODE_W / 2, y: pos.y - NODE_H / 2 };
    }
  }

  return { nodes, edges };
}

export interface UseChatTreeHandle {
  flowNodes: ComputedRef<Node<BranchTreeNodeData>[]>;
  flowEdges: ComputedRef<Edge[]>;
  selectedTurnId: Ref<string | null>;
  selectNode: (id: string | null) => void;
  hasTree: ComputedRef<boolean>;
  activeLeafId: ComputedRef<string | null>;
}

// useChatTree wraps a session's reactive tree snapshot and exposes the
// Vue Flow-shaped derivatives plus a selection model. The caller passes
// in the tree ref from useChatSession; this composable is intentionally
// decoupled from WS transport so it can be unit-tested with a static
// snapshot.
export function useChatTree(tree: Ref<TreeSnapshot | null>): UseChatTreeHandle {
  const selectedTurnId = ref<string | null>(null);

  const layout = computed(() => buildLayout(tree.value));
  const flowNodes = computed(() => layout.value.nodes);
  const flowEdges = computed(() => layout.value.edges);
  const hasTree = computed(() => (tree.value?.nodes.length ?? 0) > 0);
  const activeLeafId = computed(() => {
    const ap = tree.value?.active_path;
    if (!ap || ap.length === 0) return null;
    return ap[ap.length - 1] ?? null;
  });

  function selectNode(id: string | null) {
    selectedTurnId.value = id;
  }

  return { flowNodes, flowEdges, selectedTurnId, selectNode, hasTree, activeLeafId };
}

// Re-export the node data shape so BranchTreeNode.vue can type its
// defineProps without re-declaring fields. Tagged as an export for
// other components that want to render a branch summary outside the
// canvas (e.g. a future "compare two branches" view).
export type { BranchTreeNodeData };
