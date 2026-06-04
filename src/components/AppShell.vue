<template>
  <div class="shell">
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
</script>

<style scoped>
.shell {
  display: grid;
  grid-template-columns: 240px 1fr;
  height: 100vh;
}
.main-content {
  overflow-y: auto;
  border-left: 1px solid var(--border);
}
</style>
