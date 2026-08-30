<script setup lang="ts">
import { ref, watch, onMounted } from 'vue';
import yaml from 'js-yaml';

const DRAFT_KEY = 'rakitsu-agent-settings-draft';

const props = defineProps<{
  agentName: string;
  isConnected: boolean;
  isPaused: boolean;
  availableTools?: string[];
  allConfigTools?: string[];
  configProviders?: string[];
  currentProvider?: string;
  currentModel?: string;
}>();

const emit = defineEmits<{
  apply: [agent: string, overrides: Record<string, unknown>];
  reset: [agent: string];
}>();

type Tab = 'model' | 'prompt' | 'tools' | 'behavior';
const activeTab = ref<Tab>('model');

// Model tab — pre-fill from agent's current values
const temperature = ref(0.7);
const maxTokens = ref(1024);
const model = ref(props.currentModel ?? '');
const topP = ref(0.95);

// Provider & model discovery from session config
const discoveredModels = ref<string[]>([]);
const availableProviders = ref<string[]>([]);
const selectedProvider = ref(props.currentProvider || '');
const providerConfigs = ref<Record<string, Record<string, string>>>({});

async function fetchProvidersAndModels() {
  try {
    const sessRes = await fetch('/api/sessions');
    if (!sessRes.ok) return;
    const sessions = await sessRes.json();
    const active = sessions.find((s: { status: string }) => s.status === 'running') || sessions[0];
    if (!active?.config_yaml) return;

    const cfg = yaml.load(active.config_yaml) as Record<string, unknown>;
    const settings = cfg?.settings as Record<string, unknown>;
    const providers = settings?.providers as Record<string, Record<string, string>>;
    if (!providers) return;

    providerConfigs.value = providers;
    availableProviders.value = Object.keys(providers);
    if (!selectedProvider.value && availableProviders.value.length > 0) {
      selectedProvider.value = availableProviders.value[0] || '';
    }

    await fetchModelsForProvider(selectedProvider.value || availableProviders.value[0] || '');
  } catch (e) { console.warn('[AgentSettingsEditor] provider fetch failed:', e); }
}

async function fetchModelsForProvider(provName: string) {
  const prov = providerConfigs.value[provName];
  if (!prov?.base_url) { discoveredModels.value = []; return; }

  try {
    const params = new URLSearchParams({ base_url: prov.base_url });
    const headers: HeadersInit = {};
    // prov.api_key comes from /api/sessions, which always returns configs
    // through config.Redacted() — a real key never reaches this component,
    // only the literal mask. Sending it would just make a doomed request.
    if (prov.api_key && prov.api_key !== '[REDACTED]') headers['Authorization'] = `Bearer ${prov.api_key}`;
    const res = await fetch(`/api/providers/models?${params}`, { headers });
    if (res.ok) {
      const data = await res.json();
      const models = data.models || [];
      discoveredModels.value = models.map((m: string | { id?: string }) =>
        typeof m === 'string' ? m : m.id
      ).filter(Boolean);
    }
  } catch { /* ignore */ }
}

function onProviderChange(e: Event) {
  const val = (e.target as HTMLSelectElement).value;
  selectedProvider.value = val;
  model.value = ''; // reset model when provider changes
  fetchModelsForProvider(val);
}

onMounted(fetchProvidersAndModels);
watch(() => props.agentName, fetchProvidersAndModels);

// Prompt tab
const systemPrompt = ref('');

// Tools tab
const enabledTools = ref<Set<string>>(new Set(props.availableTools ?? []));

// Behavior tab
const maxIterations = ref(10);
const sticky = ref(false);

// Advanced: reflection + ground check
const reflectionEnabled = ref(false);
const reflectionMode = ref('after_tool');
const groundCheckEnabled = ref(false);
const groundCheckThreshold = ref(0.7);

// Draft persistence
let draftTimer: ReturnType<typeof setTimeout> | undefined;

function saveDraft() {
  clearTimeout(draftTimer);
  draftTimer = setTimeout(() => {
    try {
      localStorage.setItem(`${DRAFT_KEY}:${props.agentName}`, JSON.stringify({
        temperature: temperature.value,
        maxTokens: maxTokens.value,
        model: model.value,
        topP: topP.value,
        systemPrompt: systemPrompt.value,
        enabledTools: [...enabledTools.value],
        maxIterations: maxIterations.value,
        sticky: sticky.value,
      }));
    } catch { /* ignore */ }
  }, 500);
}

function loadDraft(): boolean {
  try {
    const raw = localStorage.getItem(`${DRAFT_KEY}:${props.agentName}`);
    if (!raw) return false;
    const d = JSON.parse(raw);
    temperature.value = d.temperature ?? 0.7;
    maxTokens.value = d.maxTokens ?? 1024;
    model.value = d.model ?? '';
    topP.value = d.topP ?? 0.95;
    systemPrompt.value = d.systemPrompt ?? '';
    enabledTools.value = new Set(d.enabledTools ?? []);
    maxIterations.value = d.maxIterations ?? 10;
    sticky.value = d.sticky ?? false;
    return true;
  } catch { return false; }
}

function clearDraft() {
  try { localStorage.removeItem(`${DRAFT_KEY}:${props.agentName}`); } catch { /* */ }
}

const hasDraft = ref(false);
const showDraftPrompt = ref(false);

watch(() => props.agentName, () => {
  hasDraft.value = !!localStorage.getItem(`${DRAFT_KEY}:${props.agentName}`);
  showDraftPrompt.value = hasDraft.value;
}, { immediate: true });

function restoreDraft() {
  loadDraft();
  showDraftPrompt.value = false;
}

function dismissDraft() {
  showDraftPrompt.value = false;
}

// Auto-save on changes
watch([temperature, maxTokens, model, topP, systemPrompt, enabledTools, maxIterations, sticky], saveDraft, { deep: true });

function toggleTool(tool: string) {
  if (enabledTools.value.has(tool)) enabledTools.value.delete(tool);
  else enabledTools.value.add(tool);
  enabledTools.value = new Set(enabledTools.value); // trigger reactivity
}

function apply() {
  const ovr: Record<string, unknown> = {
    temperature: temperature.value,
    max_tokens: maxTokens.value,
    top_p: topP.value,
    sticky: sticky.value,
  };
  if (model.value) ovr.model = model.value;
  if (systemPrompt.value) ovr.system_prompt = systemPrompt.value;
  if (maxIterations.value) ovr.max_iterations = maxIterations.value;
  // Tools: always send if any are toggled
  ovr.tools = [...enabledTools.value];
  // Reflection
  ovr.reflection = { enabled: reflectionEnabled.value, mode: reflectionMode.value };
  // Ground check
  ovr.ground_check = { enabled: groundCheckEnabled.value, confidence_threshold: groundCheckThreshold.value };
  emit('apply', props.agentName, ovr);
}

function reset() {
  clearDraft();
  emit('reset', props.agentName);
}

function exportYaml() {
  const lines = [
    `# Override settings for agent "${props.agentName}"`,
    `name: ${props.agentName}`,
    `model: ${model.value || '(current)'}`,
    `settings:`,
    `  temperature: ${temperature.value}`,
    `  max_tokens: ${maxTokens.value}`,
    `  top_p: ${topP.value}`,
    `  max_iterations: ${maxIterations.value}`,
  ];
  if (systemPrompt.value) {
    lines.push(`system_prompt: |`);
    for (const line of systemPrompt.value.split('\n')) {
      lines.push(`  ${line}`);
    }
  }
  if (props.availableTools && enabledTools.value.size !== props.availableTools.length) {
    lines.push(`tools:`);
    for (const t of enabledTools.value) {
      lines.push(`  - ${t}`);
    }
  }
  const blob = new Blob([lines.join('\n') + '\n'], { type: 'text/yaml' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `${props.agentName}-overrides.yaml`;
  a.click();
  URL.revokeObjectURL(url);
}
</script>

<template>
  <div class="settings-editor">
    <h4>Agent Settings <span class="badge">debug</span></h4>

    <!-- Draft restore prompt -->
    <div v-if="showDraftPrompt" class="draft-prompt">
      <span>Saved draft found.</span>
      <button class="draft-btn" @click="restoreDraft">Restore</button>
      <button class="draft-btn dismiss" @click="dismissDraft">Dismiss</button>
    </div>

    <!-- Tabs -->
    <div class="tabs">
      <button v-for="t in (['model', 'prompt', 'tools', 'behavior'] as Tab[])" :key="t"
        class="tab" :class="{ active: activeTab === t }" @click="activeTab = t">
        {{ t }}
      </button>
    </div>

    <!-- Model tab -->
    <div v-if="activeTab === 'model'" class="tab-content">
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
        <label>Provider</label>
        <select v-if="(availableProviders.length || configProviders?.length)" :value="selectedProvider" @change="onProviderChange" class="text-input">
          <option v-for="p in (availableProviders.length ? availableProviders : configProviders)" :key="p" :value="p">{{ p }}</option>
        </select>
        <span v-else class="current-val">{{ currentProvider || '(default)' }}</span>
      </div>
      <div class="param-row">
        <label>Model</label>
        <select v-if="discoveredModels.length > 0" v-model="model" class="text-input">
          <option value="">{{ currentModel || '(keep current)' }}</option>
          <option v-for="m in discoveredModels" :key="m" :value="m" :selected="m === currentModel">{{ m }}</option>
        </select>
        <input v-else type="text" v-model="model" :placeholder="currentModel || '(keep current)'" class="text-input" />
      </div>
      <div class="param-row">
        <label>Top P</label>
        <input type="range" v-model.number="topP" min="0" max="1" step="0.05" class="slider" />
        <span class="param-val">{{ topP.toFixed(2) }}</span>
      </div>
    </div>

    <!-- Prompt tab -->
    <div v-if="activeTab === 'prompt'" class="tab-content">
      <textarea
        v-model="systemPrompt"
        class="prompt-textarea"
        placeholder="Override system prompt (leave empty to keep current)"
        rows="8"
      ></textarea>
    </div>

    <!-- Tools tab: show all config tools, agent's current tools pre-checked -->
    <div v-if="activeTab === 'tools'" class="tab-content">
      <div v-if="(allConfigTools?.length || availableTools?.length)" class="tool-list">
        <label v-for="tool in (allConfigTools?.length ? allConfigTools : availableTools)" :key="tool" class="tool-check">
          <input type="checkbox" :checked="enabledTools.has(tool)" @change="toggleTool(tool)" />
          {{ tool }}
          <span v-if="!(availableTools || []).includes(tool)" class="tool-hint">(not assigned)</span>
        </label>
      </div>
      <div v-else class="empty-msg">No tools in config.</div>
    </div>

    <!-- Behavior tab -->
    <div v-if="activeTab === 'behavior'" class="tab-content">
      <div class="param-row">
        <label>Max Iterations</label>
        <input type="number" v-model.number="maxIterations" min="1" max="100" class="num-input" />
      </div>
      <div class="param-row">
        <label class="checkbox-label">
          <input type="checkbox" v-model="sticky" />
          Sticky (persist across iterations)
        </label>
      </div>

      <div class="section-divider">Reflection</div>
      <div class="param-row">
        <label class="checkbox-label">
          <input type="checkbox" v-model="reflectionEnabled" />
          Enable reflection
        </label>
      </div>
      <div v-if="reflectionEnabled" class="param-row">
        <label>Mode</label>
        <select v-model="reflectionMode" class="text-input">
          <option value="after_tool">After tool</option>
          <option value="before_answer">Before answer</option>
          <option value="both">Both</option>
        </select>
      </div>

      <div class="section-divider">Ground Check</div>
      <div class="param-row">
        <label class="checkbox-label">
          <input type="checkbox" v-model="groundCheckEnabled" />
          Enable ground check
        </label>
      </div>
      <div v-if="groundCheckEnabled" class="param-row">
        <label>Threshold</label>
        <input type="range" v-model.number="groundCheckThreshold" min="0" max="1" step="0.05" class="slider" />
        <span class="param-val">{{ groundCheckThreshold.toFixed(2) }}</span>
      </div>
    </div>

    <!-- Actions -->
    <div class="actions">
      <button class="action-btn apply" :disabled="!isConnected" @click="apply">Apply</button>
      <button class="action-btn reset" :disabled="!isConnected" @click="reset">Reset</button>
      <button class="action-btn export" @click="exportYaml">Export YAML</button>
    </div>
  </div>
</template>

<style scoped>
.settings-editor h4 {
  margin: 0 0 8px;
  font-size: 11px;
  text-transform: uppercase;
  color: var(--text-secondary);
  letter-spacing: 0.5px;
}
.badge {
  font-size: 9px;
  background: var(--accent-agent);
  color: white;
  padding: 1px 5px;
  border-radius: 3px;
  font-weight: 400;
  margin-left: 6px;
  vertical-align: middle;
}

.draft-prompt {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 8px;
  background: rgba(245, 158, 11, 0.1);
  border-radius: var(--node-radius);
  font-size: 11px;
  margin-bottom: 8px;
  color: var(--text-primary);
}
.draft-btn {
  padding: 2px 8px;
  border: 1px solid var(--border-default);
  border-radius: 3px;
  background: var(--surface-3);
  font-size: 10px;
  cursor: pointer;
  color: var(--text-primary);
}
.draft-btn:hover { background: var(--surface-4); }
.draft-btn.dismiss { color: var(--text-muted); }

.tabs {
  display: flex;
  gap: 0;
  margin-bottom: 8px;
}
.tab {
  padding: 4px 10px;
  border: 1px solid var(--border-default);
  background: var(--surface-3);
  font-size: 10px;
  text-transform: capitalize;
  cursor: pointer;
  color: var(--text-secondary);
}
.tab:first-child { border-radius: 3px 0 0 3px; }
.tab:last-child { border-radius: 0 3px 3px 0; }
.tab:not(:first-child) { border-left: none; }
.tab.active { background: var(--accent-agent); color: white; border-color: var(--accent-agent); }
.tab:hover:not(.active) { background: var(--surface-4); }

.tab-content { min-height: 60px; }

.param-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
  font-size: 12px;
}
.param-row label { min-width: 80px; color: var(--text-secondary); font-size: 11px; }
.slider { flex: 1; height: 4px; accent-color: var(--accent-agent); }
.param-val { width: 36px; text-align: right; font-size: 11px; color: var(--text-primary); font-family: var(--font-mono); }
.num-input, .text-input {
  flex: 1; padding: 3px 6px; border: 1px solid var(--input-border); border-radius: var(--node-radius); font-size: 11px; outline: none;
  background: var(--input-bg); color: var(--input-text);
}
.current-val {
  font-size: 11px; color: var(--text-primary); font-family: var(--font-mono);
  background: var(--surface-4); padding: 2px 6px; border-radius: 3px;
}
.num-input:focus, .text-input:focus { border-color: var(--border-focus); }

.prompt-textarea {
  width: 100%;
  box-sizing: border-box;
  padding: 6px 8px;
  border: 1px solid var(--input-border);
  border-radius: var(--node-radius);
  font-size: 11px;
  font-family: var(--font-mono);
  resize: vertical;
  outline: none;
  background: var(--input-bg);
  color: var(--input-text);
}
.prompt-textarea:focus { border-color: var(--border-focus); }

.tool-list { display: flex; flex-direction: column; gap: 4px; }
.tool-check {
  display: flex; align-items: center; gap: 6px;
  font-size: 11px; color: var(--text-primary); cursor: pointer;
}
.empty-msg { font-size: 11px; color: var(--text-secondary); font-style: italic; }
.tool-hint { font-size: 10px; color: var(--text-muted); margin-left: 4px; }
.section-divider {
  font-size: 10px;
  text-transform: uppercase;
  color: var(--text-muted);
  letter-spacing: 0.5px;
  margin: 10px 0 6px;
  padding-top: 8px;
  border-top: 1px solid var(--border-subtle);
}

.checkbox-label {
  display: flex; align-items: center; gap: 4px;
  font-size: 11px; color: var(--text-secondary); cursor: pointer;
}

.actions {
  display: flex;
  gap: 6px;
  margin-top: 10px;
}
.action-btn {
  padding: 4px 12px; border: none; border-radius: var(--node-radius); font-size: 11px; cursor: pointer;
}
.action-btn:disabled { opacity: 0.4; cursor: default; }
.action-btn.apply { background: var(--accent-agent); color: white; }
.action-btn.apply:hover:not(:disabled) { background: #4a7cdf; }
.action-btn.reset { background: var(--surface-4); color: var(--text-secondary); }
.action-btn.reset:hover:not(:disabled) { background: var(--surface-3); }
.action-btn.export { background: var(--surface-4); color: var(--text-secondary); }
.action-btn.export:hover { background: var(--surface-3); }
</style>
