import { computed, ref } from 'vue';
import type { ConfigEntry } from './useAgentRun';
import type { ChatSessionMeta } from '../types';

// Singleton workspace state shared by Builder, Debugger, Chat. The "what am
// I working on right now" context — current config identity, workdir, env
// vars — lives here so all tabs operate on the same inputs instead of each
// maintaining its own parallel copies.
//
// Pattern mirrors useEventStream: module-level refs + a factory that returns
// a stable handle. No Pinia; not worth the dep for five fields.

export interface WorkspaceEnvVar {
  key: string;
  value: string;
}

// ConfigSource tracks how the workspace got its current config. Chat and
// other surfaces use this to set expectations: a "builder" config is a live
// draft that can drift as the user edits; a "session" config is frozen
// history; a "debug" config is the YAML of the currently running debugger
// session. null = nothing is active.
export type ConfigSource = 'builder' | 'debug' | 'session' | null;

const workdir = ref<string>('');
const envVars = ref<WorkspaceEnvVar[]>([]);
const currentConfigId = ref<string | null>(null);
const currentConfig = ref<ConfigEntry | null>(null);
const currentSource = ref<ConfigSource>(null);
const builderSnapshotHash = ref<string>(''); // hash of YAML last uploaded from Builder
const knownConfigs = ref<ConfigEntry[]>([]);
const knownChatSessions = ref<ChatSessionMeta[]>([]);
const activeChatSessionId = ref<string | null>(null);

let initialized = false;

async function initFromServer() {
  if (initialized) return;
  initialized = true;
  // Pull defaults in parallel; failures are silent so we don't block the UI.
  await Promise.all([
    fetch('/api/workdir')
      .then((r) => r.json())
      .then((d) => {
        if (!workdir.value && typeof d.workdir === 'string') workdir.value = d.workdir;
      })
      .catch(() => {}),
    refreshConfigs(),
  ]);
}

async function refreshConfigs() {
  try {
    const res = await fetch('/api/configs');
    const list = (await res.json()) as ConfigEntry[];
    knownConfigs.value = list;
    if (currentConfigId.value) {
      currentConfig.value = list.find((c) => c.id === currentConfigId.value) ?? null;
    }
  } catch {
    /* ignore */
  }
}

// refreshChatSessions pulls the server's view of active chat sessions so
// the ChatView sidebar and the Debugger chat pane can render the same
// list. Cheap — one GET /api/chat per call.
async function refreshChatSessions() {
  try {
    const res = await fetch('/api/chat');
    const list = (await res.json()) as ChatSessionMeta[];
    knownChatSessions.value = Array.isArray(list) ? list : [];
    // If the currently-selected chat session disappeared (e.g. stopped
    // elsewhere), clear the selection so the UI doesn't try to WS-attach
    // to a dead id.
    if (
      activeChatSessionId.value &&
      !knownChatSessions.value.some((s) => s.id === activeChatSessionId.value)
    ) {
      activeChatSessionId.value = null;
    }
  } catch {
    knownChatSessions.value = [];
  }
}

function setActiveChatSession(id: string | null) {
  activeChatSessionId.value = id;
}

function setConfig(id: string | null, source: ConfigSource = null) {
  currentConfigId.value = id;
  currentConfig.value = id ? (knownConfigs.value.find((c) => c.id === id) ?? null) : null;
  currentSource.value = id ? source : null;
  if (source !== 'builder') builderSnapshotHash.value = '';
}

function setWorkdir(w: string) {
  workdir.value = w;
}

function setEnvVars(vars: WorkspaceEnvVar[]) {
  envVars.value = vars;
}

function envVarsMap(): Record<string, string> {
  const out: Record<string, string> = {};
  for (const v of envVars.value) {
    if (v.key) out[v.key] = v.value;
  }
  return out;
}

// setInlineConfig: upload YAML via /api/configs/inline, then set it as the
// current config. Auto-tags source = 'builder' by default — the Builder is
// the only caller that routinely uploads inline YAML.
async function setInlineConfig(yaml: string, source: ConfigSource = 'builder'): Promise<ConfigEntry | null> {
  try {
    const res = await fetch('/api/configs/inline', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ yaml }),
    });
    if (!res.ok) return null;
    const entry = (await res.json()) as ConfigEntry;
    await refreshConfigs();
    setConfig(entry.id, source);
    if (source === 'builder') builderSnapshotHash.value = cheapHash(yaml);
    return entry;
  } catch {
    return null;
  }
}

// cheapHash is a small non-crypto hash used to detect if the Builder's
// canvas YAML has drifted since the last inline upload. 32-bit djb2.
function cheapHash(s: string): string {
  let h = 5381;
  for (let i = 0; i < s.length; i++) h = ((h << 5) + h + s.charCodeAt(i)) | 0;
  return (h >>> 0).toString(36);
}

// builderDrifted returns true if the provided YAML differs from what was
// last uploaded as a builder-sourced config. Chat UI uses this to show a
// "restart to apply changes" hint.
function builderDrifted(yaml: string): boolean {
  if (currentSource.value !== 'builder') return false;
  if (!builderSnapshotHash.value) return false;
  return cheapHash(yaml) !== builderSnapshotHash.value;
}

export function useWorkspace() {
  return {
    // state (read/write via setters below; refs exposed for v-model)
    workdir,
    envVars,
    currentConfigId: computed(() => currentConfigId.value),
    currentConfig: computed(() => currentConfig.value),
    currentSource: computed(() => currentSource.value),
    knownConfigs: computed(() => knownConfigs.value),
    knownChatSessions: computed(() => knownChatSessions.value),
    activeChatSessionId: computed(() => activeChatSessionId.value),
    isInteractive: computed(() => currentConfig.value?.interactive === true),

    // actions
    initFromServer,
    refreshConfigs,
    refreshChatSessions,
    setConfig,
    setActiveChatSession,
    setWorkdir,
    setEnvVars,
    setInlineConfig,
    envVarsMap,
    builderDrifted,
  };
}
