<template>
  <div class="entry-row" @click="$emit('view')">
    <span class="er-title">{{ entry.title }}</span>
    <div class="er-actions">
      <button class="er-btn" @click.stop="$emit('links')" title="Linked notes">
        <Link2 :size="13" />
      </button>
      <button class="er-btn" @click.stop="$emit('edit')" title="Edit">
        <Pencil :size="13" />
      </button>
      <button class="er-btn" @click.stop="$emit('view')" title="View">
        <Eye :size="13" />
      </button>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { Link2, Pencil, Eye } from 'lucide-vue-next'
import { useCategoriesStore } from '../stores/useCategoriesStore.js'

const props = defineProps({ entry: Object })
defineEmits(['view', 'edit', 'links'])

const cats = useCategoriesStore()
const catColor = computed(() => cats.categoryFor(props.entry?.category)?.color ?? 'var(--text-muted)')
</script>

<style scoped>
.entry-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 9px 16px 9px 32px;
  cursor: pointer;
  border-radius: var(--radius);
  transition: background var(--transition);
}
.entry-row:hover { background: var(--bg-hover); }
.er-title {
  flex: 1;
  font-size: 0.9em;
  color: var(--text-primary);
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
  color: var(--text-faint);
  cursor: pointer;
  padding: 4px 5px;
  border-radius: var(--radius);
  display: flex;
  align-items: center;
  transition: color var(--transition), background var(--transition);
}
.er-btn:hover { color: var(--text-primary); background: var(--bg-raised); }
</style>
