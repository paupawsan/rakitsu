// GLSL source for the animated canvas wallpapers.
// Palette is locked to the rakitsu accent colors:
//   blue   #5b8def  vec3(0.357, 0.553, 0.937)
//   teal   #2dd4a0  vec3(0.176, 0.831, 0.627)
//   violet #c084fc  vec3(0.753, 0.518, 0.988)
//   pink   #f472b6  vec3(0.957, 0.447, 0.714)
//   bg     #12121f / #1a1a2e

export const VERT = `
attribute vec2 a_pos;
void main(){ gl_Position = vec4(a_pos, 0.0, 1.0); }
`;

// 01 Circuit Flow
const FRAG_1 = `precision highp float;
uniform vec2 u_res; uniform vec2 u_mouse; uniform float u_time; uniform vec4 u_click; uniform float u_active;
float hash(vec2 p){ return fract(sin(dot(p, vec2(127.1,311.7)))*43758.5453); }
void main(){
  vec2 frag = gl_FragCoord.xy;
  vec2 uv = (frag - u_res*0.5) / min(u_res.x, u_res.y);
  vec2 m = (u_mouse - u_res*0.5) / min(u_res.x, u_res.y);
  float d = distance(uv, m);
  vec2 dir = (uv - m) / max(d, 0.0001);
  uv += dir * exp(-d*d*4.0) * 0.35 * u_active;
  float cell = 0.12;
  vec2 gid = floor(uv/cell);
  vec2 g = fract(uv/cell) - 0.5;
  float r = hash(gid);
  float hasH = step(0.55, hash(gid + 11.3));
  float hasV = step(0.55, hash(gid + 77.7));
  float line = 0.0;
  if(hasH > 0.5) line = max(line, smoothstep(0.018, 0.0, abs(g.y)));
  if(hasV > 0.5) line = max(line, smoothstep(0.018, 0.0, abs(g.x)));
  float bus = 0.0;
  if(mod(gid.y, 6.0) == 0.0) bus = max(bus, smoothstep(0.028, 0.0, abs(g.y)));
  if(mod(gid.x, 6.0) == 0.0) bus = max(bus, smoothstep(0.028, 0.0, abs(g.x)));
  float speed = 0.6 + r*1.4;
  float phase = fract(u_time * speed * 0.25 + r);
  float along = 0.0;
  if(hasH > 0.5){ float x = mix(-0.5, 0.5, phase);
    along += smoothstep(0.04, 0.0, distance(g, vec2(x, 0.0))) * (1.0 - abs(g.y)*10.0); }
  if(hasV > 0.5){ float y = mix(-0.5, 0.5, 1.0 - phase);
    along += smoothstep(0.04, 0.0, distance(g, vec2(0.0, y))) * (1.0 - abs(g.x)*10.0); }
  along = max(along, 0.0);
  float nodes = 0.0;
  if(hasH > 0.5 && hasV > 0.5) nodes = smoothstep(0.035, 0.0, length(g));
  float clickAge = u_time - u_click.z;
  vec2 cm = (u_click.xy - u_res*0.5) / min(u_res.x, u_res.y);
  float cd = distance(uv, cm);
  float ring = exp(-pow(cd - clickAge*0.8, 2.0) * 80.0) * exp(-clickAge*1.2) * u_click.w;
  vec3 bg = mix(vec3(0.071,0.071,0.122), vec3(0.102,0.102,0.180), 0.5 + 0.5*sin(uv.x*0.6 + uv.y*0.4));
  vec3 blue = vec3(0.357,0.553,0.937), teal = vec3(0.176,0.831,0.627), violet = vec3(0.753,0.518,0.988);
  vec3 col = bg;
  col += line * blue * 0.45;
  col += bus * teal * 0.6;
  col += along * (blue + teal) * 1.6;
  col += nodes * violet * 1.2;
  col += ring * (teal + blue) * 1.4;
  float v = 1.0 - dot(uv*1.1, uv*1.1)*0.35;
  col *= clamp(v, 0.55, 1.0);
  gl_FragColor = vec4(col, 1.0);
}`;

// 02 Plasma Lattice
const FRAG_2 = `precision highp float;
uniform vec2 u_res; uniform vec2 u_mouse; uniform float u_time; uniform vec4 u_click; uniform float u_active;
mat2 rot(float a){ return mat2(cos(a),-sin(a),sin(a),cos(a)); }
void main(){
  vec2 frag = gl_FragCoord.xy;
  vec2 uv = (frag - u_res*0.5) / min(u_res.x,u_res.y);
  vec2 m = (u_mouse - u_res*0.5) / min(u_res.x,u_res.y);
  vec2 d = uv - m; float r = length(d);
  float pull = 1.0 / (1.0 + r*r*8.0);
  uv -= d * pull * 0.55 * u_active;
  vec2 cm = (u_click.xy - u_res*0.5) / min(u_res.x,u_res.y);
  float age = u_time - u_click.z;
  float burst = exp(-age*2.0) * u_click.w;
  vec2 cd = uv - cm; float cr = length(cd);
  uv += normalize(cd) * burst * 0.25 * exp(-cr*4.0);
  uv *= rot(u_time*0.05 + sin(u_time*0.2)*0.08);
  vec2 p = uv * 14.0;
  vec2 cell = fract(p) - 0.5;
  vec2 gid = floor(p);
  float pulse = 0.5 + 0.5*sin(u_time*1.4 + (gid.x+gid.y)*0.4);
  float size = mix(0.22, 0.38, pulse);
  float diamond = abs(cell.x) + abs(cell.y);
  float shape = smoothstep(size, size - 0.04, diamond);
  float outline = smoothstep(size+0.02, size, diamond) - smoothstep(size, size-0.01, diamond);
  float ang = atan(uv.y - m.y, uv.x - m.x);
  float h = (ang/6.2831 + 0.5);
  vec3 blue = vec3(0.357,0.553,0.937), teal = vec3(0.176,0.831,0.627), violet = vec3(0.753,0.518,0.988), pink = vec3(0.957,0.447,0.714);
  vec3 c = mix(blue, teal, smoothstep(0.0,0.5,h));
  c = mix(c, violet, smoothstep(0.5,0.8,h));
  c = mix(c, pink, smoothstep(0.8,1.0,h));
  float glow = exp(-r*1.5);
  vec3 bg = mix(vec3(0.071,0.071,0.122), vec3(0.102,0.102,0.180), 0.5 + 0.5*uv.y);
  vec3 col = bg;
  col += shape * c * (0.18 + 0.6*glow);
  col += outline * c * (0.35 + 0.8*glow);
  float ring = exp(-pow(cr - age*0.7, 2.0)*60.0) * exp(-age*1.4) * u_click.w;
  col += ring * (violet + pink) * 0.8;
  float v = 1.0 - dot(uv, uv)*0.4;
  col *= clamp(v, 0.55, 1.0);
  gl_FragColor = vec4(col, 1.0);
}`;

// 03 Volumetric Fog
const FRAG_3 = `precision highp float;
uniform vec2 u_res; uniform vec2 u_mouse; uniform float u_time; uniform vec4 u_click; uniform float u_active;
float hash(vec2 p){ return fract(sin(dot(p, vec2(127.1,311.7)))*43758.5453); }
float noise(vec2 p){ vec2 i=floor(p); vec2 f=fract(p);
  float a=hash(i), b=hash(i+vec2(1,0)), c=hash(i+vec2(0,1)), d=hash(i+vec2(1,1));
  vec2 u=f*f*(3.0-2.0*f);
  return mix(a,b,u.x) + (c-a)*u.y*(1.0-u.x) + (d-b)*u.x*u.y; }
float fbm(vec2 p){ float v=0.0, a=0.5; for(int i=0;i<6;i++){ v+=a*noise(p); p*=2.03; a*=0.5; } return v; }
void main(){
  vec2 frag = gl_FragCoord.xy;
  vec2 uv = (frag - u_res*0.5) / min(u_res.x,u_res.y);
  vec2 m = (u_mouse - u_res*0.5) / min(u_res.x,u_res.y);
  vec2 q = uv * 2.0;
  q += 0.3*vec2(fbm(q + u_time*0.08), fbm(q - u_time*0.06));
  vec2 dm = uv - m; float rm = length(dm);
  float swirl = 1.2 * exp(-rm*rm*4.0) * u_active;
  float a = atan(dm.y, dm.x) + swirl;
  q += vec2(cos(a), sin(a)) * swirl * 0.3;
  float n = fbm(q + u_time*0.05);
  float light = exp(-rm*rm*3.0) * 1.1 * u_active;
  vec2 cm = (u_click.xy - u_res*0.5) / min(u_res.x,u_res.y);
  float age = u_time - u_click.z;
  float cd = distance(uv, cm);
  float wave = exp(-pow(cd - age*0.6, 2.0)*40.0) * exp(-age*1.2) * u_click.w;
  vec3 bg = vec3(0.071,0.071,0.122);
  vec3 pink=vec3(0.957,0.447,0.714), violet=vec3(0.753,0.518,0.988), teal=vec3(0.176,0.831,0.627);
  vec3 fogA = mix(violet, pink, smoothstep(0.3,0.8,n));
  vec3 fogB = mix(fogA, teal, 0.15 + 0.15*sin(u_time*0.2));
  vec3 col = mix(bg, fogB, smoothstep(0.2, 0.9, n) * 0.85);
  col += light * (pink*0.8 + violet*0.3);
  col += wave * (teal*1.2);
  col = pow(col, vec3(0.95));
  float v = 1.0 - dot(uv, uv)*0.3;
  col *= clamp(v, 0.55, 1.0);
  gl_FragColor = vec4(col, 1.0);
}`;

// 04 Voronoi Cells
const FRAG_4 = `precision highp float;
uniform vec2 u_res; uniform vec2 u_mouse; uniform float u_time; uniform vec4 u_click; uniform float u_active;
vec2 hash2(vec2 p){ p = vec2(dot(p,vec2(127.1,311.7)), dot(p,vec2(269.5,183.3)));
  return fract(sin(p)*43758.5453); }
vec4 voronoi(vec2 uv){
  vec2 iv = floor(uv); vec2 fv = fract(uv);
  float md1=10.0, md2=10.0; vec2 mpt=vec2(0.0);
  for(int y=-1;y<=1;y++) for(int x=-1;x<=1;x++){
    vec2 g = vec2(float(x), float(y));
    vec2 o = hash2(iv + g);
    o = 0.5 + 0.5*sin(u_time*0.6 + 6.2831*o);
    vec2 r = g + o - fv;
    float d = dot(r,r);
    if(d < md1){ md2 = md1; md1 = d; mpt = r; } else if(d < md2){ md2 = d; }
  }
  return vec4(mpt, md1, md2 - md1);
}
void main(){
  vec2 frag = gl_FragCoord.xy;
  vec2 uv = (frag - u_res*0.5) / min(u_res.x,u_res.y);
  vec2 m = (u_mouse - u_res*0.5) / min(u_res.x,u_res.y);
  float rm = distance(uv, m);
  vec2 p = uv * 6.0;
  vec4 v = voronoi(p);
  float edge = smoothstep(0.0, 0.06, v.w);
  float cell = 1.0 - edge;
  vec2 idCell = floor(uv*6.0);
  float cid = fract(sin(dot(idCell, vec2(41.3, 17.1)))*43758.5453);
  vec3 blue=vec3(0.357,0.553,0.937), teal=vec3(0.176,0.831,0.627), violet=vec3(0.753,0.518,0.988), pink=vec3(0.957,0.447,0.714);
  vec3 cc = blue;
  int idx = int(mod(floor(cid*4.0), 4.0));
  if(idx == 1) cc = teal;
  if(idx == 2) cc = violet;
  if(idx == 3) cc = pink;
  float infl = exp(-rm*rm*1.8) * u_active;
  vec2 cm = (u_click.xy - u_res*0.5) / min(u_res.x,u_res.y);
  float age = u_time - u_click.z;
  float cd = distance(uv, cm);
  float ping = exp(-pow(cd - age*0.5, 2.0)*50.0) * exp(-age*1.5) * u_click.w;
  vec3 bg = vec3(0.071,0.071,0.122);
  vec3 col = mix(bg, cc*0.55, cell*(0.15 + 0.55*infl + 0.3*cell));
  col += edge * mix(vec3(0.2,0.25,0.4), cc, 0.5) * (0.35 + 0.5*infl);
  col += infl * cc * 0.6;
  col += ping * (cc + vec3(0.2)) * 1.0;
  float vg = 1.0 - dot(uv, uv)*0.35;
  col *= clamp(vg, 0.55, 1.0);
  gl_FragColor = vec4(col, 1.0);
}`;

// 05 Aurora Waves
const FRAG_5 = `precision highp float;
uniform vec2 u_res; uniform vec2 u_mouse; uniform float u_time; uniform vec4 u_click; uniform float u_active;
float hash(vec2 p){ return fract(sin(dot(p, vec2(127.1,311.7)))*43758.5453); }
float noise(vec2 p){ vec2 i=floor(p); vec2 f=fract(p);
  float a=hash(i), b=hash(i+vec2(1,0)), c=hash(i+vec2(0,1)), d=hash(i+vec2(1,1));
  vec2 u=f*f*(3.0-2.0*f);
  return mix(a,b,u.x) + (c-a)*u.y*(1.0-u.x) + (d-b)*u.x*u.y; }
float fbm(vec2 p){ float v=0.0, a=0.5; for(int i=0;i<5;i++){ v+=a*noise(p); p*=2.05; a*=0.5; } return v; }
void main(){
  vec2 frag = gl_FragCoord.xy;
  vec2 uv = (frag - u_res*0.5) / min(u_res.x,u_res.y);
  vec2 m = (u_mouse - u_res*0.5) / min(u_res.x,u_res.y);
  float wind = m.x * 2.0;
  float intensity = 1.0 - clamp(length(m-uv)*0.6, 0.0, 0.9);
  float y = uv.y * 2.2;
  float band = fbm(vec2(uv.x*1.5 + u_time*0.07 + wind*0.6, y + u_time*0.04));
  float band2 = fbm(vec2(uv.x*2.6 - u_time*0.09 - wind*0.4, y*0.9 + u_time*0.05));
  float drape = smoothstep(0.4, 0.9, band);
  float drape2 = smoothstep(0.35, 0.85, band2);
  float curtain = 0.0;
  for(int i=0; i<4; i++){ float fi = float(i); float yy = mix(-0.6, 0.6, fi/3.0);
    float w = fbm(vec2(uv.x*3.0 + u_time*(0.05+fi*0.02) + wind*(0.3+fi*0.1), fi*13.0));
    float d = abs(uv.y - yy - (w-0.5)*0.4);
    curtain += smoothstep(0.28, 0.0, d) * (0.4 + 0.2*w); }
  vec3 teal=vec3(0.176,0.831,0.627), blue=vec3(0.357,0.553,0.937), violet=vec3(0.753,0.518,0.988), pink=vec3(0.957,0.447,0.714);
  float hueMix = 0.5 + 0.5*sin(uv.x*1.6 + u_time*0.15 + wind);
  vec3 a = mix(teal, blue, hueMix);
  vec3 b = mix(violet, pink, hueMix);
  vec3 bandCol = mix(a, b, smoothstep(-0.2, 0.4, uv.y));
  vec2 cm = (u_click.xy - u_res*0.5) / min(u_res.x,u_res.y);
  float age = u_time - u_click.z;
  float crest = exp(-pow(uv.y - cm.y + age*0.3, 2.0)*50.0) * exp(-age*1.1) * u_click.w;
  vec3 bg = mix(vec3(0.071,0.071,0.122), vec3(0.102,0.102,0.180), 0.5 + 0.5*uv.y);
  vec3 col = bg;
  col += bandCol * drape * 0.35 * (0.5 + 0.8*intensity*u_active);
  col += bandCol * drape2 * 0.25;
  col += bandCol * curtain * 0.55 * (0.6 + 0.6*intensity*u_active);
  col += crest * (teal + vec3(0.2)) * 0.9;
  float rm = distance(uv, m);
  col += exp(-rm*rm*3.5) * 0.15 * mix(teal, blue, 0.5) * u_active;
  float star = pow(hash(floor(uv*vec2(180.0, 100.0))), 80.0);
  col += star * 0.4 * smoothstep(-0.2, 0.5, uv.y);
  float v = 1.0 - dot(uv, uv)*0.25;
  col *= clamp(v, 0.6, 1.0);
  gl_FragColor = vec4(col, 1.0);
}`;

// 06 Water Ripple — pool seen from above. Click & drag emit ripples that
// stack via a circular buffer of MAX_R slots, expanding outward with a
// dispersive wavefront (sharp leading crest + decaying secondary train +
// capillary fizz). The wave gradient drives refraction into a procedural
// reef floor with sand, coral mounds, sea-grass and drifting caustics.
const FRAG_6 = `precision highp float;
#define MAX_R 12
uniform vec2  u_res;
uniform vec2  u_mouse;
uniform float u_time;
uniform float u_active;
uniform vec4  u_ripples[MAX_R]; // xy=norm pos (uv space), z=tStart, w=strength
float wave(vec2 uv, vec2 src, float age, float str){
  if(age < 0.0 || age > 9.0 || str < 0.001) return 0.0;
  float d = distance(uv, src);
  float spd = 0.40;
  float front = age * spd;
  float dr = d - front;
  float crest = exp(-dr*dr * 130.0);
  float train = cos(48.0 * dr - age * 3.5)
              * exp(-dr*dr * 14.0)
              * smoothstep(-0.02, -0.0005, dr);
  float capil = sin(140.0 * dr - age * 9.0)
              * exp(-dr*dr * 280.0) * 0.35;
  float spread = 1.0 / sqrt(max(d, 0.05) * 1.6);
  float decay = exp(-age * 0.42);
  return (crest * 0.65 + train * 0.42 + capil) * decay * str * spread;
}
float field(vec2 uv){
  float h = 0.0;
  for(int i = 0; i < MAX_R; i++){
    vec4 r = u_ripples[i];
    h += wave(uv, r.xy, u_time - r.z, r.w);
  }
  return h;
}
float hash(vec2 p){ return fract(sin(dot(p, vec2(127.1, 311.7)))*43758.5453); }
float n2(vec2 p){
  vec2 i=floor(p), f=fract(p);
  float a=hash(i), b=hash(i+vec2(1,0)), c=hash(i+vec2(0,1)), d=hash(i+vec2(1,1));
  vec2 u=f*f*(3.0-2.0*f);
  return mix(a,b,u.x)+(c-a)*u.y*(1.0-u.x)+(d-b)*u.x*u.y;
}
vec3 floorImage(vec2 p){
  vec2 sway = 0.014 * vec2(sin(p.y*3.4 + u_time*0.6), cos(p.x*3.0 + u_time*0.5));
  p += sway;
  float sandRipple = sin(p.x*22.0 + n2(p*4.0)*6.0) * 0.5 + 0.5;
  float sandTex = n2(p*30.0) * 0.5 + n2(p*80.0) * 0.3;
  vec3 sand = mix(vec3(0.78, 0.70, 0.52), vec3(0.62, 0.55, 0.42), sandTex);
  sand *= 0.90 + sandRipple * 0.10;
  vec3 col = sand;
  vec2 sc = p * 3.4;
  vec2 ic = floor(sc), fc = fract(sc);
  float md = 1.0; vec2 mid = vec2(0.0); vec2 mco = vec2(0.0);
  for(int j=-1;j<=1;j++){
    for(int i=-1;i<=1;i++){
      vec2 g = vec2(float(i), float(j));
      vec2 o = vec2(hash(ic+g+1.7), hash(ic+g+9.1));
      vec2 r = g + o - fc;
      float d2 = dot(r,r);
      if(d2 < md){ md = d2; mid = ic + g; mco = r; }
    }
  }
  float cellType = hash(mid + 7.3);
  vec3 coralPink   = vec3(0.95, 0.45, 0.55);
  vec3 coralOrange = vec3(0.96, 0.58, 0.32);
  vec3 coralPurple = vec3(0.62, 0.42, 0.78);
  vec3 coralYellow = vec3(0.92, 0.80, 0.42);
  vec3 anemTeal    = vec3(0.30, 0.78, 0.72);
  vec3 seaweedGr   = vec3(0.28, 0.55, 0.38);
  vec3 coralCol = mix(
    mix(coralPink, coralOrange, smoothstep(0.0, 0.5, cellType)),
    mix(coralPurple, mix(coralYellow, anemTeal, smoothstep(0.7,1.0,cellType)),
        smoothstep(0.5, 1.0, cellType)),
    step(0.5, cellType)
  );
  float d = length(mco);
  float coralR = 0.30 + hash(mid + 2.1) * 0.15;
  float occupied = step(0.45, hash(mid));
  if(occupied > 0.5){
    float colony = smoothstep(coralR, coralR - 0.10, d);
    float bumps = 0.75 + n2(p*22.0 + mid*4.0) * 0.35;
    vec3 colonyCol = coralCol * bumps;
    colonyCol *= 0.7 + 0.4 * smoothstep(coralR, 0.0, d);
    col = mix(col, colonyCol, colony * occupied);
  }
  vec2 gp = p * vec2(8.0, 2.5);
  float bladeX = floor(gp.x);
  float bladeNoise = hash(vec2(bladeX, 11.0));
  if(bladeNoise > 0.78){
    float wv = sin(p.y*4.0 + u_time*1.2 + bladeNoise*30.0) * 0.04;
    float dx = abs(fract(gp.x) - 0.5 + wv);
    float blade = smoothstep(0.04, 0.0, dx);
    blade *= smoothstep(0.05, 0.25, p.y) * smoothstep(0.85, 0.55, p.y);
    col = mix(col, seaweedGr * (0.6 + 0.4*hash(vec2(bladeX, 3.0))), blade * 0.85);
  }
  vec2 cuv = p * 3.5 + vec2(u_time*0.05, -u_time*0.03);
  float caustic = abs(n2(cuv) - 0.5) + 0.7 * abs(n2(cuv*2.1 + 11.0) - 0.5);
  caustic = pow(1.0 - clamp(caustic*1.4, 0.0, 1.0), 4.0);
  col += caustic * vec3(0.55, 0.80, 0.75) * 0.6;
  float speck = step(0.985, hash(floor(p*55.0)));
  col = mix(col, vec3(0.9, 0.85, 0.78), speck * 0.4);
  return col;
}
void main(){
  vec2 frag = gl_FragCoord.xy;
  vec2 res  = u_res;
  float aspect = res.x / res.y;
  vec2 uv = vec2(frag.x / res.y, frag.y / res.y);
  float e = 4.0 / res.y;
  float h  = field(uv);
  float hx = field(uv + vec2(e, 0.0));
  float hy = field(uv + vec2(0.0, e));
  vec2 grad = vec2(hx - h, hy - h) / e;
  vec2 sway = vec2(
    n2(uv*1.4 + vec2(u_time*0.18, 0.0)) - 0.5,
    n2(uv*1.4 + vec2(0.0, u_time*0.22) + 31.7) - 0.5
  ) * 0.06;
  vec2 refr = -grad * 0.022 + sway;
  vec3 floorCol = floorImage(uv + refr);
  vec2 cv = vec2(uv.x - aspect*0.5, uv.y - 0.5);
  float edgeDepth = clamp(dot(cv, cv) * 1.0, 0.0, 1.0);
  float vertDepth = 1.0 - clamp(uv.y * 0.85, 0.0, 1.0);
  float baseDepth = mix(edgeDepth, vertDepth, 0.6);
  float depth = clamp(0.30 + baseDepth*0.75 - h*0.40, 0.05, 1.0);
  vec3 deep    = vec3(0.005, 0.045, 0.130);
  vec3 mid     = vec3(0.020, 0.180, 0.330);
  vec3 shallow = vec3(0.180, 0.620, 0.700);
  vec3 surface = vec3(0.500, 0.880, 0.880);
  vec3 sky     = vec3(0.620, 0.820, 0.980);
  vec3 waterTint = mix(shallow, mix(mid, deep, depth*depth), depth);
  vec3 transmit = mix(vec3(1.0), waterTint*1.6, depth*0.85);
  vec3 col = mix(floorCol * transmit, waterTint, depth * 0.65);
  float rays = pow(clamp(n2(uv*vec2(2.5,1.0) + vec2(u_time*0.04,0.0))*1.4 - 0.2, 0.0, 1.0), 2.0)
             * smoothstep(0.0, 0.7, 1.0 - uv.y) * (1.0 - depth*0.5);
  col += rays * surface * 0.18;
  float fres = pow(clamp(length(grad)*0.04, 0.0, 1.0), 1.6);
  col = mix(col, sky*0.55 + waterTint*0.45, fres * 0.30);
  float spec = pow(clamp(h*1.2, 0.0, 1.0), 4.0);
  col += spec * mix(surface, waterTint, 0.7) * 0.18;
  float foam = smoothstep(0.6, 1.0, length(grad)*0.05) * smoothstep(0.0, 0.4, h);
  foam *= 0.4 + 0.6 * n2(uv*60.0 + u_time*1.5);
  col += foam * vec3(0.85, 0.95, 1.0) * 0.35;
  float glint = step(0.9962, hash(floor(uv*220.0) + floor(u_time*7.0)*0.013));
  glint *= smoothstep(0.22, 1.0, length(grad)*0.06 + foam);
  col += glint * vec3(1.0, 0.97, 0.88) * 0.9;
  col -= clamp(-h, 0.0, 1.0) * deep * 0.6;
  float md = distance(uv, vec2(u_mouse.x / res.y, u_mouse.y / res.y));
  col += exp(-md*md*30.0) * 0.04 * surface * u_active;
  float v = 1.0 - dot(cv, cv) * 0.65;
  col *= clamp(v, 0.62, 1.0);
  gl_FragColor = vec4(col, 1.0);
}`;

// 07 Suminagashi — Japanese ink marbling floated on still water. This is the
// FALLBACK renderer (analytic mathematical marbling): every drop displaces
// the whole existing pattern radially, so each pixel's color is recovered by
// walking the drop list newest→oldest applying the INVERSE displacement
// until it lands inside a drop. The primary renderer is the GPU fluid sim in
// utils/sumiFluid.ts; this shader is used when half-float FBOs are missing.
const FRAG_7 = `precision highp float;
#define ND 64
uniform vec2  u_res;
uniform vec2  u_mouse;
uniform float u_time;
uniform float u_active;
uniform vec4  u_drops[ND]; // xy = center (uv), z = radius, w = color id
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
  vec2 frag = gl_FragCoord.xy;
  vec2 res  = u_res;
  vec2 uv   = frag / res.y;
  vec2 m  = u_mouse / res.y;
  vec2 dm = uv - m;
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
  float fiber = fbm(uv*vec2(60.0, 14.0)) * 0.5 + fbm(uv*vec2(9.0, 42.0)) * 0.5;
  float grain = n2(uv*420.0);
  vec3 paper = vec3(0.912, 0.882, 0.815);
  paper -= fiber * 0.045;
  paper -= grain * 0.022;
  float fleck = step(0.9965, hash(floor(uv*330.0)));
  paper = mix(paper, vec3(0.68, 0.63, 0.54), fleck * 0.5);
  vec3 sumi   = vec3(0.118, 0.110, 0.102);
  vec3 indigo = vec3(0.165, 0.255, 0.420);
  vec3 vermil = vec3(0.690, 0.270, 0.160);
  vec3 col = paper;
  if(colId > 0.5 && colId < 1.5) col = sumi;
  else if(colId > 1.5 && colId < 2.5) col = indigo;
  else if(colId > 2.5) col = vermil;
  if(colId > 0.5){
    float gran = fbm(uv*26.0 + colId*7.0);
    col = mix(col, paper, gran*gran*0.38);
    col = mix(col, col*0.7, smoothstep(0.75, 1.0, n2(uv*90.0))*0.25);
  }
  col += exp(-rm*rm*9.0) * 0.035 * u_active;
  vec2 cv = vec2(uv.x - (res.x/res.y)*0.5, uv.y - 0.5);
  float v = 1.0 - dot(cv, cv) * 0.32;
  col *= clamp(v, 0.78, 1.0);
  gl_FragColor = vec4(col, 1.0);
}`;

export const FRAGS: string[] = [FRAG_1, FRAG_2, FRAG_3, FRAG_4, FRAG_5, FRAG_6, FRAG_7];

/** Index of the water shader (used by the ripple buffer wiring in useShaderWallpaper). */
export const WATER_INDEX = 5;
/** Ripple buffer slot count — must match `#define MAX_R` in FRAG_6. */
export const WATER_RIPPLE_SLOTS = 12;
/** Index of the Suminagashi shader (fluid sim with FRAG_7 marbling fallback). */
export const SUMI_INDEX = 6;
/** Marble drop list size — must match `#define ND` in FRAG_7 and sumiFluid's marble shader. */
export const SUMI_DROP_SLOTS = 64;
