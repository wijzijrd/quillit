<template>
  <div class="entries-browser">
    <div class="tree-col" v-if="!isMobile || !hasActiveEntry">
      <div class="browser-header">
        <span class="browser-title">{{ inProject ? project?.name ?? 'Entries' : 'Entries' }}</span>
        <button class="new-btn" @click="createNew">+ New</button>
      </div>

      <p v-if="moveError" class="move-error">
        {{ moveError }}
        <button class="move-error-dismiss" @click="moveError = null">×</button>
      </p>

      <div class="browser-scroll">
        <div class="browser-empty" v-if="entries.entries.length === 0">
          No entries yet.
        </div>

        <DirectoryNode v-else :node="tree" :depth="0" />
      </div>
    </div>

    <div class="pane-col" v-if="!isMobile || hasActiveEntry">
      <EntryPane v-model:mode="paneMode" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, provide, watch, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import DirectoryNode from '../components/DirectoryNode.vue'
import EntryPane from '../components/EntryPane.vue'
import { useEntriesStore } from '../stores/useEntriesStore'
import { useProjectStore } from '../stores/useProjectStore'
import { useUIStore } from '../stores/useUIStore'
import { apiErrorMessage } from '../api/client'
import { buildDirectoryTree, directoryTreeActionsKey, type DirectoryTreeActions } from '../lib/directoryTree'
import type { ContentEntry } from '../stores/useEntryStore'
import { useBreakpoint } from '../composables/useBreakpoint'
import { entryPath } from '../utils/links'

const route = useRoute()
const router = useRouter()
const entries = useEntriesStore()
const projectStore = useProjectStore()
const ui = useUIStore()
const { isMobile } = useBreakpoint()

const projectId = computed(() => {
  const p = route.params.projectId
  return typeof p === 'string' ? p : null
})
const inProject = computed(() => !!projectId.value)
const project = computed(() => projectStore.projects.find(p => p.id === projectId.value) ?? null)

// Directory-tree UI-local state (Task 6). Declared here, ahead of the
// projectId watcher below, so that watcher can reset all three on every
// project switch — see the reset lines in that watcher for why: without it,
// a pending folder, a collapsed path, or a stale move error from the
// previous project would otherwise leak into the newly loaded project's
// tree.
const pendingFolders = ref<Set<string>>(new Set())
const collapsedPaths = ref<Set<string>>(new Set())
const moveError = ref<string | null>(null)

onMounted(async () => {
  const id = projectId.value
  if (id) {
    await Promise.all([
      entries.init(id),
      projectStore.fetchProjects(),
    ])
  }
})

// Vue Router reuses this component instance across /projects/:projectId/entries
// navigations (same matched component, different param) — onMounted alone
// only fires once, so without this watcher switching projects in-app would
// leave the previous project's entries on screen. useEntriesStore.init's
// guard is keyed by project id (not a plain boolean), so calling it again
// here for the new id re-fetches correctly without needing to poke any
// store-internal state first. Also clears the list when navigating out of
// a project entirely (content-svc has no cross-project list, so there's
// nothing to show there either).
//
// Not awaited — this is a route-driven refetch, not something to block
// rendering on — so failures are caught explicitly rather than left as an
// unhandled rejection. Rapid A→B→C switching firing overlapping requests is
// safe: useEntriesStore.init has its own per-call token guard (see
// useEntriesStore.ts) that discards a response once a newer call has
// superseded it, so an out-of-order resolution can't clobber the currently
// displayed project's entries.
watch(projectId, (id) => {
  pendingFolders.value = new Set()
  collapsedPaths.value = new Set()
  moveError.value = null
  if (!id) {
    entries.clear()
    return
  }
  entries.init(id).catch(() => {})
})

const hasActiveEntry = computed(() => typeof route.params.id === 'string' && !!route.params.id)
const paneMode = ref<'view' | 'edit'>('view')

// Single writer of the active entry — every navigation into/out of an entry
// (tree click, wikilink, browser back/forward, deep link) flows through
// /entries/:id, so this is the only place ui.activeEntryId gets set.
watch(() => route.params.id, (id) => {
  ui.setActiveEntry(typeof id === 'string' ? id : null)
  paneMode.value = 'view'
}, { immediate: true })

const tree = computed(() => buildDirectoryTree(entries.entries, [...pendingFolders.value]))

function isExpanded(path: string) {
  return !collapsedPaths.value.has(path)
}
function onToggle(path: string) {
  const next = new Set(collapsedPaths.value)
  if (next.has(path)) next.delete(path)
  else next.add(path)
  collapsedPaths.value = next
}

async function handleMove(entryId: string, destPath: string) {
  const entry = entries.getById(entryId)
  if (!entry || entry.directoryPath === destPath) return
  try {
    await entries.assignEntry(entryId, destPath)
    moveError.value = null
  } catch (e) {
    moveError.value = apiErrorMessage(e, 'Could not move entry')
  }
}

function handleCreateFolder(parentPath: string, name: string) {
  const path = parentPath ? `${parentPath}/${name}` : name
  pendingFolders.value = new Set([...pendingFolders.value, path])
}

async function openView(entry: ContentEntry) {
  await router.push(entryPath(projectId.value, entry.id))
  paneMode.value = 'view'
}

async function openEdit(entry: ContentEntry) {
  await router.push(entryPath(projectId.value, entry.id))
  // The route.params.id watcher above resets paneMode to 'view' on every
  // navigation (Vue's pre-flush queue) — nextTick ensures that reset has
  // already run before this explicit 'edit' wins.
  await nextTick()
  paneMode.value = 'edit'
}

async function deleteEntry(entry: ContentEntry) {
  if (!confirm(`Delete "${entry.title}"?`)) return
  const wasActive = ui.activeEntryId === entry.id
  await entries.deleteEntry(entry.id)
  if (wasActive) await router.push(projectId.value ? `/projects/${projectId.value}/entries` : '/entries')
}

provide<DirectoryTreeActions>(directoryTreeActionsKey, {
  isExpanded,
  onToggle,
  onMove: handleMove,
  onCreateFolder: handleCreateFolder,
  onView: openView,
  onEdit: openEdit,
  onLinks: openView,
  onDelete: deleteEntry,
})

async function createNew() {
  const id = projectId.value
  if (!id) return
  const entry = await entries.createEntry(id, 'Untitled')
  await router.push(entryPath(id, entry.id))
  await nextTick()
  paneMode.value = 'edit'
}
</script>

<style scoped>
.entries-browser { display: grid; grid-template-columns: clamp(260px, 22vw, 340px) 1fr; height: 100vh; }
@media (max-width: 767px) {
  .entries-browser { grid-template-columns: 1fr; }
}

.tree-col { display: flex; flex-direction: column; height: 100vh; padding: 0 20px; border-right: 1px solid var(--border); min-width: 0; }
@media (max-width: 767px) { .tree-col { border-right: none; } }

.pane-col { height: 100vh; overflow: hidden; min-width: 0; }

.browser-header { display: flex; align-items: center; justify-content: space-between; padding: 28px 0 20px; flex-shrink: 0; }
.browser-title { font-family: var(--font-display); font-size: 1.1em; letter-spacing: 0.06em; color: var(--muted-foreground); }
.new-btn { background: var(--secondary); border: none; color: var(--primary); font-size: 0.82em; padding: 5px 12px; border-radius: var(--radius); cursor: pointer; transition: background var(--transition); }
.new-btn:hover { background: var(--primary); color: var(--background); }
.browser-scroll { flex: 1; overflow-y: auto; }
.browser-empty { color: var(--muted-foreground); font-size: 0.9em; padding: 32px 0; }
.move-error {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  background: rgba(220, 38, 38, 0.1);
  color: var(--destructive);
  font-size: 0.85em;
  padding: 8px 12px;
  border-radius: var(--radius);
  margin-bottom: 8px;
  flex-shrink: 0;
}
.move-error-dismiss {
  background: none;
  border: none;
  color: inherit;
  cursor: pointer;
  font-size: 1.1em;
  line-height: 1;
  padding: 0 4px;
}
</style>
