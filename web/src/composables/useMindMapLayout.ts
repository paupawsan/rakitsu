import { computed, type Ref } from 'vue';
import type { DebugTreeNode } from '../types';
import type { Node, Edge } from '@vue-flow/core';
import dagre from '@dagrejs/dagre';

const NODE_W = 180;
const NODE_H = 50;

function vueFlowNodeType(type: string): string {
  if (type === 'pipeline') return 'debug-pipeline';
  if (type === 'step') return 'debug-step';
  if (type === 'agent') return 'debug-agent';
  if (type === 'tool_call') return 'debug-tool';
  return 'debug-generic';
}

function nodeData(node: DebugTreeNode, activeIds: Set<string>) {
  return {
    label: node.label,
    nodeType: node.type,
    status: node.status,
    active: activeIds.has(node.id),
    tokens: node.tokens,
    durationMs: node.durationMs,
    model: (node.startEvent?.payload?.model as string) || '',
    agentName: node.agentName,
    stepType: (node.details?.stepType as string) || '',
    agents: (node.details?.agents as string[]) || [],
  };
}

// Types to show in the mind map (skip iterations for cleaner layout)
const MIND_MAP_TYPES = ['pipeline', 'step', 'agent', 'tool_call', 'thought', 'reflection', 'ground_check', 'handoff', 'error'];

export function useMindMapLayout(
  treeRoots: Ref<DebugTreeNode[]>,
  activeNodeIds: Ref<Set<string>>,
) {
  function buildMindMapData() {
    const nodes: Node[] = [];
    const edges: Edge[] = [];

    const g = new dagre.graphlib.Graph();
    g.setGraph({ rankdir: 'TB', ranksep: 80, nodesep: 60, marginx: 40, marginy: 40 });
    g.setDefaultEdgeLabel(() => ({}));

    // Walk tree, add nodes/edges to dagre (skip iterations, show their children directly)
    function addNode(n: DebugTreeNode, parentId?: string) {
      if (!MIND_MAP_TYPES.includes(n.type)) {
        // Skip this node type (e.g., iteration) but process children
        for (const child of n.children) {
          addNode(child, parentId);
        }
        return;
      }

      g.setNode(n.id, { width: NODE_W, height: NODE_H });
      nodes.push({
        id: n.id,
        type: vueFlowNodeType(n.type),
        position: { x: 0, y: 0 }, // dagre will set this
        data: nodeData(n, activeNodeIds.value),
        style: { width: `${NODE_W}px`, height: `${NODE_H}px` },
      });

      if (parentId) {
        g.setEdge(parentId, n.id);
        edges.push({
          id: `e-${parentId}-${n.id}`,
          source: parentId,
          target: n.id,
          type: 'default',
          animated: activeNodeIds.value.has(n.id),
        });
      }

      for (const child of n.children) {
        addNode(child, n.id);
      }
    }

    for (const root of treeRoots.value) {
      addNode(root);
    }

    if (nodes.length === 0) return { nodes: [], edges: [] };

    // Run dagre layout
    dagre.layout(g);

    // Apply computed positions
    for (const node of nodes) {
      const pos = g.node(node.id);
      if (pos) {
        node.position = { x: pos.x - NODE_W / 2, y: pos.y - NODE_H / 2 };
      }
    }

    return { nodes, edges };
  }

  const data = computed(() => buildMindMapData());
  const graphNodes = computed(() => data.value.nodes);
  const graphEdges = computed(() => data.value.edges);

  return { graphNodes, graphEdges };
}
