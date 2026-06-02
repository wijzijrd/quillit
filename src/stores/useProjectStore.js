import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api } from '../api/client.js'

export const useProjectStore = defineStore('projects', () => {
  const projects = ref([])
  const types = ref([])
  const loaded = ref(false)
  const membersCache = ref({})   // projectId → ProjectMember[]
  const invitesCache = ref({})   // projectId → ProjectInvite[]

  async function fetchProjects() {
    projects.value = await api('/projects')
    loaded.value = true
  }

  async function fetchTypes() {
    if (types.value.length) return
    types.value = await api('/projects/types')
  }

  async function createProject(payload) {
    const p = await api('/projects', { method: 'POST', body: payload })
    projects.value.push(p)
    return p
  }

  async function updateProject(id, payload) {
    const p = await api(`/projects/${id}`, { method: 'PATCH', body: payload })
    const idx = projects.value.findIndex(x => x.id === id)
    if (idx !== -1) projects.value[idx] = p
    return p
  }

  async function deleteProject(id) {
    await api(`/projects/${id}`, { method: 'DELETE' })
    projects.value = projects.value.filter(x => x.id !== id)
  }

  async function fetchMembers(projectId) {
    const data = await api(`/projects/${projectId}/members`)
    membersCache.value[projectId] = data
    return data
  }

  async function addMember(projectId, userId, role, username = '') {
    const m = await api(`/projects/${projectId}/members`, { method: 'POST', body: { userId, role, username } })
    if (membersCache.value[projectId]) membersCache.value[projectId].push(m)
    return m
  }

  async function removeMember(projectId, userId) {
    await api(`/projects/${projectId}/members/${userId}`, { method: 'DELETE' })
    if (membersCache.value[projectId])
      membersCache.value[projectId] = membersCache.value[projectId].filter(m => m.userId !== userId)
  }

  async function generateInvite(projectId, role) {
    const inv = await api(`/projects/${projectId}/invite`, { method: 'POST', body: { role } })
    if (!invitesCache.value[projectId]) invitesCache.value[projectId] = []
    invitesCache.value[projectId].push(inv)
    return inv
  }

  async function revokeInvite(projectId, token) {
    await api(`/projects/${projectId}/invite/${token}`, { method: 'DELETE' })
    if (invitesCache.value[projectId])
      invitesCache.value[projectId] = invitesCache.value[projectId].filter(i => i.token !== token)
  }

  async function join(token) {
    const membership = await api('/projects/join', { method: 'POST', body: { token } })
    await fetchProjects()
    return membership
  }

  function init() {
    return fetchProjects()
  }

  return {
    projects, types, loaded, membersCache, invitesCache,
    fetchProjects, fetchTypes, createProject, updateProject, deleteProject,
    fetchMembers, addMember, removeMember, generateInvite, revokeInvite, join, init,
  }
})
