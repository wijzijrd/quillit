<template>
  <AppShell />
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, watch } from 'vue'
import AppShell from './components/AppShell.vue'
import { useUIStore } from './stores/useUIStore'
import { useAuthStore } from './stores/useAuthStore'

const ui = useUIStore()
const auth = useAuthStore()

// Sync server-side settings once per session transition (login, register,
// cold load with existing session).
watch(() => auth.isLoggedIn, (v) => { if (v) ui.syncSettingsFromServer() })

onMounted(() => {
  ui.initTheme()
  window.addEventListener('keydown', onKeydown)
})
onUnmounted(() => window.removeEventListener('keydown', onKeydown))

let shiftCount = 0
let shiftTimer = null
function onKeydown(e) {
  if (e.key === 'Shift' && !e.ctrlKey && !e.altKey && !e.metaKey) {
    shiftCount++
    clearTimeout(shiftTimer)
    shiftTimer = setTimeout(() => { shiftCount = 0 }, 500)
    if (shiftCount >= 3) {
      shiftCount = 0
      ui.toggleSearchOverlay()
    }
  }
}
</script>
