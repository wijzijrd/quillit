<template>
  <AppShell />
</template>

<script setup>
import { onMounted, onUnmounted } from 'vue'
import AppShell from './components/AppShell.vue'
import { useAnnotationsStore } from './stores/useAnnotationsStore.js'
import { useCampaignStore } from './stores/useCampaignStore.js'
import { useProjectStore } from './stores/useProjectStore.js'
import { useAuthStore } from './stores/useAuthStore.js'
import { useUIStore } from './stores/useUIStore.js'

const auth = useAuthStore()
const annotations = useAnnotationsStore()
const campaign = useCampaignStore()
const projects = useProjectStore()
const ui = useUIStore()

onMounted(async () => {
  await auth.fetchMe()
  if (auth.isLoggedIn) {
    annotations.init()
    campaign.init()
    projects.init()
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
