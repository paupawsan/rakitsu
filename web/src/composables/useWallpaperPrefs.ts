import { ref, watch } from 'vue';

const STORAGE_KEY = 'rakitsu-wallpaper-prefs';
const DEBOUNCE_MS = 500;
const SHADER_COUNT = 7;

interface StoredPrefs {
  enabled?: boolean;
  shaderIndex?: number;
  dim?: number;
}

function loadInitial(): Required<StoredPrefs> {
  const defaults = { enabled: false, shaderIndex: 0, dim: 55 };
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return defaults;
    const parsed = JSON.parse(raw) as StoredPrefs;
    return {
      enabled: typeof parsed.enabled === 'boolean' ? parsed.enabled : defaults.enabled,
      shaderIndex:
        typeof parsed.shaderIndex === 'number' && parsed.shaderIndex >= 0 && parsed.shaderIndex < SHADER_COUNT
          ? Math.floor(parsed.shaderIndex)
          : defaults.shaderIndex,
      dim:
        typeof parsed.dim === 'number' && parsed.dim >= 0 && parsed.dim <= 100
          ? parsed.dim
          : defaults.dim,
    };
  } catch {
    return defaults;
  }
}

const initial = loadInitial();
const enabled = ref(initial.enabled);
const shaderIndex = ref(initial.shaderIndex);
const dim = ref(initial.dim);

let saveTimer: ReturnType<typeof setTimeout> | null = null;
watch([enabled, shaderIndex, dim], () => {
  if (saveTimer) clearTimeout(saveTimer);
  saveTimer = setTimeout(() => {
    try {
      const payload: StoredPrefs = {
        enabled: enabled.value,
        shaderIndex: shaderIndex.value,
        dim: dim.value,
      };
      localStorage.setItem(STORAGE_KEY, JSON.stringify(payload));
    } catch {
      /* storage full or blocked — silently ignore, prefs still live in memory */
    }
  }, DEBOUNCE_MS);
});

export function useWallpaperPrefs() {
  return { enabled, shaderIndex, dim, shaderCount: SHADER_COUNT };
}
