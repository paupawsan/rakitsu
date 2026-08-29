import { onBeforeUnmount, onActivated, onDeactivated, watch, type Ref } from 'vue';
import { VERT, FRAGS, WATER_INDEX, WATER_RIPPLE_SLOTS, SUMI_INDEX } from '../shaders/fragments';
import { useWaterScene } from './useWaterScene';
import { useSumiScene } from './useSumiScene';
import { useWallpaperAudio } from './useWallpaperAudio';

interface ShaderWallpaperOpts {
  /** The <canvas> element the shader renders into. */
  canvasRef: Ref<HTMLCanvasElement | null>;
  /** Container element used for mouse coordinate translation (the canvas itself is pointer-events:none so VueFlow keeps drags). */
  rootRef: Ref<HTMLElement | null>;
  shaderIndex: Ref<number>;
  dim: Ref<number>;
  enabled: Ref<boolean>;
}

interface ProgramUniforms {
  program: WebGLProgram;
  res: WebGLUniformLocation | null;
  mouse: WebGLUniformLocation | null;
  time: WebGLUniformLocation | null;
  click: WebGLUniformLocation | null;
  active: WebGLUniformLocation | null;
  ripples: WebGLUniformLocation | null;
  drops: WebGLUniformLocation | null;
}

export function useShaderWallpaper(opts: ShaderWallpaperOpts) {
  const { canvasRef, rootRef, shaderIndex, dim, enabled } = opts;

  const water = useWaterScene();
  const sumi = useSumiScene();
  const audio = useWallpaperAudio();

  let gl: WebGLRenderingContext | null = null;
  let programs: ProgramUniforms[] = [];
  let vbo: WebGLBuffer | null = null;
  let rafId: number | null = null;
  let installedCanvas: HTMLCanvasElement | null = null;
  let t0 = 0;
  let mx = 0;
  let my = 0;
  let smx = 0;
  let smy = 0;
  let targetActive = 0;
  let smActive = 0;
  let click = { x: 0, y: 0, t: -100, s: 0 };
  let initialized = false;
  let paused = false; // set by onDeactivated (keep-alive)
  // Reusable Float32Array for the water shader's u_ripples uniform.
  // Allocated once so the per-frame upload doesn't churn the GC.
  const rippleUniform = new Float32Array(WATER_RIPPLE_SLOTS * 4);

  // Drag-wake throttle for the water shader. Distance + time gate from the
  // design (8px / 60ms) — both must hit, so a long press doesn't pile up
  // ripples in one spot. Wake strength 0.035 keeps the tail subtle.
  let isMouseDown = false;
  let lastDragX = -9999;
  let lastDragY = -9999;
  let lastDragT = 0;

  // Suminagashi pointer state: ink-trail gate, hover-stir rate limit.
  let lastInkX = -9999;
  let lastInkY = -9999;
  let lastPointerX = -9999;
  let lastPointerY = -9999;
  let lastStirT = 0;

  function compile(ctx: WebGLRenderingContext, type: number, src: string): WebGLShader | null {
    const s = ctx.createShader(type);
    if (!s) return null;
    ctx.shaderSource(s, src);
    ctx.compileShader(s);
    if (!ctx.getShaderParameter(s, ctx.COMPILE_STATUS)) {
      console.error('[shader] compile error:', ctx.getShaderInfoLog(s));
      ctx.deleteShader(s);
      return null;
    }
    return s;
  }

  function makeProgram(ctx: WebGLRenderingContext, frag: string): ProgramUniforms | null {
    const v = compile(ctx, ctx.VERTEX_SHADER, VERT);
    const f = compile(ctx, ctx.FRAGMENT_SHADER, frag);
    if (!v || !f) return null;
    const p = ctx.createProgram();
    if (!p) return null;
    ctx.attachShader(p, v);
    ctx.attachShader(p, f);
    ctx.linkProgram(p);
    if (!ctx.getProgramParameter(p, ctx.LINK_STATUS)) {
      console.error('[shader] link error:', ctx.getProgramInfoLog(p));
      ctx.deleteProgram(p);
      return null;
    }
    return {
      program: p,
      res: ctx.getUniformLocation(p, 'u_res'),
      mouse: ctx.getUniformLocation(p, 'u_mouse'),
      time: ctx.getUniformLocation(p, 'u_time'),
      click: ctx.getUniformLocation(p, 'u_click'),
      active: ctx.getUniformLocation(p, 'u_active'),
      ripples: ctx.getUniformLocation(p, 'u_ripples[0]'),
      drops: ctx.getUniformLocation(p, 'u_drops[0]'),
    };
  }

  function init(canvas: HTMLCanvasElement) {
    gl = canvas.getContext('webgl', { antialias: false, premultipliedAlpha: false });
    if (!gl) {
      console.warn('[shader] WebGL unavailable — wallpaper disabled');
      return false;
    }
    programs = [];
    for (const frag of FRAGS) {
      const pu = makeProgram(gl, frag);
      if (pu) programs.push(pu);
    }
    if (programs.length === 0) {
      gl = null;
      return false;
    }
    vbo = gl.createBuffer();
    gl.bindBuffer(gl.ARRAY_BUFFER, vbo);
    gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1, -1, 1, -1, -1, 1, 1, 1]), gl.STATIC_DRAW);
    t0 = performance.now();
    water.setShaderClockOrigin(t0);
    installedCanvas = canvas;
    resize(canvas);
    attachDomListeners();
    initialized = true;
    return true;
  }

  /** Suminagashi renders through the fluid layer's own context — the main canvas idles. */
  function sumiFluidActive(): boolean {
    return shaderIndex.value === SUMI_INDEX && sumi.fluidSupported.value === true;
  }

  function currentProgram(): ProgramUniforms | null {
    const idx = Math.max(0, Math.min(programs.length - 1, shaderIndex.value));
    return programs[idx] ?? null;
  }

  function bindAttrib(pu: ProgramUniforms) {
    if (!gl) return;
    gl.useProgram(pu.program);
    const loc = gl.getAttribLocation(pu.program, 'a_pos');
    gl.enableVertexAttribArray(loc);
    gl.vertexAttribPointer(loc, 2, gl.FLOAT, false, 0, 0);
  }

  function resize(canvas: HTMLCanvasElement) {
    const dpr = Math.min(window.devicePixelRatio || 1, 2);
    const rect = canvas.getBoundingClientRect();
    const w = Math.max(1, Math.floor(rect.width * dpr));
    const h = Math.max(1, Math.floor(rect.height * dpr));
    if (canvas.width !== w || canvas.height !== h) {
      canvas.width = w;
      canvas.height = h;
    }
  }

  let resizeObserver: ResizeObserver | null = null;
  let onWindowMove: ((e: MouseEvent | TouchEvent) => void) | null = null;
  let onWindowDown: ((e: MouseEvent | TouchEvent) => void) | null = null;
  let onWindowUp: (() => void) | null = null;
  let onVisibility: (() => void) | null = null;

  function readClient(e: MouseEvent | TouchEvent): { x: number; y: number } | null {
    if ('touches' in e) {
      const t = e.touches[0];
      return t ? { x: t.clientX, y: t.clientY } : null;
    }
    return { x: e.clientX, y: e.clientY };
  }

  /** True when the event targets a real control — don't paint behind buttons/sliders. */
  function isInteractiveTarget(e: Event): boolean {
    const el = e.target as HTMLElement | null;
    return !!el?.closest?.('button, input, select, textarea, a, [contenteditable="true"]');
  }

  /** Translate window-client coords → canvas-local (y-up). Returns null if outside the container. */
  function translate(clientX: number, clientY: number): { x: number; y: number } | null {
    const root = rootRef.value;
    if (!root) return null;
    const rect = root.getBoundingClientRect();
    if (
      clientX < rect.left ||
      clientX > rect.right ||
      clientY < rect.top ||
      clientY > rect.bottom
    ) {
      return null;
    }
    return {
      x: clientX - rect.left,
      y: rect.height - (clientY - rect.top),
    };
  }

  function rippleCtx(rect: DOMRect, clientX: number, clientY: number) {
    const dpr = Math.min(window.devicePixelRatio || 1, 2);
    return {
      canvasHeight: installedCanvas?.height ?? Math.max(1, Math.floor(rect.height * dpr)),
      rootWidth: rect.width,
      rootHeight: rect.height,
      dpr,
      x: clientX - rect.left,
      y: clientY - rect.top,
    };
  }

  function attachDomListeners() {
    onWindowMove = (e) => {
      const p = readClient(e);
      if (!p) return;
      const local = translate(p.x, p.y);
      if (!local) {
        targetActive = 0;
        return;
      }
      mx = local.x;
      my = local.y;
      targetActive = 1;
      const root = rootRef.value;

      // Drag-wake: water shader only. While the mouse is down, push a faint
      // tail ripple every 8px / 60ms (both gates must trip).
      if (
        shaderIndex.value === WATER_INDEX &&
        isMouseDown &&
        !water.fishingMode.value &&
        !water.feedMode.value &&
        root
      ) {
        const now = performance.now();
        const dist = Math.hypot(p.x - lastDragX, p.y - lastDragY);
        if (dist > 8 && now - lastDragT > 60) {
          lastDragX = p.x;
          lastDragY = p.y;
          lastDragT = now;
          water.emitRipple(rippleCtx(root.getBoundingClientRect(), p.x, p.y), 0.035);
        }
      }

      // Suminagashi interactions.
      // Fluid sim: drag paints a smoke trail of the selected ink; hovering
      // (no button) gently stirs the ink like breathing on the water.
      // Marble fallback: drag combs the ink with tiny clear drops.
      if (shaderIndex.value === SUMI_INDEX && root) {
        const rect = root.getBoundingClientRect();
        const sim = sumi.getActiveSim();
        if (sim && sumi.sumiMode.value === 'fluid') {
          const u = (p.x - rect.left) / Math.max(1, rect.width);
          const v = 1 - (p.y - rect.top) / Math.max(1, rect.height);
          const du = lastPointerX < -999 ? 0 : (p.x - lastPointerX) / Math.max(1, rect.width);
          const dv = lastPointerY < -999 ? 0 : -(p.y - lastPointerY) / Math.max(1, rect.height);
          if (isMouseDown) {
            const dist = Math.hypot(p.x - lastInkX, p.y - lastInkY);
            if (dist > 6) {
              lastInkX = p.x;
              lastInkY = p.y;
              sim.trail(u, v, du, dv, sumi.inkAbs());
              if (Math.random() < 0.1) audio.playSwish();
            }
          } else {
            // hover = breathe on the ink; rate-limited so high-Hz mice don't
            // flood the GPU with splat passes
            const nowMs = performance.now();
            if (nowMs - lastStirT > 16) {
              lastStirT = nowMs;
              sim.stir(u, v, du, dv);
            }
          }
        } else if (isMouseDown) {
          const dist = Math.hypot(p.x - lastInkX, p.y - lastInkY);
          if (dist > 14) {
            lastInkX = p.x;
            lastInkY = p.y;
            sumi.addInkDrop(p.x - rect.left, p.y - rect.top, rect.height, {
              instant: true,
              r: 0.011,
              clear: true,
            });
            if (Math.random() < 0.2) audio.playSwish();
          }
        }
      }
      lastPointerX = p.x;
      lastPointerY = p.y;
    };
    onWindowDown = (e) => {
      if (isInteractiveTarget(e)) return;
      const p = readClient(e);
      if (!p) return;
      const local = translate(p.x, p.y);
      if (!local) return;
      mx = local.x;
      my = local.y;
      click = { x: mx, y: my, t: performance.now() / 1000, s: 1 };
      isMouseDown = true;
      const root = rootRef.value;

      if (shaderIndex.value === WATER_INDEX && root) {
        audio.ensureAudio();
        audio.startAmbient();
        if (water.rainMode.value) audio.startRainSound();
        const ctx = rippleCtx(root.getBoundingClientRect(), p.x, p.y);
        if (water.fishingMode.value) {
          water.castOrReel(ctx);
        } else if (water.feedMode.value) {
          water.dropPellet(ctx.x, ctx.y);
          // visual splash so feed click matches sound
          water.emitRipple(ctx, 0.18);
        } else {
          water.emitRipple(ctx, 0.2);
        }
        lastDragX = p.x;
        lastDragY = p.y;
        lastDragT = performance.now();
      } else if (shaderIndex.value === SUMI_INDEX && root) {
        audio.ensureAudio();
        const rect = root.getBoundingClientRect();
        const pan = (p.x - rect.left) / Math.max(1, rect.width);
        const sim = sumi.getActiveSim();
        if (sim && sumi.sumiMode.value === 'fluid') {
          sim.bloom(
            (p.x - rect.left) / Math.max(1, rect.width),
            1 - (p.y - rect.top) / Math.max(1, rect.height),
            sumi.inkAbs(),
          );
          audio.playInk(false, pan);
        } else {
          // marbling: alternate ink / clear dispersant
          const clear = sumi.nextDropIsClear();
          sumi.addInkDrop(p.x - rect.left, p.y - rect.top, rect.height, { clear });
          audio.playInk(clear, pan);
        }
        lastInkX = p.x;
        lastInkY = p.y;
      }
    };
    onWindowUp = () => {
      isMouseDown = false;
    };
    onVisibility = () => evaluateLoop();

    // Capture phase: the Vue Flow pane sits above the wallpaper and its
    // d3 pan/drag handlers stopImmediatePropagation() on mousedown/move/up,
    // so bubble listeners never hear clicks over the canvas. Window capture
    // fires first; we only observe, so graph panning keeps working.
    window.addEventListener('mousemove', onWindowMove as EventListener, { capture: true, passive: true });
    window.addEventListener('touchmove', onWindowMove as EventListener, { capture: true, passive: true });
    window.addEventListener('mousedown', onWindowDown as EventListener, { capture: true });
    window.addEventListener('touchstart', onWindowDown as EventListener, { capture: true, passive: true });
    window.addEventListener('mouseup', onWindowUp as EventListener, { capture: true });
    window.addEventListener('touchend', onWindowUp as EventListener, { capture: true, passive: true });
    document.addEventListener('visibilitychange', onVisibility);

    if (installedCanvas) {
      resizeObserver = new ResizeObserver(() => {
        if (installedCanvas) resize(installedCanvas);
      });
      resizeObserver.observe(installedCanvas);
    }
  }

  function detachDomListeners() {
    if (onWindowMove) {
      window.removeEventListener('mousemove', onWindowMove as EventListener, { capture: true });
      window.removeEventListener('touchmove', onWindowMove as EventListener, { capture: true });
    }
    if (onWindowDown) {
      window.removeEventListener('mousedown', onWindowDown as EventListener, { capture: true });
      window.removeEventListener('touchstart', onWindowDown as EventListener, { capture: true });
    }
    if (onWindowUp) {
      window.removeEventListener('mouseup', onWindowUp as EventListener, { capture: true });
      window.removeEventListener('touchend', onWindowUp as EventListener, { capture: true });
    }
    if (onVisibility) document.removeEventListener('visibilitychange', onVisibility);
    if (resizeObserver) {
      resizeObserver.disconnect();
      resizeObserver = null;
    }
    onWindowMove = onWindowDown = onWindowUp = onVisibility = null;
  }

  function applyDim(canvas: HTMLCanvasElement) {
    const v = Math.max(0, Math.min(100, dim.value)) / 100;
    const brightness = 1 - v * 0.75;
    const saturation = 1 - v * 0.5;
    canvas.style.filter = `saturate(${saturation}) brightness(${brightness})`;
  }

  function clear() {
    if (!gl) return;
    gl.clearColor(0, 0, 0, 0);
    gl.clear(gl.COLOR_BUFFER_BIT);
  }

  function frame(now: number) {
    rafId = null;
    const canvas = canvasRef.value;
    if (!gl || !canvas) return;
    const pu = currentProgram();
    if (!pu) return;

    const t = (now - t0) / 1000;
    smx += (mx - smx) * 0.14;
    smy += (my - smy) * 0.14;
    smActive += (targetActive - smActive) * 0.08;

    const dpr = Math.min(window.devicePixelRatio || 1, 2);
    bindAttrib(pu);
    gl.viewport(0, 0, canvas.width, canvas.height);
    gl.uniform2f(pu.res, canvas.width, canvas.height);
    gl.uniform2f(pu.mouse, smx * dpr, smy * dpr);
    gl.uniform1f(pu.time, t);
    gl.uniform4f(pu.click, click.x * dpr, click.y * dpr, click.t - t0 / 1000, click.s);
    gl.uniform1f(pu.active, smActive);
    if (pu.ripples && shaderIndex.value === WATER_INDEX) {
      const ripples = water.ripples;
      for (let j = 0; j < WATER_RIPPLE_SLOTS; j++) {
        const r = ripples[j]!;
        rippleUniform[j * 4 + 0] = r.x;
        rippleUniform[j * 4 + 1] = r.y;
        rippleUniform[j * 4 + 2] = r.t;
        rippleUniform[j * 4 + 3] = r.s;
      }
      gl.uniform4fv(pu.ripples, rippleUniform);
    }
    // Suminagashi fallback (no half-float FBO support): animate the marble
    // drop blooms and feed the analytic marbling shader.
    if (pu.drops && shaderIndex.value === SUMI_INDEX) {
      gl.uniform4fv(pu.drops, sumi.buildMarbleDrops(false));
    }
    gl.drawArrays(gl.TRIANGLE_STRIP, 0, 4);

    if (shouldRun()) {
      rafId = requestAnimationFrame(frame);
    }
  }

  function shouldRun(): boolean {
    return (
      enabled.value &&
      initialized &&
      !paused &&
      // the fluid layer owns rendering while the sumi scene runs the sim —
      // the main canvas stays cleared instead of burning a parallel rAF
      !sumiFluidActive() &&
      typeof document !== 'undefined' &&
      document.visibilityState === 'visible'
    );
  }

  function evaluateLoop() {
    const canvas = canvasRef.value;
    if (!canvas) return;

    if (enabled.value && !initialized) {
      if (!init(canvas)) return;
      applyDim(canvas);
    }

    if (shouldRun()) {
      if (rafId === null) {
        rafId = requestAnimationFrame(frame);
      }
    } else {
      if (rafId !== null) {
        cancelAnimationFrame(rafId);
        rafId = null;
      }
      if (initialized) clear();
    }
  }

  function teardown() {
    if (rafId !== null) {
      cancelAnimationFrame(rafId);
      rafId = null;
    }
    detachDomListeners();
    if (gl) {
      for (const pu of programs) gl.deleteProgram(pu.program);
      if (vbo) gl.deleteBuffer(vbo);
      const lose = gl.getExtension('WEBGL_lose_context');
      if (lose) lose.loseContext();
    }
    programs = [];
    vbo = null;
    gl = null;
    installedCanvas = null;
    initialized = false;
  }

  watch(
    canvasRef,
    (canvas, old) => {
      if (old && !canvas) {
        teardown();
      } else if (canvas) {
        evaluateLoop();
      }
    },
    { immediate: true },
  );

  watch(enabled, (v) => {
    if (!v && initialized) {
      teardown();
    } else {
      evaluateLoop();
    }
  });

  // Scene switches can start/stop the main loop (sumi + fluid idles it).
  watch([shaderIndex, sumi.fluidSupported], () => evaluateLoop());

  watch(dim, () => {
    const canvas = canvasRef.value;
    if (canvas) applyDim(canvas);
  });

  // keep-alive lifecycle: pause rAF while detached, resume on re-activation.
  onActivated(() => {
    paused = false;
    evaluateLoop();
  });
  onDeactivated(() => {
    paused = true;
    evaluateLoop();
  });

  onBeforeUnmount(() => teardown());

  return { teardown };
}
