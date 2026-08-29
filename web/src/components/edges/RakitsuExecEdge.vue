<script setup lang="ts">
import { computed } from 'vue';
import { BaseEdge, getSmoothStepPath, type EdgeProps } from '@vue-flow/core';

const props = defineProps<EdgeProps>();

const path = computed(() => {
  const [d] = getSmoothStepPath({
    sourceX: props.sourceX,
    sourceY: props.sourceY,
    targetX: props.targetX,
    targetY: props.targetY,
    sourcePosition: props.sourcePosition,
    targetPosition: props.targetPosition,
    borderRadius: 8,
  });
  return d;
});

const isActive = computed(
  () => props.animated || Boolean((props.data as any)?.active),
);
const isBranch = computed(() => Boolean((props.data as any)?.branch));

// Inline styles beat VueFlow core's `.vue-flow__edge.animated path` selector.
const glowStyle = computed(() => {
  if (isActive.value) {
    return {
      stroke: 'rgba(34, 197, 94, 0.55)',
      strokeWidth: 9,
      filter: 'blur(5px)',
      opacity: 0.9,
      strokeDasharray: 'none',
      animation: 'none',
      fill: 'none',
      pointerEvents: 'none' as const,
    };
  }
  return {
    stroke: isBranch.value
      ? 'rgba(139, 169, 232, 0.65)'
      : 'rgba(91, 141, 239, 0.6)',
    strokeWidth: 8,
    filter: 'blur(4px)',
    opacity: 0.85,
    strokeDasharray: 'none',
    animation: 'none',
    fill: 'none',
    pointerEvents: 'none' as const,
  };
});

const mainStyle = computed(() => {
  if (isActive.value) {
    return {
      stroke: '#22c55e',
      strokeWidth: 3,
      strokeLinecap: 'round' as const,
      strokeLinejoin: 'round' as const,
      strokeDasharray: '6 4',
      filter: 'drop-shadow(0 0 4px rgba(34, 197, 94, 0.6))',
      fill: 'none',
    };
  }
  if (isBranch.value) {
    return {
      stroke: '#cfd8ee',
      strokeWidth: 2.6,
      strokeLinecap: 'round' as const,
      strokeLinejoin: 'round' as const,
      strokeDasharray: 'none',
      animation: 'none',
      fill: 'none',
    };
  }
  return {
    stroke: '#b6c5e6',
    strokeWidth: 2.4,
    strokeLinecap: 'round' as const,
    strokeLinejoin: 'round' as const,
    strokeDasharray: 'none',
    animation: 'none',
    fill: 'none',
  };
});
</script>

<template>
  <BaseEdge
    :id="id + '__glow'"
    :path="path"
    :style="glowStyle"
  />
  <BaseEdge
    :id="id"
    :path="path"
    :style="mainStyle"
    :marker-end="markerEnd"
    :class="isActive ? 'rakitsu-edge-active' : ''"
  />
</template>

<style>
@keyframes rakitsu-edge-dash {
  to {
    stroke-dashoffset: -20;
  }
}
.vue-flow__edge .rakitsu-edge-active .vue-flow__edge-path {
  animation: rakitsu-edge-dash 1.2s linear infinite !important;
}
</style>
