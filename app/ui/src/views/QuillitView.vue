<template>
  <div class="entries-browser">
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

  <EntryEditModal
    v-if="editingEntry"
    :entry-id="editingEntry.id"
    @close="editingEntry = null"
  />
  <EntryViewModal
    v-if="viewingEntry"
    :entry-id="viewingEntry.id"
    :can-go-back="viewHistory.length > 0"
    :can-go-forward="viewFuture.length > 0"
    @close="closeViewModal"
    @edit="switchToEdit"
    @navigate="navigateTo"
    @back="viewGoBack"
    @forward="viewGoForward"
  />
</template>

<script setup lang="ts">
import { ref, computed, onMounted, provide, watch } from 'vue'
import { useRoute } from 'vue-router'
import DirectoryNode from '../components/DirectoryNode.vue'
import EntryEditModal from '../components/EntryEditModal.vue'
import EntryViewModal from '../components/EntryViewModal.vue'
import { useEntriesStore } from '../stores/useEntriesStore'
import { useProjectStore } from '../stores/useProjectStore'
import { apiErrorMessage } from '../api/client'
import { buildDirectoryTree, directoryTreeActionsKey, type DirectoryTreeActions } from '../lib/directoryTree'
import type { ContentEntry } from '../stores/useEntryStore'

const route = useRoute()
const entries = useEntriesStore()
const projectStore = useProjectStore()

const projectId = computed(() => {
  const p = route.params.projectId
  return typeof p === 'string' ? p : null
})
const inProject = computed(() => !!projectId.value)
const project = computed(() => projectStore.projects.find(p => p.id === projectId.value) ?? null)

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
  if (!id) {
    entries.clear()
    return
  }
  entries.init(id).catch(() => {})
})

type EditingEntry = ContentEntry | { id: string }

const editingEntry = ref<EditingEntry | null>(null)
const viewingEntry = ref<ContentEntry | null>(null)
const viewHistory = ref<ContentEntry[]>([])
const viewFuture = ref<ContentEntry[]>([])

// Escape hatch for /entries/:id — open the editor for the routed entry id
// directly, independent of the (list-backed) EntryRow click path above, so
// deep-linking works even when the list itself hasn't loaded that entry.
// EntryEditModal only needs `.id` off this object (it forwards it as the
// `entry-id` prop and calls ui.setActiveEntry); EntryEditor fetches the
// full entry itself via useEntryStore.get.
watch(() => route.params.id, (id) => {
  if (typeof id === 'string' && id) editingEntry.value = { id }
}, { immediate: true })

const pendingFolders = ref<Set<string>>(new Set())
const tree = computed(() => buildDirectoryTree(entries.entries, [...pendingFolders.value]))

const collapsedPaths = ref<Set<string>>(new Set())
function isExpanded(path: string) {
  return !collapsedPaths.value.has(path)
}
function onToggle(path: string) {
  const next = new Set(collapsedPaths.value)
  if (next.has(path)) next.delete(path)
  else next.add(path)
  collapsedPaths.value = next
}

const moveError = ref<string | null>(null)
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
  editingEntry.value = entry
}

function openEdit(entry: ContentEntry) {
  viewingEntry.value = null
  editingEntry.value = entry
}

function openView(entry: ContentEntry) {
  editingEntry.value = null
  viewingEntry.value = entry
}

async function deleteEntry(entry: ContentEntry) {
  if (!confirm(`Delete "${entry.title}"?`)) return
  await entries.deleteEntry(entry.id)
  if (editingEntry.value?.id === entry.id) editingEntry.value = null
  if (viewingEntry.value?.id === entry.id) viewingEntry.value = null
}

function switchToEdit() {
  if (!viewingEntry.value) return
  const e = viewingEntry.value
  closeViewModal()
  editingEntry.value = e
}

function navigateTo(id: string) {
  const next = entries.getById(id)
  if (!next || !viewingEntry.value) return
  viewHistory.value.push(viewingEntry.value)
  viewFuture.value = []
  viewingEntry.value = next
}

function viewGoBack() {
  if (!viewHistory.value.length || !viewingEntry.value) return
  viewFuture.value.unshift(viewingEntry.value)
  viewingEntry.value = viewHistory.value.pop() ?? null
}

function viewGoForward() {
  if (!viewFuture.value.length || !viewingEntry.value) return
  viewHistory.value.push(viewingEntry.value)
  viewingEntry.value = viewFuture.value.shift() ?? null
}

function closeViewModal() {
  viewingEntry.value = null
  viewHistory.value = []
  viewFuture.value = []
}
</script>

<style scoped>
.entries-browser { display: flex; flex-direction: column; height: 100vh; padding: 0 24px; }
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
