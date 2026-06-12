<template>
  <div class="notes-browser">
    <div class="browser-header">
      <span class="browser-title">{{ inProject ? project?.name ?? 'Notes' : 'Notes' }}</span>
      <button class="new-btn" @click="createNew">+ New</button>
    </div>

    <div class="browser-scroll">
      <div class="browser-empty" v-if="inProject ? groupedEntries.length === 0 : visibleEntries.length === 0">
        No entries yet.
      </div>

      <!-- Flat list: no project context -->
      <template v-if="!inProject">
        <EntryRow
          v-for="entry in visibleEntries"
          :key="entry.id"
          :entry="entry"
          @view="openView(entry)"
          @edit="openEdit(entry)"
          @links="openView(entry)"
          @delete="deleteEntry(entry)"
        />
      </template>

      <!-- Grouped by category: inside a project -->
      <template v-else>
        <div
          class="category-group"
          v-for="group in groupedEntries"
          :key="group.category"
        >
          <button class="cat-group-header" @click="toggle(group.category)">
            <component
              :is="catIcon(group.category)"
              :size="14"
              :style="{ color: catColor(group.category) }"
            />
            <span class="cat-group-name" :style="{ color: catColor(group.category) }">
              {{ group.category }}
            </span>
            <span class="cat-group-count">{{ group.entries.length }}</span>
            <ChevronDown
              :size="13"
              class="chevron"
              :class="{ rotated: collapsed[group.category] }"
            />
          </button>
          <div class="cat-group-entries" v-show="!collapsed[group.category]">
            <EntryRow
              v-for="entry in group.entries"
              :key="entry.id"
              :entry="entry"
              @view="openView(entry)"
              @edit="openEdit(entry)"
              @links="openView(entry)"
              @delete="deleteEntry(entry)"
            />
          </div>
        </div>
      </template>
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
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ChevronDown } from 'lucide-vue-next'
import EntryRow from '../components/EntryRow.vue'
import EntryEditModal from '../components/EntryEditModal.vue'
import EntryViewModal from '../components/EntryViewModal.vue'
import { useEntriesStore } from '../stores/useEntriesStore'
import { useCategoriesStore } from '../stores/useCategoriesStore'
import { useProjectStore } from '../stores/useProjectStore'
import { resolveIcon } from '../utils/categoryIcons'

const route = useRoute()
const entries = useEntriesStore()
const cats = useCategoriesStore()
const projectStore = useProjectStore()

const projectId = computed(() => route.params.projectId ?? null)
const inProject = computed(() => !!projectId.value)
const project = computed(() => projectStore.projects.find(p => p.id === projectId.value) ?? null)

onMounted(async () => {
  await entries.init()
  if (inProject.value) {
    await Promise.all([
      cats.init(),
      cats.initForProject(projectId.value),
      projectStore.fetchProjects(),
    ])
  }
})

watch(projectId, (newId) => {
  if (newId) cats.initForProject(newId)
  else cats.projectCategories.value = []
})

const collapsed = ref({})
const editingEntry = ref(null)
const viewingEntry = ref(null)
const viewHistory = ref([])
const viewFuture = ref([])

// Flat sorted list for global notes view
const visibleEntries = computed(() =>
  [...entries.entries].sort((a, b) => a.title.localeCompare(b.title))
)

// Grouped list for project view
const groupedEntries = computed(() => {
  if (!inProject.value) return []
  const order = cats.projectCategories.map(c => c.name)
  const map = {}
  for (const e of [...entries.entries].sort((a, b) => a.title.localeCompare(b.title))) {
    if (!map[e.category]) map[e.category] = []
    map[e.category].push(e)
  }
  const known = order.filter(name => map[name]?.length).map(name => ({ category: name, entries: map[name] }))
  const extra = Object.keys(map).filter(name => !order.includes(name)).map(name => ({ category: name, entries: map[name] }))
  return [...known, ...extra]
})

function toggle(category) {
  collapsed.value[category] = !collapsed.value[category]
}

function catIcon(categoryName) {
  const c = cats.projectCategoryFor(categoryName)
  return c ? resolveIcon(c.icon) : resolveIcon('')
}

function catColor(categoryName) {
  return cats.projectCategoryFor(categoryName)?.color ?? 'var(--muted-foreground)'
}

async function createNew() {
  if (inProject.value) {
    await cats.initForProject(projectId.value)
    const category = cats.projectCategories[0]?.name ?? 'Characters'
    const entry = await entries.createEntry(category)
    editingEntry.value = entry
  } else {
    const entry = await entries.createEntry('Characters')
    editingEntry.value = entry
  }
}

function openEdit(entry) {
  viewingEntry.value = null
  editingEntry.value = entry
}

function openView(entry) {
  editingEntry.value = null
  viewingEntry.value = entry
}

async function deleteEntry(entry) {
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
  if (!viewHistory.value.length) return
  viewFuture.value.unshift(viewingEntry.value)
  viewingEntry.value = viewHistory.value.pop()
}

function viewGoForward() {
  if (!viewFuture.value.length) return
  viewHistory.value.push(viewingEntry.value)
  viewingEntry.value = viewFuture.value.shift()
}

function closeViewModal() {
  viewingEntry.value = null
  viewHistory.value = []
  viewFuture.value = []
}
</script>

<style scoped>
.notes-browser { display: flex; flex-direction: column; height: 100vh; padding: 0 24px; }
.browser-header { display: flex; align-items: center; justify-content: space-between; padding: 28px 0 20px; flex-shrink: 0; }
.browser-title { font-family: var(--font-display); font-size: 1.1em; letter-spacing: 0.06em; color: var(--muted-foreground); }
.new-btn { background: var(--secondary); border: none; color: var(--primary); font-size: 0.82em; padding: 5px 12px; border-radius: var(--radius); cursor: pointer; transition: background var(--transition); }
.new-btn:hover { background: var(--primary); color: var(--background); }
.browser-scroll { flex: 1; overflow-y: auto; }
.browser-empty { color: var(--muted-foreground); font-size: 0.9em; padding: 32px 0; }
.category-group { margin-bottom: 4px; }
.cat-group-header { display: flex; align-items: center; gap: 8px; width: 100%; padding: 8px 12px; background: none; border: none; cursor: pointer; border-radius: var(--radius); font-family: var(--font-body); font-size: 0.82em; font-weight: 600; letter-spacing: 0.04em; text-transform: uppercase; transition: background var(--transition); }
.cat-group-header:hover { background: var(--muted); }
.cat-group-name { flex: 1; text-align: left; }
.cat-group-count { font-size: 0.75em; font-weight: normal; color: var(--muted-foreground); background: var(--muted); border-radius: 10px; padding: 1px 7px; }
.chevron { color: var(--muted-foreground); transition: transform 0.15s ease; flex-shrink: 0; }
.chevron.rotated { transform: rotate(-90deg); }
.cat-group-entries { padding-bottom: 8px; }
</style>
