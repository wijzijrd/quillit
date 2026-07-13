import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { api } from '../api/client'
import { useAuthStore } from './useAuthStore'
import type { UserSettings } from '../types'

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

  function applyTheme(t: 'light' | 'dark') {
    theme.value = t
    document.documentElement.classList.toggle('dark', t === 'dark')
    localStorage.setItem('quillit-theme', t)
  }

  // Pull settings from the server after login; server wins over localStorage
  // so a new device picks up the user's saved theme. If the server has no
  // theme yet (first login after this feature shipped), seed it from local.
  async function syncSettingsFromServer() {
    try {
      const settings = await api<UserSettings>('/me/settings')
      if (settings.theme === 'light' || settings.theme === 'dark') {
        applyTheme(settings.theme)
      } else {
        api('/me/settings', { method: 'PATCH', body: { theme: theme.value } }).catch(() => {})
      }
    } catch { /* offline or no session — local theme stands */ }
  }

  function toggleTheme() {
    applyTheme(theme.value === 'light' ? 'dark' : 'light')
    if (useAuthStore().isLoggedIn) {
      api('/me/settings', { method: 'PATCH', body: { theme: theme.value } }).catch(() => {})
    }
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
    initTheme, toggleTheme, syncSettingsFromServer,
  }
})
