import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

export const useUIStore = defineStore('ui', () => {
  const activeEntryId = ref(null)
  const activeCategory = ref(null)
  const searchOverlayOpen = ref(false)
  const navigationHistory = ref([])

  function setActiveEntry(id) {
    if (activeEntryId.value && activeEntryId.value !== id) {
      navigationHistory.value = [...navigationHistory.value, activeEntryId.value].slice(-50)
    }
    activeEntryId.value = id
  }

  function goBack() {
    if (navigationHistory.value.length === 0) return null
    const prev = navigationHistory.value[navigationHistory.value.length - 1]
    navigationHistory.value = navigationHistory.value.slice(0, -1)
    activeEntryId.value = prev
    return prev
  }

  const canGoBack = computed(() => navigationHistory.value.length > 0)

  function setCategory(cat) { activeCategory.value = cat }
  function toggleSearchOverlay() { searchOverlayOpen.value = !searchOverlayOpen.value }
  function closeSearchOverlay() { searchOverlayOpen.value = false }

  return {
    activeEntryId, activeCategory, searchOverlayOpen,
    navigationHistory, canGoBack,
    setActiveEntry, goBack, setCategory, toggleSearchOverlay, closeSearchOverlay,
  }
})
