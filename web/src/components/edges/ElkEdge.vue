<script setup lang="ts">
import { computed } from 'vue';
import { BaseEdge, type EdgeProps } from '@vue-flow/core';

interface BendPoint { x: number; y: number }

const props = defineProps<EdgeProps>();

const path = computed(() => {
  const route = (props.data as any)?.route as {
    startPoint: BendPoint;
    bendPoints: BendPoint[];
    endPoint: BendPoint;
  } | undefined;

  if (!route) {
    // Fallback: straight line between source/target handles
    return `M ${props.sourceX},${props.sourceY} L ${props.targetX},${props.targetY}`;
  }

  const { startPoint, bendPoints, endPoint } = route;
  const segments = [startPoint, ...bendPoints, endPoint];

  // Build orthogonal polyline path with rounded corners
  if (segments.length <= 2) {
    return `M ${segments[0]!.x},${segments[0]!.y} L ${segments[1]!.x},${segments[1]!.y}`;
  }

  const r = 6; // corner radius
  let d = `M ${segments[0]!.x},${segments[0]!.y}`;

  for (let i = 1; i < segments.length - 1; i++) {
    const prev = segments[i - 1]!;
    const curr = segments[i]!;
    const next = segments[i + 1]!;

    // Direction vectors
    const dx1 = curr.x - prev.x;
    const dy1 = curr.y - prev.y;
    const dx2 = next.x - curr.x;
    const dy2 = next.y - curr.y;

    const len1 = Math.sqrt(dx1 * dx1 + dy1 * dy1);
    const len2 = Math.sqrt(dx2 * dx2 + dy2 * dy2);

    if (len1 === 0 || len2 === 0) {
      d += ` L ${curr.x},${curr.y}`;
      continue;
    }

    const cr = Math.min(r, len1 / 2, len2 / 2);

    // Point before the corner
    const bx = curr.x - (dx1 / len1) * cr;
    const by = curr.y - (dy1 / len1) * cr;
    // Point after the corner
    const ax = curr.x + (dx2 / len2) * cr;
    const ay = curr.y + (dy2 / len2) * cr;

    d += ` L ${bx},${by} Q ${curr.x},${curr.y} ${ax},${ay}`;
  }

  const last = segments[segments.length - 1]!;
  d += ` L ${last.x},${last.y}`;

  return d;
});
</script>

<template>
  <BaseEdge :id="id" :path="path" :style="style" :marker-end="markerEnd" />
</template>
