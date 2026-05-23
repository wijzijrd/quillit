import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api } from '../api/client.js'

export const useMemberStore = defineStore('member', () => {
  const sharedNotes = ref([])
  const folders = ref([])
  const sessionNotes = ref([])

  async function fetchSharedNotes() {
    sharedNotes.value = await api('/member/shared')
  }

  async function fetchFolders() {
    folders.value = await api('/member/folders')
  }

  async function createFolder(payload) {
    const f = await api('/member/folders', { method: 'POST', body: payload })
    folders.value.push(f)
    return f
  }

  async function updateFolder(id, payload) {
    const f = await api(`/member/folders/${id}`, { method: 'PATCH', body: payload })
    const idx = folders.value.findIndex(x => x.id === id)
    if (idx !== -1) folders.value[idx] = f
    return f
  }

  async function deleteFolder(id) {
    await api(`/member/folders/${id}`, { method: 'DELETE' })
    folders.value = folders.value.filter(x => x.id !== id)
  }

  async function addToFolder(folderId, entryId) {
    await api(`/member/folders/${folderId}/entries`, { method: 'POST', body: { entryId } })
  }

  async function removeFromFolder(folderId, entryId) {
    await api(`/member/folders/${folderId}/entries/${entryId}`, { method: 'DELETE' })
  }

  async function upsertEntryMeta(entryId, payload) {
    return await api(`/member/entries/${entryId}/meta`, { method: 'PUT', body: payload })
  }

  async function fetchSessionNotes() {
    sessionNotes.value = await api('/member/session-notes')
  }

  async function createSessionNote(payload = {}) {
    const n = await api('/member/session-notes', { method: 'POST', body: payload })
    sessionNotes.value.unshift(n)
    return n
  }

  async function updateSessionNote(id, payload) {
    const n = await api(`/member/session-notes/${id}`, { method: 'PATCH', body: payload })
    const idx = sessionNotes.value.findIndex(x => x.id === id)
    if (idx !== -1) sessionNotes.value[idx] = n
    return n
  }

  async function deleteSessionNote(id) {
    await api(`/member/session-notes/${id}`, { method: 'DELETE' })
    sessionNotes.value = sessionNotes.value.filter(x => x.id !== id)
  }

  async function searchUsers(q) {
    return await api(`/users/search?q=${encodeURIComponent(q)}`)
  }

  async function fetchEntryShares(entryId) {
    return await api(`/entries/${entryId}/shares`)
  }

  async function addShares(entryId, userIds) {
    return await api(`/entries/${entryId}/shares`, { method: 'POST', body: { userIds } })
  }

  async function removeShare(entryId, userId) {
    return await api(`/entries/${entryId}/shares/${userId}`, { method: 'DELETE' })
  }

  return {
    sharedNotes, folders, sessionNotes,
    fetchSharedNotes, fetchFolders, createFolder, updateFolder, deleteFolder,
    addToFolder, removeFromFolder, upsertEntryMeta,
    fetchSessionNotes, createSessionNote, updateSessionNote, deleteSessionNote,
    searchUsers, fetchEntryShares, addShares, removeShare,
  }
})
