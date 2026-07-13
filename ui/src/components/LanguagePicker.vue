<template>
  <div class="lang-picker">
    <TooltipProvider :delay-duration="150">
      <Tooltip>
        <TooltipTrigger as-child>
          <!-- span wrapper so the tooltip fires despite the disabled button -->
          <span class="lang-wrap">
            <button class="lang-btn" disabled aria-disabled="true" aria-label="Language: English">
              <Languages :size="15" />
              <span>EN</span>
            </button>
          </span>
        </TooltipTrigger>
        <TooltipContent side="bottom">English only — more languages coming</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  </div>
</template>

<script setup lang="ts">
import { Languages } from 'lucide-vue-next'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from './ui/tooltip'
</script>

<style scoped>
.lang-wrap { display: inline-flex; cursor: not-allowed; }
.lang-btn {
  display: flex; align-items: center; gap: 6px;
  padding: 6px 12px;
  border-radius: 99px;
  background: var(--popover);
  border: 1px solid var(--border);
  color: var(--muted-foreground);
  font-family: var(--font-body);
  font-size: 0.8em;
  font-weight: 500;
  letter-spacing: 0.06em;
  opacity: 0.7;
  pointer-events: none;
}
</style>
