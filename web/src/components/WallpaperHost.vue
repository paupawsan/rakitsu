<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useShaderWallpaper } from '../composables/useShaderWallpaper';
import { useWallpaperPrefs } from '../composables/useWallpaperPrefs';
import { useWaterScene } from '../composables/useWaterScene';
import { useWallpaperAudio } from '../composables/useWallpaperAudio';
import { WATER_INDEX, SUMI_INDEX } from '../shaders/fragments';
import WallpaperPicker from './WallpaperPicker.vue';
import WaterOverlay from './wallpaper/WaterOverlay.vue';
import WaterModeControls from './wallpaper/WaterModeControls.vue';
import SumiFluidLayer from './wallpaper/SumiFluidLayer.vue';
import SumiControls from './wallpaper/SumiControls.vue';
import WallpaperChrome from './wallpaper/WallpaperChrome.vue';

const { enabled, shaderIndex, dim } = useWallpaperPrefs();
const water = useWaterScene();
const audio = useWallpaperAudio();
const canvasRef = ref<HTMLCanvasElement | null>(null);
const rootRef = ref<HTMLElement | null>(null);

const isWater = computed(() => enabled.value && shaderIndex.value === WATER_INDEX);
const isSumi = computed(() => enabled.value && shaderIndex.value === SUMI_INDEX);

// Pond beds (ambient lapping + rain) only play while the water scene is up.
// They start on the first water click (audio unlock) — this watch handles
// scene switches after the context already exists.
watch(isWater, (v) => {
  if (v) {
    audio.startAmbient();
    if (water.rainMode.value) audio.startRainSound();
  } else {
    audio.stopAmbient();
    audio.stopRainSound();
  }
});

useShaderWallpaper({
  canvasRef,
  rootRef,
  shaderIndex,
  dim,
  enabled,
});
</script>

<template>
  <div ref="rootRef" class="wallpaper-host" aria-hidden="true">
    <!-- The suminagashi fluid sim renders on its own canvas (it needs FBO
         ping-pong). It sits under the main canvas, which stays cleared
         while the fluid owns the scene. -->
    <SumiFluidLayer v-if="enabled" v-show="isSumi" class="wallpaper-sumi-layer" />
    <canvas v-if="enabled" ref="canvasRef" class="wallpaper-canvas" />
    <!-- Clock + hint chips sit between the shader and the koi overlay, so
         on the water scene the fish swim OVER the clock (underwater feel). -->
    <WallpaperChrome v-if="enabled" />
    <!-- Water-mode overlays (fish, pellets, bobber, catch banner) ride
         on top of the shader and below the picker. -->
    <WaterOverlay v-if="isWater" class="wallpaper-water-overlay" />
    <div v-if="enabled" class="wallpaper-grid" :class="{ 'over-water': isWater, 'over-sumi': isSumi }" />
    <div v-if="enabled && !isSumi" class="wallpaper-vignette" />
    <WaterModeControls v-if="isWater" class="wallpaper-host-water-controls" />
    <SumiControls v-if="isSumi" class="wallpaper-host-sumi-controls" />
    <WallpaperPicker class="wallpaper-host-picker" :visible="true" />
  </div>
</template>

<style scoped>
.wallpaper-host {
  position: absolute;
  inset: 0;
  pointer-events: none;
  overflow: hidden;
}

.wallpaper-canvas {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  display: block;
  pointer-events: none;
}

.wallpaper-sumi-layer {
  position: absolute;
  inset: 0;
}

.wallpaper-grid {
  position: absolute;
  inset: 0;
  background-image: radial-gradient(circle, var(--canvas-grid-color) 1px, transparent 1px);
  background-size: 18px 18px;
  pointer-events: none;
  mix-blend-mode: plus-lighter;
  opacity: 0.7;
}

/* Dim the dot grid heavily over water so the koi + caustics stay readable. */
.wallpaper-grid.over-water {
  opacity: 0.18;
  mix-blend-mode: normal;
}

/* Washi paper is light — plus-lighter dots would blow out. */
.wallpaper-grid.over-sumi {
  opacity: 0.12;
  mix-blend-mode: multiply;
}

.wallpaper-water-overlay {
  position: absolute;
  inset: 0;
  z-index: 5;
}

.wallpaper-host-water-controls {
  position: absolute;
  top: 12px;
  left: 12px;
  z-index: 41;
  pointer-events: auto;
}

.wallpaper-host-sumi-controls {
  position: absolute;
  top: 12px;
  left: 12px;
  z-index: 41;
  pointer-events: auto;
}

.wallpaper-vignette {
  position: absolute;
  inset: 0;
  pointer-events: none;
  background: radial-gradient(80% 80% at 50% 50%, transparent 0%, rgba(18, 18, 31, 0.35) 100%);
}

.wallpaper-host-picker {
  position: absolute;
  top: 12px;
  right: 12px;
  z-index: 40;
  pointer-events: auto;
}
</style>
