<script setup lang="ts">
import { getAllVisualizationStyles } from './visualizations';

defineProps<{ modelValue: string }>();
defineEmits<{ 'update:modelValue': [id: string] }>();

const styles = getAllVisualizationStyles();
</script>

<template>
  <div class="style-selector">
    <button
      v-for="s in styles"
      :key="s.meta.id"
      class="style-btn"
      :class="{ active: modelValue === s.meta.id }"
      :title="s.meta.description"
      @click="$emit('update:modelValue', s.meta.id)"
    >
      <span class="style-icon">{{ s.meta.icon }}</span>
      <span class="style-label">{{ s.meta.label }}</span>
    </button>
  </div>
</template>

<style scoped>
.style-selector {
  display: flex;
  border: 1px solid var(--border-default);
  border-radius: var(--node-radius);
  overflow: hidden;
}
.style-btn {
  display: flex;
  align-items: center;
  gap: 3px;
  padding: 5px 10px;
  border: none;
  background: var(--surface-2);
  font-size: 11px;
  cursor: pointer;
  color: var(--text-secondary);
}
.style-btn:not(:last-child) {
  border-right: 1px solid var(--border-default);
}
.style-btn.active {
  background: var(--accent-agent);
  color: white;
}
.style-btn:hover:not(.active) {
  background: var(--surface-4);
}
.style-icon {
  font-weight: 700;
  font-size: 10px;
}
.style-label {
  font-size: 11px;
}
</style>
