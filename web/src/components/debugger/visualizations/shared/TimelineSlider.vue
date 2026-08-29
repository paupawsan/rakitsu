<script setup lang="ts">
defineProps<{
  visibleUpTo: number;
  totalEvents: number;
  isHistory: boolean;
}>();

const emit = defineEmits<{
  'update:visibleUpTo': [value: number];
}>();
</script>

<template>
  <div v-if="isHistory && totalEvents > 0" class="timeline-slider">
    <input
      type="range"
      :min="0"
      :max="totalEvents - 1"
      :value="visibleUpTo >= 0 ? visibleUpTo : totalEvents - 1"
      @input="emit('update:visibleUpTo', Number(($event.target as HTMLInputElement).value))"
      class="slider"
    />
    <span class="slider-label">
      Event {{ (visibleUpTo >= 0 ? visibleUpTo : totalEvents - 1) + 1 }} / {{ totalEvents }}
    </span>
  </div>
</template>

<style scoped>
.timeline-slider {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 12px;
  border-top: 1px solid #e0e0e0;
  background: #fafafa;
  flex-shrink: 0;
}
.slider {
  flex: 1;
  cursor: pointer;
  accent-color: #667eea;
}
.slider-label {
  font-size: 11px;
  color: #888;
  white-space: nowrap;
}
</style>
