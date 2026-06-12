<template>
  <Dialog :open="true" @update:open="(v) => !v && emit('close')">
    <DialogContent :show-close-button="false" class="max-md:top-0 max-md:left-0 max-md:translate-x-0 max-md:translate-y-0 max-md:w-screen max-md:h-dvh max-md:max-w-none max-md:rounded-none md:max-w-[1200px] md:w-[96vw] md:h-[88vh] md:max-h-[900px] p-0 flex flex-col overflow-hidden">

      <!-- Popup controls bar: nav + close only -->
      <div class="popup-controls">
        <div class="popup-nav">
          <button v-if="canGoBack"    class="ctrl-btn" @click="emit('back')"    title="Back">    <ChevronLeft  :size="14" /></button>
          <button v-if="canGoForward" class="ctrl-btn" @click="emit('forward')" title="Forward"> <ChevronRight :size="14" /></button>
        </div>
        <button class="ctrl-btn" @click="emit('close')" title="Close">
          <X :size="14" />
        </button>
      </div>

      <!-- Title header: metadata left, content actions right -->
      <div class="entry-header">
        <div class="entry-meta">
          <span class="entry-category" :style="{ color: catColor ?? 'var(--muted-foreground)' }">
            {{ entry?.category }}
          </span>
          <h2 class="entry-title">{{ entry?.title }}</h2>
          <div class="entry-tags" v-if="entry?.tags?.length">
            <span
              v-for="tag in entry.tags"
              :key="tag"
              class="tag-chip"
              :style="tagColor ? { color: tagColor, background: hexToAlpha(tagColor, 0.12), borderColor: hexToAlpha(tagColor, 0.3) } : { color: 'var(--muted-foreground)', borderColor: 'var(--border)' }"
            >{{ tag }}</span>
          </div>
        </div>
        <div class="entry-actions">
          <button class="ctrl-btn" @click="emit('edit')" title="Edit">
            <Pencil :size="14" />
          </button>
        </div>
      </div>

      <div class="flex flex-1 overflow-hidden">
        <div
          class="flex-1 overflow-y-auto px-9 py-7 text-[0.95em] leading-[1.75] text-[var(--foreground)]"
          v-html="renderedBody"
          @click="handleBodyClick"
        />
        <LinkedEntriesPanel v-if="entry" :entry-id="entryId" class="border-l border-[var(--border)] overflow-y-auto flex-shrink-0" />
      </div>
    </DialogContent>
  </Dialog>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Pencil, ChevronLeft, ChevronRight, X } from 'lucide-vue-next'
import { Dialog, DialogContent } from './ui/dialog'
import LinkedEntriesPanel from './LinkedEntriesPanel.vue'
import { useEntriesStore } from '../stores/useEntriesStore'
import { useCategoriesStore } from '../stores/useCategoriesStore'
import { hexToAlpha } from '../utils/color'

const props = defineProps<{
  entryId?: string
  canGoBack?: boolean
  canGoForward?: boolean
}>()
const emit = defineEmits<{ close: []; edit: []; navigate: [id: string]; back: []; forward: [] }>()

const entries = useEntriesStore()
const cats = useCategoriesStore()

const entry = computed(() => entries.getById(props.entryId ?? ''))
const catColor = computed(() => cats.categoryFor(entry.value?.category ?? '')?.color ?? null)
const tagColor = catColor

const renderedBody = computed(() => entry.value?.body ?? '')

function handleBodyClick(e: MouseEvent) {
  const mention = (e.target as HTMLElement).closest('.entry-mention') as HTMLElement | null
  if (mention?.dataset.entryId) {
    e.preventDefault()
    emit('navigate', mention.dataset.entryId)
  }
}
</script>

<style scoped>
.popup-controls {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 10px;
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
}
.popup-nav { display: flex; gap: 2px; }

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
  transition: color var(--transition), background var(--transition);
}
.ctrl-btn:hover { color: var(--foreground); background: var(--card); }

.entry-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding: 16px 24px;
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
}
.entry-meta { display: flex; flex-direction: column; gap: 4px; flex: 1; min-width: 0; }
.entry-category {
  font-size: 0.72em;
  text-transform: uppercase;
  letter-spacing: 0.1em;
  font-weight: 600;
}
.entry-title {
  font-family: var(--font-display);
  font-size: 1.5em;
  font-weight: 400;
  color: var(--foreground);
  margin: 0;
  line-height: 1.2;
}
.entry-tags { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 4px; }
.tag-chip {
  display: inline-flex;
  align-items: center;
  padding: 2px 8px;
  border-radius: 99px;
  font-size: 0.75em;
  border: 1px solid;
}
.entry-actions { display: flex; flex-direction: column; gap: 4px; flex-shrink: 0; }

/* Prose styling for rendered body */
:deep(h2) { font-family: var(--font-display); font-size: 1.2em; margin: 1.4em 0 0.4em; color: var(--foreground); }
:deep(h3) { font-size: 1.05em; margin: 1.2em 0 0.3em; color: var(--foreground); }
:deep(p) { margin-bottom: 0.85em; }
:deep(ul), :deep(ol) { padding-left: 1.4em; margin-bottom: 0.8em; }
:deep(li) { margin-bottom: 0.3em; }
:deep(hr) { border: none; border-top: 1px solid var(--border); margin: 1.5em 0; }
:deep(blockquote) { border-left: 3px solid var(--border); padding-left: 1em; color: var(--muted-foreground); margin: 1em 0; }
:deep(strong) { color: var(--foreground); font-weight: 600; }
:deep(.entry-mention) { color: var(--primary); cursor: pointer; text-decoration: underline; text-underline-offset: 2px; }
:deep(a) { color: var(--primary); text-decoration: underline; text-underline-offset: 2px; }
:deep(a):hover, :deep(.entry-mention):hover { opacity: 0.8; }
</style>
