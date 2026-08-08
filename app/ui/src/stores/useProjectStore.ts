import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api } from '../api/client'
import type { Project, ProjectType, ProjectMember, ProjectInvite } from '../types'

export const useProjectStore = defineStore('projects', () => {
  const projects = ref<Project[]>([])
  const types = ref<ProjectType[]>([])
  const loaded = ref(false)
  const membersCache = ref<Record<string, ProjectMember[]>>({})
  const invitesCache = ref<Record<string, ProjectInvite[]>>({})

  async function fetchProjects() {
    if (loaded.value) return
    loaded.value = true
    try {
      projects.value = await api('/projects')
    } catch (e) {
      loaded.value = false
      throw e
    }
  }

  async function fetchTypes() {
    if (types.value.length) return
    types.value = await api('/projects/types')
  }

  async function createProject(payload: Partial<Project>): Promise<Project> {
    const p: Project = await api('/projects', { method: 'POST', body: payload })
    projects.value.push(p)
    return p
  }

  async function updateProject(id: string, payload: Partial<Project>): Promise<Project> {
    const p: Project = await api(`/projects/${id}`, { method: 'PATCH', body: payload })
    const idx = projects.value.findIndex(x => x.id === id)
    if (idx !== -1) projects.value[idx] = p
    return p
  }

  async function deleteProject(id: string) {
    await api(`/projects/${id}`, { method: 'DELETE' })
    projects.value = projects.value.filter(x => x.id !== id)
  }

  async function fetchMembers(projectId: string): Promise<ProjectMember[]> {
    const data: ProjectMember[] = await api(`/projects/${projectId}/members`)
    membersCache.value[projectId] = data
    return data
  }

  async function addMember(projectId: string, userId: string, role: string, username = ''): Promise<ProjectMember> {
    const m: ProjectMember = await api(`/projects/${projectId}/members`, { method: 'POST', body: { userId, role, username } })
    if (membersCache.value[projectId]) membersCache.value[projectId].push(m)
    return m
  }

  async function removeMember(projectId: string, userId: string) {
    await api(`/projects/${projectId}/members/${userId}`, { method: 'DELETE' })
    if (membersCache.value[projectId])
      membersCache.value[projectId] = membersCache.value[projectId].filter(m => m.userId !== userId)
  }

  async function generateInvite(projectId: string, role: string): Promise<ProjectInvite> {
    const inv: ProjectInvite = await api(`/projects/${projectId}/invite`, { method: 'POST', body: { role } })
    if (!invitesCache.value[projectId]) invitesCache.value[projectId] = []
    invitesCache.value[projectId].push(inv)
    return inv
  }

  async function revokeInvite(projectId: string, token: string) {
    await api(`/projects/${projectId}/invite/${token}`, { method: 'DELETE' })
    if (invitesCache.value[projectId])
      invitesCache.value[projectId] = invitesCache.value[projectId].filter(i => i.token !== token)
  }

  async function join(token: string) {
    const membership = await api('/projects/join', { method: 'POST', body: { token } })
    await fetchProjects()
    return membership
  }

  function init() { return fetchProjects() }

  return {
    projects, types, loaded, membersCache, invitesCache,
    fetchProjects, fetchTypes, createProject, updateProject, deleteProject,
    fetchMembers, addMember, removeMember, generateInvite, revokeInvite, join, init,
  }
})
