import { type Ref, nextTick } from 'vue';
import type { Node } from '@vue-flow/core';

// Minimum fallback dimensions by type (used when no rendered size available)
const MIN_NODE_SIZE: Record<string, { w: number; h: number }> = {
  agent: { w: 200, h: 120 },
  tool: { w: 200, h: 100 },
  skill: { w: 200, h: 100 },
  orchestrator: { w: 200, h: 160 },
  group: { w: 250, h: 120 },
};

const PADDING = { top: 50, right: 40, bottom: 20, left: 40 };
const MIN_WIDTH = 250;
const MIN_HEIGHT = 120;

export function useGroupAutoFit(
  nodes: Ref<Node[]>,
  /** Optional: returns a VueFlow node with rendered .dimensions */
  findNode?: (id: string) => Node | undefined,
) {
  /** Get actual rendered dimensions for a node, falling back to style then defaults */
  function getChildSize(child: Node): { w: number; h: number } {
    // 1. Try VueFlow rendered dimensions
    if (findNode) {
      const live = findNode(child.id);
      const dim = (live as any)?.dimensions;
      if (dim && dim.width > 0 && dim.height > 0) {
        return { w: dim.width, h: dim.height };
      }
    }
    // 2. For groups, read style width/height
    if (child.type === 'group') {
      const style = child.style as Record<string, string> | undefined;
      return {
        w: parseInt(style?.width || '250', 10),
        h: parseInt(style?.height || '120', 10),
      };
    }
    // 3. Fallback to minimums
    return MIN_NODE_SIZE[child.type || 'agent'] ?? MIN_NODE_SIZE.agent!;
  }

  /** Get depth of a group node (0 = top-level) */
  function groupDepth(groupId: string): number {
    let d = 0;
    let current = nodes.value.find(n => n.id === groupId);
    while (current?.parentNode) {
      d++;
      current = nodes.value.find(n => n.id === current!.parentNode);
    }
    return d;
  }

  /** Recalculate a single group's size to fit its children */
  function recalculateGroupSize(groupId: string) {
    const group = nodes.value.find(n => n.id === groupId && n.type === 'group');
    if (!group) return;

    const data = group.data as { autoFit?: boolean };
    if (data.autoFit === false) return; // manual mode

    const children = nodes.value.filter(n =>
      n.parentNode === groupId && !n.hidden && !((n.class as string) || '').includes('compact-hidden')
    );
    if (children.length === 0) {
      group.style = { ...group.style as Record<string, string>, width: `${MIN_WIDTH}px`, height: `${MIN_HEIGHT}px` };
      return;
    }

    let maxX = 0;
    let maxY = 0;

    for (const child of children) {
      const sz = getChildSize(child);
      const right = child.position.x + sz.w;
      const bottom = child.position.y + sz.h;
      if (right > maxX) maxX = right;
      if (bottom > maxY) maxY = bottom;
    }

    const newW = Math.max(maxX + PADDING.right, MIN_WIDTH);
    const newH = Math.max(maxY + PADDING.bottom, MIN_HEIGHT);

    group.style = {
      ...group.style as Record<string, string>,
      width: `${newW}px`,
      height: `${newH}px`,
    };
  }

  /** Recalculate all groups bottom-up (deepest first) */
  function autoFitAllGroups() {
    const groups = nodes.value.filter(n => n.type === 'group');
    const sorted = groups
      .map(g => ({ id: g.id, depth: groupDepth(g.id) }))
      .sort((a, b) => b.depth - a.depth);

    for (const { id } of sorted) {
      recalculateGroupSize(id);
    }
  }

  /** Debounced auto-fit for a specific group (batches via nextTick) */
  let pendingGroups = new Set<string>();
  let scheduled = false;

  function scheduleAutoFit(groupId: string) {
    pendingGroups.add(groupId);
    if (!scheduled) {
      scheduled = true;
      nextTick(() => {
        for (const id of pendingGroups) {
          recalculateGroupSize(id);
        }
        // Also recalc parent groups (bottom-up propagation)
        const parentIds = new Set<string>();
        for (const id of pendingGroups) {
          const group = nodes.value.find(n => n.id === id);
          if (group?.parentNode) parentIds.add(group.parentNode);
        }
        for (const pid of parentIds) {
          recalculateGroupSize(pid);
        }
        pendingGroups = new Set();
        scheduled = false;
      });
    }
  }

  return {
    recalculateGroupSize,
    autoFitAllGroups,
    scheduleAutoFit,
  };
}
