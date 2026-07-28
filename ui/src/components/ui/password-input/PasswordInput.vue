<script setup lang="ts">
import type { HTMLAttributes } from "vue"
import { ref } from "vue"
import { Eye, EyeOff } from "lucide-vue-next"
import { useVModel } from "@vueuse/core"
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from "@/components/ui/input-group"

defineOptions({ inheritAttrs: false })

const props = defineProps<{
  defaultValue?: string | number
  modelValue?: string | number
  class?: HTMLAttributes["class"]
}>()

const emits = defineEmits<{
  (e: "update:modelValue", payload: string | number): void
}>()

const modelValue = useVModel(props, "modelValue", emits, {
  passive: true,
  defaultValue: props.defaultValue,
})

const visible = ref(false)
</script>

<template>
  <InputGroup :class="props.class">
    <InputGroupInput
      v-bind="$attrs"
      v-model="modelValue"
      :type="visible ? 'text' : 'password'"
    />
    <InputGroupAddon align="inline-end">
      <InputGroupButton
        type="button"
        size="icon-xs"
        aria-label="Toggle password visibility"
        @click="visible = !visible"
      >
        <EyeOff v-if="visible" class="size-4" />
        <Eye v-else class="size-4" />
      </InputGroupButton>
    </InputGroupAddon>
  </InputGroup>
</template>
