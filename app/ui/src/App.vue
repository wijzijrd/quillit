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
// cold load with existing session). immediate: on cold load the router guard
// resolves fetchMe() before this component mounts, so isLoggedIn may already
// be true when the watcher is registered.
watch(() => auth.isLoggedIn, (v) => { if (v) ui.syncSettingsFromServer() }, { immediate: true })

onMounted(() => {
  window.addEventListener('keydown', onKeydown)
})
onUnmounted(() => window.removeEventListener('keydown', onKeydown))

let shiftCount = 0
let shiftTimer: ReturnType<typeof setTimeout> | null = null
function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Shift' && !e.ctrlKey && !e.altKey && !e.metaKey) {
    shiftCount++
    if (shiftTimer) clearTimeout(shiftTimer)
    shiftTimer = setTimeout(() => { shiftCount = 0 }, 500)
    if (shiftCount >= 3) {
      shiftCount = 0
      ui.toggleSearchOverlay()
    }
  }
}
</script>
