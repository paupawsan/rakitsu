<script setup lang="ts">
// Suminagashi (07) controls: audio mute, ink palette, and the SUMINAGASHI
// panel (FLUID/MARBLE mode switch, FX sliders, CLEAR). Washi-light styling
// to match the paper scene. Shown only while shader 07 is active.
//
// Keyboard: M flips fluid/marble, C clears to fresh paper.
// The palette and FX sliders only apply to the fluid sim; when the GPU
// lacks fluid support the scene is the analytic marbling fallback and only
// the mute button (and C) remain.

import { onBeforeUnmount, onMounted } from 'vue';
import { useSumiScene } from '../../composables/useSumiScene';
import { useWallpaperAudio } from '../../composables/useWallpaperAudio';

const sumi = useSumiScene();
const audio = useWallpaperAudio();

function toggleAudio() {
  audio.setMuted(!audio.audioMuted.value);
  if (!audio.audioMuted.value) audio.ensureAudio();
}

function onKey(e: KeyboardEvent) {
  const target = e.target as HTMLElement | null;
  if (target?.tagName === 'INPUT' || target?.tagName === 'TEXTAREA' || target?.isContentEditable) {
    return;
  }
  if ((e.key === 'm' || e.key === 'M') && sumi.fluidSupported.value) {
    sumi.setMode(sumi.sumiMode.value === 'fluid' ? 'marble' : 'fluid');
  }
  if (e.key === 'c' || e.key === 'C') {
    sumi.clearScene();
  }
}

onMounted(() => window.addEventListener('keydown', onKey));
onBeforeUnmount(() => window.removeEventListener('keydown', onKey));
</script>

<template>
  <div class="sumi-controls">
    <div class="row-bar">
      <button
        type="button"
        class="control-btn"
        :class="{ muted: audio.audioMuted.value }"
        :title="audio.audioMuted.value ? 'Unmute ink sounds' : 'Mute ink sounds'"
        @click="toggleAudio"
      >
        <span class="icon" aria-hidden="true">{{ audio.audioMuted.value ? '✕' : '♪' }}</span>
      </button>
      <div
        v-if="sumi.fluidSupported.value && sumi.sumiMode.value === 'fluid'"
        class="ink-palette"
      >
        <span class="lbl">INK</span>
        <button
          v-for="(ink, i) in sumi.INKS"
          :key="ink.name"
          type="button"
          class="ink"
          :class="{ active: sumi.selectedInk.value === i }"
          :style="{ '--c': ink.swatch }"
          :title="ink.name"
          @click="sumi.selectedInk.value = i"
        />
        <button
          type="button"
          class="ink auto"
          :class="{ active: sumi.selectedInk.value === -1 }"
          title="Auto — hue drifts with every stroke"
          @click="sumi.selectedInk.value = -1"
        />
      </div>
    </div>
    <div v-if="sumi.fluidSupported.value" class="sumi-panel">
      <span class="ttl">SUMINAGASHI</span>
      <div class="seg">
        <button
          type="button"
          :class="{ active: sumi.sumiMode.value === 'fluid' }"
          title="Fluid ink — smoke-like simulation (M)"
          @click="sumi.setMode('fluid')"
        >
          FLUID
        </button>
        <button
          type="button"
          :class="{ active: sumi.sumiMode.value === 'marble' }"
          title="Classic marbling — concentric ring suminagashi (M)"
          @click="sumi.setMode('marble')"
        >
          MARBLE
        </button>
      </div>
      <template v-if="sumi.sumiMode.value === 'fluid'">
        <div class="fx-row">
          <span class="k">SPEED</span>
          <input v-model.number="sumi.fxParams.speed" type="range" min="0.25" max="3" step="0.05" />
        </div>
        <div class="fx-row">
          <span class="k">SWIRL</span>
          <input v-model.number="sumi.fxParams.swirl" type="range" min="0" max="3" step="0.05" />
        </div>
        <div class="fx-row">
          <span class="k">FADE</span>
          <input v-model.number="sumi.fxParams.fade" type="range" min="0" max="4" step="0.05" />
        </div>
        <div class="fx-row">
          <span class="k">BRUSH</span>
          <input v-model.number="sumi.fxParams.size" type="range" min="0.4" max="2.5" step="0.05" />
        </div>
      </template>
      <button type="button" class="clear-btn" title="Fresh paper (C)" @click="sumi.clearScene()">
        CLEAR · C
      </button>
    </div>
  </div>
</template>

<style scoped>
.sumi-controls {
  display: flex;
  flex-direction: column;
  gap: 8px;
  align-items: flex-start;
  font-family: var(--font-mono, 'SF Mono', ui-monospace, Menlo, monospace);
  pointer-events: auto;
}

.row-bar {
  display: flex;
  gap: 8px;
  align-items: center;
}

.control-btn {
  all: unset;
  cursor: pointer;
  height: 28px;
  min-width: 28px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 4px;
  background: rgba(250, 246, 236, 0.78);
  border: 1px solid rgba(60, 50, 40, 0.2);
  color: #6b6354;
  font-size: 13px;
  transition: color 0.15s ease, border-color 0.15s ease;
}
.control-btn:hover {
  color: #38332b;
  border-color: rgba(60, 50, 40, 0.45);
}
.control-btn.muted {
  color: rgba(107, 99, 84, 0.5);
}

.ink-palette {
  display: flex;
  gap: 8px;
  align-items: center;
  height: 28px;
  padding: 0 10px;
  border-radius: 14px;
  background: rgba(250, 246, 236, 0.78);
  border: 1px solid rgba(60, 50, 40, 0.2);
  backdrop-filter: blur(10px);
  -webkit-backdrop-filter: blur(10px);
}
.ink-palette .lbl {
  font-size: 9px;
  letter-spacing: 0.1em;
  color: #8a8172;
}
.ink {
  all: unset;
  cursor: pointer;
  width: 15px;
  height: 15px;
  border-radius: 50%;
  background: var(--c);
  box-sizing: border-box;
  border: 2px solid rgba(250, 246, 236, 0);
  transition: transform 0.15s ease, box-shadow 0.15s ease;
}
.ink:hover {
  transform: scale(1.18);
}
.ink.active {
  border-color: rgba(250, 246, 236, 0.95);
  box-shadow: 0 0 0 1.5px rgba(60, 50, 40, 0.6);
  transform: scale(1.12);
}
.ink.auto {
  background: conic-gradient(#c2451e, #c89031, #5d6b2f, #2c6b58, #2b4a78, #6b3a5b, #c2451e);
}

/* Washi card with mode switch + FX sliders */
.sumi-panel {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 176px;
  padding: 10px 12px 11px;
  border-radius: 10px;
  background: rgba(250, 246, 236, 0.82);
  border: 1px solid rgba(60, 50, 40, 0.18);
  backdrop-filter: blur(10px);
  -webkit-backdrop-filter: blur(10px);
  box-shadow: 0 6px 24px rgba(0, 0, 0, 0.18);
}
.sumi-panel .ttl {
  font-size: 9px;
  letter-spacing: 0.18em;
  color: #8a8172;
}
.seg {
  display: flex;
  border: 1px solid rgba(60, 50, 40, 0.28);
  border-radius: 7px;
  overflow: hidden;
}
.seg button {
  all: unset;
  cursor: pointer;
  flex: 1;
  text-align: center;
  padding: 5px 0;
  font-size: 9.5px;
  letter-spacing: 0.12em;
  color: #8a8172;
  transition: background 0.15s ease, color 0.15s ease;
}
.seg button:hover {
  color: #38332b;
}
.seg button.active {
  background: #38332b;
  color: #efe9da;
}
.fx-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.fx-row .k {
  font-size: 9px;
  letter-spacing: 0.1em;
  color: #8a8172;
  width: 38px;
  flex: none;
}
.fx-row input[type='range'] {
  flex: 1;
  min-width: 0;
  height: 14px;
  accent-color: #9a4a2a;
  cursor: pointer;
}
.clear-btn {
  all: unset;
  cursor: pointer;
  text-align: center;
  padding: 5px 0;
  border-radius: 7px;
  border: 1px dashed rgba(60, 50, 40, 0.35);
  font-size: 9.5px;
  letter-spacing: 0.12em;
  color: #8a8172;
  transition: color 0.15s ease, border-color 0.15s ease;
}
.clear-btn:hover {
  color: #9a4a2a;
  border-color: #9a4a2a;
}
</style>
