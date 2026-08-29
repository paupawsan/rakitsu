<script setup lang="ts">
// Suminagashi (07) fluid layer: hosts the GPU ink simulation on its own
// canvas (the sim needs FBO ping-pong, which the plain wallpaper canvas
// pipeline doesn't do). Marble mode renders through the same GL context.
//
// FPS discipline (from the design handoff):
//   - The fluid composites a soft, smoky image — it doesn't need the full
//     retina backing store. The canvas renders at min(dpr, 1.25×).
//   - Adaptive quality: if the fluid scene can't hold ~50fps while actually
//     visible, the render scale steps down (1.25 → 1.0 → 0.75) until it can.
//   - The rAF loop only runs while this scene is visible; switching scenes
//     keeps the GL state (your ink survives) but costs zero frames.
//
// If the GPU lacks half-float render targets the sim never claims this
// canvas; the analytic FRAG_7 marbling on the main wallpaper canvas is the
// scene instead, and this layer stays hidden.

import { computed, onActivated, onBeforeUnmount, onDeactivated, onMounted, ref, watch } from 'vue';
import { useWallpaperPrefs } from '../../composables/useWallpaperPrefs';
import { useSumiScene } from '../../composables/useSumiScene';
import { SUMI_INDEX } from '../../shaders/fragments';
import { SumiFluidSim } from '../../utils/sumiFluid';

const { shaderIndex, dim } = useWallpaperPrefs();
const sumi = useSumiScene();

const rootRef = ref<HTMLDivElement | null>(null);
const canvasRef = ref<HTMLCanvasElement | null>(null);
const isActive = computed(() => shaderIndex.value === SUMI_INDEX);

let sim: SumiFluidSim | null = null;
let initTried = false;
let raf: number | null = null;
let paused = false; // keep-alive
let t0 = performance.now();

// adaptive render scale: 1.25 → 1.0 → 0.75 when fps sags
let renderScale = 1.25;
let lowFpsStreak = 0;
let fpsLast = 0;
let fpsCount = 0;

// smoothed local mouse (y-up CSS px) for the marble shader's stir/sheen
let mx = 0;
let my = 0;
let smx = 0;
let smy = 0;
let targetActive = 0;
let smActive = 0;
let rootW = 1;
let rootH = 1;

let resizeObs: ResizeObserver | null = null;

function onMouseMove(e: MouseEvent | TouchEvent) {
  const t = 'touches' in e ? e.touches[0] : (e as MouseEvent);
  const root = rootRef.value;
  if (!t || !root) return;
  const rect = root.getBoundingClientRect();
  const x = t.clientX - rect.left;
  const y = t.clientY - rect.top;
  if (x < 0 || y < 0 || x > rect.width || y > rect.height) {
    targetActive = 0;
    return;
  }
  mx = x;
  my = rect.height - y;
  targetActive = 1;
}

function resize() {
  const c = canvasRef.value;
  const root = rootRef.value;
  if (!c || !root) return;
  const rect = root.getBoundingClientRect();
  rootW = Math.max(1, rect.width);
  rootH = Math.max(1, rect.height);
  const d = Math.min(Math.min(window.devicePixelRatio || 1, 2), renderScale);
  c.width = Math.max(1, Math.floor(rootW * d));
  c.height = Math.max(1, Math.floor(rootH * d));
  c.style.width = rootW + 'px';
  c.style.height = rootH + 'px';
  if (sim?.ready) sim.resize();
}

function applyDim() {
  const c = canvasRef.value;
  if (!c) return;
  const v = Math.max(0, Math.min(100, dim.value)) / 100;
  c.style.filter = `saturate(${1 - v * 0.5}) brightness(${1 - v * 0.75})`;
}

function ensureInit() {
  if (initTried) return;
  initTried = true;
  const c = canvasRef.value;
  if (!c) return;
  resize();
  const s = new SumiFluidSim();
  if (s.init(c)) {
    sim = s;
    sumi.fluidSupported.value = true;
  } else {
    s.dispose();
    sumi.fluidSupported.value = false;
  }
  // restoring a stored 'marble' mode seeds the ring pattern; fluid mode
  // seeds three soft blooms so the paper opens alive
  sumi.setMode(sumi.sumiMode.value);
  sumi.seedScene(sim, rootW, rootH);
}

function shouldRun(): boolean {
  return isActive.value && !paused && sim?.ready === true && document.visibilityState === 'visible';
}

function frame(now: number) {
  raf = null;
  if (!sim?.ready || !canvasRef.value) return;
  const t = (now - t0) / 1000;
  smx += (mx - smx) * 0.14;
  smy += (my - smy) * 0.14;
  smActive += (targetActive - smActive) * 0.08;

  if (sumi.sumiMode.value === 'fluid') {
    sim.step(now / 1000);
  } else {
    // marble: animate drop blooms, render via the fluid's GL context.
    // Mouse scales by the ACTUAL backing-store ratio (renderScale cap).
    const ms = canvasRef.value.width / rootW;
    sim.renderMarble(sumi.buildMarbleDrops(false), smx * ms, smy * ms, t, smActive);
  }

  // adaptive quality: if the fluid can't hold ~50fps while visible, step
  // the render scale down (1.25 → 1.0 → 0.75) until it does
  fpsCount++;
  if (fpsLast === 0) fpsLast = now;
  if (now - fpsLast > 500) {
    const fps = (fpsCount * 1000) / (now - fpsLast);
    fpsCount = 0;
    fpsLast = now;
    if (sumi.sumiMode.value === 'fluid' && document.visibilityState === 'visible' && fps > 0 && fps < 50) {
      lowFpsStreak++;
      if (lowFpsStreak >= 4 && renderScale > 0.75) {
        renderScale = +(renderScale - 0.25).toFixed(2);
        lowFpsStreak = 0;
        resize();
      }
    } else {
      lowFpsStreak = 0;
    }
  }

  if (shouldRun()) raf = requestAnimationFrame(frame);
}

function evaluateLoop() {
  if (isActive.value && !paused) ensureInit();
  if (sim) {
    if (isActive.value && !paused) sumi.registerSim(sim);
    else sumi.unregisterSim(sim);
  }
  if (shouldRun()) {
    if (raf === null) {
      fpsLast = 0;
      fpsCount = 0;
      raf = requestAnimationFrame(frame);
    }
  } else if (raf !== null) {
    cancelAnimationFrame(raf);
    raf = null;
  }
}

const onVisibility = () => evaluateLoop();

watch(isActive, evaluateLoop);
watch(dim, applyDim);

onMounted(() => {
  t0 = performance.now();
  applyDim();
  resizeObs = new ResizeObserver(resize);
  if (rootRef.value) resizeObs.observe(rootRef.value);
  // capture phase — the Vue Flow pane stops propagation during drags
  window.addEventListener('mousemove', onMouseMove, { capture: true, passive: true });
  window.addEventListener('touchmove', onMouseMove, { capture: true, passive: true });
  document.addEventListener('visibilitychange', onVisibility);
  evaluateLoop();
});
onActivated(() => {
  paused = false;
  evaluateLoop();
});
onDeactivated(() => {
  paused = true;
  evaluateLoop();
});
onBeforeUnmount(() => {
  if (raf !== null) cancelAnimationFrame(raf);
  raf = null;
  if (resizeObs) resizeObs.disconnect();
  window.removeEventListener('mousemove', onMouseMove as EventListener, { capture: true });
  window.removeEventListener('touchmove', onMouseMove as EventListener, { capture: true });
  document.removeEventListener('visibilitychange', onVisibility);
  if (sim) {
    sumi.unregisterSim(sim);
    sim.dispose();
    sim = null;
  }
});
</script>

<template>
  <div ref="rootRef" class="sumi-layer" aria-hidden="true">
    <canvas v-show="sumi.fluidSupported.value !== false" ref="canvasRef" class="sumi-canvas" />
  </div>
</template>

<style scoped>
.sumi-layer {
  position: absolute;
  inset: 0;
  pointer-events: none;
}

.sumi-canvas {
  position: absolute;
  inset: 0;
  display: block;
  pointer-events: none;
}
</style>
