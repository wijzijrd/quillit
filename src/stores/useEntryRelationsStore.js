import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api } from '../api/client.js'

export const useEntryRelationsStore = defineStore('entryRelations', () => {
  const cache = ref({})
  const labels = ref([])
  const labelsLoaded = ref(false)

  async function fetchForEntry(entryId) {
    const rels = await api(`/entries/${entryId}/relations`)
    cache.value[entryId] = rels
    return rels
  }

  function getForEntry(entryId) {
    return cache.value[entryId] ?? []
  }

  async function fetchLabels() {
    if (labelsLoaded.value) return
    labels.value = await api('/relation-labels')
    labelsLoaded.value = true
  }

  async function create(fromId, toId, label) {
    const rel = await api(`/entries/${fromId}/relations`, {
      method: 'POST',
      body: { toId, label },
    })
    if (cache.value[fromId]) cache.value[fromId].push(rel)
    if (!labels.value.includes(label)) {
      labels.value = [...labels.value, label].sort()
    }
    return rel
  }

  async function remove(fromId, relationId) {
    await api(`/entry-relations/${relationId}`, { method: 'DELETE' })
    if (cache.value[fromId]) {
      cache.value[fromId] = cache.value[fromId].filter(r => r.id !== relationId)
    }
  }

  function invalidate(entryId) {
    delete cache.value[entryId]
  }

  return { cache, labels, fetchForEntry, getForEntry, fetchLabels, create, remove, invalidate }
})
