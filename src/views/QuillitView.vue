<template>
  <div class="notes-browser">
    <div class="browser-header">
      <span class="browser-title">Notes</span>
      <button class="new-btn" @click="createNew">+ New</button>
    </div>

    <div class="browser-scroll">
      <div class="browser-empty" v-if="groupedEntries.length === 0">
        No entries yet.
      </div>

      <div
        class="category-group"
        v-for="group in groupedEntries"
        :key="group.category"
      >
        <button
          class="cat-group-header"
          @click="toggle(group.category)"
        >
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
          />
        </div>
      </div>
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
    @close="viewingEntry = null"
    @edit="switchToEdit"
    @navigate="navigateTo"
  />
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { ChevronDown } from 'lucide-vue-next'
import EntryRow from '../components/EntryRow.vue'
import EntryEditModal from '../components/EntryEditModal.vue'
import EntryViewModal from '../components/EntryViewModal.vue'
import { useEntriesStore } from '../stores/useEntriesStore.js'
import { useCategoriesStore } from '../stores/useCategoriesStore.js'
import { resolveIcon } from '../utils/categoryIcons.js'

const entries = useEntriesStore()
const cats = useCategoriesStore()

onMounted(async () => {
  await entries.init()
  await cats.init()
})

const collapsed = ref({})
const editingEntry = ref(null)
const viewingEntry = ref(null)

const groupedEntries = computed(() => {
  const order = cats.categories.map(c => c.name)
  const map = {}
  for (const e of [...entries.entries].sort((a, b) => a.title.localeCompare(b.title))) {
    if (!map[e.category]) map[e.category] = []
    map[e.category].push(e)
  }
  // categories with entries, in admin-defined order; uncategorised ones appended
  const known = order.filter(name => map[name]?.length).map(name => ({ category: name, entries: map[name] }))
  const extra = Object.keys(map).filter(name => !order.includes(name)).map(name => ({ category: name, entries: map[name] }))
  return [...known, ...extra]
})

function catColor(name) {
  return cats.categoryFor(name)?.color ?? 'var(--text-muted)'
}
function catIcon(name) {
  const cat = cats.categoryFor(name)
  return resolveIcon(cat?.icon ?? '')
}

function toggle(cat) {
  collapsed.value[cat] = !collapsed.value[cat]
}

function openEdit(entry) {
  viewingEntry.value = null
  editingEntry.value = entry
}
function openView(entry) {
  editingEntry.value = null
  viewingEntry.value = entry
}
function switchToEdit() {
  if (!viewingEntry.value) return
  const e = viewingEntry.value
  viewingEntry.value = null
  editingEntry.value = e
}
function navigateTo(id) {
  const e = entries.getById(id)
  if (e) viewingEntry.value = e
}

async function createNew() {
  await cats.init()
  const category = cats.categories[0]?.name ?? 'Characters'
  const entry = await entries.createEntry(category)
  editingEntry.value = entry
}
</script>

<style scoped>
.notes-browser {
  display: flex;
  flex-direction: column;
  height: 100vh;
  max-width: 800px;
  margin: 0 auto;
  padding: 0 24px;
}
.browser-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 28px 0 20px;
  flex-shrink: 0;
}
.browser-title {
  font-family: var(--font-display);
  font-size: 1.1em;
  letter-spacing: 0.06em;
  color: var(--text-muted);
}
.new-btn {
  background: var(--accent-dim);
  border: none;
  color: var(--accent);
  font-size: 0.82em;
  padding: 5px 12px;
  border-radius: var(--radius);
  cursor: pointer;
  transition: background var(--transition);
}
.new-btn:hover { background: var(--accent); color: var(--bg-deep); }

.browser-scroll { flex: 1; overflow-y: auto; }
.browser-empty { color: var(--text-faint); font-size: 0.9em; padding: 32px 0; }

.category-group { margin-bottom: 4px; }
.cat-group-header {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 8px 12px;
  background: none;
  border: none;
  cursor: pointer;
  border-radius: var(--radius);
  font-family: var(--font-body);
  font-size: 0.82em;
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  transition: background var(--transition);
}
.cat-group-header:hover { background: var(--bg-hover); }
.cat-group-name { flex: 1; text-align: left; }
.cat-group-count {
  font-size: 0.75em;
  font-weight: normal;
  color: var(--text-faint);
  background: var(--bg-raised);
  border-radius: 10px;
  padding: 1px 7px;
}
.chevron {
  color: var(--text-faint);
  transition: transform 0.15s ease;
  flex-shrink: 0;
}
.chevron.rotated { transform: rotate(-90deg); }

.cat-group-entries { padding-bottom: 8px; }
</style>
