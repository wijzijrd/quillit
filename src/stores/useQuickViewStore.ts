import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api } from '../api/client'
import { loadQuickViewTemplates, saveQuickViewTemplates } from '../composables/usePersistence'
import { DEFAULT_TEMPLATES } from '../config/quickViewTemplates'
import type { QuickViewField, QuickViewTemplates } from '../types'

export const useQuickViewStore = defineStore('quickView', () => {
  const templates = ref<QuickViewTemplates>({ ...DEFAULT_TEMPLATES })
  const loaded = ref(false)

  async function init() {
    if (loaded.value) return
    loaded.value = true
    try {
      const cached = await loadQuickViewTemplates()
      if (cached) templates.value = { ...DEFAULT_TEMPLATES, ...cached }
      const data: QuickViewTemplates = await api('/quickview')
      templates.value = { ...DEFAULT_TEMPLATES, ...data }
      await saveQuickViewTemplates(data)
    } catch (e) {
      loaded.value = false
      throw e
    }
  }

  function getTemplate(category: string): QuickViewField[] {
    return templates.value[category] ?? []
  }

  async function addField(category: string, label: string) {
    const { nanoid } = await import('nanoid')
    const key = label.toLowerCase().replace(/\s+/g, '_') + '_' + nanoid(4)
    const current = templates.value[category] ?? []
    const fields: QuickViewField[] = [...current, { key, label, type: 'text' }]
    templates.value = { ...templates.value, [category]: fields }
    await api(`/quickview/${category}`, { method: 'PUT', body: { fields } })
    await saveQuickViewTemplates(templates.value)
  }

  async function removeField(category: string, key: string) {
    const current = templates.value[category] ?? []
    const fields = current.filter(f => f.key !== key)
    templates.value = { ...templates.value, [category]: fields }
    await api(`/quickview/${category}`, { method: 'PUT', body: { fields } })
    await saveQuickViewTemplates(templates.value)
  }

  async function renameField(category: string, key: string, newLabel: string) {
    const current = templates.value[category] ?? []
    const fields = current.map(f => f.key === key ? { ...f, label: newLabel } : f)
    templates.value = { ...templates.value, [category]: fields }
    await api(`/quickview/${category}`, { method: 'PUT', body: { fields } })
    await saveQuickViewTemplates(templates.value)
  }

  async function resetCategory(category: string) {
    const fields = DEFAULT_TEMPLATES[category] ?? []
    templates.value = { ...templates.value, [category]: fields }
    await api(`/quickview/${category}`, { method: 'DELETE' })
    await saveQuickViewTemplates(templates.value)
  }

  return { templates, loaded, init, getTemplate, addField, removeField, renameField, resetCategory }
})
