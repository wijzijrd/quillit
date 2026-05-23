<template>
  <Teleport to="body">
    <div class="modal-backdrop" @click.self="$emit('close')">
      <div class="view-panel">

        <div class="view-header">
          <div class="view-header-left">
            <span class="view-cat" :style="{ color: catColor }">{{ entry?.category }}</span>
            <h2 class="view-title">{{ entry?.title }}</h2>
            <div class="view-tags" v-if="entry?.tags?.length">
              <span
                v-for="tag in entry.tags"
                :key="tag"
                class="view-tag"
                :style="tagColor ? { color: tagColor, background: hexToAlpha(tagColor, 0.12), borderColor: hexToAlpha(tagColor, 0.3) } : {}"
              >{{ tag }}</span>
            </div>
          </div>
          <div class="view-header-actions">
            <button class="hbtn" @click="$emit('edit')" title="Edit">
              <Pencil :size="14" />
            </button>
            <button class="hbtn" @click="$emit('close')" title="Close">✕</button>
          </div>
        </div>

        <div class="view-body-wrap">
          <div class="view-body prose" v-html="renderedBody" @click="handleBodyClick" />
          <LinkedEntriesPanel v-if="entry" :entry-id="entryId" class="view-links" />
        </div>

      </div>
    </div>
  </Teleport>
</template>

<script setup>
import { computed } from 'vue'
import { Pencil } from 'lucide-vue-next'
import LinkedEntriesPanel from './LinkedEntriesPanel.vue'
import { useEntriesStore } from '../stores/useEntriesStore.js'
import { useCategoriesStore } from '../stores/useCategoriesStore.js'
import { hexToAlpha } from '../utils/color.js'

const props = defineProps({ entryId: String })
const emit = defineEmits(['close', 'edit', 'navigate'])

const entries = useEntriesStore()
const cats = useCategoriesStore()

const entry = computed(() => entries.getById(props.entryId))
const catColor = computed(() => cats.categoryFor(entry.value?.category)?.color ?? null)
const tagColor = catColor

const renderedBody = computed(() => entry.value?.body ?? '')

function handleBodyClick(e) {
  const mention = e.target.closest('.entry-mention')
  if (mention?.dataset.entryId) {
    e.preventDefault()
    emit('navigate', mention.dataset.entryId)
  }
}
</script>

<style scoped>
.modal-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.65);
  z-index: 100;
  display: flex;
  align-items: center;
  justify-content: center;
}
.view-panel {
  width: min(1100px, 96vw);
  height: min(88vh, 820px);
  background: var(--bg-base);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.view-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  padding: 20px 24px 16px;
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
  gap: 16px;
}
.view-header-left { display: flex; flex-direction: column; gap: 4px; }
.view-cat {
  font-size: 0.7em;
  text-transform: uppercase;
  letter-spacing: 0.1em;
  font-weight: 600;
}
.view-title {
  font-family: var(--font-display);
  font-size: 1.5em;
  color: var(--text-primary);
  margin: 0;
}
.view-tags { display: flex; flex-wrap: wrap; gap: 5px; margin-top: 6px; }
.view-tag {
  display: inline-flex;
  align-items: center;
  padding: 2px 9px;
  border: 1px solid var(--border-light);
  border-radius: 20px;
  font-size: 0.78em;
  background: var(--bg-raised);
  color: var(--text-muted);
}
.view-header-actions { display: flex; gap: 6px; align-items: center; flex-shrink: 0; }
.hbtn {
  background: var(--bg-raised);
  border: 1px solid var(--border-light);
  border-radius: var(--radius);
  color: var(--text-muted);
  cursor: pointer;
  width: 28px;
  height: 28px;
  font-size: 0.78em;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: color var(--transition), background var(--transition);
}
.hbtn:hover { background: var(--bg-hover); color: var(--text-primary); }

.view-body-wrap {
  display: flex;
  flex: 1;
  overflow: hidden;
}
.view-body {
  flex: 1;
  overflow-y: auto;
  padding: 28px 36px;
  font-size: 0.95em;
  line-height: 1.75;
  color: var(--text-primary);
}
.view-links {
  border-left: 1px solid var(--border);
  overflow-y: auto;
  flex-shrink: 0;
}

/* Prose styling for rendered body */
.view-body :deep(h2) { font-family: var(--font-display); font-size: 1.2em; margin: 1.4em 0 0.4em; color: var(--text-primary); }
.view-body :deep(h3) { font-size: 1.05em; margin: 1.2em 0 0.3em; color: var(--text-primary); }
.view-body :deep(p) { margin-bottom: 0.85em; }
.view-body :deep(ul), .view-body :deep(ol) { padding-left: 1.4em; margin-bottom: 0.8em; }
.view-body :deep(li) { margin-bottom: 0.3em; }
.view-body :deep(hr) { border: none; border-top: 1px solid var(--border); margin: 1.5em 0; }
.view-body :deep(blockquote) { border-left: 3px solid var(--border); padding-left: 1em; color: var(--text-muted); margin: 1em 0; }
.view-body :deep(strong) { color: var(--text-primary); font-weight: 600; }

/* Clickable links in view mode */
.view-body :deep(.entry-mention) {
  color: var(--accent);
  cursor: pointer;
  text-decoration: underline;
  text-underline-offset: 2px;
}
.view-body :deep(a) {
  color: var(--accent);
  text-decoration: underline;
  text-underline-offset: 2px;
}
.view-body :deep(a):hover, .view-body :deep(.entry-mention):hover {
  opacity: 0.8;
}
</style>
