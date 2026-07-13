<template>
  <div class="app-shell relative h-screen">
    <SeasonBackdrop />
    <!-- Public routes (login/register/share) get a chromeless full-bleed main. -->
    <template v-if="isPublic">
      <main class="h-full overflow-y-auto">
        <RouterView />
      </main>
    </template>
    <template v-else>
      <div class="grid h-full grid-cols-[56px_1fr] gap-3 p-3">
        <SideNav class="surface rounded-lg" />
        <main class="surface overflow-y-auto rounded-lg">
          <RouterView />
        </main>
      </div>
    </template>
    <SearchOverlay />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { RouterView, useRoute } from 'vue-router'
import SideNav from './SideNav.vue'
import SearchOverlay from './SearchOverlay.vue'
import SeasonBackdrop from './SeasonBackdrop.vue'

const route = useRoute()
const isPublic = computed(() => !!route.meta.public)
</script>
