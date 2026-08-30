<script setup lang="ts">
import { Handle, Position } from '@vue-flow/core';
import BuilderSideHandles from './BuilderSideHandles.vue';
import type { SkillConfig } from '../../types';

defineProps<{
  data: SkillConfig;
  selected?: boolean;
}>();
</script>

<template>
  <div class="skill-node" :class="{ selected }">
    <Handle type="target" :position="Position.Top" class="handle-input" />
    <BuilderSideHandles />

    <div class="node-header">
      <span class="node-title">{{ data.name }}</span>
    </div>

    <div class="node-content">
      <div class="node-field description">
        <label>DESC</label>
        <span class="field-value">{{ data.description?.slice(0, 50) }}{{ data.description && data.description.length > 50 ? '...' : '' }}</span>
      </div>
      <div class="node-field tools-list">
        <label>TOOLS</label>
        <span class="field-value">{{ data.tools?.length > 0 ? data.tools.join(', ') : 'none' }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.skill-node {
  background: var(--node-bg);
  border: 1px solid var(--node-border);
  border-left: 4px solid var(--accent-skill);
  border-radius: var(--node-radius);
  color: var(--text-primary);
  min-width: 200px;
  font-size: 12px;
}

.skill-node.selected {
  box-shadow: 0 0 0 1px var(--accent-skill), 0 0 0 3px rgba(192, 132, 252, 0.3);
}

.node-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 7px 10px;
  border-bottom: 1px solid var(--border-subtle);
}

.node-title { font-weight: 600; font-size: 13px; color: var(--text-primary); }

.node-content { padding: 6px 10px; }

.node-field {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 3px;
}

.node-field label {
  font-size: 9px;
  font-weight: 500;
  color: var(--text-muted);
  font-family: var(--font-mono);
  letter-spacing: 0.05em;
  min-width: 40px;
}

.field-value {
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--text-secondary);
}

.description,
.tools-list {
  max-width: 180px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.handle-input {
  background: var(--text-secondary) !important;
  width: var(--handle-width) !important;
  height: var(--handle-height) !important;
  border-radius: var(--handle-radius) !important;
  border: none !important;
}
</style>
