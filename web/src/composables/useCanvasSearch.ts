import { type Ref, ref, computed } from 'vue'
import type { Node } from '@vue-flow/core'

type SearchField = 'all' | 'name' | 'type' | 'provider' | 'model'

function subsequenceMatch(haystack: string, needle: string): boolean {
  const h = haystack.toLowerCase()
  const n = needle.toLowerCase()
  let j = 0
  for (let i = 0; i < h.length && j < n.length; i++) {
    if (h[i] === n[j]) j++
  }
  return j === n.length
}

function extractField(node: Node, field: SearchField): string {
  const data = node.data as Record<string, unknown>
  switch (field) {
    case 'name':
      return String(data?.name ?? '')
    case 'type':
      return String(node.type ?? '')
    case 'provider':
      return String(data?.provider ?? '')
    case 'model':
      return String(data?.model ?? '')
    case 'all':
      return [
        String(data?.name ?? ''),
        String(node.type ?? ''),
        String(data?.provider ?? ''),
        String(data?.model ?? ''),
      ].join(' ')
  }
}

export function useCanvasSearch(nodes: Ref<Node[]>) {
  const searchOpen = ref(false)
  const searchQuery = ref('')
  const searchField = ref<SearchField>('all')
  const currentMatchIndex = ref(0)

  const matchedNodeIds = computed<Set<string>>(() => {
    const query = searchQuery.value.trim()
    if (!query) return new Set<string>()

    const ids = new Set<string>()
    for (const node of nodes.value) {
      const text = extractField(node, searchField.value)
      if (subsequenceMatch(text, query)) {
        ids.add(node.id)
      }
    }
    return ids
  })

  const matchCount = computed(() => matchedNodeIds.value.size)

  const matchedIdList = computed(() => Array.from(matchedNodeIds.value))

  function focusResult(nodeId: string): Node | undefined {
    const node = nodes.value.find((n) => n.id === nodeId)
    if (node) {
      const idx = matchedIdList.value.indexOf(nodeId)
      if (idx >= 0) currentMatchIndex.value = idx
    }
    return node
  }

  function nextResult(): Node | undefined {
    if (matchCount.value === 0) return undefined
    currentMatchIndex.value = (currentMatchIndex.value + 1) % matchCount.value
    const id = matchedIdList.value[currentMatchIndex.value]
    return id ? focusResult(id) : undefined
  }

  function prevResult(): Node | undefined {
    if (matchCount.value === 0) return undefined
    currentMatchIndex.value =
      (currentMatchIndex.value - 1 + matchCount.value) % matchCount.value
    const id = matchedIdList.value[currentMatchIndex.value]
    return id ? focusResult(id) : undefined
  }

  function toggleSearch() {
    searchOpen.value = !searchOpen.value
    if (!searchOpen.value) {
      searchQuery.value = ''
      currentMatchIndex.value = 0
    }
  }

  function closeSearch() {
    searchOpen.value = false
    searchQuery.value = ''
    currentMatchIndex.value = 0
  }

  return {
    searchOpen,
    searchQuery,
    searchField,
    matchedNodeIds,
    matchCount,
    currentMatchIndex,
    focusResult,
    nextResult,
    prevResult,
    toggleSearch,
    closeSearch,
  }
}
