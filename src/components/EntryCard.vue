<template>
  <div class="entry-card" :class="{ active: isActive }" @click="emit('select')">
    <div class="card-header">
      <span class="cat-badge" :style="{ color: `var(--cat-${entry.category.toLowerCase()})` }">
        {{ entry.category }}
      </span>
      <Globe v-if="(entry.visibility ?? 'private') === 'public'" :size="12" class="vis-indicator" title="Shared with players" />
    </div>
    <h3 class="card-title">{{ entry.title }}</h3>
    <p class="card-date">{{ timeAgo(entry.updatedAt) }}</p>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { Globe } from 'lucide-vue-next'
import { useUIStore } from '../stores/useUIStore.js'

const props = defineProps({ entry: Object })
const emit = defineEmits(['select'])
const ui = useUIStore()
const isActive = computed(() => ui.activeEntryId === props.entry.id)

function timeAgo(ts) {
  const diff = Date.now() - ts
  const m = Math.floor(diff / 60000)
  if (m < 1) return 'just now'
  if (m < 60) return `${m}m ago`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h ago`
  return new Date(ts).toLocaleDateString()
}
</script>

<style scoped>
.entry-card {
  padding: var(--space-md) var(--space-lg);
  border-bottom: 1px solid var(--border);
  cursor: pointer;
  transition: background var(--transition);
}
.entry-card:hover { background: var(--bg-hover); }
.entry-card.active { background: var(--bg-raised); border-left: 2px solid var(--accent); }
.card-header { margin-bottom: var(--space-xs); display: flex; align-items: center; justify-content: space-between; }
.cat-badge { font-size: var(--text-xs); text-transform: uppercase; letter-spacing: 0.1em; font-weight: 600; }
.vis-indicator { color: var(--text-faint); opacity: 0.7; }
.card-title {
  font-family: var(--font-display);
  font-size: var(--text-md);
  font-weight: 400;
  color: var(--text-primary);
  margin-bottom: var(--space-xs);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.card-date { font-size: var(--text-sm); color: var(--text-faint); }
</style>
