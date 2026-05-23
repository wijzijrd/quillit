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

<script setup>
import { computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import EntryCard from '../components/EntryCard.vue'
import EntryEditor from '../components/EntryEditor.vue'
import { useEntriesStore } from '../stores/useEntriesStore.js'
import { useUIStore } from '../stores/useUIStore.js'

const entries = useEntriesStore()
const ui = useUIStore()
const route = useRoute()

onMounted(() => entries.init())

const tagEntries = computed(() =>
  entries.byTag(route.params.tag)
    .slice()
    .sort((a, b) => b.updatedAt - a.updatedAt)
)
</script>

<style scoped>
.quillit-view {
  display: grid;
  grid-template-columns: 280px 1fr;
  height: 100vh;
}
.entry-list {
  background: var(--bg-surface);
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
  color: var(--text-muted);
}
.list-title em { color: var(--text-faint); font-style: normal; }
.list-scroll { overflow-y: auto; flex: 1; }
.list-empty { padding: 24px 16px; color: var(--text-faint); font-size: 0.88em; }
.editor-panel { overflow-y: auto; }
</style>
