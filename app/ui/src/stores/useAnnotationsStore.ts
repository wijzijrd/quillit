import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api } from '../api/client'
import { loadAnnotations, saveAnnotations } from '../composables/usePersistence'
import type { Annotation, AnnotationVisibility } from '../types'

export const useAnnotationsStore = defineStore('annotations', () => {
  const annotations = ref<Annotation[]>([])
  const loaded = ref(false)

  async function init() {
    if (loaded.value) return
    loaded.value = true
    try {
      const cached = await loadAnnotations()
      if (cached?.length) annotations.value = cached
      const fresh = await api('/annotations')
      annotations.value = fresh
      await saveAnnotations(fresh)
    } catch (e) {
      loaded.value = false
      throw e
    }
  }

  function getByEntry(entryId: string): Annotation[] {
    return annotations.value.filter(a => a.entryId === entryId)
  }

  async function createAnnotation(entryId: string, { text, visibility = 'gm' as AnnotationVisibility }): Promise<string> {
    const annotation: Annotation = await api('/annotations', {
      method: 'POST',
      body: { entryId, text, visibility, sharedWith: [] },
    })
    annotations.value.push(annotation)
    await saveAnnotations(annotations.value)
    return annotation.id
  }

  async function updateAnnotation(id: string, patch: Partial<Annotation>) {
    const updated: Annotation = await api(`/annotations/${id}`, { method: 'PATCH', body: patch })
    const idx = annotations.value.findIndex(a => a.id === id)
    if (idx !== -1) annotations.value[idx] = updated
    await saveAnnotations(annotations.value)
  }

  async function deleteAnnotation(id: string) {
    await api(`/annotations/${id}`, { method: 'DELETE' })
    annotations.value = annotations.value.filter(a => a.id !== id)
    await saveAnnotations(annotations.value)
  }

  return { annotations, loaded, init, getByEntry, createAnnotation, updateAnnotation, deleteAnnotation }
})
