<template>
  <aside class="links-panel">

    <div class="links-section">
      <p class="section-heading">Relations</p>
      <p class="section-error" v-if="linkError">{{ linkError }}</p>

      <!-- Outgoing: from this entry -->
      <template v-if="outgoingGroups.length > 0">
        <div class="dir-label">From this entry</div>
        <template v-for="group in outgoingGroups" :key="'o-' + group.label">
          <p class="rel-group-label">{{ group.label }}</p>
          <div class="link-item" v-for="rel in group.items" :key="rel.id">
            <span class="link-cat" :style="{ color: catColor(rel.relatedEntry.category) }">
              <component :is="catIcon(rel.relatedEntry.category)" :size="12" />
            </span>
            <button class="link-title" @click="navigate(rel.relatedEntry.id)">
              {{ rel.relatedEntry.title }}
            </button>
            <button class="link-remove" @click="removeRelation(rel.id)" title="Remove">✕</button>
          </div>
        </template>
      </template>

      <!-- Incoming: other entries referencing this one -->
      <template v-if="incomingGroups.length > 0">
        <div class="dir-label dir-label--in">Referenced by</div>
        <template v-for="group in incomingGroups" :key="'i-' + group.label">
          <p class="rel-group-label">{{ group.label }}</p>
          <div class="link-item link-item--back" v-for="rel in group.items" :key="rel.id">
            <span class="link-cat" :style="{ color: catColor(rel.relatedEntry.category) }">
              <component :is="catIcon(rel.relatedEntry.category)" :size="12" />
            </span>
            <button class="link-title" @click="navigate(rel.relatedEntry.id)">
              {{ rel.relatedEntry.title }}
            </button>
          </div>
        </template>
      </template>

      <p class="section-empty"
         v-if="outgoingGroups.length === 0 && incomingGroups.length === 0 && !adding">
        No relations yet.
      </p>

      <!-- Add relation form -->
      <div class="add-rel" v-if="adding">
        <input
          class="search-input"
          v-model="newLabel"
          list="rel-label-list"
          placeholder="Label (e.g. lives in, owns…)"
          autocomplete="off"
        />
        <datalist id="rel-label-list">
          <option v-for="l in relStore.labels" :key="l" :value="l" />
        </datalist>
        <div class="search-wrap">
          <input
            class="search-input"
            v-model="searchQuery"
            placeholder="Search entry to link…"
            @focus="showDropdown = true"
            @blur="hideDropdown"
            @keydown.escape="showDropdown = false"
          />
          <ul class="search-dropdown" v-if="showDropdown && searchResults.length > 0">
            <li
              v-for="result in searchResults"
              :key="result.id"
              class="search-result"
              @mousedown.prevent="confirmAdd(result.id)"
            >
              <span class="result-cat" :style="{ color: catColor(result.category) }">
                <component :is="catIcon(result.category)" :size="12" />
              </span>
              {{ result.title }}
            </li>
          </ul>
        </div>
        <button class="btn-ghost btn-sm" @click="adding = false; resetAdd()">Cancel</button>
      </div>

      <button class="btn-ghost btn-sm add-btn" v-else @click="startAdding">
        + Add relation
      </button>
    </div>

    <!-- Legacy simple links -->
    <div class="links-section" v-if="outboundLinks.length > 0 || backlinks.length > 0">
      <p class="section-heading">
        Simple Links <span class="legacy-badge">legacy</span>
      </p>
      <div class="link-item" v-for="linked in outboundLinks" :key="linked.id">
        <span class="link-cat" :style="{ color: catColor(linked.category) }">
          <component :is="catIcon(linked.category)" :size="12" />
        </span>
        <button class="link-title" @click="navigate(linked.id)">{{ linked.title }}</button>
        <button class="link-remove" @click="entries.removeLink(props.entryId, linked.id)">✕</button>
      </div>
      <div class="link-item link-item--back" v-for="linked in backlinks" :key="linked.id">
        <span class="link-cat" :style="{ color: catColor(linked.category) }">
          <component :is="catIcon(linked.category)" :size="12" />
        </span>
        <button class="link-title" @click="navigate(linked.id)">{{ linked.title }}</button>
      </div>
    </div>

  </aside>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useEntriesStore } from '../stores/useEntriesStore'
import { useUIStore } from '../stores/useUIStore'
import { useCategoriesStore } from '../stores/useCategoriesStore'
import { useEntryRelationsStore } from '../stores/useEntryRelationsStore'
import { resolveIcon } from '../utils/categoryIcons'

const props = defineProps<{ entryId?: string }>()

const entries = useEntriesStore()
const ui = useUIStore()
const cats = useCategoriesStore()
const relStore = useEntryRelationsStore()
const router = useRouter()

const adding = ref(false)
const newLabel = ref('')
const searchQuery = ref('')
const showDropdown = ref(false)
const linkError = ref('')

onMounted(async () => {
  try {
    await relStore.fetchForEntry(props.entryId)
    await relStore.fetchLabels()
  } catch (e: any) {
    linkError.value = e?.data?.error ?? 'Could not load relations'
  }
})

function groupByLabel(rels: any[]) {
  const map: Record<string, { label: string; items: any[] }> = {}
  for (const rel of rels) {
    if (!map[rel.label]) map[rel.label] = { label: rel.label, items: [] }
    map[rel.label].items.push(rel)
  }
  return Object.values(map).sort((a, b) => a.label.localeCompare(b.label))
}

const outgoingGroups = computed(() =>
  groupByLabel(relStore.getForEntry(props.entryId).filter((r: any) => r.direction === 'outgoing'))
)

const incomingGroups = computed(() =>
  groupByLabel(relStore.getForEntry(props.entryId).filter((r: any) => r.direction === 'incoming'))
)

const outboundLinks = computed(() => {
  const entry = entries.getById(props.entryId)
  return (entry?.linkedEntries ?? []).map((id: string) => entries.getById(id)).filter(Boolean)
})
const backlinks = computed(() => entries.backlinksFor(props.entryId))

const searchResults = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return []
  const existingIds = new Set(relStore.getForEntry(props.entryId).map((r: any) => r.relatedEntry?.id))
  return entries.entries
    .filter((e: any) => e.id !== props.entryId && !existingIds.has(e.id) && e.title.toLowerCase().includes(q))
    .slice(0, 8)
})

function catColor(category: string) {
  return cats.categoryFor(category)?.color ?? 'var(--muted-foreground)'
}
function catIcon(category: string) {
  const cat = cats.categoryFor(category)
  return resolveIcon(cat?.icon ?? '')
}

function startAdding() {
  adding.value = true
  newLabel.value = ''
  searchQuery.value = ''
}

async function confirmAdd(toId: string) {
  const label = newLabel.value.trim()
  if (!label) return
  try {
    await relStore.create(props.entryId, toId, label)
    resetAdd()
    adding.value = false
  } catch (e: any) {
    linkError.value = e?.data?.error ?? 'Could not add relation'
  }
}

async function removeRelation(relationId: string) {
  try {
    await relStore.remove(props.entryId, relationId)
  } catch (e: any) {
    linkError.value = e?.data?.error ?? 'Could not remove relation'
  }
}

function resetAdd() {
  newLabel.value = ''
  searchQuery.value = ''
}

function hideDropdown() {
  setTimeout(() => { showDropdown.value = false }, 150)
}

function navigate(id: string) {
  ui.setActiveEntry(id)
  router.push(`/notes/${id}`)
}
</script>

<style scoped>
.links-panel {
  width: 260px; min-width: 260px; border-left: 1px solid var(--border);
  display: flex; flex-direction: column; overflow-y: auto; background: var(--card);
}
.links-section {
  padding: 12px 14px; border-bottom: 1px solid var(--border);
  display: flex; flex-direction: column; gap: 4px;
}
.section-heading {
  font-size: 0.68em; letter-spacing: 0.12em; text-transform: uppercase;
  color: var(--muted-foreground); margin-bottom: 4px; display: flex; align-items: center; gap: 6px;
}
.legacy-badge {
  font-size: 0.75em; background: var(--muted); border: 1px solid var(--border);
  border-radius: 8px; padding: 1px 5px; text-transform: none; letter-spacing: 0; color: var(--muted-foreground);
}
.dir-label {
  font-size: 0.65em; text-transform: uppercase; letter-spacing: 0.1em;
  color: var(--muted-foreground); padding: 8px 0 2px; font-weight: 600;
}
.dir-label--in { color: var(--primary); opacity: 0.7; }
.rel-group-label {
  font-size: 0.72em; font-weight: 600; letter-spacing: 0.04em;
  color: var(--muted-foreground); margin: 4px 0 2px; font-style: italic;
}
.section-empty { font-size: 0.82em; color: var(--muted-foreground); padding: 2px 0; }
.section-error { font-size: 0.82em; color: var(--destructive); padding: 2px 0; }
.link-item { display: flex; align-items: center; gap: 6px; padding: 3px 0; }
.link-cat { font-size: 0.9em; flex-shrink: 0; width: 18px; }
.link-title {
  flex: 1; background: none; border: none; color: var(--foreground);
  font-family: var(--font-body); font-size: 0.88em; text-align: left;
  cursor: pointer; padding: 0; white-space: nowrap; overflow: hidden;
  text-overflow: ellipsis; transition: color var(--transition);
}
.link-title:hover { color: var(--primary); }
.link-remove {
  background: none; border: none; color: var(--muted-foreground); font-size: 0.75em;
  cursor: pointer; padding: 2px 4px; border-radius: var(--radius); flex-shrink: 0;
  opacity: 0; transition: opacity var(--transition), color var(--transition);
}
.link-item:hover .link-remove { opacity: 1; }
.link-remove:hover { color: var(--destructive); }
.link-item--back .link-title { color: var(--muted-foreground); font-style: italic; }
.add-rel { display: flex; flex-direction: column; gap: 6px; margin-top: 6px; }
.search-wrap { position: relative; }
.add-btn { margin-top: 6px; width: 100%; }
.search-input {
  width: 100%; background: var(--muted); border: 1px solid var(--border);
  border-radius: var(--radius); color: var(--foreground); font-family: var(--font-body);
  font-size: 0.84em; padding: 6px 10px; outline: none;
  transition: border-color var(--transition); box-sizing: border-box;
}
.search-input:focus { border-color: var(--secondary); }
.search-input::placeholder { color: var(--muted-foreground); }
.search-dropdown {
  position: absolute; top: calc(100% + 2px); left: 0; right: 0;
  background: var(--muted); border: 1px solid var(--border);
  border-radius: var(--radius); list-style: none; z-index: 20; overflow: hidden;
  box-shadow: 0 4px 16px rgba(0,0,0,0.12);
}
.search-result {
  display: flex; align-items: center; gap: 8px; padding: 7px 10px;
  cursor: pointer; font-size: 0.86em; color: var(--foreground);
  transition: background var(--transition);
}
.search-result:hover { background: var(--muted); }
.result-cat { font-size: 0.9em; flex-shrink: 0; width: 18px; }
.btn-ghost {
  background: var(--muted); color: var(--muted-foreground); border: 1px solid var(--border);
  border-radius: var(--radius); padding: 8px 14px; cursor: pointer;
  font-size: 0.88em; transition: background var(--transition);
}
.btn-ghost:hover { background: var(--muted); color: var(--foreground); }
.btn-sm { padding: 4px 10px !important; font-size: 0.8em !important; }
</style>
