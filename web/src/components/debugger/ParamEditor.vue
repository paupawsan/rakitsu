<script setup lang="ts">
import { ref } from 'vue';

const props = defineProps<{
  agentName: string;
  isConnected: boolean;
  currentModel?: string;
  currentTemperature?: number;
  currentMaxTokens?: number;
  currentProvider?: string;
}>();

const emit = defineEmits<{
  apply: [agent: string, overrides: {
    temperature?: number;
    max_tokens?: number;
    model?: string;
    top_p?: number;
    sticky: boolean;
  }];
  reset: [agent: string];
}>();

const temperature = ref(props.currentTemperature ?? 0.7);
const maxTokens = ref(props.currentMaxTokens ?? 1024);
const model = ref(props.currentModel ?? '');
const topP = ref(0.95);
const sticky = ref(false);

function apply() {
  emit('apply', props.agentName, {
    temperature: temperature.value,
    max_tokens: maxTokens.value,
    model: model.value || undefined,
    top_p: topP.value,
    sticky: sticky.value,
  });
}
</script>

<template>
  <div class="param-editor">
    <h4>Experiment <span class="exp-badge">debug</span></h4>

    <div class="param-row">
      <label>Temperature</label>
      <input type="range" v-model.number="temperature" min="0" max="2" step="0.1" class="slider" />
      <span class="param-val">{{ temperature.toFixed(1) }}</span>
    </div>

    <div class="param-row">
      <label>Max Tokens</label>
      <input type="number" v-model.number="maxTokens" min="1" max="128000" class="num-input" />
    </div>

    <div class="param-row">
      <label>Model</label>
      <input type="text" v-model="model" :placeholder="currentModel || '(not set)'" class="text-input" />
    </div>

    <div v-if="currentProvider" class="param-row">
      <label>Provider</label>
      <span class="current-val">{{ currentProvider }}</span>
    </div>

    <div class="param-row">
      <label>Top P</label>
      <input type="range" v-model.number="topP" min="0" max="1" step="0.05" class="slider" />
      <span class="param-val">{{ topP.toFixed(2) }}</span>
    </div>

    <div class="param-row">
      <label class="checkbox-label">
        <input type="checkbox" v-model="sticky" />
        Sticky (persist across iterations)
      </label>
    </div>

    <div class="param-actions">
      <button class="action-btn apply" :disabled="!isConnected" @click="apply">Apply</button>
      <button class="action-btn reset" :disabled="!isConnected" @click="emit('reset', agentName)">Reset</button>
    </div>
  </div>
</template>

<style scoped>
.param-editor {
  padding: 0;
}
.param-editor h4 {
  margin: 0 0 8px;
  font-size: 11px;
  text-transform: uppercase;
  color: var(--text-secondary);
  letter-spacing: 0.5px;
}

.exp-badge {
  font-size: 9px;
  background: var(--accent-agent);
  color: white;
  padding: 1px 5px;
  border-radius: 3px;
  font-weight: 400;
  margin-left: 6px;
  vertical-align: middle;
}

.param-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
  font-size: 12px;
}
.param-row label {
  min-width: 80px;
  color: var(--text-secondary);
  font-size: 11px;
}

.slider {
  flex: 1;
  height: 4px;
  accent-color: var(--accent-agent);
}

.param-val {
  width: 36px;
  text-align: right;
  font-size: 11px;
  color: var(--text-primary);
  font-family: var(--font-mono);
}

.current-val {
  font-size: 11px;
  color: var(--text-primary);
  font-family: var(--font-mono);
  background: var(--surface-4);
  padding: 2px 6px;
  border-radius: 3px;
}

.num-input, .text-input {
  flex: 1;
  padding: 3px 6px;
  border: 1px solid var(--input-border);
  border-radius: var(--node-radius);
  font-size: 11px;
  outline: none;
  background: var(--input-bg);
  color: var(--input-text);
}
.num-input:focus, .text-input:focus { border-color: var(--border-focus); }

.checkbox-label {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 11px;
  color: var(--text-secondary);
  cursor: pointer;
}

.param-actions {
  display: flex;
  gap: 6px;
  margin-top: 8px;
}

.action-btn {
  padding: 4px 12px;
  border: none;
  border-radius: var(--node-radius);
  font-size: 11px;
  cursor: pointer;
}
.action-btn:disabled { opacity: 0.4; cursor: default; }
.action-btn.apply { background: var(--accent-agent); color: white; }
.action-btn.apply:hover:not(:disabled) { background: #4a7cdf; }
.action-btn.reset { background: var(--surface-4); color: var(--text-secondary); }
.action-btn.reset:hover:not(:disabled) { background: var(--surface-3); }
</style>
