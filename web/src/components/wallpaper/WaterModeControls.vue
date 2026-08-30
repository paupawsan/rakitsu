<script setup lang="ts">
// Audio mute / Feed mode / Fishing mode / Rain toggles.
// Shown only when shader 06 (water) is active. Feed and Fish are mutually
// exclusive — toggling one disables the other. Rain stacks with either.

import { onBeforeUnmount, onMounted } from 'vue';
import { useWaterScene } from '../../composables/useWaterScene';
import { useWallpaperAudio } from '../../composables/useWallpaperAudio';

const water = useWaterScene();
const audio = useWallpaperAudio();

function toggleAudio() {
  audio.setMuted(!audio.audioMuted.value);
  if (!audio.audioMuted.value) {
    audio.ensureAudio();
    audio.startAmbient();
    if (water.rainMode.value) audio.startRainSound();
  }
}
function toggleFeed() {
  water.feedMode.value = !water.feedMode.value;
  if (water.feedMode.value && water.fishingMode.value) {
    water.fishingMode.value = false;
    water.bobber.value = null;
  }
}
function toggleFish() {
  water.fishingMode.value = !water.fishingMode.value;
  if (water.fishingMode.value && water.feedMode.value) {
    water.feedMode.value = false;
  }
  if (!water.fishingMode.value) water.bobber.value = null;
}
function toggleRain() {
  water.rainMode.value = !water.rainMode.value;
  audio.ensureAudio();
  if (water.rainMode.value) audio.startRainSound();
  else audio.stopRainSound();
}

function onKey(e: KeyboardEvent) {
  const tag = (e.target as HTMLElement | null)?.tagName;
  if (tag === 'INPUT' || tag === 'TEXTAREA' || (e.target as HTMLElement | null)?.isContentEditable) {
    return;
  }
  if (e.key === 'f' || e.key === 'F') {
    if (e.shiftKey) toggleFish();
    else toggleFeed();
  }
  if (e.key === 'r' || e.key === 'R') {
    toggleRain();
  }
}

onMounted(() => window.addEventListener('keydown', onKey));
onBeforeUnmount(() => window.removeEventListener('keydown', onKey));
</script>

<template>
  <div class="water-mode-controls">
    <button
      type="button"
      class="control-btn"
      :class="{ muted: audio.audioMuted.value }"
      :title="audio.audioMuted.value ? 'Unmute water sounds' : 'Mute water sounds'"
      @click="toggleAudio"
    >
      <span class="icon" aria-hidden="true">{{ audio.audioMuted.value ? '✕' : '♪' }}</span>
    </button>
    <button
      type="button"
      class="control-btn feed-btn"
      :class="{ active: water.feedMode.value }"
      title="Feed mode — drop pellets for the koi (F)"
      @click="toggleFeed"
    >
      <span class="icon" aria-hidden="true">🍙</span>
      <span class="lbl">FEED</span>
    </button>
    <button
      type="button"
      class="control-btn fish-btn"
      :class="{ active: water.fishingMode.value }"
      title="Fishing mode — cast a bobber and wait for a bite (Shift+F)"
      @click="toggleFish"
    >
      <span class="icon" aria-hidden="true">🎣</span>
      <span class="lbl">FISH</span>
    </button>
    <button
      type="button"
      class="control-btn rain-btn"
      :class="{ active: water.rainMode.value }"
      title="Rain — gentle rainfall patters the pond (R)"
      @click="toggleRain"
    >
      <span class="icon" aria-hidden="true">🌧</span>
      <span class="lbl">RAIN</span>
    </button>
  </div>
</template>

<style scoped>
.water-mode-controls {
  display: flex;
  gap: 6px;
  align-items: center;
  padding: 6px;
  background: rgba(15, 28, 48, 0.72);
  border: 1px solid rgba(255, 255, 255, 0.08);
  backdrop-filter: blur(12px) saturate(140%);
  -webkit-backdrop-filter: blur(12px) saturate(140%);
  border-radius: 6px;
  box-shadow: 0 6px 24px rgba(0, 0, 0, 0.45);
  pointer-events: auto;
  font-family: var(--font-mono, 'SF Mono', ui-monospace, Menlo, monospace);
}

.control-btn {
  all: unset;
  cursor: pointer;
  height: 28px;
  min-width: 28px;
  padding: 0 8px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border-radius: 4px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  color: var(--text-secondary, #c9d3e6);
  font-size: 11px;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  transition: border-color 0.15s ease, color 0.15s ease, background 0.15s ease;
}
.control-btn:hover {
  border-color: rgba(255, 255, 255, 0.22);
  color: var(--text-primary, #e0e0e0);
}
.control-btn .icon {
  font-size: 13px;
  line-height: 1;
}
.control-btn .lbl {
  font-size: 10px;
}
.control-btn.muted {
  color: rgba(200, 200, 220, 0.45);
}
.feed-btn.active {
  border-color: #e8a253;
  color: #fcd9a8;
  background: rgba(232, 162, 83, 0.18);
  box-shadow: 0 0 0 1px rgba(232, 162, 83, 0.45);
}
.fish-btn.active {
  border-color: var(--accent-agent, #5b8def);
  color: #c9dbff;
  background: rgba(91, 141, 239, 0.18);
  box-shadow: 0 0 0 1px rgba(91, 141, 239, 0.45);
}
.rain-btn.active {
  border-color: var(--accent-tool, #2dd4a0);
  color: #a9f0dc;
  background: rgba(45, 212, 160, 0.16);
  box-shadow: 0 0 0 1px rgba(45, 212, 160, 0.45);
}
</style>
