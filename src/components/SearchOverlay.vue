<template>
  <Teleport to="body">
    <div class="overlay-backdrop" v-if="ui.searchOverlayOpen" @click.self="ui.closeSearchOverlay()">
      <div class="overlay-modal" @keydown.escape="ui.closeSearchOverlay()">
        <div class="overlay-search-row">
          <Search :size="18" class="overlay-search-icon" />
          <input
            ref="inputRef"
            class="overlay-input"
            v-model="query"
            placeholder="Search entries…"
            @input="onInput"
            @keydown="onKeydown"
            @blur="() => {}"
          />
          <button class="overlay-close" @click="ui.closeSearchOverlay()" title="Close (Escape)">
            <X :size="16" />
          </button>
        </div>
        <div class="overlay-results" v-if="results.length > 0">
          <div
            v-for="(entry, i) in results"
            :key="entry.id"
            class="overlay-result"
            :class="{ focused: i === focusedIdx }"
            @mousedown.prevent="select(entry)"
          >
            <span class="or-cat" :style="{ color: `var(--cat-${entry.category.toLowerCase()})` }">
              {{ entry.category }}
            </span>
            <div class="or-content">
              <span class="or-title">{{ entry.title }}</span>
              <span class="or-excerpt" v-if="entry.excerpt">{{ entry.excerpt }}</span>
            </div>
          </div>
        </div>
        <p class="overlay-empty" v-else-if="query.trim()">No results for "{{ query }}"</p>
        <p class="overlay-hint" v-else>Type to search entries…</p>
      </div>
    </div>
  </Teleport>
</template>

<script setup>
import { ref, computed, watch, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import { Search, X } from 'lucide-vue-next'
import { useEntriesStore } from '../stores/useEntriesStore.js'
import { useUIStore } from '../stores/useUIStore.js'

const entries = useEntriesStore()
const ui = useUIStore()
const router = useRouter()

const query = ref('')
const focusedIdx = ref(0)
const inputRef = ref(null)

watch(() => ui.searchOverlayOpen, async (open) => {
  if (open) {
    query.value = ''
    focusedIdx.value = 0
    await nextTick()
    inputRef.value?.focus()
  }
})

const results = computed(() => {
  if (!query.value.trim()) return []
  const raw = entries.search(query.value).slice(0, 12)
  const q = query.value.toLowerCase()
  return raw.map(e => {
    const bodyText = e.body.replace(/<[^>]+>/g, '')
    const idx = bodyText.toLowerCase().indexOf(q)
    const excerpt = idx !== -1
      ? '…' + bodyText.slice(Math.max(0, idx - 40), idx + 80).trim() + '…'
      : ''
    return { ...e, excerpt }
  })
})

function onInput() {
  focusedIdx.value = 0
}

function onKeydown(e) {
  if (e.key === 'Escape') { ui.closeSearchOverlay(); return }
  if (!results.value.length) return
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
  }
}

function select(entry) {
  ui.setActiveEntry(entry.id)
  router.push('/notes/' + entry.id)
  ui.closeSearchOverlay()
}
</script>

<style scoped>
.overlay-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.65);
  z-index: 500;
  display: flex;
  align-items: flex-start;
  justify-content: center;
  padding-top: 15vh;
}

.overlay-modal {
  width: 520px;
  max-width: calc(100vw - 32px);
  background: var(--bg-surface);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  box-shadow: 0 8px 40px rgba(0, 0, 0, 0.6);
  overflow: hidden;
}

.overlay-search-row {
  display: flex;
  align-items: center;
  gap: var(--space-sm);
  padding: var(--space-md) var(--space-lg);
  border-bottom: 1px solid var(--border);
}

.overlay-search-icon { color: var(--text-faint); flex-shrink: 0; }

.overlay-input {
  flex: 1;
  background: none;
  border: none;
  outline: none;
  color: var(--text-primary);
  font-family: var(--font-body);
  font-size: var(--text-xl);
}
.overlay-input::placeholder { color: var(--text-faint); }

.overlay-close {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: none;
  border: none;
  color: var(--text-faint);
  cursor: pointer;
  height: var(--h-sm);
  width: var(--h-sm);
  border-radius: var(--radius);
  flex-shrink: 0;
  transition: color var(--transition), background var(--transition);
}
.overlay-close:hover { color: var(--text-primary); background: var(--bg-hover); }

.overlay-results { max-height: 400px; overflow-y: auto; }

.overlay-result {
  display: flex;
  align-items: flex-start;
  gap: var(--space-md);
  padding: var(--space-md) var(--space-lg);
  cursor: pointer;
  transition: background var(--transition);
  border-bottom: 1px solid var(--border);
}
.overlay-result:last-child { border-bottom: none; }
.overlay-result:hover, .overlay-result.focused { background: var(--bg-hover); }

.or-cat {
  font-size: var(--text-xs);
  text-transform: uppercase;
  letter-spacing: 0.08em;
  font-weight: 600;
  width: 60px;
  flex-shrink: 0;
  padding-top: 2px;
}
.or-content { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
.or-title { font-size: var(--text-lg); color: var(--text-primary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.or-excerpt { font-size: var(--text-sm); color: var(--text-faint); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }

.overlay-empty, .overlay-hint {
  padding: var(--space-xl) var(--space-lg);
  font-size: var(--text-md);
  color: var(--text-faint);
  text-align: center;
}
</style>
