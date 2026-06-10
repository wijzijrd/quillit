import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

export const useUIStore = defineStore('ui', () => {
  const activeEntryId = ref<string | null>(null)
  const activeCategory = ref<string | null>(null)
  const searchOverlayOpen = ref(false)
  const navigationHistory = ref<string[]>([])

  const theme = ref<'light' | 'dark'>('light')

  function initTheme() {
    const stored = localStorage.getItem('quillit-theme') as 'light' | 'dark' | null
    theme.value = stored ?? 'light'
    document.documentElement.classList.toggle('dark', theme.value === 'dark')
  }

  function toggleTheme() {
    theme.value = theme.value === 'light' ? 'dark' : 'light'
    document.documentElement.classList.toggle('dark', theme.value === 'dark')
    localStorage.setItem('quillit-theme', theme.value)
  }

  function setActiveEntry(id: string | null) {
    if (activeEntryId.value && activeEntryId.value !== id) {
      navigationHistory.value = [...navigationHistory.value, activeEntryId.value].slice(-50)
    }
    activeEntryId.value = id
  }

  function goBack(): string | null {
    if (navigationHistory.value.length === 0) return null
    const prev = navigationHistory.value[navigationHistory.value.length - 1]
    navigationHistory.value = navigationHistory.value.slice(0, -1)
    activeEntryId.value = prev
    return prev
  }

  const canGoBack = computed(() => navigationHistory.value.length > 0)

  function setCategory(cat: string | null) { activeCategory.value = cat }
  function toggleSearchOverlay() { searchOverlayOpen.value = !searchOverlayOpen.value }
  function closeSearchOverlay() { searchOverlayOpen.value = false }

  return {
    activeEntryId, activeCategory, searchOverlayOpen,
    navigationHistory, canGoBack,
    theme,
    setActiveEntry, goBack, setCategory, toggleSearchOverlay, closeSearchOverlay,
    initTheme, toggleTheme,
  }
})
