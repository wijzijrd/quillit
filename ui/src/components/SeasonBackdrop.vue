<template>
  <div class="season-backdrop" aria-hidden="true">
    <img v-if="src" :src="src" alt="" class="h-full w-full object-cover" />
  </div>
</template>

<script setup lang="ts">
// Depth-0 opaque base layer. Renders the per-season gradient placeholder;
// pass `src` to slot in real seasonal artwork later.
defineProps<{ src?: string }>()
</script>
