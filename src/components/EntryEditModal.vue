<template>
  <Teleport to="body">
    <div class="modal-backdrop" @click.self="$emit('close')">
      <div class="modal-panel">
        <button class="modal-close" @click="$emit('close')" title="Close">✕</button>
        <EntryEditor />
      </div>
    </div>
  </Teleport>
</template>

<script setup>
import { onMounted } from 'vue'
import EntryEditor from './EntryEditor.vue'
import { useUIStore } from '../stores/useUIStore.js'

const props = defineProps({ entryId: String })
defineEmits(['close'])

const ui = useUIStore()
onMounted(() => ui.setActiveEntry(props.entryId))
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
.modal-panel {
  position: relative;
  width: min(960px, 96vw);
  height: min(88vh, 800px);
  background: var(--bg-base);
  border-radius: var(--radius);
  border: 1px solid var(--border);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.modal-close {
  position: absolute;
  top: 10px;
  right: 12px;
  z-index: 10;
  background: var(--bg-raised);
  border: 1px solid var(--border-light);
  border-radius: var(--radius);
  color: var(--text-faint);
  cursor: pointer;
  width: 24px;
  height: 24px;
  font-size: 0.75em;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: color var(--transition);
}
.modal-close:hover { color: var(--text-primary); }
</style>
