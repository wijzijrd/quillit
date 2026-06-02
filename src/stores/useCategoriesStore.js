import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api } from '../api/client.js'

export const useCategoriesStore = defineStore('categories', () => {
  // Global admin-managed categories (used by AdminCategoriesView only)
  const categories = ref([])
  const loaded = ref(false)

  // Categories for the currently active project (own + opted-in globals)
  const projectCategories = ref([])

  // ── Global (admin) methods ────────────────────────────────────────────────

  async function init() {
    if (loaded.value) return
    categories.value = await api('/categories')
    loaded.value = true
  }

  async function createCategory(data) {
    const cat = await api('/categories', { method: 'POST', body: data })
    categories.value.push(cat)
    return cat
  }

  async function updateCategory(id, data) {
    const updated = await api(`/categories/${id}`, { method: 'PATCH', body: data })
    const idx = categories.value.findIndex(c => c.id === id)
    if (idx !== -1) categories.value[idx] = updated
    return updated
  }

  async function deleteCategory(id) {
    await api(`/categories/${id}`, { method: 'DELETE' })
    categories.value = categories.value.filter(c => c.id !== id)
  }

  async function addDefaultTag(categoryId, label) {
    const tag = await api(`/categories/${categoryId}/tags`, { method: 'POST', body: { label } })
    const cat = categories.value.find(c => c.id === categoryId)
    if (cat) cat.defaultTags.push(tag)
    return tag
  }

  async function removeDefaultTag(categoryId, tagId) {
    await api(`/categories/${categoryId}/tags/${tagId}`, { method: 'DELETE' })
    const cat = categories.value.find(c => c.id === categoryId)
    if (cat) cat.defaultTags = cat.defaultTags.filter(t => t.id !== tagId)
  }

  function defaultTagsFor(categoryName) {
    const cat = categories.value.find(c => c.name === categoryName)
    return cat ? cat.defaultTags.map(t => t.label) : []
  }

  function categoryFor(name) {
    return categories.value.find(c => c.name === name) ?? null
  }

  // ── Project-scoped methods ────────────────────────────────────────────────

  async function initForProject(projectId) {
    projectCategories.value = await api(`/projects/${projectId}/categories`)
  }

  async function createProjectCategory(projectId, data) {
    const cat = await api(`/projects/${projectId}/categories`, { method: 'POST', body: data })
    projectCategories.value.push(cat)
    return cat
  }

  async function addGlobalToProject(projectId, catId) {
    const cat = await api(`/projects/${projectId}/categories/global/${catId}`, { method: 'POST' })
    projectCategories.value.push(cat)
    return cat
  }

  async function removeFromProject(projectId, catId) {
    await api(`/projects/${projectId}/categories/${catId}`, { method: 'DELETE' })
    projectCategories.value = projectCategories.value.filter(c => c.id !== catId)
  }

  function projectCategoryFor(name) {
    return projectCategories.value.find(c => c.name === name) ?? null
  }

  return {
    categories, projectCategories, loaded,
    init, createCategory, updateCategory, deleteCategory,
    addDefaultTag, removeDefaultTag, defaultTagsFor, categoryFor,
    initForProject, createProjectCategory, addGlobalToProject, removeFromProject, projectCategoryFor,
  }
})
