import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { api } from '../api/client'
import { loadEntries, saveEntries } from '../composables/usePersistence'
import type { Entry } from '../types'

export const useEntriesStore = defineStore('entries', () => {
  const entries = ref<Entry[]>([])
  const loaded = ref(false)

  async function init() {
    if (loaded.value) return
    loaded.value = true
    try {
      const cached = await loadEntries()
      if (cached?.length) entries.value = cached
      const fresh = await api('/entries')
      entries.value = fresh
      await saveEntries(fresh)
    } catch (e) {
      loaded.value = false
      throw e
    }
  }

  async function createEntry(category = 'Lore'): Promise<Entry> {
    const entry: Entry = await api('/entries', { method: 'POST', body: { category } })
    entries.value.unshift(entry)
    await saveEntries(entries.value)
    return entry
  }

  async function updateEntry(id: string, patch: Partial<Entry>) {
    const updated: Entry = await api(`/entries/${id}`, { method: 'PATCH', body: patch })
    const idx = entries.value.findIndex(e => e.id === id)
    if (idx !== -1) entries.value[idx] = updated
    await saveEntries(entries.value)
  }

  async function deleteEntry(id: string) {
    await api(`/entries/${id}`, { method: 'DELETE' })
    entries.value = entries.value.filter(e => e.id !== id)
    await saveEntries(entries.value)
  }

  const byCategory = computed(() => {
    return (cat: string | null) => entries.value.filter(e => !cat || e.category === cat)
  })

  function getById(id: string): Entry | null {
    return entries.value.find(e => e.id === id) ?? null
  }

  async function addLink(fromId: string, toId: string) {
    const entry = entries.value.find(e => e.id === fromId)
    if (!entry) return
    const current = entry.linkedEntries ?? []
    if (current.includes(toId)) return
    await updateEntry(fromId, { linkedEntries: [...current, toId] })
  }

  async function removeLink(fromId: string, toId: string) {
    const entry = entries.value.find(e => e.id === fromId)
    if (!entry) return
    await updateEntry(fromId, { linkedEntries: (entry.linkedEntries ?? []).filter(id => id !== toId) })
  }

  function backlinksFor(id: string): Entry[] {
    return entries.value.filter(e => e.linkedEntries?.includes(id))
  }

  function search(query: string): Entry[] {
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

  function byTag(tag: string): Entry[] {
    return entries.value.filter(e => e.tags?.includes(tag))
  }

  return { entries, loaded, init, createEntry, updateEntry, deleteEntry, byCategory, getById, addLink, removeLink, backlinksFor, search, allTags, byTag }
})
