import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { api } from '../api/client'
import { useAuthStore } from './useAuthStore'
import type { Season, UserSettings } from '../types'

const SEASONS: readonly Season[] = ['spring', 'summer', 'autumn', 'winter']
const DARK_SEASONS: ReadonlySet<Season> = new Set<Season>(['autumn', 'winter'])

function isSeason(v: unknown): v is Season {
  return SEASONS.includes(v as Season)
}

// Northern-hemisphere calendar season.
function calendarSeason(d = new Date()): Season {
  const m = d.getMonth()
  if (m >= 2 && m <= 4) return 'spring'   // Mar–May
  if (m >= 5 && m <= 7) return 'summer'   // Jun–Aug
  if (m >= 8 && m <= 10) return 'autumn'  // Sep–Nov
  return 'winter'                         // Dec–Feb
}

// Migrate a legacy light/dark choice: keep the brightness the user picked,
// preferring the current calendar season when it matches.
function legacyThemeToSeason(theme: 'light' | 'dark'): Season {
  const cal = calendarSeason()
  const wantDark = theme === 'dark'
  if (DARK_SEASONS.has(cal) === wantDark) return cal
  return wantDark
    ? (cal === 'spring' ? 'winter' : 'autumn')
    : (cal === 'winter' ? 'spring' : 'summer')
}

export const useUIStore = defineStore('ui', () => {
  const activeEntryId = ref<string | null>(null)
  const activeCategory = ref<string | null>(null)
  const searchOverlayOpen = ref(false)
  const navigationHistory = ref<string[]>([])

  const season = ref<Season>('spring')
  const glass = ref(true)
  const isDark = computed(() => DARK_SEASONS.has(season.value))

  function applySeason(s: Season) {
    season.value = s
    const el = document.documentElement
    el.dataset.season = s
    el.classList.toggle('dark', DARK_SEASONS.has(s))
    localStorage.setItem('quillit-season', s)
  }

  function applyGlass(g: boolean) {
    glass.value = g
    document.documentElement.classList.toggle('glass', g)
    localStorage.setItem('quillit-glass', String(g))
  }

  function initTheme() {
    const storedSeason = localStorage.getItem('quillit-season')
    const legacy = localStorage.getItem('quillit-theme') as 'light' | 'dark' | null
    applySeason(
      isSeason(storedSeason) ? storedSeason
      : legacy ? legacyThemeToSeason(legacy)
      : calendarSeason(),
    )
    const storedGlass = localStorage.getItem('quillit-glass')
    applyGlass(storedGlass === null ? true : storedGlass === 'true')
    if (legacy) localStorage.removeItem('quillit-theme')
  }

  function patchSettings() {
    if (!useAuthStore().isLoggedIn) return
    api('/me/settings', {
      method: 'PATCH',
      body: { season: season.value, glass: glass.value },
    }).catch(() => {})
  }

  function setSeason(s: Season) {
    applySeason(s)
    if (useAuthStore().isLoggedIn) {
      localStorage.removeItem('quillit-season-explicit')
      patchSettings()
    } else {
      // Picked while logged out (auth pages): remember the intent so the
      // next login pushes this choice to the server instead of restoring
      // the server's saved season.
      localStorage.setItem('quillit-season-explicit', '1')
    }
  }

  function setGlass(g: boolean) {
    applyGlass(g)
    patchSettings()
  }

  // Pull settings from the server after login; server wins over localStorage
  // so a new device picks up the user's saved season/glass. Legacy `theme`
  // values and empty bags are migrated by seeding {season, glass} back.
  async function syncSettingsFromServer() {
    try {
      const settings = await api<UserSettings>('/me/settings')
      let seed = false
      if (localStorage.getItem('quillit-season-explicit')) {
        // Season was picked on an auth page before this login — that choice
        // wins over the server's saved season.
        localStorage.removeItem('quillit-season-explicit')
        seed = true
      } else if (isSeason(settings.season)) {
        applySeason(settings.season)
      } else if (settings.theme === 'light' || settings.theme === 'dark') {
        applySeason(legacyThemeToSeason(settings.theme))
        seed = true
      } else {
        seed = true
      }
      if (typeof settings.glass === 'boolean') applyGlass(settings.glass)
      else seed = true
      if (seed) patchSettings()
    } catch { /* offline or no session — local settings stand */ }
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
    season, glass, isDark,
    setActiveEntry, goBack, setCategory, toggleSearchOverlay, closeSearchOverlay,
    initTheme, setSeason, setGlass, syncSettingsFromServer,
  }
})
