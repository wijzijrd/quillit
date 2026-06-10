<template>
  <div class="grid grid-cols-[52px_1fr] h-screen">
    <SideNav />
    <main class="overflow-y-auto border-l border-[var(--border)]">
      <RouterView />
    </main>
  </div>
  <SearchOverlay />
</template>

<script setup lang="ts">
import { RouterView } from 'vue-router'
import SideNav from './SideNav.vue'
import SearchOverlay from './SearchOverlay.vue'
</script>
