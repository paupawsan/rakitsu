<script setup lang="ts">
import { ref, computed, watch, nextTick } from 'vue';

export interface ComboOption {
  value: string;
  label: string;
}

const props = withDefaults(defineProps<{
  modelValue: string;
  options: (string | ComboOption)[];
  placeholder?: string;
  loading?: boolean;
  loadingText?: string;
  hint?: string;
  hintClass?: string;
  allowCustom?: boolean;  // allow typing custom values (true for models/providers, false for enums)
}>(), {
  placeholder: 'Select or type...',
  loading: false,
  loadingText: 'loading...',
  hint: '',
  hintClass: '',
  allowCustom: true,
});

const emit = defineEmits<{
  'update:modelValue': [value: string];
}>();

// Normalize options to { value, label } pairs
const normalizedOptions = computed<ComboOption[]>(() =>
  props.options.map(o => typeof o === 'string' ? { value: o, label: o } : o)
);

const inputRef = ref<HTMLInputElement | null>(null);
const open = ref(false);
const filter = ref('');
const filterMode = ref(false);

watch(open, (isOpen) => {
  if (isOpen) {
    filter.value = '';
    nextTick(() => inputRef.value?.focus());
  }
});

const filtered = computed(() => {
  if (!filterMode.value || !filter.value) return normalizedOptions.value;
  const q = filter.value.toLowerCase();
  return normalizedOptions.value.filter(o =>
    o.label.toLowerCase().includes(q) || o.value.toLowerCase().includes(q)
  );
});

// Display: find label for current value, or show value itself
const displayValue = computed(() => {
  if (!props.modelValue) return props.placeholder;
  const match = normalizedOptions.value.find(o => o.value === props.modelValue);
  return match ? match.label : props.modelValue;
});

const isPlaceholder = computed(() => !props.modelValue);

function select(value: string) {
  emit('update:modelValue', value);
  open.value = false;
}

function handleInputKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter') {
    e.preventDefault();
    const trimmed = filter.value.trim();
    if (trimmed && props.allowCustom) {
      select(trimmed);
    } else if (filtered.value.length === 1) {
      select(filtered.value[0]!.value);
    }
  } else if (e.key === 'Escape') {
    open.value = false;
  }
}

function handleClickOutside(e: MouseEvent) {
  const target = e.target as HTMLElement;
  if (!target.closest('.combobox')) {
    open.value = false;
  }
}

watch(open, (isOpen) => {
  if (isOpen) {
    setTimeout(() => document.addEventListener('click', handleClickOutside), 0);
  } else {
    document.removeEventListener('click', handleClickOutside);
  }
});
</script>

<template>
  <div class="combobox" :class="{ open }">
    <button
      v-if="!open"
      class="combobox-trigger"
      :class="{ placeholder: isPlaceholder }"
      @click="open = true"
    >
      <span class="trigger-text">{{ displayValue }}</span>
      <span class="trigger-arrow">&#x25BE;</span>
    </button>

    <div v-else class="combobox-dropdown">
      <div class="combobox-header">
        <input
          ref="inputRef"
          v-model="filter"
          class="combobox-input"
          :placeholder="filterMode ? 'Filter...' : (allowCustom ? 'Type custom value...' : 'Search...')"
          @keydown="handleInputKeydown"
        />
        <button
          class="filter-toggle"
          :class="{ active: filterMode }"
          :title="filterMode ? 'Showing filtered — click for all' : 'Showing all — click to filter'"
          @mousedown.prevent="filterMode = !filterMode"
        >{{ filterMode ? 'F' : 'A' }}</button>
      </div>
      <div class="combobox-options">
        <div v-if="loading" class="combobox-loading">{{ loadingText }}</div>
        <template v-else>
          <div
            v-for="opt in filtered"
            :key="opt.value"
            class="combobox-option"
            :class="{ selected: opt.value === modelValue }"
            @mousedown.prevent="select(opt.value)"
          >{{ opt.label }}</div>
          <div v-if="filtered.length === 0 && filter && allowCustom" class="combobox-empty">
            Press Enter to use "{{ filter }}"
          </div>
          <div v-else-if="filtered.length === 0 && filter" class="combobox-empty">
            No matches
          </div>
        </template>
      </div>
      <div v-if="hint" class="combobox-hint" :class="hintClass">{{ hint }}</div>
    </div>
  </div>
</template>

<style scoped>
.combobox {
  position: relative;
}

.combobox-trigger {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  padding: 5px 8px;
  border: 1px solid var(--border-default);
  border-radius: var(--node-radius);
  background: var(--surface-1);
  font-size: 12px;
  cursor: pointer;
  text-align: left;
  color: var(--text-primary);
}
.combobox-trigger:hover {
  border-color: var(--border-focus);
}
.combobox-trigger.placeholder {
  color: var(--text-muted);
}
.trigger-text {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.trigger-arrow {
  color: var(--text-muted);
  font-size: 10px;
  margin-left: 4px;
}

.combobox-dropdown {
  border: 1px solid var(--accent-agent);
  border-radius: var(--node-radius);
  background: var(--surface-3);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
}

.combobox-header {
  display: flex;
  border-bottom: 1px solid var(--border-subtle);
}

.combobox-input {
  flex: 1;
  padding: 6px 8px;
  border: none;
  font-size: 12px;
  outline: none;
  border-radius: var(--node-radius) 0 0 0;
  box-sizing: border-box;
  min-width: 0;
  background: var(--surface-3);
  color: var(--text-primary);
}

.filter-toggle {
  padding: 4px 8px;
  border: none;
  border-left: 1px solid var(--border-subtle);
  background: var(--surface-2);
  font-size: 10px;
  font-weight: 700;
  color: var(--text-muted);
  cursor: pointer;
  border-radius: 0 var(--node-radius) 0 0;
}
.filter-toggle:hover {
  background: var(--surface-4);
  color: var(--accent-agent);
}
.filter-toggle.active {
  background: var(--accent-agent);
  color: var(--surface-0);
}

.combobox-options {
  max-height: 200px;
  overflow-y: auto;
}

.combobox-option {
  padding: 4px 8px;
  font-size: 12px;
  cursor: pointer;
  color: var(--text-primary);
}
.combobox-option:hover {
  background: var(--surface-4);
}
.combobox-option.selected {
  background: var(--surface-4);
  font-weight: 500;
  color: var(--accent-agent);
}

.combobox-empty {
  padding: 8px;
  font-size: 11px;
  color: var(--text-muted);
  text-align: center;
}

.combobox-loading {
  padding: 8px;
  font-size: 11px;
  color: var(--text-muted);
  text-align: center;
}

.combobox-hint {
  padding: 3px 8px;
  font-size: 10px;
  color: var(--text-muted);
  border-top: 1px solid var(--border-subtle);
}
.combobox-hint.discovered {
  color: var(--status-running);
}
</style>
