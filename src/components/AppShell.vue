<template>
  <div class="shell" :class="{ 'sidebar-collapsed': !ui.sidebarOpen }">
    <SideNav />
    <main class="main-content">
      <RouterView />
    </main>
  </div>
  <SearchOverlay />
</template>

<script setup>
import { RouterView } from 'vue-router'
import SideNav from './SideNav.vue'
import SearchOverlay from './SearchOverlay.vue'
import { useUIStore } from '../stores/useUIStore.js'

const ui = useUIStore()
</script>

<style scoped>
.shell {
  display: grid;
  grid-template-columns: 240px 1fr;
  height: 100vh;
  transition: grid-template-columns var(--transition);
}
.shell.sidebar-collapsed {
  grid-template-columns: 56px 1fr;
}
.main-content {
  overflow-y: auto;
  border-left: 1px solid var(--border);
}
</style>
