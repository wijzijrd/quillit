import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api } from '../api/client.js'
import { DEFAULT_TEMPLATES } from '../config/quickViewTemplates.js'

export const useQuickViewStore = defineStore('quickView', () => {
  const templates = ref({ ...DEFAULT_TEMPLATES })
  const loaded = ref(false)

  async function init() {
    if (loaded.value) return
    const data = await api('/quickview')
    templates.value = { ...DEFAULT_TEMPLATES, ...data }
    loaded.value = true
  }

  function getTemplate(category) {
    return templates.value[category] ?? []
  }

  async function addField(category, label) {
    const { nanoid } = await import('nanoid')
    const key = label.toLowerCase().replace(/\s+/g, '_') + '_' + nanoid(4)
    const current = templates.value[category] ?? []
    const fields = [...current, { key, label, type: 'text' }]
    templates.value = { ...templates.value, [category]: fields }
    await api(`/quickview/${category}`, { method: 'PUT', body: { fields } })
  }

  async function removeField(category, key) {
    const current = templates.value[category] ?? []
    const fields = current.filter(f => f.key !== key)
    templates.value = { ...templates.value, [category]: fields }
    await api(`/quickview/${category}`, { method: 'PUT', body: { fields } })
  }

  async function renameField(category, key, newLabel) {
    const current = templates.value[category] ?? []
    const fields = current.map(f => f.key === key ? { ...f, label: newLabel } : f)
    templates.value = { ...templates.value, [category]: fields }
    await api(`/quickview/${category}`, { method: 'PUT', body: { fields } })
  }

  async function resetCategory(category) {
    const fields = DEFAULT_TEMPLATES[category] ?? []
    templates.value = { ...templates.value, [category]: fields }
    await api(`/quickview/${category}`, { method: 'DELETE' })
  }

  return { templates, loaded, init, getTemplate, addField, removeField, renameField, resetCategory }
})
