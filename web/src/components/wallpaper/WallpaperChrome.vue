<script setup lang="ts">
// Wallpaper chrome overlay from the design prototype: the big mono clock
// (bottom-left) and the dock hint chips (bottom-right). Pure decoration —
// pointer-events: none throughout.
//
// On the water scene the clock reads as if it were under the surface: it
// sways with live ripple activity (transform + skew + blur driven by the
// shared ripple buffer) and the koi overlay renders above it. On the
// suminagashi scene the chrome flips to ink-on-washi tones. The sway rAF
// only runs while the water scene is visible.

import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useWallpaperPrefs } from '../../composables/useWallpaperPrefs';
import { useWaterScene } from '../../composables/useWaterScene';
import { WATER_INDEX, SUMI_INDEX } from '../../shaders/fragments';

const { shaderIndex } = useWallpaperPrefs();
const water = useWaterScene();

const isWater = computed(() => shaderIndex.value === WATER_INDEX);
const isSumi = computed(() => shaderIndex.value === SUMI_INDEX);

const timeText = ref('00:00');
const dateText = ref('—');
const clockEl = ref<HTMLDivElement | null>(null);

const MONTHS = ['JAN', 'FEB', 'MAR', 'APR', 'MAY', 'JUN', 'JUL', 'AUG', 'SEP', 'OCT', 'NOV', 'DEC'];
const WEEKDAYS = ['SUN', 'MON', 'TUE', 'WED', 'THU', 'FRI', 'SAT'];

function pad(n: number): string {
  return n < 10 ? '0' + n : '' + n;
}

function tick() {
  const d = new Date();
  timeText.value = `${pad(d.getHours())}:${pad(d.getMinutes())}`;
  dateText.value = `${WEEKDAYS[d.getDay()]} · ${pad(d.getDate())} ${MONTHS[d.getMonth()]} ${d.getFullYear()}`;
}
let tickTimer: number | null = null;

const hint = computed(() => {
  if (isWater.value) return 'click to ripple · F feed · ⇧F fish · R rain';
  if (isSumi.value) return 'click to drop ink · M marble · C clear';
  return 'click to pulse';
});

// === Clock sway: rocks subtly with the average wave activity ===
let raf: number | null = null;

function sway() {
  raf = null;
  const el = clockEl.value;
  if (!el) return;
  const tNow = water.shaderTimeNow();
  let waveAct = 0;
  for (const r of water.ripples) {
    const a = tNow - r.t;
    if (a > 0 && a < 5) waveAct += r.s * Math.exp(-a * 0.5);
  }
  const baseSway = Math.sin(tNow * 0.7) * 2.8 + Math.sin(tNow * 0.31) * 2.0;
  const rippleSway = Math.min(18, waveAct * 28);
  const sx = baseSway + Math.sin(tNow * 2.1) * rippleSway * 1.2;
  const sy = Math.cos(tNow * 0.55) * 1.6 + Math.cos(tNow * 1.7) * rippleSway * 0.8;
  const skew = Math.sin(tNow * 0.9) * 0.7 + rippleSway * 0.5;
  el.style.transform = `translate(${sx.toFixed(2)}px, ${sy.toFixed(2)}px) skewX(${skew.toFixed(2)}deg) rotate(${(skew * 0.4).toFixed(2)}deg)`;
  el.style.filter = `blur(${(0.5 + rippleSway * 0.25).toFixed(2)}px) saturate(0.85)`;
  if (shouldSway()) raf = requestAnimationFrame(sway);
}

function shouldSway(): boolean {
  return isWater.value && document.visibilityState === 'visible';
}

function evaluateSway() {
  if (shouldSway()) {
    if (raf === null) raf = requestAnimationFrame(sway);
  } else {
    if (raf !== null) {
      cancelAnimationFrame(raf);
      raf = null;
    }
    const el = clockEl.value;
    if (el) {
      el.style.transform = '';
      el.style.filter = '';
    }
  }
}

const onVisibility = () => evaluateSway();

watch(isWater, evaluateSway);

onMounted(() => {
  tick();
  tickTimer = window.setInterval(tick, 10_000);
  document.addEventListener('visibilitychange', onVisibility);
  evaluateSway();
});
onBeforeUnmount(() => {
  if (tickTimer !== null) window.clearInterval(tickTimer);
  if (raf !== null) cancelAnimationFrame(raf);
  document.removeEventListener('visibilitychange', onVisibility);
});
</script>

<template>
  <div class="wallpaper-chrome" :class="{ sumi: isSumi, water: isWater }" aria-hidden="true">
    <div ref="clockEl" class="clock">
      <div class="time">{{ timeText }}</div>
      <div class="date">{{ dateText }}</div>
    </div>
    <div class="dock">
      <div class="chip">{{ hint }}</div>
      <div class="chip">1–7 switch</div>
    </div>
  </div>
</template>

<style scoped>
.wallpaper-chrome {
  position: absolute;
  inset: 0;
  pointer-events: none;
  font-family: var(--font-mono, 'SF Mono', ui-monospace, 'Fira Code', Menlo, Consolas, monospace);
}

.clock {
  position: absolute;
  bottom: 64px;
  left: 32px;
  mix-blend-mode: screen;
  will-change: transform, filter;
}
/* Underwater feel: the koi overlay (z5) renders above this layer, and the
   sway loop adds ripple-driven transform + blur. */
.wallpaper-chrome.water .clock {
  opacity: 0.85;
}
.clock .time {
  font-weight: 300;
  font-size: clamp(44px, 6.5vw, 96px);
  letter-spacing: -0.04em;
  color: var(--text-primary, #e0e0e0);
  line-height: 0.95;
  text-shadow: 0 2px 28px rgba(0, 0, 0, 0.4);
}
.clock .date {
  font-size: 11px;
  letter-spacing: 0.18em;
  text-transform: uppercase;
  color: var(--text-secondary, #9a9ab0);
  margin-top: 7px;
}

.dock {
  position: absolute;
  bottom: 18px;
  right: 16px;
  display: flex;
  gap: 8px;
  mix-blend-mode: screen;
}
.chip {
  padding: 5px 9px;
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 4px;
  background: rgba(18, 18, 31, 0.3);
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
  font-size: 10px;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: var(--text-muted, #777790);
  white-space: nowrap;
}

/* Suminagashi: light washi paper — flip the chrome to ink tones */
.wallpaper-chrome.sumi .clock {
  mix-blend-mode: normal;
}
.wallpaper-chrome.sumi .clock .time {
  color: #2e2a24;
  text-shadow: none;
}
.wallpaper-chrome.sumi .clock .date {
  color: #6b6354;
}
.wallpaper-chrome.sumi .dock {
  mix-blend-mode: normal;
}
.wallpaper-chrome.sumi .chip {
  border-color: rgba(60, 50, 40, 0.25);
  background: rgba(255, 252, 245, 0.45);
  color: #8a8172;
}
</style>
