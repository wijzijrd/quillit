import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api } from '../api/client.js'

export const useAnnotationsStore = defineStore('annotations', () => {
  const annotations = ref([])
  const loaded = ref(false)

  async function init() {
    if (loaded.value) return
    annotations.value = await api('/annotations')
    loaded.value = true
  }

  function getByEntry(entryId) {
    return annotations.value.filter(a => a.entryId === entryId)
  }

  async function createAnnotation(entryId, { text, visibility = 'gm' }) {
    const annotation = await api('/annotations', {
      method: 'POST',
      body: { entryId, text, visibility, sharedWith: [] },
    })
    annotations.value.push(annotation)
    return annotation.id
  }

  async function updateAnnotation(id, patch) {
    const updated = await api(`/annotations/${id}`, { method: 'PATCH', body: patch })
    const idx = annotations.value.findIndex(a => a.id === id)
    if (idx !== -1) annotations.value[idx] = updated
  }

  async function deleteAnnotation(id) {
    await api(`/annotations/${id}`, { method: 'DELETE' })
    annotations.value = annotations.value.filter(a => a.id !== id)
  }

  return { annotations, loaded, init, getByEntry, createAnnotation, updateAnnotation, deleteAnnotation }
})
