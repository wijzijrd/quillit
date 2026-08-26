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
          @select="router.push(`/entries/${entry.id}`)"
        />
        <p v-if="tagEntries.length === 0" class="list-empty">No entries with this tag.</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import EntryCard from '../components/EntryCard.vue'
import { useProjectStore } from '../stores/useProjectStore'
import { api } from '../api/client'
import type { ContentEntry } from '../stores/useEntryStore'

const projectStore = useProjectStore()
const route = useRoute()
const router = useRouter()

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
  height: 100vh;
  max-width: 640px;
  margin: 0 auto;
  display: flex;
  flex-direction: column;
}
.entry-list {
  display: flex;
  flex-direction: column;
  min-height: 0;
  flex: 1;
}
.list-header {
  padding: 20px 16px 16px;
}
.list-title {
  font-family: var(--font-display);
  font-size: 1.1em;
  letter-spacing: 0.06em;
  color: var(--muted-foreground);
}
.list-title em { color: var(--muted-foreground); font-style: normal; }
.list-scroll { overflow-y: auto; flex: 1; padding: 0 16px; }
.list-empty { padding: 24px 0; color: var(--muted-foreground); font-size: 0.88em; }
</style>
