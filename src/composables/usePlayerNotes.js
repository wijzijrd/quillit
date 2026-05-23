import { ref } from 'vue'
import { api } from '../api/client.js'
const CATEGORIES = ['Note', 'Characters', 'Location', 'Quest', 'Lore', 'Other']

export function usePlayerNotes(token) {
  const notes = ref([])
  const loaded = ref(false)

  async function init() {
    if (loaded.value) return
    notes.value = await api(`/share/${token}/notes`)
    loaded.value = true
  }

  async function createNote(patch = {}) {
    const note = await api(`/share/${token}/notes`, {
      method: 'POST',
      body: {
        title: 'Untitled Note',
        body: '',
        category: CATEGORIES[0],
        visibility: 'private',
        tags: [],
        ...patch,
      },
    })
    notes.value = [note, ...notes.value]
    return note
  }

  async function updateNote(id, patch) {
    const updated = await api(`/share/${token}/notes/${id}`, { method: 'PATCH', body: patch })
    const idx = notes.value.findIndex(n => n.id === id)
    if (idx !== -1) notes.value[idx] = updated
  }

  async function deleteNote(id) {
    await api(`/share/${token}/notes/${id}`, { method: 'DELETE' })
    notes.value = notes.value.filter(n => n.id !== id)
  }

  return { notes, loaded, init, createNote, updateNote, deleteNote }
}
