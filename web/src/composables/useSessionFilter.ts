import { ref, computed } from 'vue';

export type FilterField = 'name' | 'query' | 'agent' | 'status' | 'path' | 'text';

export interface FilterChip {
  field: FilterField;
  value: string;
  isRegex: boolean;
}

export const FILTER_FIELDS: { key: FilterField; label: string; placeholder: string }[] = [
  { key: 'text', label: 'Text', placeholder: 'Search all fields...' },
  { key: 'name', label: 'Name', placeholder: 'Session name...' },
  { key: 'query', label: 'Query', placeholder: 'Prompt/query...' },
  { key: 'agent', label: 'Agent', placeholder: 'Agent name...' },
  { key: 'status', label: 'Status', placeholder: 'running, error, success...' },
  { key: 'path', label: 'Config', placeholder: 'Config path...' },
];

interface Filterable {
  name: string;
  query: string;
  agents: string[];
  status: string;
  config_path?: string;
}

function getFieldValue(session: Filterable, field: FilterField): string {
  switch (field) {
    case 'name': return session.name;
    case 'query': return session.query;
    case 'agent': return (session.agents ?? []).join(' ');
    case 'status': return session.status;
    case 'path': return session.config_path ?? '';
    case 'text': return [session.name, session.query, (session.agents ?? []).join(' '), session.config_path ?? ''].join(' ');
  }
}

// Subsequence match: "Del" matches "Deletion" and "Dev Plan" (D..e..l in order)
function subsequenceMatch(haystack: string, needle: string): boolean {
  const h = haystack.toLowerCase();
  const n = needle.toLowerCase();
  let j = 0;
  for (let i = 0; i < h.length && j < n.length; i++) {
    if (h[i] === n[j]) j++;
  }
  return j === n.length;
}

function matchesChip(session: Filterable, chip: FilterChip): boolean {
  const haystack = getFieldValue(session, chip.field);
  if (chip.isRegex) {
    try {
      return new RegExp(chip.value, 'i').test(haystack);
    } catch {
      return subsequenceMatch(haystack, chip.value);
    }
  }
  if (chip.field === 'status') {
    return haystack.toLowerCase() === chip.value.toLowerCase();
  }
  return subsequenceMatch(haystack, chip.value);
}

export function useSessionFilter() {
  const chips = ref<FilterChip[]>([]);

  // Live input state (filters in real-time as you type)
  const inputField = ref<FilterField>('text');
  const inputValue = ref('');
  const inputRegex = ref(false);

  function addChip(field?: FilterField, value?: string, isRegex?: boolean) {
    const f = field ?? inputField.value;
    const v = value ?? inputValue.value.trim();
    if (!v) return;
    chips.value = [...chips.value, { field: f, value: v, isRegex: isRegex ?? inputRegex.value }];
    inputValue.value = '';
  }

  function removeChip(index: number) {
    chips.value = chips.value.filter((_, i) => i !== index);
  }

  function clearAll() {
    chips.value = [];
    inputValue.value = '';
    inputRegex.value = false;
    inputField.value = 'text';
  }

  // Filters by committed chips AND the live input value (real-time as you type)
  function filteredList<T extends Filterable>(list: T[]): T[] {
    let result = list;
    if (chips.value.length > 0) {
      result = result.filter(session => chips.value.every(chip => matchesChip(session, chip)));
    }
    const live = inputValue.value.trim();
    if (live) {
      result = result.filter(session =>
        matchesChip(session, { field: inputField.value, value: live, isRegex: inputRegex.value }),
      );
    }
    return result;
  }

  const hasFilters = computed(() => chips.value.length > 0 || inputValue.value.trim().length > 0);

  return { chips, inputField, inputValue, inputRegex, addChip, removeChip, clearAll, filteredList, hasFilters };
}
