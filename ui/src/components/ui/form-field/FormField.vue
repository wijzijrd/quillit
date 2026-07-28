<script setup lang="ts">
import type { HTMLAttributes } from "vue"
import type { Validate, ValidationResult } from "."
import { onUnmounted, ref, watch } from "vue"
import { cn } from "@/lib/utils"

const props = defineProps<{
  label?: string
  for?: string
  required?: boolean
  modelValue?: string | number
  validate?: Validate
  debounceMs?: number
  class?: HTMLAttributes["class"]
}>()

const touched = ref(false)
const result = ref<ValidationResult | null>(null)
let debounceTimer: ReturnType<typeof setTimeout> | undefined
let seq = 0

async function runValidate(value: string | number) {
  if (!props.validate) return
  const mySeq = ++seq
  try {
    const outcome = (await props.validate(value)) ?? null
    if (mySeq === seq) result.value = outcome
  } catch {
    if (mySeq === seq) result.value = null
  }
}

async function validateNow(value: string | number) {
  touched.value = true
  if (debounceTimer) clearTimeout(debounceTimer)
  await runValidate(value)
  return result.value
}

defineExpose({ validateNow })

watch(
  () => props.modelValue,
  (newValue) => {
    if (!props.validate || newValue === undefined) return
    touched.value = true
    if (debounceTimer) clearTimeout(debounceTimer)
    if (props.debounceMs && props.debounceMs > 0) {
      debounceTimer = setTimeout(() => runValidate(newValue), props.debounceMs)
    } else {
      runValidate(newValue)
    }
  },
)

onUnmounted(() => {
  if (debounceTimer) clearTimeout(debounceTimer)
})
</script>

<template>
  <div :class="cn('flex flex-col gap-2', props.class)">
    <label v-if="label" :for="props.for" class="auth-label">
      {{ label }}<span v-if="required" class="text-[var(--destructive)]"> *</span>
    </label>
    <slot />
    <p
      v-if="touched && result"
      class="text-xs"
      :class="result.type === 'success' ? 'text-[var(--primary)]' : 'text-[var(--destructive)]'"
    >
      {{ result.message }}
    </p>
  </div>
</template>
