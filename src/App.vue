<template>
  <AppShell />
</template>

<script setup>
import { onMounted, onUnmounted } from 'vue'
import AppShell from './components/AppShell.vue'
import { useAnnotationsStore } from './stores/useAnnotationsStore.js'
import { useCampaignStore } from './stores/useCampaignStore.js'
import { useUIStore } from './stores/useUIStore.js'

const annotations = useAnnotationsStore()
const campaign = useCampaignStore()
const ui = useUIStore()

onMounted(() => {
  // Only init data stores if logged in (share routes init their own data)
  const token = localStorage.getItem('quillit:auth-token')
  if (token) {
    annotations.init()
    campaign.init()
  }
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
