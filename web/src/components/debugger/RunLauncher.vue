<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, type Ref } from 'vue';
import { useAgentRun, type ConfigEntry } from '../../composables/useAgentRun';
import { useDebugControl } from '../../composables/useDebugControl';
import ComboBox from '../builder/ComboBox.vue';

const props = defineProps<{
  baseUrl: string;
}>();

const emit = defineEmits<{
  'run-started': [debug: boolean];
}>();

const baseUrlRef = ref(props.baseUrl) as Ref<string>;

const {
  configs, runStatus, runError, elapsedMs,
  fetchConfigs, fetchWorkdir, uploadConfig, startRun, stopRun, cleanup,
} = useAgentRun(baseUrlRef);

const { breakpoints } = useDebugControl(baseUrlRef);

const selectedConfigId = ref<string | null>(null);
const query = ref('');
const timeout = ref(300);
const workdir = ref('');

const selectedConfig = computed<ConfigEntry | null>(() =>
  configs.value.find((c) => c.id === selectedConfigId.value) ?? null,
);

const canRun = computed(() =>
  !!selectedConfigId.value && !!query.value.trim() && runStatus.value !== 'running',
);

const workdirMissing = computed(() => !workdir.value.trim());

function fmtElapsed(ms: number): string {
  return ms >= 1000 ? `${(ms / 1000).toFixed(1)}s` : `${ms}ms`;
}

async function handleRun(debug = false) {
  if (!selectedConfigId.value || !query.value.trim()) return;
  if (workdirMissing.value) {
    if (!confirm('No workspace directory set. Tools (CLI, file system) will run in the server\'s current directory. Continue anyway?')) return;
  }
  const bps = debug ? breakpoints.value.map(b => ({ event_type: b.event_type, agent_name: b.agent_name })) : [];
  await startRun(selectedConfigId.value, query.value.trim(), timeout.value, workdir.value.trim(), debug, {}, bps);
  if (runStatus.value === 'running') {
    emit('run-started', debug);
  }
}

async function handleUpload(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  if (!file) return;
  const entry = await uploadConfig(file);
  if (entry) {
    selectedConfigId.value = entry.id;
  }
  input.value = '';
}

onMounted(async () => {
  fetchConfigs();
  const defaultDir = await fetchWorkdir();
  if (defaultDir && !workdir.value) {
    workdir.value = defaultDir;
  }
});

onUnmounted(() => {
  cleanup();
});
</script>

<template>
  <div class="run-launcher">
    <h3 class="launcher-title">Run Agent</h3>

    <!-- Config selector -->
    <div class="field">
      <label class="field-label">Config</label>
      <div class="config-row">
        <ComboBox
          :model-value="selectedConfigId ?? ''"
          :options="configs.map(c => ({ value: c.id, label: c.name || c.path }))"
          placeholder="Select config..."
          :allow-custom="false"
          @update:model-value="selectedConfigId = $event"
        />
        <label class="upload-btn">
          Upload
          <input type="file" accept=".yaml,.yml,.zip" @change="handleUpload" hidden />
        </label>
      </div>
    </div>

    <!-- Config preview -->
    <div v-if="selectedConfig" class="config-preview">
      <div v-if="selectedConfig.agents?.length" class="preview-item">
        <span class="preview-label">Agents</span>
        <span class="preview-value">{{ selectedConfig.agents.join(', ') }}</span>
      </div>
      <div v-if="selectedConfig.tools?.length" class="preview-item">
        <span class="preview-label">Tools</span>
        <span class="preview-value">{{ selectedConfig.tools.join(', ') }}</span>
      </div>
      <div v-if="selectedConfig.strategy" class="preview-item">
        <span class="preview-label">Strategy</span>
        <span class="preview-value">{{ selectedConfig.strategy }}</span>
      </div>
    </div>

    <!-- Workspace -->
    <div class="field">
      <label class="field-label">Workspace</label>
      <input
        v-model="workdir"
        type="text"
        class="workdir-input"
        placeholder="/path/to/project"
      />
      <div v-if="workdirMissing" class="workdir-warning">
        Tools will run without a workspace directory
      </div>
    </div>

    <!-- Prompt -->
    <div class="field">
      <label class="field-label">Prompt</label>
      <textarea
        v-model="query"
        class="prompt-input"
        placeholder="Enter your prompt..."
        rows="3"
        @keydown.meta.enter="handleRun(false)"
        @keydown.ctrl.enter="handleRun(false)"
      ></textarea>
    </div>

    <!-- Options -->
    <div class="options-row">
      <div class="field inline">
        <label class="field-label">Timeout</label>
        <input v-model.number="timeout" type="number" class="timeout-input" min="10" max="3600" />
        <span class="unit">s</span>
      </div>
    </div>

    <!-- Actions -->
    <div class="actions">
      <template v-if="runStatus !== 'running'">
        <button class="run-btn" :disabled="!canRun" @click="handleRun(false)">Run</button>
        <button class="debug-btn" :disabled="!canRun" @click="handleRun(true)" title="Run with debugger — breakpoints, pause/resume, param override">Debug</button>
      </template>
      <button v-else class="stop-btn" @click="stopRun">Stop</button>
    </div>

    <!-- Status -->
    <div v-if="runStatus === 'running'" class="status-bar running">
      Running... {{ fmtElapsed(elapsedMs) }}
    </div>
    <div v-else-if="runStatus === 'completed'" class="status-bar completed">
      Completed in {{ fmtElapsed(elapsedMs) }}
    </div>
    <div v-else-if="runStatus === 'error' && runError" class="status-bar error">
      Error: {{ runError }}
    </div>
    <div v-else-if="runStatus === 'cancelled'" class="status-bar cancelled">
      Cancelled
    </div>
  </div>
</template>

<style scoped>
.run-launcher {
  padding: 8px;
}
.launcher-title {
  font-size: 13px;
  font-weight: 600;
  margin: 0 0 8px;
  color: var(--text-primary);
}
.field { margin-bottom: 8px; }
.field.inline {
  display: flex;
  align-items: center;
  gap: 4px;
}
.field-label {
  display: block;
  font-size: 11px;
  font-weight: 500;
  color: var(--text-secondary);
  margin-bottom: 2px;
}
.field.inline .field-label { margin-bottom: 0; }
.config-row {
  display: flex;
  gap: 4px;
}
.config-select {
  flex: 1;
  padding: 4px 6px;
  border: 1px solid var(--input-border);
  border-radius: var(--node-radius);
  font-size: 11px;
  outline: none;
  min-width: 0;
  background: var(--input-bg);
  color: var(--input-text);
}
.config-select:focus { border-color: var(--border-focus); }
.upload-btn {
  padding: 4px 8px;
  background: var(--surface-3);
  border: 1px solid var(--border-default);
  border-radius: var(--node-radius);
  font-size: 10px;
  cursor: pointer;
  color: var(--text-secondary);
  flex-shrink: 0;
}
.upload-btn:hover { border-color: var(--accent-agent); color: var(--accent-agent); }
.config-preview {
  background: var(--surface-3);
  border: 1px solid var(--border-subtle);
  border-radius: var(--node-radius);
  padding: 4px 8px;
  margin-bottom: 8px;
  font-size: 10px;
}
.preview-item {
  display: flex;
  gap: 4px;
  margin-bottom: 1px;
}
.preview-label { color: var(--text-muted); min-width: 50px; }
.preview-value { color: var(--text-primary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.workdir-input {
  width: 100%;
  padding: 4px 6px;
  border: 1px solid var(--input-border);
  border-radius: var(--node-radius);
  font-size: 11px;
  font-family: var(--font-mono);
  outline: none;
  box-sizing: border-box;
  background: var(--input-bg);
  color: var(--input-text);
}
.workdir-input:focus { border-color: var(--border-focus); }
.workdir-warning {
  font-size: 10px;
  color: var(--status-paused);
  margin-top: 2px;
}
.prompt-input {
  width: 100%;
  padding: 4px 6px;
  border: 1px solid var(--input-border);
  border-radius: var(--node-radius);
  font-size: 11px;
  font-family: inherit;
  resize: vertical;
  outline: none;
  box-sizing: border-box;
  background: var(--input-bg);
  color: var(--input-text);
}
.prompt-input:focus { border-color: var(--border-focus); }
.options-row {
  display: flex;
  gap: 8px;
  margin-bottom: 8px;
}
.timeout-input {
  width: 50px;
  padding: 3px 6px;
  border: 1px solid var(--input-border);
  border-radius: var(--node-radius);
  font-size: 11px;
  outline: none;
  background: var(--input-bg);
  color: var(--input-text);
}
.timeout-input:focus { border-color: var(--border-focus); }
.unit { font-size: 11px; color: var(--text-secondary); }
.actions { margin-bottom: 8px; display: flex; gap: 4px; }
.run-btn {
  padding: 6px 16px;
  background: var(--accent-agent);
  color: white;
  border: none;
  border-radius: var(--node-radius);
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
  flex: 1;
}
.run-btn:hover:not(:disabled) { background: #4a7cdf; }
.run-btn:disabled { opacity: 0.5; cursor: not-allowed; }
.debug-btn {
  padding: 6px 16px;
  background: var(--status-paused);
  color: white;
  border: none;
  border-radius: var(--node-radius);
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
  flex: 1;
}
.debug-btn:hover:not(:disabled) { background: #d97706; }
.debug-btn:disabled { opacity: 0.5; cursor: not-allowed; }
.stop-btn {
  padding: 6px 16px;
  background: var(--status-error);
  color: white;
  border: none;
  border-radius: var(--node-radius);
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
  width: 100%;
}
.stop-btn:hover { background: #dc2626; }
.status-bar {
  padding: 4px 8px;
  border-radius: var(--node-radius);
  font-size: 11px;
  font-weight: 500;
}
.status-bar.running { background: rgba(91, 141, 239, 0.15); color: var(--accent-agent); }
.status-bar.completed { background: rgba(34, 197, 94, 0.15); color: var(--status-success); }
.status-bar.error { background: rgba(239, 68, 68, 0.15); color: var(--status-error); }
.status-bar.cancelled { background: rgba(245, 158, 11, 0.15); color: var(--status-paused); }
</style>
