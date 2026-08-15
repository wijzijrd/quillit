import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api } from '../api/client'

/** Response shape shared by the facet create/delete endpoints (see app/content facets.go). */
export interface FacetResponse {
  name: string
  added?: boolean
  message?: string
}

export const useFacetsStore = defineStore('facets', () => {
  const global = ref<string[]>([])
  const globalLoaded = ref(false)
  /** Effective vocabulary (global ∪ project-specific) per project id, as returned by the server. */
  const projectEffective = ref<Record<string, string[]>>({})

  async function fetchGlobal() {
    global.value = await api('/content/facets')
    globalLoaded.value = true
  }

  async function fetchForProject(projectId: string) {
    projectEffective.value[projectId] = await api(`/content/projects/${projectId}/facets`)
  }

  async function createGlobal(name: string): Promise<FacetResponse> {
    const res: FacetResponse = await api('/content/facets', { method: 'POST', body: { name } })
    if (res.added && !global.value.includes(res.name)) global.value.push(res.name)
    return res
  }

  async function deleteGlobal(name: string): Promise<FacetResponse> {
    const res: FacetResponse = await api(`/content/facets/${encodeURIComponent(name)}`, { method: 'DELETE' })
    global.value = global.value.filter(f => f !== name)
    return res
  }

  async function createForProject(projectId: string, name: string): Promise<FacetResponse> {
    const res: FacetResponse = await api(`/content/projects/${projectId}/facets`, { method: 'POST', body: { name } })
    if (res.added) {
      const list = projectEffective.value[projectId] ?? []
      if (!list.includes(res.name)) list.push(res.name)
      projectEffective.value[projectId] = list
    }
    return res
  }

  async function deleteForProject(projectId: string, name: string): Promise<FacetResponse> {
    const res: FacetResponse = await api(`/content/projects/${projectId}/facets/${encodeURIComponent(name)}`, { method: 'DELETE' })
    const list = projectEffective.value[projectId]
    if (list) projectEffective.value[projectId] = list.filter(f => f !== name)
    return res
  }

  /** This project's own facets — the effective list minus whatever's already global. */
  function projectOnlyFacets(projectId: string): string[] {
    const effective = projectEffective.value[projectId] ?? []
    return effective.filter(f => !global.value.includes(f))
  }

  return {
    global, globalLoaded, projectEffective,
    fetchGlobal, fetchForProject,
    createGlobal, deleteGlobal, createForProject, deleteForProject,
    projectOnlyFacets,
  }
})
