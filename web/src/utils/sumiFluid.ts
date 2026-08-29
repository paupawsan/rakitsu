// GPU ink fluid simulation for the Suminagashi wallpaper scene (07).
//
// Stable-fluids (Stam) on ping-pong half-float textures with vorticity
// confinement, so injected ink curls and billows like smoke in water.
// Dye is stored as per-channel pigment ABSORPTION and composited over a
// procedural washi paper with Beer-Lambert (paper * exp(-dye)), which makes
// overlapping inks blend subtractively — indigo over ochre really makes
// green, everything over sumi goes black — like actual pigment.
//
// Perf notes from the design handoff (these are deliberate, keep them):
//   - The washi paper is baked ONCE into a static byte texture. Computing
//     its fbm per pixel per frame was the single biggest cost of the sim.
//   - The marble-mode shader is compiled into THIS context — a second
//     canvas/context pushed the page over the browser's WebGL context limit
//     and silently evicted the fluid context (black screen).
//   - GPU-capability probing happens on a throwaway canvas whose context is
//     explicitly released, so a failed probe never burns the real canvas's
//     one-and-only context (the analytic marbling fallback needs it).

type GL = WebGLRenderingContext | WebGL2RenderingContext;

interface FBO {
  tex: WebGLTexture;
  fbo: WebGLFramebuffer;
  w: number;
  h: number;
  texel: [number, number];
}

interface DoubleFBO {
  readonly read: FBO;
  readonly write: FBO;
  swap(): void;
  w: number;
  h: number;
  texel: [number, number];
}

interface Prog {
  p: WebGLProgram;
  u: Record<string, WebGLUniformLocation | null>;
}

export interface SumiFluidParams {
  /** Global time scale (0.25–3). */
  speed: number;
  /** Vorticity multiplier (0–3). */
  swirl: number;
  /** Dye dissipation multiplier (0 = permanent ink, up to 4). */
  fade: number;
  /** Brush scale (0.4–2.5). */
  size: number;
}

const SIM_BASE = 128; // velocity grid resolution (short side)
const DYE_BASE = 512; // dye/ink resolution (short side)
const PRESSURE_ITERS = 16;
const CURL_STRENGTH = 16;
const VEL_DISSIPATION = 0.16;
const DYE_DISSIPATION = 0.045;

const VERT = `
precision highp float;
attribute vec2 aPos;
varying vec2 vUv;
void main(){ vUv = aPos * 0.5 + 0.5; gl_Position = vec4(aPos, 0.0, 1.0); }
`;
const FRAG_COPY = `
precision highp float; varying vec2 vUv;
uniform sampler2D uTex;
void main(){ gl_FragColor = texture2D(uTex, vUv); }
`;
const FRAG_CLEAR = `
precision highp float; varying vec2 vUv;
uniform sampler2D uTex; uniform float uValue;
void main(){ gl_FragColor = uValue * texture2D(uTex, vUv); }
`;
const FRAG_SPLAT = `
precision highp float; varying vec2 vUv;
uniform sampler2D uTarget;
uniform float uAspect;
uniform vec3  uColor;
uniform vec2  uPoint;
uniform float uRadius;
void main(){
  vec2 p = vUv - uPoint;
  p.x *= uAspect;
  vec3 splat = exp(-dot(p, p) / uRadius) * uColor;
  gl_FragColor = vec4(texture2D(uTarget, vUv).xyz + splat, 1.0);
}
`;
const FRAG_ADVECT = `
precision highp float; varying vec2 vUv;
uniform sampler2D uVelocity, uSource;
uniform vec2  uTexel;
uniform float uDt, uDissipation;
void main(){
  vec2 coord = vUv - uDt * texture2D(uVelocity, vUv).xy * uTexel;
  vec4 result = texture2D(uSource, coord);
  gl_FragColor = result / (1.0 + uDissipation * uDt);
}
`;
const FRAG_DIV = `
precision highp float; varying vec2 vUv;
uniform sampler2D uVelocity; uniform vec2 uTexel;
void main(){
  float L = texture2D(uVelocity, vUv - vec2(uTexel.x, 0.0)).x;
  float R = texture2D(uVelocity, vUv + vec2(uTexel.x, 0.0)).x;
  float B = texture2D(uVelocity, vUv - vec2(0.0, uTexel.y)).y;
  float T = texture2D(uVelocity, vUv + vec2(0.0, uTexel.y)).y;
  gl_FragColor = vec4(0.5 * (R - L + T - B), 0.0, 0.0, 1.0);
}
`;
const FRAG_CURL = `
precision highp float; varying vec2 vUv;
uniform sampler2D uVelocity; uniform vec2 uTexel;
void main(){
  float L = texture2D(uVelocity, vUv - vec2(uTexel.x, 0.0)).y;
  float R = texture2D(uVelocity, vUv + vec2(uTexel.x, 0.0)).y;
  float B = texture2D(uVelocity, vUv - vec2(0.0, uTexel.y)).x;
  float T = texture2D(uVelocity, vUv + vec2(0.0, uTexel.y)).x;
  float vorticity = R - L - T + B;
  gl_FragColor = vec4(0.5 * vorticity, 0.0, 0.0, 1.0);
}
`;
const FRAG_VORT = `
precision highp float; varying vec2 vUv;
uniform sampler2D uVelocity, uCurl;
uniform vec2  uTexel;
uniform float uCurlStrength, uDt;
void main(){
  float L = texture2D(uCurl, vUv - vec2(uTexel.x, 0.0)).x;
  float R = texture2D(uCurl, vUv + vec2(uTexel.x, 0.0)).x;
  float B = texture2D(uCurl, vUv - vec2(0.0, uTexel.y)).x;
  float T = texture2D(uCurl, vUv + vec2(0.0, uTexel.y)).x;
  float C = texture2D(uCurl, vUv).x;
  vec2 force = 0.5 * vec2(abs(T) - abs(B), abs(R) - abs(L));
  force /= length(force) + 0.0001;
  force *= uCurlStrength * C;
  force.y *= -1.0;
  vec2 vel = texture2D(uVelocity, vUv).xy + force * uDt;
  vel = clamp(vel, vec2(-1000.0), vec2(1000.0));
  gl_FragColor = vec4(vel, 0.0, 1.0);
}
`;
const FRAG_PRESSURE = `
precision highp float; varying vec2 vUv;
uniform sampler2D uPressure, uDivergence;
uniform vec2 uTexel;
void main(){
  float L = texture2D(uPressure, vUv - vec2(uTexel.x, 0.0)).x;
  float R = texture2D(uPressure, vUv + vec2(uTexel.x, 0.0)).x;
  float B = texture2D(uPressure, vUv - vec2(0.0, uTexel.y)).x;
  float T = texture2D(uPressure, vUv + vec2(0.0, uTexel.y)).x;
  float div = texture2D(uDivergence, vUv).x;
  gl_FragColor = vec4((L + R + B + T - div) * 0.25, 0.0, 0.0, 1.0);
}
`;
const FRAG_GRAD = `
precision highp float; varying vec2 vUv;
uniform sampler2D uPressure, uVelocity;
uniform vec2 uTexel;
void main(){
  float L = texture2D(uPressure, vUv - vec2(uTexel.x, 0.0)).x;
  float R = texture2D(uPressure, vUv + vec2(uTexel.x, 0.0)).x;
  float B = texture2D(uPressure, vUv - vec2(0.0, uTexel.y)).x;
  float T = texture2D(uPressure, vUv + vec2(0.0, uTexel.y)).x;
  vec2 vel = texture2D(uVelocity, vUv).xy - vec2(R - L, T - B);
  gl_FragColor = vec4(vel, 0.0, 1.0);
}
`;
// Washi paper, rendered ONCE into a static texture (rgb = lit paper with
// vignette baked in, a = granulation field).
const FRAG_PAPER = `
precision highp float; varying vec2 vUv;
uniform vec2 uRes;
float hash(vec2 p){ return fract(sin(dot(p, vec2(127.1, 311.7)))*43758.5453); }
float n2(vec2 p){
  vec2 i=floor(p), f=fract(p);
  float a=hash(i), b=hash(i+vec2(1,0)), c=hash(i+vec2(0,1)), d=hash(i+vec2(1,1));
  vec2 u=f*f*(3.0-2.0*f);
  return mix(a,b,u.x)+(c-a)*u.y*(1.0-u.x)+(d-b)*u.x*u.y;
}
float fbm(vec2 p){
  float v=0.0, a=0.5;
  for(int i=0;i<4;i++){ v+=a*n2(p); p*=2.1; a*=0.5; }
  return v;
}
void main(){
  vec2 uvA = vUv * vec2(uRes.x / uRes.y, 1.0);
  float fiber = fbm(uvA*vec2(60.0, 14.0)) * 0.5 + fbm(uvA*vec2(9.0, 42.0)) * 0.5;
  float grain = n2(uvA*420.0);
  vec3 p = vec3(0.912, 0.882, 0.815);
  p -= fiber * 0.045;
  p -= grain * 0.022;
  float fleck = step(0.9965, hash(floor(uvA*330.0)));
  p = mix(p, vec3(0.68, 0.63, 0.54), fleck * 0.5);
  vec2 cv = vUv - 0.5;
  p *= clamp(1.0 - dot(cv, cv) * 0.5, 0.82, 1.0);
  gl_FragColor = vec4(p, fbm(uvA*26.0));
}
`;
// Composite: static paper texture + Beer-Lambert pigment absorption.
const FRAG_DISPLAY = `
precision highp float; varying vec2 vUv;
uniform sampler2D uDye, uPaper;
void main(){
  vec4 pp = texture2D(uPaper, vUv);
  vec3 ink = texture2D(uDye, vUv).rgb;
  float gran = 0.85 + 0.45 * pp.a;   // pigment settles with the paper tooth
  vec3 col = pp.rgb * exp(-ink * gran * 1.15);
  float dens = dot(ink, vec3(0.333));
  col += vec3(0.040, 0.045, 0.052) * smoothstep(0.9, 2.4, dens);
  gl_FragColor = vec4(col, 1.0);
}
`;
// Persistent ring-marbling. The classic inverse-displacement walk over the
// live drop list, but pixels that escape every drop sample the BAKED
// history texture at the displaced position — so when the drop list is
// baked down, old rings keep warping under new drops instead of vanishing.
const FRAG_MARBLE = `
precision highp float;
#define ND 64
varying vec2 vUv;
uniform vec2  u_res;
uniform vec2  u_mouse;
uniform float u_time;
uniform float u_active;
uniform vec4  u_drops[ND];
uniform sampler2D u_baked;
float hash(vec2 p){ return fract(sin(dot(p, vec2(127.1, 311.7)))*43758.5453); }
float n2(vec2 p){
  vec2 i=floor(p), f=fract(p);
  float a=hash(i), b=hash(i+vec2(1,0)), c=hash(i+vec2(0,1)), d=hash(i+vec2(1,1));
  vec2 u=f*f*(3.0-2.0*f);
  return mix(a,b,u.x)+(c-a)*u.y*(1.0-u.x)+(d-b)*u.x*u.y;
}
float fbm(vec2 p){
  float v=0.0, a=0.5;
  for(int i=0;i<4;i++){ v+=a*n2(p); p*=2.1; a*=0.5; }
  return v;
}
void main(){
  vec2 res = u_res;
  vec2 uv  = gl_FragCoord.xy / res.y;
  vec2 m   = u_mouse / res.y;
  vec2 dm  = uv - m;
  float rm = length(dm);
  float stir = exp(-rm*rm*14.0) * 0.006 * u_active;
  vec2 p = uv
    + 0.006 * vec2(n2(uv*2.2 + vec2(u_time*0.05, 0.0)) - 0.5,
                   n2(uv*2.2 + vec2(0.0, u_time*0.045) + 9.7) - 0.5)
    + vec2(-dm.y, dm.x) * stir;
  float colId = -1.0;
  for(int i = ND-1; i >= 0; i--){
    vec4 d = u_drops[i];
    if(d.z < 0.0008) continue;
    vec2 q = p - d.xy;
    float r2 = dot(q, q);
    float rr = d.z * d.z;
    if(r2 < rr){ colId = d.w; break; }
    p = d.xy + q * sqrt(1.0 - rr / r2);
  }
  vec2 tuv = clamp(p * vec2(res.y / res.x, 1.0), 0.0, 1.0);
  vec3 col = texture2D(u_baked, tuv).rgb;
  if(colId > 0.5){
    vec3 sumi   = vec3(0.118, 0.110, 0.102);
    vec3 indigo = vec3(0.165, 0.255, 0.420);
    vec3 vermil = vec3(0.690, 0.270, 0.160);
    vec3 ink = colId < 1.5 ? sumi : (colId < 2.5 ? indigo : vermil);
    float gran = fbm(uv*26.0 + colId*7.0);
    ink = mix(ink, vec3(0.866, 0.838, 0.774), gran*gran*0.38);
    ink = mix(ink, ink*0.7, smoothstep(0.75, 1.0, n2(uv*90.0))*0.25);
    col = ink;
  }
  col += exp(-rm*rm*9.0) * 0.035 * u_active;
  gl_FragColor = vec4(col, 1.0);
}
`;

function testFBO(g: GL, is2: boolean, hf?: { HALF_FLOAT_OES: number } | null): boolean {
  try {
    const tex = g.createTexture();
    g.bindTexture(g.TEXTURE_2D, tex);
    g.texParameteri(g.TEXTURE_2D, g.TEXTURE_MIN_FILTER, g.NEAREST);
    g.texParameteri(g.TEXTURE_2D, g.TEXTURE_MAG_FILTER, g.NEAREST);
    if (is2) {
      const g2 = g as WebGL2RenderingContext;
      g2.texImage2D(g2.TEXTURE_2D, 0, g2.RGBA16F, 4, 4, 0, g2.RGBA, g2.HALF_FLOAT, null);
    } else if (hf) {
      g.texImage2D(g.TEXTURE_2D, 0, g.RGBA, 4, 4, 0, g.RGBA, hf.HALF_FLOAT_OES, null);
    } else {
      return false;
    }
    const fb = g.createFramebuffer();
    g.bindFramebuffer(g.FRAMEBUFFER, fb);
    g.framebufferTexture2D(g.FRAMEBUFFER, g.COLOR_ATTACHMENT0, g.TEXTURE_2D, tex, 0);
    return g.checkFramebufferStatus(g.FRAMEBUFFER) === g.FRAMEBUFFER_COMPLETE;
  } catch {
    return false;
  }
}

function loseCtx(g: GL) {
  try {
    const e = g.getExtension('WEBGL_lose_context');
    if (e) e.loseContext();
  } catch {
    /* best-effort */
  }
}

// Probe support on a throwaway canvas FIRST so a failed init never burns
// the real canvas's one-and-only context.
function probeMode(): 'webgl2' | 'webgl' | null {
  try {
    const t = document.createElement('canvas');
    const g2 = t.getContext('webgl2');
    if (g2) {
      const ok = !!g2.getExtension('EXT_color_buffer_float') && testFBO(g2, true);
      loseCtx(g2);
      if (ok) return 'webgl2';
    }
  } catch {
    /* fall through to webgl1 */
  }
  try {
    const t = document.createElement('canvas');
    const g1 = t.getContext('webgl');
    if (g1) {
      const hf = g1.getExtension('OES_texture_half_float');
      const ok = !!hf && testFBO(g1, false, hf);
      loseCtx(g1);
      if (ok) return 'webgl';
    }
  } catch {
    /* unsupported */
  }
  return null;
}

const capV = (v: number) => Math.max(-420, Math.min(420, v));

export class SumiFluidSim {
  ready = false;
  /** Live-tunable FX parameters — assign a shared (reactive) object to wire UI sliders. */
  params: SumiFluidParams = { speed: 1, swirl: 1, fade: 1, size: 1 };

  private canvas: HTMLCanvasElement | null = null;
  private gl: GL | null = null;
  private isGL2 = false;
  private halfType = 0;
  private linearOK = false;
  private velocity!: DoubleFBO;
  private dye!: DoubleFBO;
  private pressure!: DoubleFBO;
  private divergence!: FBO;
  private curl!: FBO;
  private paper!: FBO;
  private marbleBaked: { readonly read: FBO; readonly write: FBO; swap(): void; w: number; h: number } | null = null;
  private progs: Record<string, Prog> = {};
  private lastT = 0;
  private ambNext = 0;
  private bufW = 0;
  private bufH = 0;

  init(c: HTMLCanvasElement): boolean {
    const mode = probeMode();
    if (!mode) return false;
    this.canvas = c;
    this.isGL2 = mode === 'webgl2';
    const gl = c.getContext(mode, { alpha: false, depth: false, stencil: false, antialias: false }) as GL | null;
    if (!gl) return false;
    this.gl = gl;
    if (this.isGL2) {
      gl.getExtension('EXT_color_buffer_float');
      this.halfType = (gl as WebGL2RenderingContext).HALF_FLOAT;
      this.linearOK = true; // half-float filtering is core in ES3
    } else {
      const hf = gl.getExtension('OES_texture_half_float');
      if (!hf) return false;
      this.halfType = hf.HALF_FLOAT_OES;
      this.linearOK = !!gl.getExtension('OES_texture_half_float_linear');
    }
    // fullscreen quad on attrib 0
    const buf = gl.createBuffer();
    gl.bindBuffer(gl.ARRAY_BUFFER, buf);
    gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1, -1, 1, -1, -1, 1, 1, 1]), gl.STATIC_DRAW);
    gl.enableVertexAttribArray(0);
    gl.vertexAttribPointer(0, 2, gl.FLOAT, false, 0, 0);

    const vs = this.compileShader(gl.VERTEX_SHADER, VERT);
    if (!vs) return false;
    const sources: Record<string, string> = {
      copy: FRAG_COPY,
      clear: FRAG_CLEAR,
      splat: FRAG_SPLAT,
      advect: FRAG_ADVECT,
      div: FRAG_DIV,
      curl: FRAG_CURL,
      vort: FRAG_VORT,
      pressure: FRAG_PRESSURE,
      grad: FRAG_GRAD,
      paper: FRAG_PAPER,
      display: FRAG_DISPLAY,
      marble: FRAG_MARBLE,
    };
    for (const [k, src] of Object.entries(sources)) {
      const pr = this.makeProgram(vs, src);
      if (!pr) return false;
      this.progs[k] = pr;
    }

    gl.disable(gl.BLEND);
    this.rebuildTargets();
    this.ready = true;
    return true;
  }

  dispose() {
    this.ready = false;
    if (this.gl) loseCtx(this.gl);
    this.gl = null;
    this.canvas = null;
    this.progs = {};
    this.marbleBaked = null;
  }

  resize() {
    const gl = this.gl;
    if (!this.ready || !gl) return;
    if (gl.drawingBufferWidth !== this.bufW || gl.drawingBufferHeight !== this.bufH) {
      this.rebuildTargets();
    }
  }

  clear() {
    const gl = this.gl;
    if (!this.ready || !gl) return;
    for (const f of [
      this.dye.read,
      this.dye.write,
      this.velocity.read,
      this.velocity.write,
      this.pressure.read,
      this.pressure.write,
    ]) {
      gl.bindFramebuffer(gl.FRAMEBUFFER, f.fbo);
      gl.viewport(0, 0, f.w, f.h);
      gl.clearColor(0, 0, 0, 1);
      gl.clear(gl.COLOR_BUFFER_BIT);
    }
  }

  /** Drop ink at (u,v in 0..1, v up): dense core + soft halo + a radial puff of velocity. */
  bloom(x: number, y: number, color: [number, number, number]) {
    if (!this.ready) return;
    const sz = this.params.size * this.params.size;
    this.splatDye(x, y, color, 0.0008 * sz, 1.4);
    this.splatDye(x, y, color, 0.0042 * sz, 0.45);
    const a0 = Math.random() * Math.PI * 2;
    const reach = 0.016 * this.params.size;
    for (let k = 0; k < 6; k++) {
      const a = a0 + (k / 6) * Math.PI * 2;
      this.splatVel(x + Math.cos(a) * reach, y + Math.sin(a) * reach, Math.cos(a) * 130, Math.sin(a) * 130, 0.0022 * sz);
    }
  }

  /** Drag ink along a stroke. */
  trail(x: number, y: number, dx: number, dy: number, color: [number, number, number]) {
    if (!this.ready) return;
    const sz = this.params.size * this.params.size;
    this.splatVel(x, y, capV(dx * 5200), capV(dy * 5200), 0.0026 * sz);
    this.splatDye(x, y, color, 0.0016 * sz, 0.32);
  }

  /** Velocity-only — breathe on the ink. */
  stir(x: number, y: number, dx: number, dy: number) {
    if (!this.ready) return;
    this.splatVel(x, y, capV(dx * 2600), capV(dy * 2600), 0.0036);
  }

  /** Render the persistent marbling to the screen via this context. */
  renderMarble(drops: Float32Array, mx: number, my: number, time: number, active: number) {
    const gl = this.gl;
    const pr = this.progs.marble;
    if (!this.ready || !gl || !pr) return;
    this.ensureMarbleBaked();
    gl.disable(gl.BLEND);
    const u = this.use(pr);
    gl.uniform2f(u.u_res ?? null, gl.drawingBufferWidth, gl.drawingBufferHeight);
    gl.uniform2f(u.u_mouse ?? null, mx, my);
    gl.uniform1f(u.u_time ?? null, time);
    gl.uniform1f(u.u_active ?? null, active);
    gl.uniform1i(u.u_baked ?? null, this.attach(this.marbleBaked!.read, 0));
    const loc = u['u_drops[0]'] ?? u.u_drops ?? null;
    if (loc) gl.uniform4fv(loc, drops);
    this.blit(null);
  }

  /**
   * Fold the current drop list into the baked history texture. After this
   * the caller can empty its drop list — the pattern lives on in the bake.
   */
  bakeMarble(drops: Float32Array, time: number): boolean {
    const gl = this.gl;
    const pr = this.progs.marble;
    if (!this.ready || !gl || !pr) return false;
    this.ensureMarbleBaked();
    const mb = this.marbleBaked!;
    gl.disable(gl.BLEND);
    const u = this.use(pr);
    gl.uniform2f(u.u_res ?? null, mb.w, mb.h);
    gl.uniform2f(u.u_mouse ?? null, -99999.0, -99999.0);
    gl.uniform1f(u.u_time ?? null, time);
    gl.uniform1f(u.u_active ?? null, 0);
    gl.uniform1i(u.u_baked ?? null, this.attach(mb.read, 0));
    const loc = u['u_drops[0]'] ?? u.u_drops ?? null;
    if (loc) gl.uniform4fv(loc, drops);
    this.blit(mb.write);
    mb.swap();
    return true;
  }

  clearMarble() {
    const gl = this.gl;
    if (!this.ready || !gl) return;
    this.ensureMarbleBaked();
    const mb = this.marbleBaked!;
    const u = this.use(this.progs.paper!);
    gl.uniform2f(u.uRes ?? null, mb.w, mb.h);
    this.blit(mb.read);
  }

  /** Advance the sim + render one frame. `t` in seconds. */
  step(t: number) {
    const gl = this.gl;
    if (!this.ready || !gl) return;
    let dt = Math.min(0.033, Math.max(0.001, t - this.lastT));
    this.lastT = t;
    dt *= this.params.speed;

    // ambient breath: an occasional weak random stir keeps the ink alive
    if (t > this.ambNext) {
      this.ambNext = t + 1.6 + Math.random() * 2.4;
      const a = Math.random() * Math.PI * 2;
      this.splatVel(
        Math.random(),
        Math.random(),
        Math.cos(a) * (25 + Math.random() * 45),
        Math.sin(a) * (25 + Math.random() * 45),
        0.012,
      );
    }

    gl.disable(gl.BLEND);
    const sTexel = this.velocity.texel;

    let u = this.use(this.progs.curl!);
    gl.uniform2f(u.uTexel ?? null, sTexel[0], sTexel[1]);
    gl.uniform1i(u.uVelocity ?? null, this.attach(this.velocity.read, 0));
    this.blit(this.curl);

    u = this.use(this.progs.vort!);
    gl.uniform2f(u.uTexel ?? null, sTexel[0], sTexel[1]);
    gl.uniform1i(u.uVelocity ?? null, this.attach(this.velocity.read, 0));
    gl.uniform1i(u.uCurl ?? null, this.attach(this.curl, 1));
    gl.uniform1f(u.uCurlStrength ?? null, CURL_STRENGTH * this.params.swirl);
    gl.uniform1f(u.uDt ?? null, dt);
    this.blit(this.velocity.write);
    this.velocity.swap();

    u = this.use(this.progs.div!);
    gl.uniform2f(u.uTexel ?? null, sTexel[0], sTexel[1]);
    gl.uniform1i(u.uVelocity ?? null, this.attach(this.velocity.read, 0));
    this.blit(this.divergence);

    u = this.use(this.progs.clear!);
    gl.uniform1i(u.uTex ?? null, this.attach(this.pressure.read, 0));
    gl.uniform1f(u.uValue ?? null, 0.8);
    this.blit(this.pressure.write);
    this.pressure.swap();

    u = this.use(this.progs.pressure!);
    gl.uniform2f(u.uTexel ?? null, sTexel[0], sTexel[1]);
    gl.uniform1i(u.uDivergence ?? null, this.attach(this.divergence, 0));
    for (let i = 0; i < PRESSURE_ITERS; i++) {
      gl.uniform1i(u.uPressure ?? null, this.attach(this.pressure.read, 1));
      this.blit(this.pressure.write);
      this.pressure.swap();
    }

    u = this.use(this.progs.grad!);
    gl.uniform2f(u.uTexel ?? null, sTexel[0], sTexel[1]);
    gl.uniform1i(u.uPressure ?? null, this.attach(this.pressure.read, 0));
    gl.uniform1i(u.uVelocity ?? null, this.attach(this.velocity.read, 1));
    this.blit(this.velocity.write);
    this.velocity.swap();

    u = this.use(this.progs.advect!);
    gl.uniform2f(u.uTexel ?? null, sTexel[0], sTexel[1]);
    gl.uniform1f(u.uDt ?? null, dt);
    gl.uniform1i(u.uVelocity ?? null, this.attach(this.velocity.read, 0));
    gl.uniform1i(u.uSource ?? null, 0);
    gl.uniform1f(u.uDissipation ?? null, VEL_DISSIPATION);
    this.blit(this.velocity.write);
    this.velocity.swap();

    gl.uniform1i(u.uVelocity ?? null, this.attach(this.velocity.read, 0));
    gl.uniform1i(u.uSource ?? null, this.attach(this.dye.read, 1));
    gl.uniform1f(u.uDissipation ?? null, DYE_DISSIPATION * this.params.fade);
    this.blit(this.dye.write);
    this.dye.swap();

    // composite onto paper
    u = this.use(this.progs.display!);
    gl.uniform1i(u.uDye ?? null, this.attach(this.dye.read, 0));
    gl.uniform1i(u.uPaper ?? null, this.attach(this.paper, 1));
    this.blit(null);
  }

  // ======================= internals =======================

  private compileShader(type: number, src: string): WebGLShader | null {
    const gl = this.gl!;
    const sh = gl.createShader(type);
    if (!sh) return null;
    gl.shaderSource(sh, src);
    gl.compileShader(sh);
    if (!gl.getShaderParameter(sh, gl.COMPILE_STATUS)) {
      console.error('[sumi-fluid] shader error:', gl.getShaderInfoLog(sh));
      return null;
    }
    return sh;
  }

  private makeProgram(vs: WebGLShader, fragSrc: string): Prog | null {
    const gl = this.gl!;
    const fs = this.compileShader(gl.FRAGMENT_SHADER, fragSrc);
    if (!fs) return null;
    const p = gl.createProgram();
    if (!p) return null;
    gl.attachShader(p, vs);
    gl.attachShader(p, fs);
    gl.bindAttribLocation(p, 0, 'aPos');
    gl.linkProgram(p);
    if (!gl.getProgramParameter(p, gl.LINK_STATUS)) {
      console.error('[sumi-fluid] link error:', gl.getProgramInfoLog(p));
      return null;
    }
    const u: Record<string, WebGLUniformLocation | null> = {};
    const n = gl.getProgramParameter(p, gl.ACTIVE_UNIFORMS) as number;
    for (let i = 0; i < n; i++) {
      const info = gl.getActiveUniform(p, i);
      if (info) u[info.name] = gl.getUniformLocation(p, info.name);
    }
    return { p, u };
  }

  private use(pr: Prog): Record<string, WebGLUniformLocation | null> {
    this.gl!.useProgram(pr.p);
    return pr.u;
  }

  private blit(target: { fbo: WebGLFramebuffer; w: number; h: number } | null) {
    const gl = this.gl!;
    if (target) {
      gl.bindFramebuffer(gl.FRAMEBUFFER, target.fbo);
      gl.viewport(0, 0, target.w, target.h);
    } else {
      gl.bindFramebuffer(gl.FRAMEBUFFER, null);
      gl.viewport(0, 0, gl.drawingBufferWidth, gl.drawingBufferHeight);
    }
    gl.drawArrays(gl.TRIANGLE_STRIP, 0, 4);
  }

  private attach(f: { tex: WebGLTexture }, id: number): number {
    const gl = this.gl!;
    gl.activeTexture(gl.TEXTURE0 + id);
    gl.bindTexture(gl.TEXTURE_2D, f.tex);
    return id;
  }

  private makeFBO(w: number, h: number, filter: number): FBO {
    const gl = this.gl!;
    const tex = gl.createTexture()!;
    gl.activeTexture(gl.TEXTURE0);
    gl.bindTexture(gl.TEXTURE_2D, tex);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, filter);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, filter);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
    if (this.isGL2) {
      const g2 = gl as WebGL2RenderingContext;
      g2.texImage2D(g2.TEXTURE_2D, 0, g2.RGBA16F, w, h, 0, g2.RGBA, g2.HALF_FLOAT, null);
    } else {
      gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, w, h, 0, gl.RGBA, this.halfType, null);
    }
    const fbo = gl.createFramebuffer()!;
    gl.bindFramebuffer(gl.FRAMEBUFFER, fbo);
    gl.framebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, tex, 0);
    gl.viewport(0, 0, w, h);
    gl.clearColor(0, 0, 0, 1);
    gl.clear(gl.COLOR_BUFFER_BIT);
    return { tex, fbo, w, h, texel: [1 / w, 1 / h] };
  }

  /** Plain byte texture FBO (baked paper / marble history). */
  private makeByteFBO(w: number, h: number): FBO {
    const gl = this.gl!;
    const tex = gl.createTexture()!;
    gl.activeTexture(gl.TEXTURE0);
    gl.bindTexture(gl.TEXTURE_2D, tex);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
    gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, w, h, 0, gl.RGBA, gl.UNSIGNED_BYTE, null);
    const fbo = gl.createFramebuffer()!;
    gl.bindFramebuffer(gl.FRAMEBUFFER, fbo);
    gl.framebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, tex, 0);
    return { tex, fbo, w, h, texel: [1 / w, 1 / h] };
  }

  private makeDoubleFBO(w: number, h: number, filter: number): DoubleFBO {
    let f1 = this.makeFBO(w, h, filter);
    let f2 = this.makeFBO(w, h, filter);
    return {
      get read() {
        return f1;
      },
      get write() {
        return f2;
      },
      swap() {
        const t = f1;
        f1 = f2;
        f2 = t;
      },
      w,
      h,
      texel: [1 / w, 1 / h],
    };
  }

  private resizeDouble(old: DoubleFBO | undefined, w: number, h: number, filter: number): DoubleFBO {
    const nf = this.makeDoubleFBO(w, h, filter);
    if (old) {
      const u = this.use(this.progs.copy!);
      this.gl!.uniform1i(u.uTex ?? null, this.attach(old.read, 0));
      this.blit(nf.read);
    }
    return nf;
  }

  private getRes(base: number): { w: number; h: number } {
    const gl = this.gl!;
    const aspect = Math.max(0.2, gl.drawingBufferWidth / Math.max(1, gl.drawingBufferHeight));
    const w = aspect >= 1 ? Math.round(base * aspect) : base;
    const h = aspect >= 1 ? base : Math.round(base / aspect);
    return { w: Math.max(8, w), h: Math.max(8, h) };
  }

  private rebuildTargets() {
    const gl = this.gl!;
    const filter = this.linearOK ? gl.LINEAR : gl.NEAREST;
    const sr = this.getRes(SIM_BASE);
    const dr = this.getRes(DYE_BASE);
    this.velocity = this.resizeDouble(this.velocity, sr.w, sr.h, filter);
    this.dye = this.resizeDouble(this.dye, dr.w, dr.h, filter);
    this.pressure = this.makeDoubleFBO(sr.w, sr.h, gl.NEAREST);
    this.divergence = this.makeFBO(sr.w, sr.h, gl.NEAREST);
    this.curl = this.makeFBO(sr.w, sr.h, gl.NEAREST);
    this.bufW = gl.drawingBufferWidth;
    this.bufH = gl.drawingBufferHeight;
    // bake the paper once at screen resolution
    this.paper = this.makeByteFBO(Math.max(8, this.bufW), Math.max(8, this.bufH));
    const u = this.use(this.progs.paper!);
    gl.uniform2f(u.uRes ?? null, this.paper.w, this.paper.h);
    this.blit(this.paper);
    // marble history survives a resize (stretched copy)
    if (this.marbleBaked) {
      const old = this.marbleBaked;
      this.marbleBaked = this.makeMarbleBaked(false);
      const uc = this.use(this.progs.copy!);
      gl.uniform1i(uc.uTex ?? null, this.attach(old.read, 0));
      this.blit(this.marbleBaked.read);
    }
  }

  private makeMarbleBaked(fillPaper: boolean) {
    const w = Math.max(8, this.bufW);
    const h = Math.max(8, this.bufH);
    let r = this.makeByteFBO(w, h);
    let wr = this.makeByteFBO(w, h);
    const mb = {
      get read() {
        return r;
      },
      get write() {
        return wr;
      },
      swap() {
        const t = r;
        r = wr;
        wr = t;
      },
      w,
      h,
    };
    if (fillPaper) {
      const u = this.use(this.progs.paper!);
      this.gl!.uniform2f(u.uRes ?? null, w, h);
      this.blit(mb.read);
    }
    return mb;
  }

  private ensureMarbleBaked() {
    if (!this.marbleBaked) this.marbleBaked = this.makeMarbleBaked(true);
  }

  private splatVel(x: number, y: number, dx: number, dy: number, radius: number) {
    const gl = this.gl!;
    const u = this.use(this.progs.splat!);
    gl.uniform1i(u.uTarget ?? null, this.attach(this.velocity.read, 0));
    gl.uniform1f(u.uAspect ?? null, this.canvas!.width / Math.max(1, this.canvas!.height));
    gl.uniform2f(u.uPoint ?? null, x, y);
    gl.uniform3f(u.uColor ?? null, dx, dy, 0);
    gl.uniform1f(u.uRadius ?? null, radius);
    this.blit(this.velocity.write);
    this.velocity.swap();
  }

  private splatDye(x: number, y: number, color: [number, number, number], radius: number, amt: number) {
    const gl = this.gl!;
    const u = this.use(this.progs.splat!);
    gl.uniform1i(u.uTarget ?? null, this.attach(this.dye.read, 0));
    gl.uniform1f(u.uAspect ?? null, this.canvas!.width / Math.max(1, this.canvas!.height));
    gl.uniform2f(u.uPoint ?? null, x, y);
    gl.uniform3f(u.uColor ?? null, color[0] * amt, color[1] * amt, color[2] * amt);
    gl.uniform1f(u.uRadius ?? null, radius);
    this.blit(this.dye.write);
    this.dye.swap();
  }
}
