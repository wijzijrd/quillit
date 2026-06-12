import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api } from '../api/client'
import type { User, Project, ProjectMember } from '../types'

export const useAdminStore = defineStore('admin', () => {
  const users = ref<User[]>([])
  const projects = ref<Project[]>([])
  const projectMembers = ref<Record<string, ProjectMember[]>>({})

  async function fetchUsers(q = '') {
    users.value = await api(`/admin/users?q=${encodeURIComponent(q)}`)
  }

  async function setUserActive(id: string, active: boolean) {
    const u: User = await api(`/admin/users/${id}`, { method: 'PATCH', body: { active } })
    const idx = users.value.findIndex(x => x.id === id)
    if (idx !== -1) users.value[idx] = u
  }

  async function deleteUser(id: string) {
    await api(`/admin/users/${id}`, { method: 'DELETE' })
    users.value = users.value.filter(x => x.id !== id)
  }

  async function fetchProjects(q = '') {
    projects.value = await api(`/admin/projects?q=${encodeURIComponent(q)}`)
  }

  async function fetchProjectMembers(id: string): Promise<ProjectMember[]> {
    const data: ProjectMember[] = await api(`/admin/projects/${id}/members`)
    projectMembers.value[id] = data
    return data
  }

  return {
    users, projects, projectMembers,
    fetchUsers, setUserActive, deleteUser, fetchProjects, fetchProjectMembers,
  }
})
