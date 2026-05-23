import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { api } from '../api/client.js'


export const useEntriesStore = defineStore('entries', () => {
  const entries = ref([])
  const loaded = ref(false)

  async function init() {
    if (loaded.value) return
    entries.value = await api('/entries')
    loaded.value = true
  }

  async function createEntry(category = 'Lore') {
    const entry = await api('/entries', { method: 'POST', body: { category } })
    entries.value.unshift(entry)
    return entry
  }

  async function updateEntry(id, patch) {
    const updated = await api(`/entries/${id}`, { method: 'PATCH', body: patch })
    const idx = entries.value.findIndex(e => e.id === id)
    if (idx !== -1) entries.value[idx] = updated
  }

  async function deleteEntry(id) {
    await api(`/entries/${id}`, { method: 'DELETE' })
    entries.value = entries.value.filter(e => e.id !== id)
  }

  const byCategory = computed(() => {
    return (cat) => entries.value.filter(e => !cat || e.category === cat)
  })

  function getById(id) {
    return entries.value.find(e => e.id === id) ?? null
  }

  async function addLink(fromId, toId) {
    const entry = entries.value.find(e => e.id === fromId)
    if (!entry) return
    const current = entry.linkedEntries ?? []
    if (current.includes(toId)) return
    await updateEntry(fromId, { linkedEntries: [...current, toId] })
  }

  async function removeLink(fromId, toId) {
    const entry = entries.value.find(e => e.id === fromId)
    if (!entry) return
    await updateEntry(fromId, { linkedEntries: (entry.linkedEntries ?? []).filter(id => id !== toId) })
  }

  function backlinksFor(id) {
    return entries.value.filter(e => e.linkedEntries?.includes(id))
  }

  function search(query) {
    if (!query.trim()) return []
    const q = query.toLowerCase()
    return entries.value.filter(e =>
      e.title.toLowerCase().includes(q) ||
      e.body.replace(/<[^>]+>/g, '').toLowerCase().includes(q)
    )
  }

  const allTags = computed(() =>
    [...new Set(entries.value.flatMap(e => e.tags ?? []))].sort()
  )

  function byTag(tag) {
    return entries.value.filter(e => e.tags?.includes(tag))
  }

  return { entries, loaded, init, createEntry, updateEntry, deleteEntry, byCategory, getById, addLink, removeLink, backlinksFor, search, allTags, byTag }
})
