<script setup lang="ts">
import { ref, computed } from 'vue';
import type { Node } from '@vue-flow/core';

type HierarchyFilterField = 'text' | 'name' | 'type' | 'provider' | 'model';

interface HierarchyChip {
  field: HierarchyFilterField;
  value: string;
  isRegex: boolean;
}

const HIERARCHY_FIELDS: { key: HierarchyFilterField; label: string }[] = [
  { key: 'text', label: 'All' },
  { key: 'name', label: 'Name' },
  { key: 'type', label: 'Type' },
  { key: 'provider', label: 'Provider' },
  { key: 'model', label: 'Model' },
];

const props = defineProps<{
  nodes: Node[];
  selectedNodeIds: string[];
  selectionScope: string | null;
  isOpen: boolean;
}>();

const emit = defineEmits<{
  (e: 'select-node', nodeId: string, multi?: boolean): void;
  (e: 'enter-scope', groupId: string): void;
  (e: 'toggle'): void;
}>();

const selectedSet = computed(() => new Set(props.selectedNodeIds));

// Chip-based filter (same pattern as session filter)
const chips = ref<HierarchyChip[]>([]);
const inputField = ref<HierarchyFilterField>('text');
const inputValue = ref('');
const inputRegex = ref(false);
const collapsed = ref<Set<string>>(new Set());

function addChip() {
  const v = inputValue.value.trim();
  if (!v) return;
  chips.value = [...chips.value, { field: inputField.value, value: v, isRegex: inputRegex.value }];
  inputValue.value = '';
}

function removeChip(index: number) {
  chips.value = chips.value.filter((_, i) => i !== index);
}

function clearFilters() {
  chips.value = [];
  inputValue.value = '';
}

function subsequenceMatch(haystack: string, needle: string): boolean {
  const h = haystack.toLowerCase();
  const n = needle.toLowerCase();
  let j = 0;
  for (let i = 0; i < h.length && j < n.length; i++) {
    if (h[i] === n[j]) j++;
  }
  return j === n.length;
}

function getNodeFieldValue(node: Node, field: HierarchyFilterField): string {
  const d = node.data as Record<string, unknown> | undefined;
  switch (field) {
    case 'name': return (d?.name as string) ?? node.id;
    case 'type': return node.type ?? '';
    case 'provider': return (d?.provider as string) ?? '';
    case 'model': return (d?.model as string) ?? '';
    case 'text': return [d?.name ?? node.id, node.type, d?.provider, d?.model, d?.description, d?.blockType].filter(Boolean).join(' ');
  }
}

function matchesChip(node: Node, chip: HierarchyChip): boolean {
  const haystack = getNodeFieldValue(node, chip.field);
  if (chip.isRegex) {
    try { return new RegExp(chip.value, 'i').test(haystack); }
    catch { return subsequenceMatch(haystack, chip.value); }
  }
  return subsequenceMatch(haystack, chip.value);
}

function nodeMatchesAllFilters(node: Node): boolean {
  // All committed chips must match (AND)
  if (chips.value.length > 0 && !chips.value.every(c => matchesChip(node, c))) return false;
  // Live input must match
  const live = inputValue.value.trim();
  if (live && !matchesChip(node, { field: inputField.value, value: live, isRegex: inputRegex.value })) return false;
  return true;
}

function sortByPosition(a: Node, b: Node): number {
  const dy = a.position.y - b.position.y;
  return dy !== 0 ? dy : a.position.x - b.position.x;
}

interface TreeItem {
  node: Node;
  children: TreeItem[];
}

function buildTree(parentId: string | null): TreeItem[] {
  const children = props.nodes
    .filter(n => (n.parentNode ?? null) === parentId)
    .sort(sortByPosition);

  return children.map(node => ({
    node,
    children: node.type === 'group' ? buildTree(node.id) : [],
  }));
}

function filterTree(items: TreeItem[]): TreeItem[] {
  const hasFilter = chips.value.length > 0 || inputValue.value.trim();
  if (!hasFilter) return items;
  const result: TreeItem[] = [];
  for (const item of items) {
    const childrenFiltered = filterTree(item.children);
    if (nodeMatchesAllFilters(item.node) || childrenFiltered.length > 0) {
      result.push({ node: item.node, children: childrenFiltered });
    }
  }
  return result;
}

const tree = computed(() => filterTree(buildTree(null)));

function toggleCollapse(nodeId: string) {
  const next = new Set(collapsed.value);
  if (next.has(nodeId)) next.delete(nodeId);
  else next.add(nodeId);
  collapsed.value = next;
}

function onSelect(nodeId: string, multi = false) {
  emit('select-node', nodeId, multi);
}

function onDoubleClick(node: Node) {
  if (node.type === 'group') {
    emit('enter-scope', node.id);
  }
}
</script>

<template>
  <div class="hierarchy-panel" :class="{ closed: !isOpen }">
    <div class="panel-header">
      <span class="panel-title">Hierarchy</span>
      <button class="toggle-btn" @click="emit('toggle')" title="Toggle hierarchy panel">
        <span class="chevron" :class="{ flipped: !isOpen }">&#x25C0;</span>
      </button>
    </div>

    <!-- Filter chips -->
    <div v-if="chips.length > 0" class="chip-row">
      <span v-for="(chip, i) in chips" :key="i" class="filter-chip">
        <span class="chip-field">{{ chip.field }}</span>
        <span v-if="chip.isRegex" class="chip-rx">.*</span>
        {{ chip.value }}
        <button class="chip-x" @click="removeChip(i)">x</button>
      </span>
      <button class="chip-clear" @click="clearFilters">Clear</button>
    </div>

    <!-- Filter input -->
    <div class="search-bar">
      <select v-model="inputField" class="field-select" @mousedown.stop>
        <option v-for="f in HIERARCHY_FIELDS" :key="f.key" :value="f.key">{{ f.label }}</option>
      </select>
      <input
        v-model="inputValue"
        type="text"
        placeholder="Filter... (GCL = GameClone)"
        class="search-input"
        @keydown.enter="addChip"
      />
      <button
        class="regex-toggle"
        :class="{ active: inputRegex }"
        @click="inputRegex = !inputRegex"
        title="Toggle regex"
      >.*</button>
      <button class="add-chip-btn" @click="addChip" :disabled="!inputValue.trim()">+</button>
    </div>

    <div class="tree-container">
      <template v-for="item in tree" :key="item.node.id">
        <HierarchyItem
          :item="item"
          :depth="0"
          :collapsed="collapsed"
          :selectedIds="selectedSet"
          @select="onSelect"
          @double-click="onDoubleClick"
          @toggle-collapse="toggleCollapse"
        />
      </template>
    </div>
  </div>
</template>

<!-- Recursive tree item rendered inline via functional approach -->
<script lang="ts">
import { defineComponent, h, type PropType } from 'vue';

interface TreeItemType {
  node: Node;
  children: TreeItemType[];
}

const HierarchyItem = defineComponent({
  name: 'HierarchyItem',
  props: {
    item: { type: Object as PropType<TreeItemType>, required: true },
    depth: { type: Number, required: true },
    collapsed: { type: Object as PropType<Set<string>>, required: true },
    selectedIds: { type: Object as PropType<Set<string>>, required: true },
  },
  emits: ['select', 'double-click', 'toggle-collapse'],
  setup(props, { emit }) {
    function badgeForNode(node: any): { letter: string; color: string } {
      if (node.type === 'group') {
        const bt = node.data?.blockType as string | undefined;
        let color = '#888';
        if (bt === 'pipeline') color = '#42a5f5';
        else if (bt === 'parallel') color = '#66bb6a';
        else if (bt === 'team') color = '#ab47bc';
        return { letter: 'G', color };
      }
      switch (node.type) {
        case 'agent': return { letter: 'A', color: '#667eea' };
        case 'tool': return { letter: 'T', color: '#11998e' };
        case 'skill': return { letter: 'S', color: '#f093fb' };
        case 'orchestrator': return { letter: 'O', color: '#ff9800' };
        default: return { letter: '?', color: '#888' };
      }
    }

    return () => {
      const { item, depth, collapsed, selectedIds } = props;
      const node = item.node as any;
      const isGroup = node.type === 'group';
      const isCollapsed = collapsed.has(node.id);
      const badge = badgeForNode(node);
      const name = node.data?.name ?? node.id;
      const isSelected = selectedIds.has(node.id);

      const rowChildren: any[] = [];

      // Chevron for groups
      if (isGroup) {
        rowChildren.push(
          h('span', {
            class: ['tree-chevron', { rotated: !isCollapsed }],
            onClick: (e: MouseEvent) => { e.stopPropagation(); emit('toggle-collapse', node.id); },
          }, '\u25B6')
        );
      } else {
        rowChildren.push(h('span', { class: 'tree-chevron-placeholder' }));
      }

      // Type badge
      rowChildren.push(
        h('span', {
          class: 'type-badge',
          style: { backgroundColor: badge.color },
        }, badge.letter)
      );

      // Name
      rowChildren.push(h('span', { class: 'item-name' }, name));

      // Provider + model badges for agents
      if (node.type === 'agent' && node.data) {
        const parts: any[] = [];
        if (node.data.provider) {
          parts.push(h('span', { class: 'meta-badge provider-badge' }, node.data.provider));
        }
        if (node.data.model) {
          parts.push(h('span', { class: 'meta-badge model-badge' }, node.data.model));
        }
        if (parts.length) {
          rowChildren.push(h('span', { class: 'agent-meta' }, parts));
        }
      }

      const row = h('div', {
        class: ['tree-row', { selected: isSelected }],
        style: { paddingLeft: `${depth * 16 + 8}px` },
        onClick: (e: MouseEvent) => emit('select', node.id, e.metaKey || e.ctrlKey),
        onDblclick: () => { if (isGroup) emit('double-click', node); },
      }, rowChildren);

      const result: any[] = [row];

      // Children
      if (isGroup && !isCollapsed && item.children.length > 0) {
        for (const child of item.children) {
          result.push(
            h(HierarchyItem, {
              item: child,
              depth: depth + 1,
              collapsed,
              selectedIds,
              onSelect: (id: string, multi: boolean) => emit('select', id, multi),
              'onDouble-click': (n: any) => emit('double-click', n),
              'onToggle-collapse': (id: string) => emit('toggle-collapse', id),
            })
          );
        }
      }

      return result;
    };
  },
});
</script>

<style scoped>
.hierarchy-panel {
  width: 250px;
  min-width: 250px;
  background: var(--surface-1);
  border-right: 1px solid var(--border-default);
  display: flex;
  flex-direction: column;
  overflow: hidden;
  transition: width 0.2s ease, min-width 0.2s ease;
}

.hierarchy-panel.closed {
  width: 0;
  min-width: 0;
  border-right: none;
}

.panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  border-bottom: 1px solid var(--border-default);
  flex-shrink: 0;
}

.panel-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
  white-space: nowrap;
}

.toggle-btn {
  background: none;
  border: none;
  cursor: pointer;
  padding: 2px 4px;
  font-size: 12px;
  color: var(--text-secondary);
  border-radius: var(--node-radius);
}

.toggle-btn:hover {
  background: var(--surface-4);
}

.chevron {
  display: inline-block;
  transition: transform 0.2s ease;
}

.chevron.flipped {
  transform: rotate(180deg);
}

.search-bar {
  padding: 8px;
  border-bottom: 1px solid var(--border-subtle);
  flex-shrink: 0;
}

.search-bar {
  display: flex;
  gap: 4px;
}
.search-input {
  flex: 1;
  padding: 5px 8px;
  background: var(--surface-1);
  color: var(--text-primary);
  border: 1px solid var(--border-default);
  border-radius: var(--node-radius);
  font-size: 12px;
  outline: none;
  box-sizing: border-box;
}
.search-input:focus { border-color: var(--border-focus); }
.regex-toggle {
  background: none;
  border: 1px solid var(--border-default);
  border-radius: var(--node-radius);
  padding: 2px 6px;
  font-size: 10px;
  font-family: var(--font-mono);
  font-weight: 700;
  color: var(--text-muted);
  cursor: pointer;
}
.regex-toggle:hover { color: var(--text-secondary); }
.regex-toggle.active { background: var(--accent-agent); color: var(--surface-0); border-color: var(--accent-agent); }

.chip-row {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  padding: 4px 8px;
}
.filter-chip {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  background: var(--surface-3);
  border-radius: var(--node-radius);
  padding: 2px 6px;
  font-size: 10px;
  color: var(--text-primary);
}
.chip-field {
  color: var(--accent-agent);
  font-weight: 600;
  text-transform: uppercase;
  font-size: 9px;
}
.chip-rx { color: var(--status-paused); font-family: var(--font-mono); font-size: 9px; }
.chip-x {
  background: none;
  border: none;
  cursor: pointer;
  color: var(--text-muted);
  font-size: 11px;
  padding: 0 2px;
}
.chip-x:hover { color: var(--status-error); }
.chip-clear {
  background: none;
  border: none;
  cursor: pointer;
  color: var(--text-muted);
  font-size: 10px;
}
.chip-clear:hover { color: var(--status-error); }

.field-select {
  background: var(--surface-1);
  color: var(--text-primary);
  border: 1px solid var(--border-default);
  border-radius: var(--node-radius);
  font-size: 10px;
  padding: 3px 2px;
  width: 50px;
}
.field-select:focus { border-color: var(--border-focus); }
.add-chip-btn {
  background: none;
  border: 1px solid var(--accent-agent);
  border-radius: var(--node-radius);
  color: var(--accent-agent);
  cursor: pointer;
  font-size: 12px;
  font-weight: 700;
  padding: 1px 6px;
}
.add-chip-btn:hover:not(:disabled) { background: rgba(91, 141, 239, 0.1); }
.add-chip-btn:disabled { opacity: 0.3; }

.tree-container {
  flex: 1;
  overflow-y: auto;
  padding: 4px 0;
}

.tree-row {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 8px;
  cursor: pointer;
  font-size: 12px;
  color: var(--text-primary);
  white-space: nowrap;
  user-select: none;
}

.tree-row:hover {
  background: var(--surface-4);
}

.tree-row.selected {
  background: var(--surface-3);
}

.tree-chevron {
  font-size: 8px;
  cursor: pointer;
  width: 12px;
  text-align: center;
  flex-shrink: 0;
  transition: transform 0.15s ease;
  color: var(--text-muted);
}

.tree-chevron.rotated {
  transform: rotate(90deg);
}

.tree-chevron-placeholder {
  width: 12px;
  flex-shrink: 0;
}

.type-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  border-radius: var(--node-radius);
  font-size: 9px;
  font-weight: 700;
  color: var(--surface-0);
  flex-shrink: 0;
  line-height: 1;
}

.item-name {
  overflow: hidden;
  text-overflow: ellipsis;
  flex: 1;
  min-width: 0;
}

.agent-meta {
  display: flex;
  gap: 3px;
  flex-shrink: 0;
  margin-left: auto;
}

.meta-badge {
  font-size: 9px;
  padding: 1px 4px;
  border-radius: var(--node-radius);
  line-height: 1.2;
  white-space: nowrap;
  max-width: 60px;
  overflow: hidden;
  text-overflow: ellipsis;
  font-family: var(--font-mono);
}

.provider-badge {
  background: var(--surface-3);
  color: var(--accent-agent);
}

.model-badge {
  background: var(--surface-2);
  color: var(--text-secondary);
}
</style>
