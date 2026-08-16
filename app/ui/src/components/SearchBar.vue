<template>
  <div class="search-bar">
    <input
      ref="inputRef"
      class="search-input"
      v-model="query"
      placeholder="Search entries…"
      @input="onInput"
      @keydown="onKeydown"
      @blur="onBlur"
      @focus="showDropdown = true"
    />
    <div class="search-dropdown" v-if="showDropdown && results.length > 0">
      <div
        v-for="(entry, i) in results"
        :key="entry.id"
        class="search-result"
        :class="{ focused: i === focusedIdx }"
        @mousedown.prevent="select(entry)"
      >
        <span class="sr-cat" :style="{ color: `var(--cat-${entry.category.toLowerCase()})` }">
          {{ entry.category }}
        </span>
        <div class="sr-content">
          <span class="sr-title">{{ entry.title }}</span>
          <span class="sr-excerpt" v-if="entry.excerpt">{{ entry.excerpt }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { useEntriesStore } from '../stores/useEntriesStore'
import { useUIStore } from '../stores/useUIStore'

const entries = useEntriesStore()
const ui = useUIStore()
const router = useRouter()

const query = ref('')
const showDropdown = ref(false)
const focusedIdx = ref(0)
const inputRef = ref<HTMLInputElement | null>(null)

const results = computed(() => {
  if (!query.value.trim()) return []
  const raw = entries.search(query.value).slice(0, 10)
  const q = query.value.toLowerCase()
  return raw.map(e => {
    const bodyText = e.body.replace(/<[^>]+>/g, '')
    const idx = bodyText.toLowerCase().indexOf(q)
    const excerpt = idx !== -1
      ? '…' + bodyText.slice(Math.max(0, idx - 30), idx + 60).trim() + '…'
      : ''
    return { ...e, excerpt }
  })
})

let debounceTimer: ReturnType<typeof setTimeout> | null = null
function onInput() {
  focusedIdx.value = 0
  if (debounceTimer) clearTimeout(debounceTimer)
  debounceTimer = setTimeout(() => { showDropdown.value = true }, 150)
}

function onKeydown(e: KeyboardEvent) {
  if (!showDropdown.value || !results.value.length) return
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    focusedIdx.value = (focusedIdx.value + 1) % results.value.length
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    focusedIdx.value = (focusedIdx.value - 1 + results.value.length) % results.value.length
  } else if (e.key === 'Enter') {
    e.preventDefault()
    const entry = results.value[focusedIdx.value]
    if (entry) select(entry)
  } else if (e.key === 'Escape') {
    showDropdown.value = false
    inputRef.value?.blur()
  }
}

function onBlur() {
  setTimeout(() => { showDropdown.value = false }, 150)
}

function select(entry: { id: string }) {
  query.value = ''
  showDropdown.value = false
  ui.setActiveEntry(entry.id)
  router.push('/entries/' + entry.id)
}
</script>

<style scoped>
.search-bar {
  position: relative;
  padding: 8px 8px 4px;
}

.search-input {
  width: 100%;
  background: var(--muted);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  color: var(--foreground);
  font-family: var(--font-body);
  font-size: 0.85em;
  padding: 5px 10px;
  outline: none;
  transition: border-color var(--transition);
}
.search-input::placeholder { color: var(--muted-foreground); }
.search-input:focus { border-color: var(--secondary); }

.search-dropdown {
  position: absolute;
  top: 100%;
  left: 8px;
  right: 8px;
  background: var(--card);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  z-index: 200;
  overflow: hidden;
  box-shadow: 0 4px 16px rgba(0,0,0,0.12);
}

.search-result {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 8px 12px;
  cursor: pointer;
  transition: background var(--transition);
  border-bottom: 1px solid var(--border);
}
.search-result:last-child { border-bottom: none; }
.search-result:hover, .search-result.focused { background: var(--muted); }

.sr-cat {
  font-size: 0.65em;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  font-weight: 600;
  width: 52px;
  flex-shrink: 0;
  padding-top: 2px;
}

.sr-content { display: flex; flex-direction: column; gap: 1px; min-width: 0; }
.sr-title { font-size: 0.9em; color: var(--foreground); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.sr-excerpt { font-size: 0.78em; color: var(--muted-foreground); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
</style>
