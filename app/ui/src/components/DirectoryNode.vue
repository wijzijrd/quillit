<template>
  <div class="dir-node">
    <div
      v-if="!isRoot"
      class="dir-header"
      :class="{ 'dir-header--drop-hover': dropHover }"
      :style="{ paddingLeft: `${depth * 16}px` }"
      @dragover.prevent="dropHover = true"
      @dragleave="dropHover = false"
      @drop.prevent="handleDrop"
    >
      <button class="dir-toggle" @click="actions.onToggle(node.path)" :title="isOpen ? 'Collapse' : 'Expand'">
        <ChevronRight :size="13" class="dir-toggle-icon" :class="{ 'dir-toggle-icon--open': isOpen }" />
      </button>
      <span class="dir-name">{{ node.name }}</span>
      <button class="dir-add-btn" @click.stop="startCreateFolder" title="New folder">+ folder</button>
    </div>
    <div
      v-else
      class="dir-header dir-header--root"
      :class="{ 'dir-header--drop-hover': dropHover }"
      @dragover.prevent="dropHover = true"
      @dragleave="dropHover = false"
      @drop.prevent="handleDrop"
    >
      <button class="dir-add-btn" @click="startCreateFolder" title="New folder">+ folder</button>
    </div>

    <div v-if="creatingFolder" class="dir-new-folder" :style="{ paddingLeft: `${(depth + 1) * 16}px` }">
      <input
        ref="newFolderInput"
        v-model="newFolderName"
        class="dir-new-folder-input"
        placeholder="folder name"
        @keydown.enter="confirmCreateFolder"
        @keydown.escape="cancelCreateFolder"
        @blur="cancelCreateFolder"
      />
    </div>

    <template v-if="isRoot || isOpen">
      <EntryRow
        v-for="entry in node.entries"
        :key="entry.id"
        :entry="entry"
        @view="actions.onView(entry)"
        @edit="actions.onEdit(entry)"
        @links="actions.onLinks(entry)"
        @delete="actions.onDelete(entry)"
      />
      <DirectoryNode
        v-for="child in node.children"
        :key="child.path"
        :node="child"
        :depth="depth + 1"
      />
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, nextTick, ref } from 'vue'
import { ChevronRight } from 'lucide-vue-next'
import EntryRow from './EntryRow.vue'
import { directoryTreeActionsKey, type DirNode, type DirectoryTreeActions } from '../lib/directoryTree'

const props = defineProps<{ node: DirNode; depth: number }>()

const injected = inject(directoryTreeActionsKey)
if (!injected) {
  throw new Error('DirectoryNode must be rendered under a component that provides directoryTreeActionsKey')
}
const actions: DirectoryTreeActions = injected

const isRoot = computed(() => props.depth === 0)
const isOpen = computed(() => actions.isExpanded(props.node.path))

const dropHover = ref(false)
function handleDrop(e: DragEvent) {
  dropHover.value = false
  const entryId = e.dataTransfer?.getData('text/plain')
  if (entryId) actions.onMove(entryId, props.node.path)
}

const creatingFolder = ref(false)
const newFolderName = ref('')
const newFolderInput = ref<HTMLInputElement | null>(null)

async function startCreateFolder() {
  creatingFolder.value = true
  await nextTick()
  newFolderInput.value?.focus()
}
function confirmCreateFolder() {
  const name = newFolderName.value.trim()
  if (name) actions.onCreateFolder(props.node.path, name)
  cancelCreateFolder()
}
function cancelCreateFolder() {
  creatingFolder.value = false
  newFolderName.value = ''
}
</script>

<style scoped>
.dir-header {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 8px;
  border-radius: var(--radius);
  cursor: default;
  transition: background var(--transition);
}
.dir-header--root { padding-left: 0; }
.dir-header--drop-hover { background: var(--secondary); }
.dir-toggle {
  background: none;
  border: none;
  color: var(--muted-foreground);
  cursor: pointer;
  display: flex;
  align-items: center;
  padding: 2px;
}
.dir-toggle-icon { transition: transform var(--transition); }
.dir-toggle-icon--open { transform: rotate(90deg); }
.dir-name {
  flex: 1;
  font-size: 0.85em;
  font-weight: 600;
  color: var(--muted-foreground);
  text-transform: uppercase;
  letter-spacing: 0.04em;
}
.dir-add-btn {
  background: none;
  border: none;
  color: var(--muted-foreground);
  font-size: 0.78em;
  cursor: pointer;
  padding: 2px 6px;
  border-radius: var(--radius);
  opacity: 0;
  transition: opacity var(--transition), color var(--transition);
}
.dir-header:hover .dir-add-btn,
.dir-header--root .dir-add-btn { opacity: 1; }
.dir-add-btn:hover { color: var(--primary); }
.dir-new-folder { padding: 2px 8px 6px; }
.dir-new-folder-input {
  background: var(--card);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  color: var(--foreground);
  font-size: 0.85em;
  padding: 4px 8px;
  width: 180px;
}
</style>
