<script setup lang="ts">
import { onBeforeUnmount, onMounted } from 'vue';
import { useWallpaperPrefs } from '../composables/useWallpaperPrefs';

const props = defineProps<{ visible: boolean }>();

const { enabled, shaderIndex, dim, shaderCount } = useWallpaperPrefs();

const SHADER_LABELS = ['flow', 'lattice', 'fog', 'cells', 'aurora', 'water', 'sumi'];

function pick(i: number) {
  shaderIndex.value = i;
  enabled.value = true;
}

function turnOff() {
  enabled.value = false;
}

function onKey(e: KeyboardEvent) {
  if (!props.visible) return;
  const tag = (e.target as HTMLElement | null)?.tagName;
  if (tag === 'INPUT' || tag === 'TEXTAREA') return;
  const n = parseInt(e.key, 10);
  if (!isNaN(n) && n >= 1 && n <= shaderCount) {
    pick(n - 1);
  }
}

onMounted(() => window.addEventListener('keydown', onKey));
onBeforeUnmount(() => window.removeEventListener('keydown', onKey));
</script>

<template>
  <div class="wallpaper-picker" :class="{ inactive: !enabled }">
    <span class="label">WALLPAPER</span>
    <button
      type="button"
      class="off-btn"
      :class="{ active: !enabled }"
      title="Disable wallpaper"
      @click="turnOff"
    >
      <span class="off-icon">○</span>
    </button>
    <button
      v-for="(lbl, i) in SHADER_LABELS"
      :key="i"
      type="button"
      class="thumb"
      :class="[`thumb-${i + 1}`, { active: enabled && shaderIndex === i }]"
      :title="`${lbl} (press ${i + 1})`"
      @click="pick(i)"
    >
      <span class="idx">{{ String(i + 1).padStart(2, '0') }}</span>
    </button>
    <div class="intensity">
      <span class="label dim-label">DIM</span>
      <input
        v-model.number="dim"
        type="range"
        min="0"
        max="100"
        :disabled="!enabled"
        aria-label="Wallpaper dim"
      />
    </div>
  </div>
</template>

<style scoped>
.wallpaper-picker {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px;
  background: rgba(26, 26, 46, 0.7);
  border: 1px solid rgba(255, 255, 255, 0.08);
  backdrop-filter: blur(12px) saturate(140%);
  -webkit-backdrop-filter: blur(12px) saturate(140%);
  border-radius: 6px;
  box-shadow: 0 6px 24px rgba(0, 0, 0, 0.45);
  pointer-events: auto;
}

.wallpaper-picker.inactive {
  opacity: 0.8;
}

.label {
  font-family: var(--font-mono, 'SF Mono', ui-monospace, Menlo, Consolas, monospace);
  font-size: 10px;
  color: var(--text-label, #8888a0);
  letter-spacing: 0.1em;
  text-transform: uppercase;
  padding: 0 4px 0 6px;
}

.dim-label {
  padding: 0;
}

.off-btn {
  all: unset;
  cursor: pointer;
  width: 28px;
  height: 28px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 4px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  color: var(--text-secondary, #9a9ab0);
  font-size: 14px;
  transition: border-color 0.15s ease, color 0.15s ease;
}
.off-btn:hover {
  border-color: rgba(255, 255, 255, 0.2);
  color: var(--text-primary, #e0e0e0);
}
.off-btn.active {
  border-color: var(--accent-agent, #5b8def);
  color: var(--accent-agent, #5b8def);
  box-shadow: 0 0 0 1px var(--accent-agent, #5b8def);
}

.thumb {
  all: unset;
  cursor: pointer;
  width: 40px;
  height: 28px;
  border-radius: 4px;
  position: relative;
  overflow: hidden;
  border: 1px solid rgba(255, 255, 255, 0.08);
  transition: transform 0.15s ease, border-color 0.15s ease, box-shadow 0.15s ease;
}
.thumb:hover {
  transform: translateY(-1px);
  border-color: rgba(255, 255, 255, 0.2);
}
.thumb.active {
  border-color: var(--accent-agent, #5b8def);
  box-shadow: 0 0 0 1px var(--accent-agent, #5b8def);
}
.thumb .idx {
  position: absolute;
  bottom: 2px;
  right: 4px;
  font-family: var(--font-mono, 'SF Mono', ui-monospace, Menlo, Consolas, monospace);
  font-size: 8px;
  color: white;
  letter-spacing: 0.08em;
  text-shadow: 0 1px 1px rgba(0, 0, 0, 0.6);
  pointer-events: none;
}

.thumb-1 {
  background:
    radial-gradient(120% 80% at 30% 40%, #5b8def44 0%, transparent 60%),
    radial-gradient(120% 80% at 80% 80%, #2dd4a033 0%, transparent 60%),
    linear-gradient(135deg, #1a1a2e, #12121f);
}
.thumb-2 {
  background:
    repeating-linear-gradient(45deg, #c084fc22 0 2px, transparent 2px 8px),
    repeating-linear-gradient(-45deg, #5b8def22 0 2px, transparent 2px 8px),
    #12121f;
}
.thumb-3 {
  background: radial-gradient(70% 60% at 55% 45%, #f472b655 0%, #12121f 70%);
}
.thumb-4 {
  background:
    radial-gradient(40% 40% at 25% 30%, #2dd4a055 0%, transparent 60%),
    radial-gradient(40% 40% at 70% 60%, #c084fc55 0%, transparent 60%),
    radial-gradient(40% 40% at 50% 80%, #5b8def44 0%, transparent 60%),
    #12121f;
}
.thumb-5 {
  background: linear-gradient(170deg, #12121f 0%, #5b8def33 35%, #2dd4a022 55%, #c084fc33 75%, #12121f 100%);
}
.thumb-6 {
  background:
    radial-gradient(ellipse 70% 40% at 50% 30%, rgba(180, 230, 230, 0.35) 0%, transparent 60%),
    radial-gradient(circle at 30% 70%, rgba(212, 134, 55, 0.4) 0%, transparent 35%),
    radial-gradient(circle at 70% 65%, rgba(95, 180, 200, 0.35) 0%, transparent 35%),
    linear-gradient(180deg, #2c8ca8 0%, #134a72 60%, #0a2240 100%);
}
.thumb-7 {
  background:
    repeating-radial-gradient(circle at 42% 48%, #1f1d1a 0 1.5px, #e8e2d2 1.5px 7px),
    #e8e2d2;
}
/* Light washi thumb needs dark text */
.thumb-7 .idx {
  color: rgba(58, 54, 48, 0.75);
  text-shadow: none;
}

.intensity {
  display: flex;
  align-items: center;
  gap: 6px;
  padding-left: 8px;
  margin-left: 4px;
  border-left: 1px solid rgba(255, 255, 255, 0.08);
}
.intensity input[type='range'] {
  width: 60px;
  accent-color: var(--accent-agent, #5b8def);
}
.intensity input[type='range']:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}
</style>
