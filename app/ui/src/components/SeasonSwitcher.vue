<template>
  <div class="season-switcher" role="radiogroup" aria-label="Season">
    <TooltipProvider :delay-duration="150">
      <Tooltip v-for="s in seasons" :key="s.name">
        <TooltipTrigger as-child>
          <button
            class="season-btn"
            :class="{ active: ui.season === s.name }"
            role="radio"
            :aria-checked="ui.season === s.name"
            :aria-label="s.name"
            @click="ui.setSeason(s.name)"
          >
            <component :is="s.icon" :size="16" />
          </button>
        </TooltipTrigger>
        <TooltipContent side="top" class="capitalize">{{ s.name }}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  </div>
</template>

<script setup lang="ts">
import { Flower2, Sun, Leaf, Snowflake } from 'lucide-vue-next'
import { useUIStore } from '../stores/useUIStore'
import type { Season } from '../types'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from './ui/tooltip'

const seasons: { name: Season; icon: typeof Sun }[] = [
  { name: 'spring', icon: Flower2 },
  { name: 'summer', icon: Sun },
  { name: 'autumn', icon: Leaf },
  { name: 'winter', icon: Snowflake },
]
const ui = useUIStore()
</script>

<style scoped>
.season-switcher { display: flex; gap: 8px; }
.season-btn {
  display: flex; align-items: center; justify-content: center;
  width: 34px; height: 34px;
  border-radius: 50%;
  background: var(--popover);
  border: 1px solid var(--border);
  color: var(--muted-foreground);
  cursor: pointer;
  transition: border-color var(--transition), color var(--transition), background var(--transition);
}
.season-btn:hover { border-color: var(--primary); color: var(--foreground); }
.season-btn.active {
  border-color: var(--primary);
  color: var(--primary);
  background: color-mix(in srgb, var(--primary) 12%, var(--popover));
}
</style>
