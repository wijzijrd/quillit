import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api } from '../api/client.js'

export const useAdminStore = defineStore('admin', () => {
  const users = ref([])
  const projects = ref([])
  const projectMembers = ref({}) // projectId → members[]

  async function fetchUsers(q = '') {
    users.value = await api(`/admin/users?q=${encodeURIComponent(q)}`)
  }

  async function setUserActive(id, active) {
    const u = await api(`/admin/users/${id}`, { method: 'PATCH', body: { active } })
    const idx = users.value.findIndex(x => x.id === id)
    if (idx !== -1) users.value[idx] = u
  }

  async function deleteUser(id) {
    await api(`/admin/users/${id}`, { method: 'DELETE' })
    users.value = users.value.filter(x => x.id !== id)
  }

  async function fetchProjects(q = '') {
    projects.value = await api(`/admin/projects?q=${encodeURIComponent(q)}`)
  }

  async function fetchProjectMembers(id) {
    const data = await api(`/admin/projects/${id}/members`)
    projectMembers.value[id] = data
    return data
  }

  return {
    users, projects, projectMembers,
    fetchUsers, setUserActive, deleteUser, fetchProjects, fetchProjectMembers,
  }
})
