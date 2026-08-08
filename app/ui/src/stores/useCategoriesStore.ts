import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api } from '../api/client'
import type { Category, DefaultTag } from '../types'

export const useCategoriesStore = defineStore('categories', () => {
  const categories = ref<Category[]>([])
  const loaded = ref(false)
  const projectCategories = ref<Category[]>([])

  async function init() {
    if (loaded.value) return
    categories.value = await api('/categories')
    loaded.value = true
  }

  async function createCategory(data: Partial<Category>): Promise<Category> {
    const cat: Category = await api('/categories', { method: 'POST', body: data })
    categories.value.push(cat)
    return cat
  }

  async function updateCategory(id: string, data: Partial<Category>): Promise<Category> {
    const updated: Category = await api(`/categories/${id}`, { method: 'PATCH', body: data })
    const idx = categories.value.findIndex(c => c.id === id)
    if (idx !== -1) categories.value[idx] = updated
    return updated
  }

  async function deleteCategory(id: string) {
    await api(`/categories/${id}`, { method: 'DELETE' })
    categories.value = categories.value.filter(c => c.id !== id)
  }

  async function addDefaultTag(categoryId: string, label: string): Promise<DefaultTag> {
    const tag: DefaultTag = await api(`/categories/${categoryId}/tags`, { method: 'POST', body: { label } })
    const cat = categories.value.find(c => c.id === categoryId)
    if (cat) cat.defaultTags.push(tag)
    return tag
  }

  async function removeDefaultTag(categoryId: string, tagId: string) {
    await api(`/categories/${categoryId}/tags/${tagId}`, { method: 'DELETE' })
    const cat = categories.value.find(c => c.id === categoryId)
    if (cat) cat.defaultTags = cat.defaultTags.filter(t => t.id !== tagId)
  }

  function defaultTagsFor(categoryName: string): string[] {
    const cat = categories.value.find(c => c.name === categoryName)
    return cat ? cat.defaultTags.map(t => t.label) : []
  }

  function categoryFor(name: string): Category | null {
    return categories.value.find(c => c.name === name) ?? null
  }

  async function initForProject(projectId: string) {
    projectCategories.value = await api(`/projects/${projectId}/categories`)
  }

  async function createProjectCategory(projectId: string, data: Partial<Category>): Promise<Category> {
    const cat: Category = await api(`/projects/${projectId}/categories`, { method: 'POST', body: data })
    projectCategories.value.push(cat)
    return cat
  }

  async function addGlobalToProject(projectId: string, catId: string): Promise<Category> {
    const cat: Category = await api(`/projects/${projectId}/categories/global/${catId}`, { method: 'POST' })
    projectCategories.value.push(cat)
    return cat
  }

  async function removeFromProject(projectId: string, catId: string) {
    await api(`/projects/${projectId}/categories/${catId}`, { method: 'DELETE' })
    projectCategories.value = projectCategories.value.filter(c => c.id !== catId)
  }

  function projectCategoryFor(name: string): Category | null {
    return projectCategories.value.find(c => c.name === name) ?? null
  }

  return {
    categories, projectCategories, loaded,
    init, createCategory, updateCategory, deleteCategory,
    addDefaultTag, removeDefaultTag, defaultTagsFor, categoryFor,
    initForProject, createProjectCategory, addGlobalToProject, removeFromProject, projectCategoryFor,
  }
})
