<template>
  <Dialog :open="true" @update:open="(v) => !v && emit('close')">
    <DialogContent :show-close-button="false" class="max-md:top-0 max-md:left-0 max-md:translate-x-0 max-md:translate-y-0 max-md:w-screen max-md:h-dvh max-md:max-w-none max-md:rounded-none md:max-w-[1200px] md:w-[96vw] md:h-[88vh] md:max-h-[900px] p-0 flex flex-col overflow-hidden">
      <EntryEditor :on-close="() => emit('close')" />
    </DialogContent>
  </Dialog>
</template>

<script setup lang="ts">
import { onMounted, watch } from 'vue'
import { Dialog, DialogContent } from './ui/dialog'
import EntryEditor from './EntryEditor.vue'
import { useUIStore } from '../stores/useUIStore'

const props = defineProps<{ entryId?: string }>()
const emit = defineEmits<{ close: [] }>()

const ui = useUIStore()
onMounted(() => ui.setActiveEntry(props.entryId ?? null))
watch(() => ui.activeEntryId, (id) => {
  if (id === null) emit('close')
})
</script>
