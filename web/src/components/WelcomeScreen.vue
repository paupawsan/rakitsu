<script setup lang="ts">
import { ref, onMounted, computed, watch } from 'vue';
import ComboBox from './builder/ComboBox.vue';

interface DemoProvider {
  name: string;
  label: string;
  model: string;
}

const emit = defineEmits<{
  'run-demo': [provider: string, model: string, envVars: Record<string, string>];
  'dismiss': [dontShowAgain: boolean];
}>();

const dontShowAgain = ref(false);

const providers = ref<DemoProvider[]>([]);
const selectedProvider = ref('');
const selectedModel = ref('');
const loading = ref(true);

// Credential fields per provider
const apiKey = ref('');
const baseUrl = ref('');

// Connection test state
const connectionStatus = ref<'idle' | 'testing' | 'ok' | 'error'>('idle');
const connectionError = ref('');
const discoveredModels = ref<string[]>([]);

interface ProviderFields {
  keyEnv: string;
  keyPlaceholder: string;
  urlEnv?: string;
  urlDefault?: string;
  urlPlaceholder?: string;
  probeUrl?: string; // base URL to probe for models (OpenAI-compatible)
}

const providerFields: Record<string, ProviderFields> = {
  ollama: {
    keyEnv: '',
    keyPlaceholder: '',
    urlEnv: 'OLLAMA_BASE_URL',
    urlDefault: 'http://localhost:11434/v1',
    urlPlaceholder: 'http://localhost:11434/v1',
  },
  litellm: {
    keyEnv: 'LITELLM_API_KEY',
    keyPlaceholder: 'sk-...',
    urlEnv: 'LITELLM_BASE_URL',
    urlDefault: '',
    urlPlaceholder: 'https://your-litellm-server/v1',
  },
  openai: {
    keyEnv: 'OPENAI_API_KEY',
    keyPlaceholder: 'sk-...',
    probeUrl: 'https://api.openai.com/v1',
  },
  anthropic: {
    keyEnv: 'ANTHROPIC_API_KEY',
    keyPlaceholder: 'sk-ant-...',
    // Anthropic doesn't have /v1/models — skip probe
  },
  gemini: {
    keyEnv: 'GEMINI_API_KEY',
    keyPlaceholder: 'AI...',
    // Gemini uses a different API — skip probe
  },
};

const fields = computed(() => providerFields[selectedProvider.value]);
const needsKey = computed(() => fields.value?.keyEnv);
const needsUrl = computed(() => fields.value?.urlEnv);

// Can we probe this provider? (OpenAI-compatible endpoints only)
const canProbe = computed(() => {
  const p = selectedProvider.value;
  if (p === 'anthropic' || p === 'gemini') return false;
  const url = fields.value?.probeUrl || baseUrl.value;
  return !!url;
});

function selectProvider(p: DemoProvider) {
  selectedProvider.value = p.name;
  selectedModel.value = p.model;
  apiKey.value = '';
  baseUrl.value = '';
  connectionStatus.value = 'idle';
  connectionError.value = '';
  discoveredModels.value = [];
  // Set default URL after fields computed updates, then auto-probe if no key needed
  setTimeout(() => {
    baseUrl.value = providerFields[p.name]?.urlDefault ?? '';
    if (!providerFields[p.name]?.keyEnv && baseUrl.value) {
      testConnection();
    }
  }, 0);
}

// Reset connection status when credentials change
watch([apiKey, baseUrl], () => {
  if (connectionStatus.value !== 'idle') {
    connectionStatus.value = 'idle';
    connectionError.value = '';
    discoveredModels.value = [];
  }
});

async function testConnection() {
  const probeBaseUrl = fields.value?.probeUrl || baseUrl.value;
  if (!probeBaseUrl) return;

  connectionStatus.value = 'testing';
  connectionError.value = '';
  discoveredModels.value = [];

  try {
    const params = new URLSearchParams({ base_url: probeBaseUrl });
    const headers: HeadersInit = {};
    if (apiKey.value) headers['Authorization'] = `Bearer ${apiKey.value}`;

    const res = await fetch(`/api/providers/models?${params}`, { headers });
    if (!res.ok) {
      connectionStatus.value = 'error';
      connectionError.value = `HTTP ${res.status}`;
      return;
    }
    const data = await res.json();
    const models: string[] = [];
    if (Array.isArray(data?.models)) {
      for (const m of data.models) {
        if (typeof m === 'string') models.push(m);
      }
    }
    if (models.length > 0) {
      connectionStatus.value = 'ok';
      discoveredModels.value = models.sort();
      // Auto-select first model if current selection isn't in the list
      if (!models.includes(selectedModel.value)) {
        selectedModel.value = models[0] ?? '';
      }
    } else {
      connectionStatus.value = 'error';
      connectionError.value = 'Connected but no models found';
    }
  } catch {
    connectionStatus.value = 'error';
    connectionError.value = 'Connection failed';
  }
}

const canRun = computed(() => {
  if (!selectedProvider.value || !selectedModel.value) return false;
  if (needsKey.value && !apiKey.value) return false;
  if (needsUrl.value && !baseUrl.value) return false;
  return true;
});

function handleRun() {
  const envVars: Record<string, string> = {};
  if (needsKey.value && apiKey.value) {
    envVars[fields.value!.keyEnv] = apiKey.value;
  }
  if (needsUrl.value && baseUrl.value) {
    envVars[fields.value!.urlEnv!] = baseUrl.value;
  }
  emit('run-demo', selectedProvider.value, selectedModel.value, envVars);
}

onMounted(async () => {
  try {
    const res = await fetch('/api/demo');
    if (res.ok) {
      const data = await res.json();
      if (data.providers) {
        providers.value = data.providers;
        const ollama = data.providers.find((p: DemoProvider) => p.name === 'ollama');
        if (ollama) selectProvider(ollama);
        else if (data.providers.length > 0) selectProvider(data.providers[0]);
      }
    }
  } catch { /* ignore */ }
  loading.value = false;
});
</script>

<template>
  <div class="welcome-overlay">
    <div class="welcome-card">
      <div class="welcome-brand">
        <span class="brand-mark">R&gt;</span>
        <span class="brand-name">Rakitsu</span>
      </div>
      <p class="welcome-tagline">The Agent IDE</p>
      <p class="welcome-desc">
        Build, debug, and ship AI agents — all in one tool.<br>
        Pick a provider, then run the demo to see the debugger in action.
      </p>

      <div class="field-group">
        <label class="field-label">Provider</label>
        <div class="provider-list">
          <button
            v-for="p in providers"
            :key="p.name"
            class="provider-option"
            :class="{ selected: selectedProvider === p.name }"
            @click="selectProvider(p)"
          >
            {{ p.label }}
          </button>
        </div>
      </div>

      <div v-if="needsKey" class="field-group">
        <label class="field-label">API Key</label>
        <input
          v-model="apiKey"
          type="password"
          class="field-input"
          :placeholder="fields?.keyPlaceholder"
          autocomplete="off"
        />
        <span class="field-hint">Passed as {{ fields?.keyEnv ?? '' }}. Not stored.</span>
      </div>

      <div v-if="needsUrl" class="field-group">
        <label class="field-label">Base URL</label>
        <input
          v-model="baseUrl"
          class="field-input"
          :placeholder="fields?.urlPlaceholder"
        />
      </div>

      <!-- Model selection with discovery -->
      <div v-if="selectedProvider" class="field-group">
        <div class="model-header">
          <label class="field-label">Model</label>
          <button
            v-if="canProbe"
            class="test-btn"
            :class="{ testing: connectionStatus === 'testing' }"
            :disabled="connectionStatus === 'testing'"
            @click="testConnection"
          >
            <template v-if="connectionStatus === 'testing'">Testing...</template>
            <template v-else-if="connectionStatus === 'ok'">
              <span class="led led-ok"></span> {{ discoveredModels.length }} models
            </template>
            <template v-else-if="connectionStatus === 'error'">
              <span class="led led-err"></span> Retry
            </template>
            <template v-else>Test Connection</template>
          </button>
        </div>

        <ComboBox
          :model-value="selectedModel"
          :options="discoveredModels"
          :allow-custom="true"
          placeholder="Type or select a model..."
          @update:model-value="selectedModel = $event"
        />

        <span v-if="connectionStatus === 'error'" class="field-error">
          {{ connectionError }}
        </span>
      </div>

      <button
        class="demo-btn"
        :disabled="!canRun || loading"
        @click="handleRun"
      >
        Run the Demo
      </button>

      <label class="dont-show">
        <input type="checkbox" v-model="dontShowAgain" />
        Don't show on next launch
      </label>

      <button class="skip-btn" @click="emit('dismiss', dontShowAgain)">
        Skip &rarr;
      </button>
    </div>
  </div>
</template>

<style scoped>
.welcome-overlay {
  position: fixed;
  inset: 0;
  z-index: 100;
  background: var(--surface-0);
  display: flex;
  align-items: center;
  justify-content: center;
  overflow-y: auto;
}

.welcome-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 16px;
  padding: 40px 48px;
  margin: 20px;
  background: var(--surface-1);
  border: 1px solid var(--border-default);
  border-radius: var(--node-radius);
  max-width: 460px;
  width: 100%;
  text-align: center;
}

.welcome-brand {
  display: flex;
  align-items: baseline;
  gap: 8px;
}
.brand-mark {
  font-family: var(--font-mono);
  font-size: 28px;
  color: var(--accent-agent);
  font-weight: 700;
}
.brand-name {
  font-size: 28px;
  font-weight: 600;
  color: var(--text-primary);
}

.welcome-tagline {
  margin: 0;
  font-size: 14px;
  color: var(--text-secondary);
  letter-spacing: 0.5px;
  text-transform: uppercase;
}

.welcome-desc {
  margin: 0;
  font-size: 13px;
  color: var(--text-muted);
  line-height: 1.5;
}

.field-group {
  width: 100%;
  text-align: left;
}

.field-label {
  display: block;
  font-size: 11px;
  color: var(--text-label);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  margin-bottom: 6px;
}

.field-input {
  width: 100%;
  padding: 7px 10px;
  font-size: 13px;
  font-family: var(--font-mono);
  color: var(--input-text);
  background: var(--input-bg);
  border: 1px solid var(--input-border);
  border-radius: var(--node-radius);
  outline: none;
}
.field-input:focus {
  border-color: var(--border-focus);
}

.field-hint {
  display: block;
  margin-top: 4px;
  font-size: 10px;
  color: var(--text-muted);
}

.field-error {
  display: block;
  margin-top: 4px;
  font-size: 11px;
  color: var(--status-error);
}

.provider-list {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.provider-option {
  padding: 6px 12px;
  font-size: 12px;
  color: var(--text-secondary);
  background: var(--surface-2);
  border: 1px solid var(--border-subtle);
  border-radius: var(--node-radius);
  cursor: pointer;
  transition: all 0.15s;
}
.provider-option:hover {
  border-color: var(--border-default);
  color: var(--text-primary);
}
.provider-option.selected {
  background: var(--accent-agent);
  border-color: var(--accent-agent);
  color: #fff;
}

.model-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 6px;
}
.model-header .field-label {
  margin-bottom: 0;
}

.test-btn {
  display: flex;
  align-items: center;
  gap: 5px;
  padding: 3px 10px;
  font-size: 11px;
  color: var(--text-secondary);
  background: var(--surface-3);
  border: 1px solid var(--border-subtle);
  border-radius: var(--node-radius);
  cursor: pointer;
  transition: all 0.15s;
}
.test-btn:hover:not(:disabled) {
  border-color: var(--border-default);
  color: var(--text-primary);
}
.test-btn:disabled {
  opacity: 0.6;
  cursor: wait;
}
.test-btn.testing {
  color: var(--text-muted);
}

.led {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  flex-shrink: 0;
}
.led-ok { background: var(--status-running); }
.led-err { background: var(--status-error); }

.demo-btn {
  width: 100%;
  padding: 10px 20px;
  font-size: 14px;
  font-weight: 600;
  color: #fff;
  background: var(--accent-agent);
  border: none;
  border-radius: var(--node-radius);
  cursor: pointer;
  transition: opacity 0.15s;
}
.demo-btn:hover:not(:disabled) {
  opacity: 0.9;
}
.demo-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.dont-show {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
  color: var(--text-muted);
  cursor: pointer;
  user-select: none;
}
.dont-show input {
  accent-color: var(--accent-agent);
}

.skip-btn {
  background: none;
  border: none;
  color: var(--text-muted);
  font-size: 12px;
  cursor: pointer;
  padding: 4px 8px;
}
.skip-btn:hover {
  color: var(--text-secondary);
}
</style>
