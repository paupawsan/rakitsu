import { computed, type Ref } from 'vue';
import type { DebugTreeNode } from '../types';
import type { Node, Edge } from '@vue-flow/core';

const LANE_H = 80;
const LANE_PAD = 30;
const NODE_W = 160;
const NODE_H = 40;
const TIME_SCALE = 0.15; // pixels per ms
const MIN_NODE_GAP = 20;

function vueFlowNodeType(type: string): string {
  if (type === 'agent') return 'debug-agent';
  if (type === 'tool_call') return 'debug-tool';
  return 'debug-generic';
}

export function useTimelineLayout(
  treeRoots: Ref<DebugTreeNode[]>,
  activeNodeIds: Ref<Set<string>>,
) {
  function buildTimelineData() {
    const nodes: Node[] = [];
    const edges: Edge[] = [];

    // Collect all leaf-ish nodes with timestamps
    const items: { node: DebugTreeNode; startMs: number; durationMs: number }[] = [];
    let minTime = Infinity;

    function collectNodes(n: DebugTreeNode) {
      if (['agent', 'tool_call', 'thought', 'reflection', 'ground_check', 'error'].includes(n.type)) {
        const ts = n.startEvent?.timestamp ? new Date(n.startEvent.timestamp).getTime() : 0;
        if (ts > 0) {
          minTime = Math.min(minTime, ts);
          items.push({ node: n, startMs: ts, durationMs: n.durationMs ?? 0 });
        }
      }
      n.children.forEach(collectNodes);
    }
    treeRoots.value.forEach(collectNodes);

    if (items.length === 0) return { nodes: [], edges: [] };

    // Assign swim lanes by agent name
    const laneMap = new Map<string, number>();
    let laneCount = 0;
    for (const item of items) {
      const key = item.node.agentName || '_system';
      if (!laneMap.has(key)) {
        laneMap.set(key, laneCount++);
      }
    }

    // Add lane label nodes
    for (const [name, lane] of laneMap) {
      nodes.push({
        id: `lane-${name}`,
        type: 'debug-generic',
        position: { x: 0, y: lane * LANE_H + LANE_PAD },
        data: {
          label: name === '_system' ? 'System' : name,
          nodeType: 'message',
          status: 'idle',
          active: false,
        },
        style: { width: '80px', height: '24px', opacity: '0.6' },
        selectable: false,
        draggable: false,
      });
    }

    // Place nodes on timeline
    const laneXTracker = new Map<string, number>();
    let prevNodeByLane = new Map<string, string>();

    for (const item of items) {
      const key = item.node.agentName || '_system';
      const lane = laneMap.get(key) ?? 0;
      const relativeMs = item.startMs - minTime;
      let x = 100 + relativeMs * TIME_SCALE;

      // Ensure no overlap within same lane
      const lastX = laneXTracker.get(key) ?? 0;
      if (x < lastX + NODE_W + MIN_NODE_GAP) {
        x = lastX + NODE_W + MIN_NODE_GAP;
      }
      laneXTracker.set(key, x);

      const y = lane * LANE_H + LANE_PAD;

      nodes.push({
        id: item.node.id,
        type: vueFlowNodeType(item.node.type),
        position: { x, y },
        data: {
          label: item.node.label,
          nodeType: item.node.type,
          status: item.node.status,
          active: activeNodeIds.value.has(item.node.id),
          tokens: item.node.tokens,
          durationMs: item.node.durationMs,
          model: (item.node.startEvent?.payload?.model as string) || '',
          agentName: item.node.agentName,
        },
        style: { width: `${NODE_W}px`, height: `${NODE_H}px` },
      });

      // Connect sequential nodes within same lane
      const prevId = prevNodeByLane.get(key);
      if (prevId) {
        edges.push({
          id: `e-${prevId}-${item.node.id}`,
          source: prevId,
          target: item.node.id,
          type: 'smoothstep',
          animated: activeNodeIds.value.has(item.node.id),
        });
      }
      prevNodeByLane.set(key, item.node.id);
    }

    return { nodes, edges };
  }

  const data = computed(() => buildTimelineData());
  const graphNodes = computed(() => data.value.nodes);
  const graphEdges = computed(() => data.value.edges);

  return { graphNodes, graphEdges };
}
