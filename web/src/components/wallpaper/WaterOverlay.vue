<script setup lang="ts">
// Water shader (06) overlay: koi school + bubbles + pellets + bobber +
// feed/fish-mode reticles, all on a top-most canvas2D layer.
//
// Koi are drawn with an articulated spine: lateral offset is a travelling
// wave (head barely moves, tail sweeps wide and LAGS the body), plus a bend
// term from turning, so the whole fish flexes believably. Calm koi
// occasionally rise and gulp at the surface, leaving silent ripple rings.
//
// Coordinates: top-down CSS pixels relative to the wallpaper-host root.
// We render at devicePixelRatio so strokes stay crisp on retina.
//
// Pointer-events:
//   - default: 'none' so node-graph drags pass through.
//   - feed/fishing modes: 'auto' so the canvas claims clicks (and the
//     custom reticle replaces the cursor). Toggling modes off restores
//     normal IDE interaction.

import { onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useWaterScene, type WaterFish, type WaterPellet } from '../../composables/useWaterScene';
import { useWallpaperAudio } from '../../composables/useWallpaperAudio';

const rootRef = ref<HTMLDivElement | null>(null);
const canvasRef = ref<HTMLCanvasElement | null>(null);
const water = useWaterScene();
const audio = useWallpaperAudio();

let raf: number | null = null;
let lastFrame = 0;
let mouseX = 0;
let mouseY = 0;
let mouseActive = 0;
let smActive = 0;
let resizeObs: ResizeObserver | null = null;

function localFromClient(clientX: number, clientY: number) {
  const root = rootRef.value;
  if (!root) return null;
  const rect = root.getBoundingClientRect();
  const x = clientX - rect.left;
  const y = clientY - rect.top;
  if (x < 0 || y < 0 || x > rect.width || y > rect.height) return null;
  return { x, y, w: rect.width, h: rect.height };
}

function onMouseMove(e: MouseEvent | TouchEvent) {
  const t = 'touches' in e ? e.touches[0] : (e as MouseEvent);
  if (!t) return;
  const local = localFromClient(t.clientX, t.clientY);
  if (!local) {
    mouseActive = 0;
    return;
  }
  mouseX = local.x;
  mouseY = local.y;
  mouseActive = 1;
}

/** Ripple-emit geometry matching the WebGL canvas (same root, same DPR cap). */
function emitGeom(w: number, h: number) {
  const dpr = Math.min(window.devicePixelRatio || 1, 2);
  return {
    canvasHeight: Math.max(1, Math.floor(h * dpr)),
    rootWidth: w,
    rootHeight: h,
    dpr,
  };
}

function resize() {
  const c = canvasRef.value;
  const root = rootRef.value;
  if (!c || !root) return;
  const dpr = Math.min(window.devicePixelRatio || 1, 2);
  const rect = root.getBoundingClientRect();
  c.width = Math.max(1, Math.floor(rect.width * dpr));
  c.height = Math.max(1, Math.floor(rect.height * dpr));
  c.style.width = rect.width + 'px';
  c.style.height = rect.height + 'px';
  const ctx = c.getContext('2d');
  if (ctx) ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  // keep fish inside new bounds
  const margin = 40;
  for (const f of water.fish.value) {
    if (f.x < margin) f.x = margin;
    if (f.y < margin) f.y = margin;
    if (f.x > rect.width - margin) f.x = rect.width - margin;
    if (f.y > rect.height - margin) f.y = rect.height - margin;
  }
}

const SEG = 10;
// koi width profile: blunt head, widest at the shoulders, taper to peduncle
const KOI_WIDTHS = [0.175, 0.26, 0.305, 0.315, 0.3, 0.262, 0.214, 0.158, 0.108, 0.072];

function hexA(hex: string, a: number): string {
  return (
    'rgba(' +
    parseInt(hex.slice(1, 3), 16) +
    ',' +
    parseInt(hex.slice(3, 5), 16) +
    ',' +
    parseInt(hex.slice(5, 7), 16) +
    ',' +
    a +
    ')'
  );
}

function drawKoi(ctx: CanvasRenderingContext2D, f: WaterFish, gulpA: number) {
  // depth illusion + underwater color cast (gulp lifts the fish)
  const bob = Math.sin(f.bobPhase) * f.bobAmp;
  const depthScale = (1 + bob * 0.07) * (1 + gulpA * 0.12);
  const depthAlpha = Math.min(1, 0.62 + bob * 0.12 + gulpA * 0.3);
  ctx.save();
  ctx.translate(f.x, f.y);
  ctx.rotate(f.heading);
  ctx.scale(depthScale, depthScale);
  const s = f.size;
  ctx.globalAlpha = depthAlpha;

  // articulated spine — travelling wave, tail lags, bend follows turns
  const swimEff = Math.max(0, f.speedMul - 1);
  const amp = s * (0.085 + swimEff * 0.05 + f.startle * 0.06);
  const spine: { x: number; y: number }[] = [];
  for (let sg = 0; sg < SEG; sg++) {
    const u = sg / (SEG - 1);
    spine.push({
      x: s * (0.92 - u * 1.8),
      y: Math.sin(f.phase - u * 2.7) * amp * (0.12 + Math.pow(u, 1.5) * 1.35) + f.bend * s * 0.6 * u * u,
    });
  }
  const topE: { x: number; y: number }[] = [];
  const botE: { x: number; y: number }[] = [];
  for (let sg = 0; sg < SEG; sg++) {
    const pa = spine[Math.max(0, sg - 1)]!;
    const pb = spine[Math.min(SEG - 1, sg + 1)]!;
    let nx = -(pb.y - pa.y);
    let ny = pb.x - pa.x;
    const nl = Math.hypot(nx, ny) || 1;
    nx /= nl;
    ny /= nl;
    const hw = KOI_WIDTHS[sg]! * s;
    const sp = spine[sg]!;
    topE.push({ x: sp.x + nx * hw, y: sp.y + ny * hw });
    botE.push({ x: sp.x - nx * hw, y: sp.y - ny * hw });
  }
  const bodyPath = new Path2D();
  const nose = { x: spine[0]!.x + s * 0.14, y: spine[0]!.y };
  bodyPath.moveTo(nose.x, nose.y);
  bodyPath.quadraticCurveTo(nose.x - s * 0.01, topE[0]!.y, topE[0]!.x, topE[0]!.y);
  for (let sg = 1; sg < SEG; sg++) {
    const mxp = (topE[sg - 1]!.x + topE[sg]!.x) / 2;
    const myp = (topE[sg - 1]!.y + topE[sg]!.y) / 2;
    bodyPath.quadraticCurveTo(topE[sg - 1]!.x, topE[sg - 1]!.y, mxp, myp);
  }
  bodyPath.lineTo(topE[SEG - 1]!.x, topE[SEG - 1]!.y);
  bodyPath.lineTo(botE[SEG - 1]!.x, botE[SEG - 1]!.y);
  for (let sg = SEG - 2; sg >= 0; sg--) {
    const mxp = (botE[sg + 1]!.x + botE[sg]!.x) / 2;
    const myp = (botE[sg + 1]!.y + botE[sg]!.y) / 2;
    bodyPath.quadraticCurveTo(botE[sg + 1]!.x, botE[sg + 1]!.y, mxp, myp);
  }
  bodyPath.quadraticCurveTo(botE[0]!.x, botE[0]!.y, nose.x, nose.y);
  bodyPath.closePath();

  // tail fin — lags the body wave by ~a full beat, flares on darts
  const ped = spine[SEG - 1]!;
  const sw = Math.sin(f.phase - 3.6) * amp * 2.4 + f.bend * s * 0.85;
  const tl = s * (0.8 + swimEff * 0.06);
  const tailPath = new Path2D();
  tailPath.moveTo(ped.x + s * 0.05, ped.y - s * 0.07);
  tailPath.quadraticCurveTo(ped.x - tl * 0.5, ped.y - s * 0.36 + sw * 0.45, ped.x - tl, ped.y - s * 0.42 + sw);
  tailPath.quadraticCurveTo(ped.x - tl * 0.58, ped.y + sw * 0.55, ped.x - tl * 0.94, ped.y + s * 0.42 + sw);
  tailPath.quadraticCurveTo(ped.x - tl * 0.5, ped.y + s * 0.36 + sw * 0.45, ped.x + s * 0.05, ped.y + s * 0.07);
  tailPath.closePath();

  // cast shadow on the reef floor — offset in WORLD space so the sun
  // stays in one place no matter which way the fish is heading
  ctx.save();
  ctx.rotate(-f.heading);
  ctx.translate(-s * 0.14, s * 0.34 - bob * s * 0.06);
  ctx.rotate(f.heading);
  ctx.fillStyle = 'rgba(0,12,24,0.22)';
  ctx.fill(bodyPath);
  ctx.fill(tailPath);
  ctx.restore();

  // tail — translucent membrane with a faint ink edge
  ctx.fillStyle = hexA(f.kind.body, 0.72);
  ctx.fill(tailPath);
  ctx.strokeStyle = 'rgba(25,18,14,0.14)';
  ctx.lineWidth = 0.8;
  ctx.stroke(tailPath);

  // pectoral fins — swept back from the shoulders, slow sculling flap
  const finFlap = Math.sin(f.phase * 0.85 + 1.2) * 0.3;
  ctx.fillStyle = hexA(f.kind.body, 0.55);
  for (const sideF of [-1, 1]) {
    const base = sideF < 0 ? topE[2]! : botE[2]!;
    ctx.save();
    ctx.translate(base.x, base.y);
    ctx.rotate(sideF * (2.45 + finFlap));
    ctx.beginPath();
    ctx.ellipse(s * 0.26, 0, s * 0.3, s * 0.115, 0, 0, Math.PI * 2);
    ctx.fill();
    ctx.restore();
  }

  // body
  ctx.fillStyle = f.kind.body;
  ctx.fill(bodyPath);

  // pattern + shading clipped to the flexing body
  ctx.save();
  ctx.clip(bodyPath);
  for (let sp2 = 0; sp2 < f.spots.length; sp2++) {
    const sp = f.spots[sp2];
    if (!sp) continue;
    const su = Math.min(0.96, Math.max(0.04, 0.5 - sp.u * 0.9));
    const fIdx = su * (SEG - 1);
    const i0 = Math.floor(fIdx);
    const i1 = Math.min(SEG - 1, i0 + 1);
    const ftr = fIdx - i0;
    const sx2 = spine[i0]!.x + (spine[i1]!.x - spine[i0]!.x) * ftr;
    const sy2 = spine[i0]!.y + (spine[i1]!.y - spine[i0]!.y) * ftr;
    const rr2 = sp.r * s;
    ctx.fillStyle = sp2 % 3 === 2 && f.kind.spot2 ? f.kind.spot2 : f.kind.spot;
    ctx.beginPath();
    ctx.ellipse(sx2, sy2 + sp.v * s * 0.3, rr2, rr2 * 0.8, 0, 0, Math.PI * 2);
    ctx.fill();
  }
  // top-light sheen across the back, shade toward the belly side
  const sheenG = ctx.createLinearGradient(0, -s * 0.34, 0, s * 0.34);
  sheenG.addColorStop(0, 'rgba(255,255,255,0.22)');
  sheenG.addColorStop(0.45, 'rgba(255,255,255,0.02)');
  sheenG.addColorStop(1, 'rgba(0,0,0,0.16)');
  ctx.fillStyle = sheenG;
  ctx.fill(bodyPath);
  // dorsal midline traces the spine
  ctx.strokeStyle = 'rgba(35,25,20,0.16)';
  ctx.lineWidth = s * 0.045;
  ctx.beginPath();
  ctx.moveTo(spine[2]!.x, spine[2]!.y);
  for (let sg = 3; sg < SEG; sg++) ctx.lineTo(spine[sg]!.x, spine[sg]!.y);
  ctx.stroke();
  ctx.restore();

  // gill cover — flares gently with each breath
  const breath = Math.sin(f.breathPhase) * 0.5 + 0.5;
  ctx.strokeStyle = `rgba(40,18,12,${0.4 * depthAlpha})`;
  ctx.lineWidth = 1;
  ctx.beginPath();
  ctx.moveTo(s * 0.32, -s * 0.23);
  ctx.quadraticCurveTo(s * 0.21 - breath * s * 0.04, spine[0]!.y * 0.3, s * 0.32, s * 0.23);
  ctx.stroke();

  // eyes — seen from above, one each side of the head
  for (const sideE of [-1, 1]) {
    ctx.fillStyle = 'rgba(15,15,22,0.92)';
    ctx.beginPath();
    ctx.arc(s * 0.58, spine[0]!.y * 0.5 + sideE * s * 0.155, s * 0.062, 0, Math.PI * 2);
    ctx.fill();
    ctx.fillStyle = 'rgba(255,255,255,0.6)';
    ctx.beginPath();
    ctx.arc(s * 0.6, spine[0]!.y * 0.5 + sideE * s * 0.135, s * 0.02, 0, Math.PI * 2);
    ctx.fill();
  }

  // mouth — breathing sips; gapes into an O at the surface mid-gulp
  const jaw = Math.max(0, Math.sin(f.breathPhase)) * s * 0.05;
  if (gulpA > 0.25) {
    ctx.fillStyle = `rgba(70,22,16,${0.5 + gulpA * 0.4})`;
    ctx.beginPath();
    ctx.arc(nose.x - s * 0.03, nose.y, s * (0.05 + gulpA * 0.085), 0, Math.PI * 2);
    ctx.fill();
    ctx.strokeStyle = 'rgba(255,255,255,0.25)';
    ctx.lineWidth = 1;
    ctx.stroke();
  } else {
    ctx.fillStyle = 'rgba(70,22,16,0.65)';
    ctx.beginPath();
    ctx.ellipse(nose.x - s * 0.03, nose.y, s * 0.045, jaw + s * 0.012, 0, 0, Math.PI * 2);
    ctx.fill();
  }

  ctx.restore();
}

function frame(now: number) {
  raf = null;
  const c = canvasRef.value;
  const root = rootRef.value;
  if (!c || !root) return;
  const ctx = c.getContext('2d');
  if (!ctx) return;
  const rect = root.getBoundingClientRect();
  const w = rect.width;
  const h = rect.height;
  const dt = Math.min(0.05, lastFrame ? (now - lastFrame) / 1000 : 0);
  lastFrame = now;
  smActive += (mouseActive - smActive) * 0.08;

  ctx.clearRect(0, 0, w, h);

  // rain — gentle silent droplets scattered across the pond
  water.updateRain(emitGeom(w, h));

  // pellets — sink with gravity
  for (let pi = water.pellets.value.length - 1; pi >= 0; pi--) {
    const p = water.pellets.value[pi];
    if (!p) continue;
    p.life += dt;
    p.vy += 6 * dt;
    p.vy = Math.min(p.vy, 14);
    p.y += p.vy * dt;
    p.bobPhase += dt * 1.2;
    p.x += Math.sin(p.bobPhase) * 0.4;
    if (p.life > p.max || p.y > h - 20) water.pellets.value.splice(pi, 1);
  }

  // fish update + draw
  for (let fi = 0; fi < water.fish.value.length; fi++) {
    const f = water.fish.value[fi];
    if (!f) continue;

    // cursor proximity → strong startle (only outside feed mode)
    if (smActive > 0.3 && !water.feedMode.value) {
      const dx0 = f.x - mouseX;
      const dy0 = f.y - mouseY;
      const d0 = Math.hypot(dx0, dy0);
      if (d0 < 110) {
        const closeness = 1 - d0 / 110;
        f.startle = Math.min(1, f.startle + closeness * 0.85 * dt * 6);
        f.targetHeading = Math.atan2(dy0, dx0) + (Math.random() - 0.5) * 0.6;
      }
    }

    // food seeking
    let foodTarget: WaterPellet | null = null;
    let foodDist = Infinity;
    for (const p of water.pellets.value) {
      const dx = p.x - f.x;
      const dy = p.y - f.y;
      const d = Math.hypot(dx, dy);
      if (d < 320 && d < foodDist) {
        foodDist = d;
        foodTarget = p;
      }
    }
    if (foodTarget && f.startle < 0.3) {
      f.targetHeading = Math.atan2(foodTarget.y - f.y, foodTarget.x - f.x);
      f.speedMul = Math.max(f.speedMul, 1.8);
      if (foodDist < f.size * 0.9) {
        const idx = water.pellets.value.indexOf(foodTarget);
        if (idx >= 0) water.pellets.value.splice(idx, 1);
        audio.playMunch();
        f.phase += 0.8;
        f.speedMul = 2.0;
      }
    }

    // schooling: weak alignment + cohesion + separation
    let alignX = 0,
      alignY = 0,
      cohX = 0,
      cohY = 0,
      sepX = 0,
      sepY = 0,
      n = 0;
    for (let gi = 0; gi < water.fish.value.length; gi++) {
      if (gi === fi) continue;
      const g = water.fish.value[gi];
      if (!g) continue;
      const dx = g.x - f.x;
      const dy = g.y - f.y;
      const d2 = dx * dx + dy * dy;
      if (d2 < 22500) {
        n++;
        alignX += Math.cos(g.heading);
        alignY += Math.sin(g.heading);
        cohX += g.x;
        cohY += g.y;
        if (d2 < 2500) {
          const inv = 1 / Math.max(8, Math.sqrt(d2));
          sepX -= dx * inv;
          sepY -= dy * inv;
        }
      }
    }

    if (!foodTarget && Math.random() < 0.012) {
      f.targetHeading = f.heading + (Math.random() - 0.5) * 1.2;
    }
    if (n > 0 && f.startle < 0.2 && !foodTarget) {
      const ax = alignX / n;
      const ay = alignY / n;
      const cx2 = cohX / n - f.x;
      const cy2 = cohY / n - f.y;
      const desX = ax * 0.6 + cx2 * 0.002 + sepX * 0.6;
      const desY = ay * 0.6 + cy2 * 0.002 + sepY * 0.6;
      if (desX * desX + desY * desY > 0.05) {
        f.targetHeading = Math.atan2(desY, desX);
      }
    }

    f.dartCooldown -= dt;
    if (f.dartCooldown <= 0) {
      f.speedMul = 2.2 + Math.random();
      f.dartCooldown = 4 + Math.random() * 8;
    }
    f.speedMul += (1 - f.speedMul) * Math.min(1, dt * 1.6);

    // edge avoidance
    const margin = 80;
    if (f.x < margin) f.targetHeading = 0 + (Math.random() - 0.5) * 0.4;
    if (f.x > w - margin) f.targetHeading = Math.PI + (Math.random() - 0.5) * 0.4;
    if (f.y < margin) f.targetHeading = Math.PI / 2 + (Math.random() - 0.5) * 0.4;
    if (f.y > h - margin) f.targetHeading = -Math.PI / 2 + (Math.random() - 0.5) * 0.4;

    let dh = f.targetHeading - f.heading;
    while (dh > Math.PI) dh -= Math.PI * 2;
    while (dh < -Math.PI) dh += Math.PI * 2;
    const turnRate = 1.6 + f.startle * 4 + (foodTarget ? 1.2 : 0);
    f.heading += dh * Math.min(1, dt * turnRate);
    // body bend follows the turn — carried through the spine renderer
    f.bend += (Math.max(-0.7, Math.min(0.7, dh * 1.4)) - f.bend) * Math.min(1, dt * 3.5);

    const startleBoost = 1 + f.startle * 1.6;
    const speed = f.cruise * f.speedMul * startleBoost;
    f.x += Math.cos(f.heading) * speed * dt;
    f.y += Math.sin(f.heading) * speed * dt;
    f.startle = Math.max(0, f.startle - dt * 1.2);
    f.phase += dt * (5 + f.startle * 8 + (f.speedMul - 1) * 3);
    f.breathPhase += dt * f.breathRate * (1 + f.startle * 1.5 + (f.speedMul - 1) * 0.6);
    f.bobPhase += dt * 0.6;

    // bubble emission
    const tNow = performance.now() / 1000;
    if (tNow > f.bubbleAt) {
      const cnt = 1 + Math.floor(Math.random() * 3);
      for (let bi = 0; bi < cnt; bi++) {
        const ox = Math.cos(f.heading) * f.size * 0.95 + (Math.random() - 0.5) * 4;
        const oy = Math.sin(f.heading) * f.size * 0.95 + (Math.random() - 0.5) * 4;
        water.bubbles.value.push({
          x: f.x + ox,
          y: f.y + oy,
          vy: -8 - Math.random() * 10,
          size: 1.5 + Math.random() * 2.5,
          life: 0,
          max: 2 + Math.random() * 1.5,
          drift: Math.random() * Math.PI * 2,
        });
      }
      const baseGap = 5 + Math.random() * 7;
      f.bubbleAt = tNow + baseGap / (1 + f.startle * 1.5 + (f.speedMul - 1) * 0.4);
      if (water.bubbles.value.length > 80) {
        water.bubbles.value.splice(0, water.bubbles.value.length - 80);
      }
    }

    // surface gulp: a calm koi drifts up and sips at the surface, leaving
    // little expanding rings where its mouth breaks the water
    let gulpA = 0;
    if (f.gulpT < 0 && tNow > f.gulpAt && f.startle < 0.15 && !foodTarget) {
      f.gulpT = tNow;
      f.gulpR1 = false;
      f.gulpR2 = false;
    }
    if (f.gulpT > 0) {
      const ga = tNow - f.gulpT;
      if (ga < 1.7) {
        gulpA = Math.sin((Math.PI * ga) / 1.7);
        f.speedMul = Math.min(f.speedMul, 0.45);
        const gmx = f.x + Math.cos(f.heading) * f.size;
        const gmy = f.y + Math.sin(f.heading) * f.size;
        if (!f.gulpR1 && ga > 0.4) {
          f.gulpR1 = true;
          water.emitRipple({ ...emitGeom(w, h), x: gmx, y: gmy }, 0.055, true);
          audio.playGulp(gmx / Math.max(1, w));
        }
        if (!f.gulpR2 && ga > 0.95) {
          f.gulpR2 = true;
          water.emitRipple({ ...emitGeom(w, h), x: gmx, y: gmy }, 0.075, true);
        }
      } else {
        f.gulpT = -1;
        f.gulpAt = tNow + 12 + Math.random() * 24;
      }
    }

    drawKoi(ctx, f, gulpA);
  }

  // bubbles — rise, wobble, pop
  for (let bi = water.bubbles.value.length - 1; bi >= 0; bi--) {
    const b = water.bubbles.value[bi];
    if (!b) continue;
    b.life += dt;
    b.drift += dt * 3;
    b.vy *= 0.99;
    b.x += Math.sin(b.drift) * 12 * dt;
    b.y += b.vy * dt;
    if (b.life > b.max || b.y < -10) {
      water.bubbles.value.splice(bi, 1);
      continue;
    }
    const fade = Math.min(1, (b.max - b.life) / 0.6);
    ctx.strokeStyle = `rgba(220,240,255,${0.55 * fade})`;
    ctx.lineWidth = 0.8;
    ctx.beginPath();
    ctx.arc(b.x, b.y, b.size, 0, Math.PI * 2);
    ctx.stroke();
    ctx.fillStyle = `rgba(255,255,255,${0.3 * fade})`;
    ctx.beginPath();
    ctx.arc(b.x - b.size * 0.3, b.y - b.size * 0.3, b.size * 0.35, 0, Math.PI * 2);
    ctx.fill();
  }

  // pellets after fish
  ctx.globalAlpha = 1;
  for (const p of water.pellets.value) {
    const fade = p.life > p.max - 2 ? Math.max(0, (p.max - p.life) / 2) : 1;
    ctx.fillStyle = `rgba(232,162,83,${0.18 * fade})`;
    ctx.beginPath();
    ctx.arc(p.x, p.y, p.size * 2.2, 0, Math.PI * 2);
    ctx.fill();
    ctx.fillStyle = `rgba(212,134,55,${0.95 * fade})`;
    ctx.beginPath();
    ctx.arc(p.x, p.y, p.size, 0, Math.PI * 2);
    ctx.fill();
    ctx.fillStyle = `rgba(255,225,180,${0.7 * fade})`;
    ctx.beginPath();
    ctx.arc(p.x - p.size * 0.3, p.y - p.size * 0.3, p.size * 0.35, 0, Math.PI * 2);
    ctx.fill();
  }

  // fishing: bobber + line
  if (water.fishingMode.value && water.bobber.value) {
    const b = water.bobber.value;
    const tNow = performance.now() / 1000;
    b.bobPhase += dt * 2.2;

    // attract nearest fish toward bobber as bite time approaches
    if (!b.hookedFish && tNow > b.biteAt - 2) {
      let best: WaterFish | null = null;
      let bestD = Infinity;
      for (const f of water.fish.value) {
        const d = Math.hypot(f.x - b.x, f.y - b.y);
        if (d < 350 && d < bestD) {
          bestD = d;
          best = f;
        }
      }
      if (best) {
        best.targetHeading = Math.atan2(b.y - best.y, b.x - best.x);
        best.speedMul = Math.max(best.speedMul, 1.6);
        if (bestD < best.size && tNow >= b.biteAt) {
          b.hookedFish = best;
          b.biteT = tNow;
          audio.playPlop(0.4, b.x / Math.max(1, w));
        }
      }
    }

    // hooked struggle — shakes the bobber and trembles the surface
    if (b.hookedFish) {
      const f = b.hookedFish;
      const struggle = Math.sin(tNow * 6) * 28;
      f.x = b.x + Math.cos(tNow * 2) * struggle;
      f.y = b.y + Math.sin(tNow * 4) * struggle * 0.6;
      f.startle = 1;
      if (tNow - b.lastShake > 0.4) {
        b.lastShake = tNow;
        water.emitRipple({ ...emitGeom(w, h), x: b.x, y: b.y }, 0.12);
      }
    }

    // line from rod tip (bottom-right of root) to bobber
    const rodX = w - 40;
    const rodY = h - 40;
    ctx.save();
    ctx.strokeStyle = 'rgba(240,240,250,0.55)';
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(rodX, rodY);
    const midX = (rodX + b.x) / 2;
    const midY = (rodY + b.y) / 2 + 18;
    ctx.quadraticCurveTo(midX, midY, b.x, b.y);
    ctx.stroke();
    const bob = Math.sin(b.bobPhase) * 1.5;
    ctx.translate(b.x, b.y + bob);
    ctx.fillStyle = 'rgba(0,0,0,0.25)';
    ctx.beginPath();
    ctx.ellipse(0, 4, 9, 4, 0, 0, Math.PI * 2);
    ctx.fill();
    ctx.fillStyle = '#d8451f';
    ctx.beginPath();
    ctx.arc(0, -3, 7, Math.PI, 0);
    ctx.fill();
    ctx.fillStyle = '#f5ede0';
    ctx.beginPath();
    ctx.arc(0, -3, 7, 0, Math.PI);
    ctx.fill();
    ctx.strokeStyle = '#d8451f';
    ctx.lineWidth = 1.5;
    ctx.beginPath();
    ctx.moveTo(0, -10);
    ctx.lineTo(0, -18);
    ctx.stroke();
    if (b.hookedFish) {
      const pulse = Math.sin(tNow * 14) * 0.5 + 0.5;
      ctx.strokeStyle = `rgba(255,200,80,${0.6 + pulse * 0.4})`;
      ctx.lineWidth = 2;
      ctx.beginPath();
      ctx.arc(0, -3, 12 + pulse * 4, 0, Math.PI * 2);
      ctx.stroke();
    }
    ctx.restore();
  }

  // mode reticles
  if (water.fishingMode.value && !water.bobber.value) {
    ctx.save();
    ctx.translate(mouseX, mouseY);
    ctx.strokeStyle = 'rgba(91,141,239,0.85)';
    ctx.lineWidth = 1.5;
    ctx.beginPath();
    ctx.moveTo(-12, 0);
    ctx.lineTo(-4, 0);
    ctx.moveTo(4, 0);
    ctx.lineTo(12, 0);
    ctx.moveTo(0, -12);
    ctx.lineTo(0, -4);
    ctx.moveTo(0, 4);
    ctx.lineTo(0, 12);
    ctx.stroke();
    ctx.beginPath();
    ctx.arc(0, 0, 3, 0, Math.PI * 2);
    ctx.stroke();
    ctx.restore();
  } else if (water.fishingMode.value && water.bobber.value && !water.bobber.value.hookedFish) {
    ctx.save();
    ctx.fillStyle = 'rgba(180,200,230,0.55)';
    ctx.font = '10px var(--font-mono, "SF Mono", ui-monospace, Menlo, monospace)';
    ctx.fillText('click to reel', water.bobber.value.x + 18, water.bobber.value.y - 6);
    ctx.restore();
  }
  if (water.feedMode.value) {
    ctx.save();
    ctx.translate(mouseX, mouseY);
    ctx.strokeStyle = 'rgba(232,162,83,0.85)';
    ctx.lineWidth = 1.5;
    ctx.beginPath();
    ctx.arc(0, 0, 14, 0, Math.PI * 2);
    ctx.stroke();
    ctx.fillStyle = 'rgba(212,134,55,0.95)';
    ctx.beginPath();
    ctx.arc(0, 0, 4, 0, Math.PI * 2);
    ctx.fill();
    ctx.strokeStyle = 'rgba(232,162,83,0.4)';
    ctx.lineWidth = 1;
    for (let a = 0; a < 4; a++) {
      const ang = (a * Math.PI) / 2;
      ctx.beginPath();
      ctx.moveTo(Math.cos(ang) * 18, Math.sin(ang) * 18);
      ctx.lineTo(Math.cos(ang) * 22, Math.sin(ang) * 22);
      ctx.stroke();
    }
    ctx.restore();
  }

  raf = requestAnimationFrame(frame);
}

function start() {
  if (raf !== null) return;
  lastFrame = 0;
  raf = requestAnimationFrame(frame);
}
function stop() {
  if (raf !== null) {
    cancelAnimationFrame(raf);
    raf = null;
  }
}

onMounted(() => {
  resize();
  resizeObs = new ResizeObserver(resize);
  if (rootRef.value) resizeObs.observe(rootRef.value);
  // capture phase — the Vue Flow pane stops propagation during drags
  window.addEventListener('mousemove', onMouseMove, { capture: true, passive: true });
  window.addEventListener('touchmove', onMouseMove, { capture: true, passive: true });
  start();
});
onBeforeUnmount(() => {
  stop();
  if (resizeObs) resizeObs.disconnect();
  window.removeEventListener('mousemove', onMouseMove as EventListener, { capture: true });
  window.removeEventListener('touchmove', onMouseMove as EventListener, { capture: true });
});

watch(
  () => document.visibilityState,
  () => {
    if (document.visibilityState === 'visible') start();
    else stop();
  },
);
</script>

<template>
  <div ref="rootRef" class="water-overlay" aria-hidden="true">
    <canvas
      ref="canvasRef"
      class="water-canvas"
      :class="{ 'mode-active': water.feedMode.value || water.fishingMode.value }"
    />
    <Transition name="catch-fade">
      <div v-if="water.catchLabel.value" class="catch-banner">
        {{ water.catchLabel.value }}
      </div>
    </Transition>
  </div>
</template>

<style scoped>
.water-overlay {
  position: absolute;
  inset: 0;
  pointer-events: none;
}

.water-canvas {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  pointer-events: none;
}

/* When feed/fishing mode is active, the canvas claims pointer events so
   the custom cursor reticle replaces the OS pointer. The shader composable
   listens at window level, so clicks still emit ripples. */
.water-canvas.mode-active {
  pointer-events: auto;
  cursor: none;
}

.catch-banner {
  position: absolute;
  top: 22%;
  left: 50%;
  transform: translateX(-50%);
  padding: 10px 20px;
  border-radius: 6px;
  background: rgba(15, 28, 48, 0.62);
  border: 1px solid rgba(232, 162, 83, 0.55);
  color: #fcd9a8;
  font-family: var(--font-mono, 'SF Mono', ui-monospace, Menlo, monospace);
  font-size: 13px;
  letter-spacing: 0.08em;
  pointer-events: none;
  box-shadow: 0 6px 26px rgba(0, 0, 0, 0.55);
}

.catch-fade-enter-active,
.catch-fade-leave-active {
  transition: opacity 0.22s ease, transform 0.22s ease;
}
.catch-fade-enter-from,
.catch-fade-leave-to {
  opacity: 0;
  transform: translate(-50%, 4px);
}
</style>
