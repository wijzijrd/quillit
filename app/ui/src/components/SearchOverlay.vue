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
          <!--
            Plain markup below, not <CommandGroup>/<CommandItem>/<CommandEmpty>:
            those primitives cache their visibility from reka-ui's internal
            filterState the moment the input value changes, computed against
            whatever's in the DOM *right then*. That's fine for the old
            synchronous client-side filter, but results here now arrive after
            a debounce + network round-trip — by the time they mount, the
            cached filter (and group visibility derived from it) is already
            stale and never recomputes, so the group renders `display:none`
            forever. Plain elements driven directly by `results`/`searching`
            sidestep that; keyboard nav and selection below are already fully
            custom (onKeydown/select), so nothing from Command*Item's own
            behavior was actually relied on.
          -->
          <div v-if="query.trim() && !searching && !results.length" class="px-4 py-6 text-center text-sm text-[var(--muted-foreground)]">
            No results for "{{ query }}"
          </div>
          <div v-else-if="results.length" class="p-1">
            <div
              v-for="(entry, i) in results"
              :key="entry.id"
              class="flex items-start gap-3 px-4 py-3 cursor-pointer rounded-lg"
              :class="{ 'bg-[var(--muted)]': i === focusedIdx }"
              @mousedown.prevent="select(entry)"
            >
              <div class="flex flex-col gap-0.5 min-w-0 flex-1">
                <span class="text-sm text-[var(--foreground)] truncate">{{ entry.title }}</span>
              </div>
              <span v-if="projectStore.projects.length > 1" class="text-[0.65em] uppercase tracking-wide text-[var(--muted-foreground)] bg-[var(--muted)] rounded px-1.5 py-0.5 flex-shrink-0">
                {{ projectNameOf(entry.projectId) }}
              </span>
              <span v-if="matchedInBody(entry)" class="text-[0.65em] text-[var(--primary)] bg-[var(--secondary)] rounded px-1.5 py-0.5 flex-shrink-0">
                Matched in content
              </span>
            </div>
          </div>
          <div v-else-if="!query.trim()" class="px-4 py-6 text-center text-sm text-[var(--muted-foreground)]">
            Type to search entries…
          </div>
          <div v-else-if="searching" class="px-4 py-6 text-center text-sm text-[var(--muted-foreground)]">
            Searching…
          </div>
        </CommandList>
      </Command>
    </DialogContent>
  </Dialog>
</template>

<script setup lang="ts">
import { ref, watch, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import { Search } from 'lucide-vue-next'
import { Dialog, DialogContent } from './ui/dialog'
import { Command, CommandInput, CommandList } from './ui/command'
import { useEntriesStore } from '../stores/useEntriesStore'
import type { EntrySearchResult } from '../stores/useEntriesStore'
import { useProjectStore } from '../stores/useProjectStore'
import { useUIStore } from '../stores/useUIStore'

/** Debounce for the command-palette search input — matches DashboardView.vue. */
const SEARCH_DEBOUNCE_MS = 300

const entries = useEntriesStore()
const projectStore = useProjectStore()
const ui = useUIStore()
const router = useRouter()

const query = ref('')
const focusedIdx = ref(0)

// Real full-text search against content-svc (issue #51) — see
// useEntriesStore.searchRemote()'s doc comment for why this fans out across
// every project the user belongs to rather than hitting a single project.
const results = ref<EntrySearchResult[]>([])
const searching = ref(false)
const appliedQuery = ref('')
let debounceTimer: ReturnType<typeof setTimeout> | null = null
let searchSeq = 0

watch(() => ui.searchOverlayOpen, async (open) => {
  if (open) {
    query.value = ''
    results.value = []
    appliedQuery.value = ''
    searching.value = false
    focusedIdx.value = 0
    await nextTick()
    // Make sure the project list (needed to fan the search out, and to
    // resolve names for display) is loaded before the user types.
    await projectStore.fetchProjects()
  }
})

watch(query, (value) => {
  focusedIdx.value = 0
  if (debounceTimer) clearTimeout(debounceTimer)
  const trimmed = value.trim()
  if (!trimmed) {
    results.value = []
    searching.value = false
    return
  }
  searching.value = true
  debounceTimer = setTimeout(() => runSearch(trimmed), SEARCH_DEBOUNCE_MS)
})

async function runSearch(q: string) {
  const seq = ++searchSeq
  const projectIds = projectStore.projects.map(p => p.id)
  const hits = await entries.searchRemote(q, projectIds)
  if (seq !== searchSeq) return // superseded by a newer search
  results.value = hits.slice(0, 12)
  appliedQuery.value = q
  searching.value = false
}

function projectNameOf(projectId: string): string {
  return projectStore.projects.find(p => p.id === projectId)?.name ?? ''
}

/**
 * No snippet/match-offset comes back from the search endpoint (see
 * DashboardView.vue's matchedInBody() doc comment for the full reasoning),
 * so this infers a body match the same way: query text absent from both
 * title and tags must mean FTS matched it in the body.
 */
function matchedInBody(entry: EntrySearchResult): boolean {
  const q = appliedQuery.value.toLowerCase()
  if (!q) return false
  const inTitle = entry.title.toLowerCase().includes(q)
  const inTags = entry.tags?.some(t => t.toLowerCase().includes(q))
  return !inTitle && !inTags
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') { ui.closeSearchOverlay(); return }
  if (!results.value.length) return
  if (e.key === 'ArrowDown') { e.preventDefault(); focusedIdx.value = (focusedIdx.value + 1) % results.value.length }
  else if (e.key === 'ArrowUp') { e.preventDefault(); focusedIdx.value = (focusedIdx.value - 1 + results.value.length) % results.value.length }
  else if (e.key === 'Enter') { e.preventDefault(); const entry = results.value[focusedIdx.value]; if (entry) select(entry) }
}

function select(entry: EntrySearchResult) {
  router.push('/entries/' + entry.id)
  ui.closeSearchOverlay()
}
</script>
