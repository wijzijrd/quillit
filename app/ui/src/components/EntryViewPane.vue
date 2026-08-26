<template>
  <div class="entry-view" v-if="entry">
    <div class="view-header">
      <div class="header-meta">
        <h2 class="view-title">{{ entry.title }}</h2>
        <div class="view-tags" v-if="entry.tags?.length">
          <span v-for="tag in entry.tags" :key="tag" class="tag-chip">{{ tag }}</span>
        </div>
      </div>
      <button class="ctrl-btn" @click="emit('edit')" title="Edit">
        <Pencil :size="14" />
      </button>
    </div>

    <div class="view-body" v-html="renderedBody" @click="onBodyClick" />
  </div>

  <div class="view-empty" v-else>
    <p class="empty-hint">Select an entry to view</p>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { Pencil } from 'lucide-vue-next'
import { useEntryStore, type ContentEntry } from '../stores/useEntryStore'
import { useUIStore } from '../stores/useUIStore'
import { renderMarkdownToHtml } from '../composables/useMarkdownRender'
import { entryPath } from '../utils/links'

const emit = defineEmits<{ edit: [] }>()

const entryStore = useEntryStore()
const ui = useUIStore()
const router = useRouter()

const entry = ref<ContentEntry | null>(null)
const renderedBody = ref('')

// Fetches the entry directly rather than reading it off useEntriesStore's
// list cache — that cache is scoped to a single active project, so a view
// reached cross-project (e.g. from TagView's results) would otherwise come
// up empty. Mirrors EntryEditor.vue's own self-fetch.
watch(() => ui.activeEntryId, async (id) => {
  renderedBody.value = ''
  if (!id) { entry.value = null; return }
  try {
    const found = await entryStore.get(id)
    entry.value = found
    renderedBody.value = await renderMarkdownToHtml(found.body ?? '')
  } catch {
    entry.value = null
  }
}, { immediate: true })

function onBodyClick(e: MouseEvent) {
  const link = (e.target as HTMLElement).closest('.wikilink') as HTMLElement | null
  if (link?.dataset.resolvedId) {
    e.preventDefault()
    const projectId = router.currentRoute.value.params.projectId
    router.push(entryPath(typeof projectId === 'string' ? projectId : null, link.dataset.resolvedId))
  }
}
</script>

<style scoped>
.entry-view { display: flex; flex-direction: column; height: 100%; }

.view-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-md);
  padding: var(--space-xl) var(--space-2xl) var(--space-md);
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
}
.header-meta { display: flex; flex-direction: column; gap: 4px; flex: 1; min-width: 0; }
.view-title {
  font-family: var(--font-display);
  font-size: var(--text-2xl);
  font-weight: 400;
  color: var(--foreground);
  margin: 0;
  line-height: 1.2;
}
.view-tags { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 4px; }
.tag-chip {
  display: inline-flex;
  align-items: center;
  background: var(--secondary);
  color: var(--primary);
  border-radius: var(--radius);
  height: var(--h-xs);
  padding: 0 var(--space-sm);
  font-size: var(--text-xs);
}
.ctrl-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: var(--radius);
  border: 1px solid var(--border);
  background: var(--muted);
  color: var(--muted-foreground);
  cursor: pointer;
  flex-shrink: 0;
  transition: color var(--transition), background var(--transition);
}
.ctrl-btn:hover { color: var(--foreground); background: var(--card); }

.view-body {
  flex: 1;
  overflow-y: auto;
  padding: var(--space-xl) var(--space-2xl);
  font-size: 0.95em;
  line-height: 1.75;
  color: var(--foreground);
}

.view-empty {
  display: flex; align-items: center; justify-content: center;
  height: 100%; color: var(--muted-foreground); font-size: var(--text-md);
}

:deep(h2) { font-family: var(--font-display); font-size: 1.2em; margin: 1.4em 0 0.4em; color: var(--foreground); }
:deep(h3) { font-size: 1.05em; margin: 1.2em 0 0.3em; color: var(--foreground); }
:deep(p) { margin-bottom: 0.85em; }
:deep(ul), :deep(ol) { padding-left: 1.4em; margin-bottom: 0.8em; }
:deep(li) { margin-bottom: 0.3em; }
:deep(hr) { border: none; border-top: 1px solid var(--border); margin: 1.5em 0; }
:deep(blockquote) { border-left: 3px solid var(--border); padding-left: 1em; color: var(--muted-foreground); margin: 1em 0; }
:deep(strong) { color: var(--foreground); font-weight: 600; }
:deep(a) { color: var(--primary); text-decoration: underline; text-underline-offset: 2px; }
:deep(a):hover { opacity: 0.8; }
</style>
