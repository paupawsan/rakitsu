// Wallpaper audio engine — shared by the water (06) and suminagashi (07)
// scenes. All sounds are synthesized with WebAudio (no audio files).
//
// Singleton: one AudioContext for the whole app, lazily created on the
// first user gesture (browsers block audio before interaction). One-shot
// sounds are stereo-panned to where they happen on screen (pan 0..1 across
// the wallpaper width); plops also feed a short lowpassed feedback delay so
// they carry a sense of open air.

import { ref } from 'vue';

const audioMuted = ref(false);

let audioCtx: AudioContext | null = null;
let audioMaster: GainNode | null = null;

function ensureAudio(): AudioContext | null {
  if (audioCtx) return audioCtx;
  try {
    const Ctor =
      window.AudioContext ||
      (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext;
    audioCtx = new Ctor();
    audioMaster = audioCtx.createGain();
    audioMaster.gain.value = audioMuted.value ? 0 : 0.5;
    audioMaster.connect(audioCtx.destination);
    // soft pond-side echo: a short feedback delay, lowpassed and quiet,
    // gives plops a sense of open air without an impulse response
    const dly = audioCtx.createDelay(0.5);
    dly.delayTime.value = 0.16;
    const fb = audioCtx.createGain();
    fb.gain.value = 0.28;
    const dlp = audioCtx.createBiquadFilter();
    dlp.type = 'lowpass';
    dlp.frequency.value = 1200;
    const wet = audioCtx.createGain();
    wet.gain.value = 0.16;
    audioMaster.connect(dly);
    dly.connect(dlp);
    dlp.connect(fb);
    fb.connect(dly);
    dlp.connect(wet);
    wet.connect(audioCtx.destination);
  } catch {
    audioCtx = null;
  }
  return audioCtx;
}

function setMuted(muted: boolean) {
  audioMuted.value = muted;
  if (audioMaster) audioMaster.gain.value = muted ? 0 : 0.5;
}

/** Destination for one-shot sounds — stereo-panned when `pan` (0..1) is given. */
function out(pan?: number): AudioNode | null {
  if (!audioMaster || !audioCtx) return audioMaster;
  if (pan !== undefined && typeof audioCtx.createStereoPanner === 'function') {
    const p = audioCtx.createStereoPanner();
    p.pan.value = Math.max(-1, Math.min(1, pan * 2 - 1)) * 0.7;
    p.connect(audioMaster);
    return p;
  }
  return audioMaster;
}

// Ambient pond bed: looping brown noise, lowpassed, with two slow LFOs
// (one breathing the filter, one breathing the gain) — gentle lapping.
let ambientNodes: { src: AudioBufferSourceNode; lfo: OscillatorNode; lfo2: OscillatorNode; g: GainNode } | null = null;

function startAmbient() {
  if (ambientNodes || audioMuted.value) return;
  const ctx = ensureAudio();
  if (!ctx || !audioMaster) return;
  const sr = ctx.sampleRate;
  const len = sr * 4;
  const buf = ctx.createBuffer(2, len, sr);
  for (let ch = 0; ch < 2; ch++) {
    const d = buf.getChannelData(ch);
    let b = 0;
    for (let i = 0; i < len; i++) {
      b = (b + 0.02 * (Math.random() * 2 - 1)) / 1.02;
      d[i] = b * 3.5;
    }
  }
  const src = ctx.createBufferSource();
  src.buffer = buf;
  src.loop = true;
  const lp = ctx.createBiquadFilter();
  lp.type = 'lowpass';
  lp.frequency.value = 420;
  lp.Q.value = 0.6;
  const g = ctx.createGain();
  g.gain.value = 0.0;
  src.connect(lp);
  lp.connect(g);
  g.connect(audioMaster);
  const lfo = ctx.createOscillator();
  lfo.frequency.value = 0.12;
  const lfoG = ctx.createGain();
  lfoG.gain.value = 160;
  lfo.connect(lfoG);
  lfoG.connect(lp.frequency);
  const lfo2 = ctx.createOscillator();
  lfo2.frequency.value = 0.071;
  const lfo2G = ctx.createGain();
  lfo2G.gain.value = 0.05;
  lfo2.connect(lfo2G);
  lfo2G.connect(g.gain);
  src.start();
  lfo.start();
  lfo2.start();
  g.gain.setTargetAtTime(0.16, ctx.currentTime, 1.2);
  ambientNodes = { src, lfo, lfo2, g };
}

function stopAmbient() {
  if (!ambientNodes || !audioCtx) return;
  const a = ambientNodes;
  ambientNodes = null;
  try {
    a.g.gain.setTargetAtTime(0.0001, audioCtx.currentTime, 0.4);
    window.setTimeout(() => {
      try {
        a.src.stop();
        a.lfo.stop();
        a.lfo2.stop();
      } catch {
        /* already stopped */
      }
    }, 1600);
  } catch {
    /* context closed */
  }
}

// Rain bed: white-noise patter — sparse impulses give individual drop ticks.
let rainNodes: { src: AudioBufferSourceNode; g: GainNode } | null = null;

function startRainSound() {
  if (rainNodes || audioMuted.value) return;
  const ctx = ensureAudio();
  if (!ctx || !audioMaster) return;
  const sr = ctx.sampleRate;
  const len = sr * 3;
  const buf = ctx.createBuffer(2, len, sr);
  for (let ch = 0; ch < 2; ch++) {
    const d = buf.getChannelData(ch);
    for (let i = 0; i < len; i++) {
      const imp = Math.random() < 0.0015 ? (Math.random() * 2 - 1) * 2.2 : 0;
      d[i] = (Math.random() * 2 - 1) * 0.16 + imp;
    }
  }
  const src = ctx.createBufferSource();
  src.buffer = buf;
  src.loop = true;
  const hp = ctx.createBiquadFilter();
  hp.type = 'highpass';
  hp.frequency.value = 900;
  const lp = ctx.createBiquadFilter();
  lp.type = 'lowpass';
  lp.frequency.value = 5200;
  const g = ctx.createGain();
  g.gain.value = 0.0;
  src.connect(hp);
  hp.connect(lp);
  lp.connect(g);
  g.connect(audioMaster);
  src.start();
  g.gain.setTargetAtTime(0.14, ctx.currentTime, 0.8);
  rainNodes = { src, g };
}

function stopRainSound() {
  if (!rainNodes || !audioCtx) return;
  const a = rainNodes;
  rainNodes = null;
  try {
    a.g.gain.setTargetAtTime(0.0001, audioCtx.currentTime, 0.3);
    window.setTimeout(() => {
      try {
        a.src.stop();
      } catch {
        /* already stopped */
      }
    }, 1200);
  } catch {
    /* context closed */
  }
}

// Plop = real water-drop sound — bubble's Minnaert frequency RISES as it
// shrinks (chirp UP, not down — design transcript called this out).
function playPlop(strength: number, pan?: number) {
  if (audioMuted.value) return;
  const ctx = ensureAudio();
  const dest = out(pan);
  if (!ctx || !dest) return;
  const t = ctx.currentTime;
  const f0 = 360 - strength * 130 + (Math.random() - 0.5) * 40;
  const f1 = 880 - strength * 220 + (Math.random() - 0.5) * 60;
  const dur = 0.18 + strength * 0.14;
  const osc = ctx.createOscillator();
  osc.type = 'sine';
  osc.frequency.setValueAtTime(f0, t);
  osc.frequency.exponentialRampToValueAtTime(f1, t + dur * 0.85);
  osc.frequency.exponentialRampToValueAtTime(f1 * 0.9, t + dur);
  const og = ctx.createGain();
  og.gain.setValueAtTime(0.0001, t);
  og.gain.exponentialRampToValueAtTime(0.62 * (0.5 + strength * 0.7), t + 0.008);
  og.gain.exponentialRampToValueAtTime(0.0001, t + dur);
  osc.connect(og);
  og.connect(dest);
  osc.start(t);
  osc.stop(t + dur + 0.02);
  // sharp impact transient — short noise click, lowpassed
  const sr = ctx.sampleRate;
  const len = Math.floor(sr * 0.018);
  const buf = ctx.createBuffer(1, len, sr);
  const d = buf.getChannelData(0);
  for (let i = 0; i < len; i++) {
    d[i] = (Math.random() * 2 - 1) * Math.exp(-i / (sr * 0.004));
  }
  const noise = ctx.createBufferSource();
  noise.buffer = buf;
  const lp = ctx.createBiquadFilter();
  lp.type = 'lowpass';
  lp.frequency.value = 1400;
  const ng = ctx.createGain();
  ng.gain.value = 0.1 * (0.4 + strength * 0.6);
  noise.connect(lp);
  lp.connect(ng);
  ng.connect(dest);
  noise.start(t);
  noise.stop(t + 0.03);
}

function playTick(pan?: number) {
  if (audioMuted.value) return;
  const ctx = ensureAudio();
  const dest = out(pan);
  if (!ctx || !dest) return;
  const t = ctx.currentTime;
  const osc = ctx.createOscillator();
  osc.type = 'sine';
  const f0 = 480 + Math.random() * 140;
  osc.frequency.setValueAtTime(f0, t);
  osc.frequency.exponentialRampToValueAtTime(f0 * 1.7, t + 0.08);
  const g = ctx.createGain();
  g.gain.setValueAtTime(0.0001, t);
  g.gain.exponentialRampToValueAtTime(0.05, t + 0.005);
  g.gain.exponentialRampToValueAtTime(0.0001, t + 0.1);
  osc.connect(g);
  g.connect(dest);
  osc.start(t);
  osc.stop(t + 0.11);
}

function playMunch() {
  if (audioMuted.value) return;
  const ctx = ensureAudio();
  if (!ctx || !audioMaster) return;
  const t = ctx.currentTime;
  for (let i = 0; i < 3; i++) {
    const osc = ctx.createOscillator();
    osc.type = 'sine';
    const f = 500 + Math.random() * 400;
    osc.frequency.setValueAtTime(f, t + i * 0.04);
    osc.frequency.exponentialRampToValueAtTime(f * 0.55, t + i * 0.04 + 0.05);
    const g = ctx.createGain();
    g.gain.setValueAtTime(0.0001, t + i * 0.04);
    g.gain.exponentialRampToValueAtTime(0.08, t + i * 0.04 + 0.005);
    g.gain.exponentialRampToValueAtTime(0.0001, t + i * 0.04 + 0.07);
    osc.connect(g);
    g.connect(audioMaster);
    osc.start(t + i * 0.04);
    osc.stop(t + i * 0.04 + 0.08);
  }
}

// Gulp — two tiny rising sips when a koi mouths the surface.
function playGulp(pan?: number) {
  if (audioMuted.value) return;
  const ctx = ensureAudio();
  const dest = out(pan);
  if (!ctx || !dest) return;
  const t = ctx.currentTime;
  for (let i = 0; i < 2; i++) {
    const osc = ctx.createOscillator();
    osc.type = 'sine';
    const f0 = 300 + Math.random() * 120;
    osc.frequency.setValueAtTime(f0, t + i * 0.09);
    osc.frequency.exponentialRampToValueAtTime(f0 * 1.8, t + i * 0.09 + 0.07);
    const g = ctx.createGain();
    g.gain.setValueAtTime(0.0001, t + i * 0.09);
    g.gain.exponentialRampToValueAtTime(0.035, t + i * 0.09 + 0.008);
    g.gain.exponentialRampToValueAtTime(0.0001, t + i * 0.09 + 0.09);
    osc.connect(g);
    g.connect(dest);
    osc.start(t + i * 0.09);
    osc.stop(t + i * 0.09 + 0.1);
  }
}

// Ink drop — softer and lower than a pond plop; clear dispersant sits higher.
function playInk(isClear: boolean, pan?: number) {
  if (audioMuted.value) return;
  const ctx = ensureAudio();
  const dest = out(pan);
  if (!ctx || !dest) return;
  const t = ctx.currentTime;
  const osc = ctx.createOscillator();
  osc.type = 'sine';
  const f0 = isClear ? 320 : 210;
  osc.frequency.setValueAtTime(f0, t);
  osc.frequency.exponentialRampToValueAtTime(f0 * 1.5, t + 0.16);
  const g = ctx.createGain();
  g.gain.setValueAtTime(0.0001, t);
  g.gain.exponentialRampToValueAtTime(0.1, t + 0.012);
  g.gain.exponentialRampToValueAtTime(0.0001, t + 0.22);
  osc.connect(g);
  g.connect(dest);
  osc.start(t);
  osc.stop(t + 0.24);
}

// Brush swish — a breath of bandpassed noise while combing the ink.
function playSwish() {
  if (audioMuted.value) return;
  const ctx = ensureAudio();
  if (!ctx || !audioMaster) return;
  const t = ctx.currentTime;
  const sr = ctx.sampleRate;
  const len = Math.floor(sr * 0.12);
  const buf = ctx.createBuffer(1, len, sr);
  const d = buf.getChannelData(0);
  for (let i = 0; i < len; i++) d[i] = (Math.random() * 2 - 1) * Math.sin((Math.PI * i) / len);
  const src = ctx.createBufferSource();
  src.buffer = buf;
  const bp = ctx.createBiquadFilter();
  bp.type = 'bandpass';
  bp.frequency.value = 900 + Math.random() * 500;
  bp.Q.value = 1.2;
  const g = ctx.createGain();
  g.gain.value = 0.025;
  src.connect(bp);
  bp.connect(g);
  g.connect(audioMaster);
  src.start(t);
}

export function useWallpaperAudio() {
  return {
    audioMuted,
    ensureAudio,
    setMuted,
    startAmbient,
    stopAmbient,
    startRainSound,
    stopRainSound,
    playPlop,
    playTick,
    playMunch,
    playGulp,
    playInk,
    playSwish,
  };
}
