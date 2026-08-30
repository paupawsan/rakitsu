<script setup lang="ts">
import { Handle, Position } from '@vue-flow/core';

// 3 pins per side × 4 sides × 2 types (source+target) = 24 handles.
// Pins are at 25% / 50% / 75% along the edge. Handle IDs encode both the
// side and the pin index: `s-<dir>-<1|2|3>` / `t-<dir>-<1|2|3>`. The layout
// composable picks the right one based on the relative position of the
// other endpoint so multiple incident edges don't converge to one dot.
const PIN_OFFSETS = ['25%', '50%', '75%'] as const;
</script>

<template>
  <!-- Top side: horizontal spread → left offset varies -->
  <template v-for="(off, i) in PIN_OFFSETS" :key="`top-${i}`">
    <Handle
      :id="`s-top-${i + 1}`"
      type="source"
      :position="Position.Top"
      :style="{ left: off }"
    />
    <Handle
      :id="`t-top-${i + 1}`"
      type="target"
      :position="Position.Top"
      :style="{ left: off }"
    />
  </template>

  <!-- Bottom side: horizontal spread -->
  <template v-for="(off, i) in PIN_OFFSETS" :key="`bot-${i}`">
    <Handle
      :id="`s-bottom-${i + 1}`"
      type="source"
      :position="Position.Bottom"
      :style="{ left: off }"
    />
    <Handle
      :id="`t-bottom-${i + 1}`"
      type="target"
      :position="Position.Bottom"
      :style="{ left: off }"
    />
  </template>

  <!-- Left side: vertical spread → top offset varies -->
  <template v-for="(off, i) in PIN_OFFSETS" :key="`left-${i}`">
    <Handle
      :id="`s-left-${i + 1}`"
      type="source"
      :position="Position.Left"
      :style="{ top: off }"
    />
    <Handle
      :id="`t-left-${i + 1}`"
      type="target"
      :position="Position.Left"
      :style="{ top: off }"
    />
  </template>

  <!-- Right side: vertical spread -->
  <template v-for="(off, i) in PIN_OFFSETS" :key="`right-${i}`">
    <Handle
      :id="`s-right-${i + 1}`"
      type="source"
      :position="Position.Right"
      :style="{ top: off }"
    />
    <Handle
      :id="`t-right-${i + 1}`"
      type="target"
      :position="Position.Right"
      :style="{ top: off }"
    />
  </template>
</template>

<style>
/* Handles are invisible by default — they exist purely as attachment points
   for the edges. On node hover they fade in faintly so the user can see
   where wires can connect. */
.vue-flow__node .vue-flow__handle {
  width: 5px !important;
  height: 5px !important;
  background: transparent !important;
  border: 1px solid transparent !important;
  opacity: 0 !important;
  transition: opacity 0.15s, background 0.15s, border-color 0.15s;
}
.vue-flow__node:hover .vue-flow__handle {
  opacity: 0.6 !important;
  background: rgba(154, 171, 209, 0.35) !important;
  border-color: rgba(154, 171, 209, 0.6) !important;
}
</style>
