<template>
  <div class="entry-pane">
    <div class="pane-mobile-back" v-if="isMobile && hasActiveEntry">
      <button class="back-btn" @click="backToList"><ChevronLeft :size="14" /> All entries</button>
    </div>
    <div class="pane-content">
      <template v-if="hasActiveEntry">
        <EntryEditor v-if="mode === 'edit'" :on-close="() => mode = 'view'" />
        <EntryViewPane v-else @edit="mode = 'edit'" />
      </template>
      <div class="pane-empty" v-else>
        <span class="pane-empty-mark">᚛</span>
        <p>Select an entry, or create a new one</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ChevronLeft } from 'lucide-vue-next'
import EntryEditor from './EntryEditor.vue'
import EntryViewPane from './EntryViewPane.vue'
import { useUIStore } from '../stores/useUIStore'
import { useBreakpoint } from '../composables/useBreakpoint'

const mode = defineModel<'view' | 'edit'>('mode', { default: 'view' })

const ui = useUIStore()
const route = useRoute()
const router = useRouter()
const { isMobile } = useBreakpoint()
const hasActiveEntry = computed(() => !!ui.activeEntryId)

function backToList() {
  const projectId = route.params.projectId
  router.push(typeof projectId === 'string' ? `/projects/${projectId}/entries` : '/entries')
}
</script>

<style scoped>
.entry-pane { height: 100%; display: flex; flex-direction: column; }
.pane-content { flex: 1; min-height: 0; }
.pane-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--space-sm);
  height: 100%;
  color: var(--muted-foreground);
  font-size: var(--text-md);
}
.pane-empty-mark { font-size: 2.4em; color: var(--primary); opacity: 0.35; }

.pane-mobile-back { padding: 8px 10px; border-bottom: 1px solid var(--border); }
.back-btn {
  display: inline-flex; align-items: center; gap: 4px;
  background: none; border: none; color: var(--muted-foreground);
  font-size: 0.85em; cursor: pointer; padding: 4px 2px;
}
.back-btn:hover { color: var(--foreground); }
</style>
