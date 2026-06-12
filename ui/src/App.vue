<template>
  <AppShell />
</template>

<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import AppShell from './components/AppShell.vue'
import { useUIStore } from './stores/useUIStore'

const ui = useUIStore()

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
