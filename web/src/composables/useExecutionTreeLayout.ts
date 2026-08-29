/**
 * Shared execution tree layout engine.
 *
 * Pure functions that convert DebugTreeNode[] into VueFlow Node[]/Edge[].
 * Consumed by:
 *   - useCanvasExecutionTree.ts (builder overlay — positions relative to design nodes)
 *   - BlockDiagramStyle.vue     (standalone debugger view — positions from origin)
 */
import type { Node, Edge } from '@vue-flow/core';
import type { DebugTreeNode, DebugNodeType } from '../types';

// --- Default dimensions for first render ---
export const DEFAULT_DIMS: Record<string, { w: number; h: number }> = {
  pipeline: { w: 220, h: 48 },
  step: { w: 200, h: 44 },
  agent: { w: 200, h: 64 },
  iteration: { w: 130, h: 34 },
  thought: { w: 220, h: 64 },
  tool_call: { w: 180, h: 52 },
  reflection: { w: 180, h: 52 },
  ground_check: { w: 180, h: 52 },
  handoff: { w: 160, h: 44 },
  message: { w: 180, h: 52 },
  error: { w: 180, h: 44 },
};

export const DEFAULT_TREE_GAP = 140;
export const MAX_ITERATIONS = 5;
export const MAX_EXEC_NODES = 60;

/** Child types that should be drawn as side-branches off their parent
 *  instead of on the main trunk. Reflective/meta steps of an iteration —
 *  the "what happened inside" — branch out; handoff/message stay on trunk
 *  as they're the actual next step. */
const SIDE_BRANCH_CHILD_TYPES = new Set<DebugNodeType>([
  'thought', 'tool_call', 'reflection', 'ground_check',
]);

export interface TreeSpacing {
  treeGap: number;
  depthIndent: number;
  nodeVGap: number;
  subtreeVGap: number;
}

export const DEFAULT_SPACING: TreeSpacing = {
  treeGap: DEFAULT_TREE_GAP,
  depthIndent: 48,
  nodeVGap: 14,
  subtreeVGap: 24,
};

export interface LayoutResult {
  nodes: Node[];
  edges: Edge[];
}

// Cache of measured node dimensions (persists across rebuilds)
export const dimCache = new Map<string, { w: number; h: number }>();

/** Map DebugTreeNode type → VueFlow node type */
export function mapNodeType(dt: DebugNodeType): string {
  switch (dt) {
    case 'pipeline': return 'debug-pipeline';
    case 'step': return 'debug-step';
    case 'agent': return 'debug-agent';
    case 'iteration': return 'exec-iteration';
    case 'thought': return 'exec-thought';
    case 'tool_call': return 'debug-tool';
    default: return 'debug-generic';
  }
}

/** Map DebugTreeNode → VueFlow node data props */
export function mapNodeData(dt: DebugTreeNode): Record<string, unknown> {
  const base = {
    label: dt.label,
    status: dt.status,
    active: dt.status === 'running' || dt.status === 'paused',
    durationMs: dt.durationMs,
    _execNode: true,
    agentName: dt.agentName,
  };
  switch (dt.type) {
    case 'pipeline':
      return { ...base };
    case 'step':
      return {
        ...base,
        stepType: dt.details?.stepType ?? dt.startEvent?.payload?.step_type,
        agents: dt.details?.agents ?? dt.startEvent?.payload?.agents,
      };
    case 'agent':
      return {
        ...base,
        model: dt.startEvent?.payload?.model,
        tokens: dt.tokens,
      };
    case 'iteration':
      return { ...base, iteration: dt.iteration ?? 0 };
    case 'thought':
      return {
        ...base,
        streamingText: dt.streamingText,
        streamingReasoning: dt.streamingReasoning,
      };
    default:
      return { ...base, nodeType: dt.type };
  }
}

/** Get node dimensions: VueFlow rendered > cache > default */
export function getNodeDims(
  nodeId: string,
  nodeType: string,
  findRendered?: (id: string) => { w: number; h: number } | null,
): { w: number; h: number } {
  if (findRendered) {
    const live = findRendered(nodeId);
    if (live && live.w > 0 && live.h > 0) {
      dimCache.set(nodeId, live);
      return live;
    }
  }
  const cached = dimCache.get(nodeId);
  if (cached) return cached;
  return DEFAULT_DIMS[nodeType] ?? DEFAULT_DIMS.error!;
}

/**
 * Compute absolute position of a node by walking up the parent chain.
 */
export function getAbsolutePosition(node: Node, allNodes: Node[]): { x: number; y: number } {
  let x = node.position.x;
  let y = node.position.y;
  let parentId = node.parentNode;
  while (parentId) {
    const parent = allNodes.find(n => n.id === parentId);
    if (!parent) break;
    x += parent.position.x;
    y += parent.position.y;
    parentId = parent.parentNode;
  }
  return { x, y };
}

/** Get bounding box of design (non-exec) nodes */
export function getDesignBounds(designNodes: Node[]): { maxRight: number; minTop: number } {
  let maxRight = 0;
  let minTop = Infinity;
  for (const n of designNodes) {
    if (n.id.startsWith('exec_')) continue;
    const abs = getAbsolutePosition(n, designNodes);
    const w = (n as any).dimensions?.width ?? 220;
    maxRight = Math.max(maxRight, abs.x + w);
    minTop = Math.min(minTop, abs.y);
  }
  return {
    maxRight: maxRight || 400,
    minTop: minTop === Infinity ? 50 : minTop,
  };
}

/**
 * Pick source/target handle IDs for an edge so it enters/exits the node
 * on the side closest to the other endpoint. Matches MultiHandles.vue IDs:
 * `s-<dir>` (source) and `t-<dir>` (target) for dir ∈ top/bottom/left/right.
 *
 * If the horizontal gap between parent-center and child-center is larger
 * than half the parent width, we treat it as a side-branch (right/left);
 * otherwise it's a vertical flow (bottom/top). This keeps trivial column
 * drift from flipping trunk edges to the side.
 */
/** Map a VueFlow node type back to DebugNodeType for DEFAULT_DIMS lookup. */
function vueFlowTypeToDebugType(t: string): string {
  const m = /^(?:debug|exec)-(.+)$/.exec(t);
  const base = m ? m[1]! : t;
  if (base === 'tool') return 'tool_call';
  return base;
}

/** Quantize a 0..1 fraction to a pin index 1..3 (matches 25/50/75% in
 *  MultiHandles.vue). Values outside [0,1] clamp. */
function pinIndex(fraction: number): 1 | 2 | 3 {
  if (fraction < 0.4) return 1;
  if (fraction < 0.7) return 2;
  return 3;
}

function pickEdgeHandles(
  existingNodes: Node[],
  parentNodeId: string,
  child: { x: number; y: number; w: number; h: number },
): { sourceHandle: string; targetHandle: string } {
  const parent = existingNodes.find(n => n.id === parentNodeId);
  if (!parent) {
    return { sourceHandle: 's-bottom-2', targetHandle: 't-top-2' };
  }
  // Prefer live-measured dims from cache; fall back to DEFAULT_DIMS keyed by
  // the original DebugNodeType (prefix stripped from the VueFlow type).
  const cached = dimCache.get(parentNodeId);
  let pw: number;
  let ph: number;
  if (cached) {
    pw = cached.w;
    ph = cached.h;
  } else {
    const debugType = vueFlowTypeToDebugType(String(parent.type || ''));
    const def = DEFAULT_DIMS[debugType] ?? DEFAULT_DIMS.error!;
    pw = def.w;
    ph = def.h;
  }
  const pcx = parent.position.x + pw / 2;
  const pcy = parent.position.y + ph / 2;
  const ccx = child.x + child.w / 2;
  const ccy = child.y + child.h / 2;
  const dx = ccx - pcx;
  const dy = ccy - pcy;

  // Side-branch only when the child is clearly *beside* the parent — i.e.
  // horizontal offset dominates vertical. Just being indented (e.g. every
  // deeper-lane child is 48px right of its parent) is NOT enough; that
  // still counts as trunk flow and stays bottom→top.
  const horizontal =
    Math.abs(dx) > 40 && Math.abs(dx) > Math.abs(dy) * 1.3;

  if (horizontal) {
    // Side routing — pin index based on the Y level of the other endpoint
    // relative to this node's top. That spreads multiple branches across
    // pins (25% / 50% / 75%) instead of all sharing the middle dot.
    const srcIdx = pinIndex((ccy - parent.position.y) / ph);
    const tgtIdx = pinIndex((pcy - child.y) / child.h);
    return dx > 0
      ? {
          sourceHandle: `s-right-${srcIdx}`,
          targetHandle: `t-left-${tgtIdx}`,
        }
      : {
          sourceHandle: `s-left-${srcIdx}`,
          targetHandle: `t-right-${tgtIdx}`,
        };
  }
  // Vertical routing — pin index based on the X offset of the other endpoint
  // relative to this node's left edge. Spreads vertical edges horizontally.
  const srcIdx = pinIndex((ccx - parent.position.x) / pw);
  const tgtIdx = pinIndex((pcx - child.x) / child.w);
  return dy >= 0
    ? {
        sourceHandle: `s-bottom-${srcIdx}`,
        targetHandle: `t-top-${tgtIdx}`,
      }
    : {
        sourceHandle: `s-top-${srcIdx}`,
        targetHandle: `t-bottom-${tgtIdx}`,
      };
}

/**
 * Layout the full debug tree as a vertical call tree.
 *
 * Structural nodes (pipeline, step, agent) are always shown.
 * Agent children (iterations + their contents) are shown only when the agent is expanded.
 * Each depth level is indented horizontally; siblings stack vertically.
 */
export function layoutCallTree(
  roots: DebugTreeNode[],
  expandedNames: Set<string>,
  originX: number,
  originY: number,
  spacing: TreeSpacing,
  findRendered?: (id: string) => { w: number; h: number } | null,
): LayoutResult {
  const nodes: Node[] = [];
  const edges: Edge[] = [];
  let currentY = originY;
  let nodeCount = 0;
  const { depthIndent, nodeVGap, subtreeVGap } = spacing;

  /** Returns the max X extent used by this subtree (for parallel width tracking) */
  function layout(
    node: DebugTreeNode,
    depth: number,
    parentNodeId: string | null,
    posX?: number,
  ): { maxX: number } {
    if (nodeCount >= MAX_EXEC_NODES) return { maxX: posX ?? originX };

    const nodeId = `exec_${node.id}`;
    const x = posX ?? (originX + depth * depthIndent);
    const dims = getNodeDims(nodeId, node.type, findRendered);

    nodes.push({
      id: nodeId,
      type: mapNodeType(node.type),
      position: { x, y: currentY },
      data: { ...mapNodeData(node), hiddenBefore: 0 },
      draggable: false,
      selectable: true,
      connectable: false,
      class: 'exec-tree-node',
    });
    nodeCount++;

    if (parentNodeId) {
      const handles = pickEdgeHandles(nodes, parentNodeId, {
        x,
        y: currentY,
        w: dims.w,
        h: dims.h,
      });
      edges.push({
        id: `exec_edge_${parentNodeId}_${nodeId}`,
        source: parentNodeId,
        target: nodeId,
        sourceHandle: handles.sourceHandle,
        targetHandle: handles.targetHandle,
        type: 'rakitsu-exec',
        class: 'exec-tree-edge',
        animated: node.status === 'running',
      });
    }

    currentY += dims.h + nodeVGap;

    // Determine which children to show
    let children = node.children;

    if (node.type === 'agent') {
      if (!expandedNames.has(node.agentName)) {
        children = [];
      } else {
        const iterations = children.filter(c => c.type === 'iteration');
        const nonIterations = children.filter(c => c.type !== 'iteration');
        const visible = iterations.slice(-MAX_ITERATIONS);
        const hidden = iterations.length - visible.length;
        children = [...nonIterations, ...visible];

        // Mark hidden count on first iteration
        if (hidden > 0) {
          for (const c of children) {
            if (c.type === 'iteration') {
              (c as any)._hiddenBefore = hidden;
              break;
            }
          }
        }
      }
    }

    let maxXExtent = x + dims.w;

    // Check if this is a parallel step — lay children out side-by-side
    const isParallel = node.type === 'step' &&
      (node.details?.stepType === 'parallel' ||
       node.startEvent?.payload?.step_type === 'parallel');

    if (isParallel && children.length > 0) {
      const parallelStartY = currentY;
      let cursorX = originX + (depth + 1) * depthIndent;
      let parallelMaxY = currentY;
      const PARALLEL_H_GAP = depthIndent;

      for (const child of children) {
        currentY = parallelStartY;
        const childResult = layout(child, depth + 1, nodeId, cursorX);
        parallelMaxY = Math.max(parallelMaxY, currentY);
        cursorX = childResult.maxX + PARALLEL_H_GAP;
        maxXExtent = Math.max(maxXExtent, childResult.maxX);
      }

      currentY = parallelMaxY + subtreeVGap - nodeVGap;
    } else {
      // Sequential: stack children vertically (default).
      //
      // Exception: "reflective" children of an iteration (thought, tool_call,
      // reflection, ground_check) are laid out as side-branches — same Y as
      // the parent, offset right. They represent meta-information about what
      // happened inside the iter, not the next step on the trunk. The handle
      // picker's horizontal condition activates and draws a clean right→left
      // wire. Multiple side-branches stack below each other on the side lane
      // so they don't overlap.
      const parentX = x;
      const parentY = currentY - dims.h - nodeVGap;
      const SIDE_GAP = 80;
      let sideBranchY = parentY;
      let anySideBranch = false;

      for (const child of children) {
        const isSideBranch = SIDE_BRANCH_CHILD_TYPES.has(child.type);
        if (isSideBranch) {
          const savedCurrentY = currentY;
          currentY = sideBranchY;
          const branchX = parentX + dims.w + SIDE_GAP;
          const childResult = layout(child, depth + 1, nodeId, branchX);
          maxXExtent = Math.max(maxXExtent, childResult.maxX);
          sideBranchY = currentY;
          currentY = savedCurrentY;
          anySideBranch = true;
          continue;
        }
        // Trunk children: center their X on the parent's center so the
        // vertical wire between them has zero horizontal drift. Without
        // this, each depth level gets indented by depthIndent and every
        // trunk edge has a small kink where smoothstep bridges the offset.
        const childNodeId = `exec_${child.id}`;
        const childVfType = mapNodeType(child.type);
        const childDims = getNodeDims(childNodeId, childVfType, findRendered);
        const centeredX = parentX + dims.w / 2 - childDims.w / 2;

        if ((child as any)._hiddenBefore) {
          const prevCount = nodeCount;
          const childResult = layout(child, depth + 1, nodeId, centeredX);
          maxXExtent = Math.max(maxXExtent, childResult.maxX);
          if (nodeCount > prevCount) {
            const addedNode = nodes.find(n => n.id === childNodeId);
            if (addedNode) {
              (addedNode.data as any).hiddenBefore = (child as any)._hiddenBefore;
            }
          }
          delete (child as any)._hiddenBefore;
        } else {
          const childResult = layout(child, depth + 1, nodeId, centeredX);
          maxXExtent = Math.max(maxXExtent, childResult.maxX);
        }
      }

      // Ensure the trunk continues below the lowest side-branch so that
      // the next sibling doesn't visually overlap a stacked branch lane.
      if (anySideBranch && sideBranchY > currentY) {
        currentY = sideBranchY;
      }

      if (children.length > 0) {
        currentY += subtreeVGap - nodeVGap;
      }
    }

    return { maxX: maxXExtent };
  }

  for (const root of roots) {
    layout(root, 0, null);
    currentY += subtreeVGap;
  }

  return { nodes, edges };
}
