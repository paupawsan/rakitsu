// Water Ripple wallpaper (shader 06) — shared scene state.
//
// Owns the ripple circular buffer, fish school, pellets, bobber, and rain
// state that the water shader and its canvas-2D overlays need. Used as a
// singleton because the wallpaper is mounted in multiple Vue subtrees
// (builder canvas and debugger canvas) but only one is visible at a time.
// Sounds live in useWallpaperAudio (shared with the suminagashi scene).
//
// Coordinates:
//   - Ripple buffer entries store `(x, y)` in shader-uv space:
//       uv.x = pixelX / canvas.height
//       uv.y = pixelY / canvas.height       (GL y-up)
//   - Fish/pellet/bobber overlays use top-down client coordinates so they
//     can render through plain canvas2D `clientX`/`clientY` math.
//
// The shader composable reads `ripples` each frame and uploads it to the
// `u_ripples[0]` uniform when shader index 5 is active.

import { ref } from 'vue';
import { WATER_RIPPLE_SLOTS } from '../shaders/fragments';
import { useWallpaperAudio } from './useWallpaperAudio';

const audio = useWallpaperAudio();

export interface WaterRipple {
  /** UV-space x (pixelX / canvas.height) */ x: number;
  /** UV-space y (GL-flipped) */ y: number;
  /** Start time in seconds (shader clock) */ t: number;
  /** Strength (amplitude multiplier) */ s: number;
}

export interface WaterFish {
  x: number;
  y: number;
  heading: number;
  targetHeading: number;
  cruise: number;
  speedMul: number;
  size: number;
  phase: number;
  breathPhase: number;
  breathRate: number;
  bobPhase: number;
  bobAmp: number;
  bubbleAt: number;
  kind: { body: string; spot: string; spot2: string | null };
  spots: { u: number; v: number; r: number }[];
  startle: number;
  dartCooldown: number;
  /** Turn-following body curvature, carried through the spine renderer. */
  bend: number;
  /** Surface-gulp start time (-1 when idle). */
  gulpT: number;
  /** Next scheduled gulp time. */
  gulpAt: number;
  /** Whether the two gulp ripple rings have fired this gulp. */
  gulpR1: boolean;
  gulpR2: boolean;
}

export interface WaterPellet {
  x: number;
  y: number;
  vy: number;
  life: number;
  max: number;
  size: number;
  bobPhase: number;
}

export interface WaterBobber {
  x: number;
  y: number;
  t: number;
  biteAt: number;
  biteT: number;
  hookedFish: WaterFish | null;
  casting: number;
  bobPhase: number;
  lastShake: number;
}

export interface WaterBubble {
  x: number;
  y: number;
  vy: number;
  size: number;
  life: number;
  max: number;
  drift: number;
}

const KOI_KINDS = [
  { body: '#f5ede0', spot: '#d8451f', spot2: '#1c1c20' }, // sanke
  { body: '#f5ede0', spot: '#d8451f', spot2: null }, // kohaku
  { body: '#1c1c20', spot: '#d8451f', spot2: '#f5ede0' }, // showa
  { body: '#e8a253', spot: '#f5ede0', spot2: null }, // ogon-ish
  { body: '#f5ede0', spot: '#e8a253', spot2: null }, // platinum + gold
  { body: '#c95a2a', spot: '#f5ede0', spot2: null }, // orange + white
];
const FISH_COUNT = 11;
const KIND_NAMES = ['SANKE', 'KOHAKU', 'SHOWA', 'OGON', 'PLATINUM', 'ASAGI'];

// Strict singleton — survives component remounts so ripples don't reset
// when the user toggles the wallpaper off/on.
const ripples: WaterRipple[] = Array.from({ length: WATER_RIPPLE_SLOTS }, () => ({
  x: 0,
  y: 0,
  t: -100,
  s: 0,
}));
let ripplePtr = 0;

const fish = ref<WaterFish[]>(spawnFish());
const pellets = ref<WaterPellet[]>([]);
const bubbles = ref<WaterBubble[]>([]);
const bobber = ref<WaterBobber | null>(null);
const catchLabel = ref<string>('');

const feedMode = ref(false);
const fishingMode = ref(false);
const rainMode = ref(false);

// Shader-clock origin — set by the shader composable when the program
// starts so plain `performance.now()` callers can convert.
let t0Seconds = performance.now() / 1000;
function setShaderClockOrigin(t0Ms: number) {
  t0Seconds = t0Ms / 1000;
}
function shaderTimeNow(): number {
  return performance.now() / 1000 - t0Seconds;
}

// Fish/pellet/bobber positions are in local CSS pixels relative to the
// wallpaper-host root (top-down). The overlay rebounds them when the host
// resizes; on first spawn we use a sane default and let the overlay's
// resize handler tighten them once mounted.
function spawnFish(): WaterFish[] {
  const w = 1280;
  const h = 720;
  return Array.from({ length: FISH_COUNT }, (_, i) => {
    const kind = KOI_KINDS[i % KOI_KINDS.length] ?? KOI_KINDS[0]!;
    return {
      x: Math.random() * w,
      y: Math.random() * h,
      heading: Math.random() * Math.PI * 2,
      targetHeading: 0,
      cruise: 22 + Math.random() * 22,
      speedMul: 1,
      size: 18 + Math.random() * 14,
      phase: Math.random() * Math.PI * 2,
      breathPhase: Math.random() * Math.PI * 2,
      breathRate: 0.9 + Math.random() * 0.4,
      bobPhase: Math.random() * Math.PI * 2,
      bobAmp: 0.5 + Math.random() * 0.6,
      bubbleAt: performance.now() / 1000 + 4 + Math.random() * 8,
      kind,
      spots: Array.from({ length: 3 + ((i * 7) % 3) }, (_, k) => ({
        u: -0.5 + ((i * 13 + k * 29) % 100) / 100,
        v: -0.4 + (((i * 23 + k * 17) % 100) / 100) * 0.8,
        r: 0.18 + ((i * 5 + k * 11) % 50) / 200,
      })),
      startle: 0,
      dartCooldown: Math.random() * 6,
      bend: 0,
      gulpT: -1,
      gulpAt: performance.now() / 1000 + 6 + Math.random() * 18,
      gulpR1: false,
      gulpR2: false,
    };
  });
}

export interface RippleEmit {
  /** Physical pixel height of the WebGL backing buffer. */
  canvasHeight: number;
  /** CSS-pixel width of the wallpaper-host root (used for stereo pan). */
  rootWidth: number;
  /** CSS-pixel height of the wallpaper-host root (used for GL y-flip). */
  rootHeight: number;
  /** Device pixel ratio used by the WebGL backing buffer. */
  dpr: number;
  /** Local x (CSS pixels, root-relative, top-down). */
  x: number;
  /** Local y (CSS pixels, root-relative, top-down). */
  y: number;
}

function chooseSlot(now: number): number {
  let slot = -1;
  let oldestT = Infinity;
  for (let i = 0; i < WATER_RIPPLE_SLOTS; i++) {
    const r = ripples[i]!;
    const age = now - r.t;
    if (age > 6.5) {
      slot = i;
      break;
    }
    if (r.t < oldestT) {
      oldestT = r.t;
      slot = i;
    }
  }
  if (slot < 0) slot = ripplePtr;
  ripplePtr = (slot + 1) % WATER_RIPPLE_SLOTS;
  return slot;
}

function uvFromLocal(p: RippleEmit) {
  // Shader uses uv.x = px / canvas.height, uv.y = py / canvas.height with
  // py in GL-space (y-up). Flip top-down → bottom-up against the root
  // height, then scale to physical pixels and normalize.
  const px = p.x * p.dpr;
  const py = (p.rootHeight - p.y) * p.dpr;
  return { ux: px / p.canvasHeight, uy: py / p.canvasHeight };
}

/**
 * Push a ripple into the circular buffer. `silent` ripples (rain drops,
 * koi gulp rings) skip the splash sound AND don't spook the school.
 */
function emitRipple(p: RippleEmit, strength: number, silent = false) {
  const now = shaderTimeNow();
  const slot = chooseSlot(now);
  const { ux, uy } = uvFromLocal(p);
  ripples[slot] = { x: ux, y: uy, t: now, s: strength };
  if (!silent) {
    const pan = p.rootWidth > 0 ? p.x / p.rootWidth : undefined;
    if (strength >= 0.2) audio.playPlop(Math.min(1, strength * 1.6), pan);
    else if (strength >= 0.08) audio.playTick(pan);
    for (const fh of fish.value) {
      const dx = fh.x - p.x;
      const dy = fh.y - p.y;
      const d = Math.hypot(dx, dy);
      if (d < 180) {
        fh.startle = Math.min(1, fh.startle + (1 - d / 180) * 0.9);
        fh.targetHeading = Math.atan2(dy, dx);
      }
    }
  }
}

// Rain: gentle silent droplets scattered across the pond. Called from the
// water overlay's frame loop while rain mode is on.
let rainNextT = 0;
function updateRain(geom: Omit<RippleEmit, 'x' | 'y'>) {
  if (!rainMode.value) return;
  const now = shaderTimeNow();
  if (rainNextT < now - 1) rainNextT = now;
  while (now > rainNextT) {
    rainNextT += 0.16 + Math.random() * 0.22;
    emitRipple(
      { ...geom, x: Math.random() * geom.rootWidth, y: Math.random() * geom.rootHeight },
      0.03 + Math.random() * 0.09,
      true,
    );
  }
}

function dropPellet(localX: number, localY: number) {
  pellets.value.push({
    x: localX,
    y: localY,
    vy: 0,
    life: 0,
    max: 12 + Math.random() * 4,
    size: 4 + Math.random() * 1.5,
    bobPhase: Math.random() * Math.PI * 2,
  });
  if (pellets.value.length > 40) pellets.value.shift();
  audio.playPlop(0.7);
}

function castOrReel(p: RippleEmit) {
  const now = performance.now() / 1000;
  if (!bobber.value) {
    bobber.value = {
      x: p.x,
      y: p.y,
      t: now,
      biteAt: now + 3 + Math.random() * 5,
      biteT: -1,
      hookedFish: null,
      casting: 0.5,
      bobPhase: Math.random() * Math.PI * 2,
      lastShake: 0,
    };
    emitRipple(p, 0.45);
  } else if (bobber.value.hookedFish) {
    const f = bobber.value.hookedFish;
    const idx = fish.value.indexOf(f);
    if (idx >= 0) {
      const kindName = KIND_NAMES[idx % KIND_NAMES.length] ?? 'KOI';
      catchLabel.value = `— ${kindName} caught! —`;
      window.setTimeout(() => {
        catchLabel.value = '';
      }, 1500);
      window.setTimeout(() => {
        if (fish.value.indexOf(f) === -1) {
          f.x = Math.random() * (p.rootWidth || p.rootHeight * 1.6);
          f.y = Math.random() * p.rootHeight;
          f.startle = 0;
          fish.value.push(f);
        }
      }, 4000);
      fish.value.splice(idx, 1);
    }
    bobber.value = null;
    audio.playPlop(0.5);
  } else {
    emitRipple({ ...p, x: bobber.value.x, y: bobber.value.y }, 0.2);
    bobber.value = null;
  }
}

/** Mark all ripples as expired so the shader is calm again on entering water mode. */
function resetRipples() {
  for (let i = 0; i < WATER_RIPPLE_SLOTS; i++) {
    const r = ripples[i]!;
    r.t = -100;
    r.s = 0;
  }
}

export function useWaterScene() {
  return {
    ripples,
    fish,
    pellets,
    bubbles,
    bobber,
    catchLabel,
    feedMode,
    fishingMode,
    rainMode,
    emitRipple,
    updateRain,
    dropPellet,
    castOrReel,
    resetRipples,
    setShaderClockOrigin,
    shaderTimeNow,
  };
}
