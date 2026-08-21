<template>
  <div class="quillit-view">
    <div class="entry-list">
      <div class="list-header">
        <span class="list-title">
          # {{ route.params.tag }}
          <em>({{ tagEntries.length }})</em>
        </span>
      </div>
      <div class="list-scroll">
        <EntryCard
          v-for="entry in tagEntries"
          :key="entry.id"
          :entry="entry"
          @select="ui.setActiveEntry(entry.id)"
        />
        <p v-if="tagEntries.length === 0" class="list-empty">No entries with this tag.</p>
      </div>
    </div>
    <div class="editor-panel">
      <EntryEditor />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import EntryCard from '../components/EntryCard.vue'
import EntryEditor from '../components/EntryEditor.vue'
import { useProjectStore } from '../stores/useProjectStore'
import { useUIStore } from '../stores/useUIStore'
import { api } from '../api/client'
import type { ContentEntry } from '../stores/useEntryStore'

const projectStore = useProjectStore()
const ui = useUIStore()
const route = useRoute()

// /tag/:tag has no :projectId — it's always been a cross-project tag
// browser — so, matching DashboardView.vue's loadRecents()/recentEntries,
// this fans out one direct api() call per project the user belongs to
// rather than routing through useEntriesStore (single-active-project cache).
const allEntries = ref<ContentEntry[]>([])

async function loadAllEntries(projectIds: string[]) {
  const settled = await Promise.allSettled(
    projectIds.map(id => api(`/content/projects/${id}/entries`))
  )
  allEntries.value = settled.flatMap(r => (r.status === 'fulfilled' ? r.value : []))
}

onMounted(async () => {
  await projectStore.fetchProjects()
  await loadAllEntries(projectStore.projects.map(p => p.id))
})

const tagEntries = computed(() => {
  const tag = String(route.params.tag)
  return allEntries.value
    .filter(e => e.tags?.includes(tag))
    .slice()
    .sort((a, b) => b.updatedAt - a.updatedAt)
})
</script>

<style scoped>
.quillit-view {
  display: grid;
  grid-template-columns: 280px 1fr;
  height: 100vh;
}
.entry-list {
  background: var(--card);
  border-right: 1px solid var(--border);
  display: flex;
  flex-direction: column;
}
.list-header {
  padding: 16px;
  border-bottom: 1px solid var(--border);
}
.list-title {
  font-family: var(--font-display);
  font-size: 0.82em;
  letter-spacing: 0.06em;
  color: var(--muted-foreground);
}
.list-title em { color: var(--muted-foreground); font-style: normal; }
.list-scroll { overflow-y: auto; flex: 1; }
.list-empty { padding: 24px 16px; color: var(--muted-foreground); font-size: 0.88em; }
.editor-panel { overflow-y: auto; }
</style>
