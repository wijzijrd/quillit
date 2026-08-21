<template>
  <div class="entry-row" @click="$emit('edit')">
    <FileText :size="14" class="er-icon" />
    <span class="er-title">{{ entry.title }}</span>
    <div class="er-actions">
      <button class="er-btn" @click.stop="$emit('links')" title="Linked entries">
        <Link2 :size="13" />
      </button>
      <button class="er-btn" @click.stop="$emit('edit')" title="Edit">
        <Pencil :size="13" />
      </button>
      <button class="er-btn" @click.stop="$emit('view')" title="View">
        <Eye :size="13" />
      </button>
      <button class="er-btn er-btn--danger" @click.stop="$emit('delete')" title="Delete">
        <Trash2 :size="13" />
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { FileText, Link2, Pencil, Eye, Trash2 } from 'lucide-vue-next'
import type { ContentEntry } from '../stores/useEntryStore'

defineProps<{ entry: ContentEntry }>()
defineEmits<{ view: []; edit: []; links: []; delete: [] }>()
</script>

<style scoped>
.entry-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 9px 16px 9px 32px;
  cursor: pointer;
  border-radius: var(--radius);
  background: var(--card);
  border: 1px solid var(--border);
  margin-bottom: 4px;
  transition: background var(--transition), border-color var(--transition);
}
.entry-row:hover { background: var(--muted); border-color: var(--secondary); }
.er-icon {
  color: var(--muted-foreground);
  flex-shrink: 0;
}
.er-title {
  flex: 1;
  font-size: 0.9em;
  color: var(--foreground);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.er-actions {
  display: flex;
  gap: 2px;
  opacity: 0;
  transition: opacity var(--transition);
  flex-shrink: 0;
}
.entry-row:hover .er-actions { opacity: 1; }
.er-btn {
  background: none;
  border: none;
  color: var(--muted-foreground);
  cursor: pointer;
  padding: 4px 5px;
  border-radius: var(--radius);
  display: flex;
  align-items: center;
  transition: color var(--transition), background var(--transition);
}
.er-btn:hover { color: var(--foreground); background: var(--muted); }
.er-btn--danger:hover { color: var(--destructive); background: rgba(220, 38, 38, 0.1); }
</style>
