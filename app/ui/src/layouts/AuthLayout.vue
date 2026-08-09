<template>
  <div class="relative grid min-h-full md:grid-cols-2">
    <LanguagePicker class="absolute top-6 right-6 z-10" />
    <SeasonSwitcher class="absolute right-6 bottom-6 z-10" />
    <!-- Artwork panel: seasonal splash. Collapses to a header strip below md. -->
    <div class="relative isolate flex min-h-36 items-end overflow-hidden p-8 md:min-h-0 md:p-12">
      <SeasonBackdrop />
      <div class="relative">
        <div class="font-[family-name:var(--font-display)] text-lg tracking-[0.08em] text-[var(--primary)]">
          ᚛ Quillit
        </div>
        <h1
          class="mt-3 font-[family-name:var(--font-display)] text-4xl font-medium tracking-[0.3em] text-[var(--foreground)] md:text-6xl"
        >
          WELCOME
        </h1>
        <p v-if="tagline" class="mt-3 max-w-sm text-[var(--muted-foreground)]">{{ tagline }}</p>
      </div>
    </div>

    <!-- Form panel: opaque, calm. -->
    <div class="flex items-center justify-center bg-[var(--background)] p-6 md:p-12">
      <div class="flex w-full max-w-md flex-col gap-8">
        <h2 class="font-[family-name:var(--font-display)] text-2xl font-medium text-[var(--foreground)]">
          {{ title }}
        </h2>
        <slot />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import SeasonBackdrop from '../components/SeasonBackdrop.vue'
import SeasonSwitcher from '../components/SeasonSwitcher.vue'
import LanguagePicker from '../components/LanguagePicker.vue'

defineProps<{ title: string; tagline?: string }>()
</script>
