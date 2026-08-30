<script setup lang="ts">
// BranchTreeView — engineer-visible inspector for a chat session's
// branching turn history. Two data-source modes:
//
//   1. Live (default): subscribe to a live ChatSession's WS-pushed tree
//      snapshots via useChatSession. Refresh button asks the server for
//      a fresh snapshot.
//
//   2. Static: the parent passes a `tree` prop directly — read-
//      only render off a persisted .chat.json snapshot (e.g. the Sessions
//      tab Inspect viewer's Tree sub-tab). No WS is opened, refresh is a
//      no-op because the source is immutable from this component's view.
//
// Picks mode based on whether `tree` is provided.

import { computed, nextTick, onActivated, ref, watch } from 'vue';
import { VueFlow, useVueFlow } from '@vue-flow/core';
import { Background } from '@vue-flow/background';
import { Controls } from '@vue-flow/controls';
import { MiniMap } from '@vue-flow/minimap';
import type { Node, Edge } from '@vue-flow/core';
import { useChatTree } from '../../composables/useChatTree';
import { useChatSession } from '../../composables/useChatSession';
import type { TreeSnapshot } from '../../types/chat';
import BranchTreeNode from './BranchTreeNode.vue';
import TurnPreviewPane from './TurnPreviewPane.vue';

const props = defineProps<{
  sessionId: string;
  baseUrl?: string;
  // Static-mode override: when provided, render this snapshot instead of
  // subscribing to the live session. Used by SessionsTab/InspectViewer's
  // Tree sub-tab to render the persisted .chat.json from REST.
  tree?: TreeSnapshot | null;
}>();

const emit = defineEmits<{
  close: [];
}>();

const isStaticMode = computed(() => props.tree !== undefined);

// In live mode we attach useChatSession (opens WS, drives tree updates).
// In static mode we skip the composable entirely and use a plain ref so
// useChatTree's reactivity still works without a live session.
const liveChat = isStaticMode.value
  ? null
  : useChatSession(props.sessionId, props.baseUrl ?? '');
// useChatTree expects Ref<TreeSnapshot | null>. The prop is `| undefined`
// when omitted, so we narrow via computed (computed returns ComputedRef which
// is structurally a Ref). In live mode we delegate to the composable's ref.
const treeSource = computed(() => liveChat ? liveChat.tree.value : (props.tree ?? null));
const { flowNodes, flowEdges, selectedTurnId, selectNode, hasTree, activeLeafId } = useChatTree(treeSource);

// Vue Flow keeps its own internal node/edge state — prop changes alone
// don't trigger re-renders. Same pattern as ExecutionGraph: explicitly
// push updates via setNodes/setEdges when the layout recomputes.
const { setNodes, setEdges, fitView, dimensions, onNodeClick } = useVueFlow();
const miniMapReady = computed(() => dimensions.value.width > 0 && dimensions.value.height > 0);

// Track whether the user has been shown a tree at least once. Until
// that happens we display an empty-state instead of an empty canvas.
const everShown = ref(false);

watch(flowNodes, (n: Node[]) => {
  setNodes(n);
  if (n.length > 0) everShown.value = true;
}, { deep: true });

watch(flowEdges, (e: Edge[]) => {
  setEdges(e);
}, { deep: true });

onActivated(async () => {
  if (flowNodes.value.length > 0) {
    await nextTick();
    fitView({ padding: 0.15 });
  }
});

onNodeClick(({ node }) => {
  selectNode(node.id);
});

// Refresh:
//   Live  — ask the server for a fresh tree_snapshot frame.
//   Static — no-op (the parent owns the source; if it needs to refresh,
//            it re-fetches and passes a new tree prop).
function refresh() {
  if (liveChat) liveChat.requestTree();
}

// Auto-fit when the snapshot changes substantially (root count or path
// length flips). This is a heuristic, not a hard rule — the user can
// still pan freely.
const rootCount = computed(() => treeSource.value?.roots.length ?? 0);
const pathLength = computed(() => treeSource.value?.active_path.length ?? 0);
watch([rootCount, pathLength], async () => {
  if (flowNodes.value.length === 0) return;
  await nextTick();
  fitView({ padding: 0.15 });
});

// On first mount in live mode, ask for a snapshot if we don't have one.
// In static mode the prop is already the snapshot — nothing to request.
if (liveChat && !liveChat.tree.value) {
  refresh();
}

const previewTurnId = computed(() => selectedTurnId.value);
</script>

<template>
  <div class="branch-tree-view">
    <header class="toolbar">
      <span class="title">Branch Tree</span>
      <span v-if="activeLeafId" class="meta" :title="activeLeafId">
        active: {{ activeLeafId.slice(0, 8) }}
      </span>
      <span v-if="!hasTree" class="meta">no tree yet</span>
      <span class="spacer"></span>
      <button type="button" class="tool-btn" @click="refresh" title="Request fresh snapshot">↻</button>
      <button type="button" class="tool-btn" @click="emit('close')" title="Close viewer">×</button>
    </header>

    <div class="split">
      <div class="canvas">
        <div v-if="!everShown && !hasTree" class="empty-state">
          The viewer is empty — a fresh snapshot is on the way.<br/>
          (Or this session has no turns yet.)
        </div>
        <VueFlow
          v-else
          :nodes="flowNodes"
          :edges="flowEdges"
          :nodes-draggable="false"
          :nodes-connectable="false"
          :elements-selectable="true"
          fit-view-on-init
        >
          <template #node-branch-tree="nodeProps">
            <BranchTreeNode
              :data="nodeProps.data"
              :selected="nodeProps.id === selectedTurnId"
            />
          </template>
          <Background pattern-color="rgba(255,255,255,0.06)" />
          <Controls position="bottom-left" />
          <MiniMap v-if="miniMapReady" position="bottom-right" pannable />
        </VueFlow>
      </div>

      <TurnPreviewPane
        class="preview"
        :session-id="sessionId"
        :turn-id="previewTurnId"
        :base-url="baseUrl"
      />
    </div>
  </div>
</template>

<style scoped>
.branch-tree-view {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--surface-0, rgba(15, 17, 22, 0.95));
  color: var(--fg, rgba(255, 255, 255, 0.92));
}

.toolbar {
  display: flex;
  align-items: center;
  gap: 0.6em;
  padding: 0.4em 0.7em;
  border-bottom: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.18));
  font-family: var(--font-sans);
  font-size: 0.9em;
}

.toolbar .title { font-weight: 600; }
.toolbar .meta { opacity: 0.6; font-family: var(--font-mono); font-size: 0.82em; }
.toolbar .spacer { flex: 1; }

.tool-btn {
  padding: 0 0.5em;
  height: 24px;
  border: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.2));
  border-radius: 4px;
  background: transparent;
  color: inherit;
  cursor: pointer;
  font-family: var(--font-sans);
  font-size: 1em;
  line-height: 1;
}
.tool-btn:hover {
  background: rgba(255, 255, 255, 0.06);
}

.split {
  flex: 1;
  display: grid;
  grid-template-columns: 1fr 320px;
  min-height: 0;
}

.canvas {
  position: relative;
  min-height: 0;
  overflow: hidden;
}

.empty-state {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  text-align: center;
  opacity: 0.65;
  font-style: italic;
  line-height: 1.5;
  padding: 1em;
}

.preview {
  min-height: 0;
  max-height: 100%;
}
</style>
