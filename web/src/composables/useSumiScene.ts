// Suminagashi wallpaper (shader 07) — shared scene state.
//
// Two render modes:
//   - 'fluid'  — GPU stable-fluids ink sim (utils/sumiFluid.ts) on its own
//                canvas layer; smoke-like ink with subtractive color blending.
//   - 'marble' — classic mathematical ring-marbling. Renders through the
//                fluid sim's GL context (context-limit safety); when the GPU
//                lacks half-float FBO support entirely, the analytic FRAG_7
//                fallback on the main shader canvas takes over.
//
// Marble persistence: when the live drop list fills up, the whole pattern
// is baked into a history texture inside the sim and the list is emptied —
// old rings keep warping under new drops instead of vanishing.
//
// Singleton for the same reason as useWaterScene: the wallpaper mounts in
// both the builder and the debugger but only one is visible at a time. The
// fluid sim itself is per-layer (GPU state is tied to a canvas); the layer
// that is currently visible registers its sim here so pointer handlers and
// the control panel reach the right instance.

import { reactive, ref } from 'vue';
import { SUMI_DROP_SLOTS } from '../shaders/fragments';
import { SumiFluidSim, type SumiFluidParams } from '../utils/sumiFluid';

export type SumiMode = 'fluid' | 'marble';

export interface InkDrop {
  x: number;
  y: number;
  r: number;
  target: number;
  t: number;
  col: number;
  instant: boolean;
}

// Selectable inks for the fluid sim. Stored as pigment color; the splat
// gets ABSORPTION = 1 - color, so overlapping inks blend subtractively.
export const INKS: { name: string; swatch: string; rgb: [number, number, number] }[] = [
  { name: 'Sumi black', swatch: '#221e1b', rgb: [0.133, 0.118, 0.106] },
  { name: 'Indigo', swatch: '#2b4a78', rgb: [0.169, 0.29, 0.471] },
  { name: 'Vermillion', swatch: '#c2451e', rgb: [0.761, 0.271, 0.118] },
  { name: 'Ochre', swatch: '#c89031', rgb: [0.784, 0.565, 0.192] },
  { name: 'Pine', swatch: '#2c6b58', rgb: [0.173, 0.42, 0.345] },
  { name: 'Plum', swatch: '#6b3a5b', rgb: [0.42, 0.227, 0.357] },
  { name: 'Sakura', swatch: '#d98a96', rgb: [0.851, 0.541, 0.588] },
  { name: 'Teal', swatch: '#2a7f8a', rgb: [0.165, 0.498, 0.541] },
  { name: 'Moss', swatch: '#5d6b2f', rgb: [0.365, 0.42, 0.184] },
];

const SUMI_MODE_KEY = 'rakitsu-sumi-mode';

function loadMode(): SumiMode {
  try {
    const m = localStorage.getItem(SUMI_MODE_KEY);
    if (m === 'marble' || m === 'fluid') return m;
  } catch {
    /* storage blocked */
  }
  return 'fluid';
}

const sumiMode = ref<SumiMode>(loadMode());
/** -1 = AUTO (hue drifts with time so a dragged stroke ribbons through the spectrum). */
const selectedInk = ref(0);
/** null = not probed yet; set by the first fluid layer's init attempt. */
const fluidSupported = ref<boolean | null>(null);
/** FX slider values, shared by reference with every sim instance. */
const fxParams = reactive<SumiFluidParams>({ speed: 1, swirl: 1, fade: 1, size: 1 });

// The sim belonging to the currently visible wallpaper host.
let activeSim: SumiFluidSim | null = null;
const fluidSeeded = new WeakSet<SumiFluidSim>();

// Marble drop list (shared across hosts, like the water ripple buffer).
const inkDrops: InkDrop[] = [];
let inkParity = 0;
let marbleSeeded = false;

function setMode(m: SumiMode) {
  sumiMode.value = m;
  if (m === 'marble' && !marbleSeeded) {
    marbleSeeded = true;
    seedMarble(window.innerWidth, window.innerHeight);
  }
  try {
    localStorage.setItem(SUMI_MODE_KEY, m);
  } catch {
    /* storage blocked */
  }
}

function registerSim(sim: SumiFluidSim) {
  sim.params = fxParams;
  activeSim = sim;
}

function unregisterSim(sim: SumiFluidSim) {
  if (activeSim === sim) activeSim = null;
}

function getActiveSim(): SumiFluidSim | null {
  return activeSim && activeSim.ready ? activeSim : null;
}

function hslToRgb(h: number, s: number, l: number): [number, number, number] {
  const f = (n: number) => {
    const k = (n + h * 12) % 12;
    return l - s * Math.min(l, 1 - l) * Math.max(-1, Math.min(k - 3, 9 - k, 1));
  };
  return [f(0), f(8), f(4)];
}

/** Absorption color for the currently selected ink (1 - pigment). */
function inkAbs(): [number, number, number] {
  if (selectedInk.value < 0) {
    // AUTO: hue drifts slowly with time — smooth along a stroke, varied per drop
    const hue = (performance.now() * 0.012) % 360;
    const rgb = hslToRgb(hue / 360, 0.62, 0.4);
    return [1 - rgb[0], 1 - rgb[1], 1 - rgb[2]];
  }
  const ink = INKS[selectedInk.value] ?? INKS[0]!;
  return [1 - ink.rgb[0], 1 - ink.rgb[1], 1 - ink.rgb[2]];
}

/**
 * Add a marble drop at local CSS coords. Coordinates normalize against the
 * root height to match the marbling shaders' `frag / res.y` uv space.
 * Persistence: when the live list is full, fold it into the baked history
 * texture instead of discarding the oldest rings.
 */
function addInkDrop(
  cx: number,
  cy: number,
  rootH: number,
  opts: { clear?: boolean; instant?: boolean; r?: number } = {},
) {
  const now = performance.now() / 1000;
  if (inkDrops.length >= SUMI_DROP_SLOTS) {
    const sim = getActiveSim();
    if (sim && sim.bakeMarble(buildMarbleDrops(true), now)) {
      inkDrops.length = 0;
    } else {
      inkDrops.shift(); // fallback context: bounded history
    }
  }
  let colorId = 0; // 0 = clear dispersant
  if (!opts.clear) {
    const rnd = Math.random();
    colorId = rnd < 0.62 ? 1 : rnd < 0.92 ? 2 : 3; // sumi / indigo / vermillion
  }
  const target = opts.r ?? 0.045 + Math.random() * 0.035;
  inkDrops.push({
    x: cx / rootH,
    y: (rootH - cy) / rootH,
    r: opts.instant ? target : 0.0012,
    target,
    t: now,
    col: colorId,
    instant: !!opts.instant,
  });
}

/**
 * Pack the live drop list for the shader. `final=true` uses fully-bloomed
 * radii (for baking); otherwise animates the ease-out bloom.
 */
function buildMarbleDrops(final: boolean): Float32Array {
  const nowS = performance.now() / 1000;
  const dd = new Float32Array(SUMI_DROP_SLOTS * 4);
  for (let j = 0; j < inkDrops.length; j++) {
    const dr = inkDrops[j]!;
    if (!dr.instant && !final) {
      const k = Math.min(1, (nowS - dr.t) / 0.55);
      dr.r = dr.target * (1 - Math.pow(1 - k, 3));
    }
    dd[j * 4 + 0] = dr.x;
    dd[j * 4 + 1] = dr.y;
    dd[j * 4 + 2] = final ? dr.target : dr.r;
    dd[j * 4 + 3] = dr.col;
  }
  return dd;
}

function seedMarble(rootW: number, rootH: number) {
  const scx = rootW * 0.5;
  const scy = rootH * 0.46;
  for (let i = 0; i < 10; i++) {
    addInkDrop(scx + (Math.random() - 0.5) * 10, scy + (Math.random() - 0.5) * 10, rootH, {
      instant: true,
      r: 0.055 + Math.random() * 0.02,
      clear: i % 2 === 1,
    });
  }
  inkParity = 0;
}

/** First-view seeding so the paper opens alive. */
function seedScene(sim: SumiFluidSim | null, rootW: number, rootH: number) {
  if (sim) {
    if (fluidSeeded.has(sim)) return;
    fluidSeeded.add(sim);
    // three soft blooms
    const seeds: [number, number, number][] = [
      [0.42, 0.56, 0],
      [0.58, 0.46, 1],
      [0.5, 0.4, 2],
    ];
    for (const [u, v, k] of seeds) {
      const ink = INKS[k]!;
      sim.bloom(u, v, [1 - ink.rgb[0], 1 - ink.rgb[1], 1 - ink.rgb[2]]);
    }
    return;
  }
  // no fluid support: the marble shader IS the scene
  if (!marbleSeeded) {
    marbleSeeded = true;
    seedMarble(rootW, rootH);
  }
}

/** Alternate ink / clear dispersant — how classic concentric rings are built. */
function nextDropIsClear(): boolean {
  const clear = inkParity % 2 === 1;
  inkParity++;
  return clear;
}

/** Fresh paper for whichever mode is active. */
function clearScene() {
  const sim = getActiveSim();
  if (sim && sumiMode.value === 'fluid' && fluidSupported.value) {
    sim.clear();
    return;
  }
  inkDrops.length = 0;
  if (sim) sim.clearMarble();
}

export function useSumiScene() {
  return {
    sumiMode,
    selectedInk,
    fluidSupported,
    fxParams,
    inkDrops,
    INKS,
    setMode,
    registerSim,
    unregisterSim,
    getActiveSim,
    inkAbs,
    addInkDrop,
    buildMarbleDrops,
    seedScene,
    nextDropIsClear,
    clearScene,
  };
}
