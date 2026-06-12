<template>
  <Dialog :open="ui.searchOverlayOpen" @update:open="(v) => !v && ui.closeSearchOverlay()">
    <DialogContent class="max-w-[520px] w-[calc(100vw-32px)] p-0 overflow-hidden">
      <Command class="rounded-lg border-0">
        <div class="flex items-center gap-2 px-4 py-3 border-b border-[var(--border)]">
          <Search :size="18" class="text-[var(--muted-foreground)] flex-shrink-0" />
          <CommandInput
            v-model="query"
            placeholder="Search entries…"
            class="flex-1 bg-transparent border-none outline-none text-[var(--foreground)] placeholder:text-[var(--muted-foreground)] text-base font-[var(--font-body)]"
            @keydown="onKeydown"
          />
        </div>
        <CommandList class="max-h-[400px] overflow-y-auto">
          <CommandEmpty v-if="query.trim() && !results.length">
            No results for "{{ query }}"
          </CommandEmpty>
          <CommandGroup v-if="results.length">
            <CommandItem
              v-for="(entry, i) in results"
              :key="entry.id"
              :value="entry.id"
              class="flex items-start gap-3 px-4 py-3 cursor-pointer"
              :class="{ 'bg-[var(--muted)]': i === focusedIdx }"
              @mousedown.prevent="select(entry)"
            >
              <span
                class="text-[0.65em] uppercase tracking-widest font-semibold w-[60px] flex-shrink-0 pt-0.5"
                :style="{ color: `var(--cat-${entry.category.toLowerCase()})` }"
              >{{ entry.category }}</span>
              <div class="flex flex-col gap-0.5 min-w-0">
                <span class="text-sm text-[var(--foreground)] truncate">{{ entry.title }}</span>
                <span v-if="entry.excerpt" class="text-xs text-[var(--muted-foreground)] truncate">{{ entry.excerpt }}</span>
              </div>
            </CommandItem>
          </CommandGroup>
          <div v-if="!query.trim()" class="px-4 py-6 text-center text-sm text-[var(--muted-foreground)]">
            Type to search entries…
          </div>
        </CommandList>
      </Command>
    </DialogContent>
  </Dialog>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import { Search } from 'lucide-vue-next'
import { Dialog, DialogContent } from './ui/dialog'
import { Command, CommandInput, CommandList, CommandEmpty, CommandGroup, CommandItem } from './ui/command'
import { useEntriesStore } from '../stores/useEntriesStore'
import { useUIStore } from '../stores/useUIStore'
import type { Entry } from '../types'

const entries = useEntriesStore()
const ui = useUIStore()
const router = useRouter()

const query = ref('')
const focusedIdx = ref(0)

watch(() => ui.searchOverlayOpen, async (open) => {
  if (open) {
    query.value = ''
    focusedIdx.value = 0
    await nextTick()
  }
})

interface SearchResult extends Entry { excerpt: string }

const results = computed((): SearchResult[] => {
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

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') { ui.closeSearchOverlay(); return }
  if (!results.value.length) return
  if (e.key === 'ArrowDown') { e.preventDefault(); focusedIdx.value = (focusedIdx.value + 1) % results.value.length }
  else if (e.key === 'ArrowUp') { e.preventDefault(); focusedIdx.value = (focusedIdx.value - 1 + results.value.length) % results.value.length }
  else if (e.key === 'Enter') { e.preventDefault(); const entry = results.value[focusedIdx.value]; if (entry) select(entry) }
}

function select(entry: Entry) {
  ui.setActiveEntry(entry.id)
  router.push('/notes/' + entry.id)
  ui.closeSearchOverlay()
}
</script>
