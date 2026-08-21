<template>
  <div class="cml-messages" ref="messagesEl">
    <p class="cml-empty" v-if="!messages.length">{{ emptyText ?? 'No messages yet — say hello.' }}</p>

    <div
      v-for="m in messages"
      :key="m.id"
      class="cml-message"
      :class="{ 'cml-message--card': m.type === 'note_card' }"
    >
      <span class="cml-sender" v-if="senderName(m)">{{ senderName(m) }}</span>
      <div v-if="m.type === 'note_card'" class="cml-card">
        <p class="cml-card-title">{{ m.cardTitle }}</p>
        <p class="cml-card-body">{{ preview(m.cardBody) }}</p>
      </div>
      <span v-else class="cml-message-body">{{ m.body }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, nextTick } from 'vue'
import type { ChatMessage } from '../types'

const props = defineProps<{
  messages: ChatMessage[]
  /** Maps senderId → display name; unmapped senders render without a name line. */
  senderNames?: Record<string, string>
  emptyText?: string
}>()

const emit = defineEmits<{ error: [message: string] }>()

const messagesEl = ref<HTMLElement | null>(null)

watch(() => props.messages.length, () => {
  nextTick(() => {
    if (messagesEl.value) messagesEl.value.scrollTop = messagesEl.value.scrollHeight
  })
})

function senderName(m: ChatMessage): string {
  return props.senderNames?.[m.senderId] ?? ''
}

function preview(body: string): string {
  const stripped = body.replace(/<[^>]+>/g, '')
  return stripped.length > 160 ? stripped.slice(0, 160) + '…' : stripped
}
</script>

<style scoped>
.cml-messages {
  display: flex; flex-direction: column; gap: var(--space-xs);
  max-height: 360px; overflow-y: auto;
  background: var(--muted); border-radius: var(--radius);
  padding: var(--space-sm);
}

.cml-empty { font-size: var(--text-sm); color: var(--muted-foreground); margin: 0; }

.cml-message { padding: var(--space-xs) var(--space-sm); }
.cml-sender {
  display: block; font-size: var(--text-xs); color: var(--muted-foreground);
  margin-bottom: 2px;
}
.cml-message-body { font-size: var(--text-md); color: var(--foreground); white-space: pre-wrap; }

.cml-message--card { padding: var(--space-xs) 0 0; }
.cml-message--card .cml-sender { padding: 0 var(--space-sm); }
.cml-card {
  display: flex; flex-direction: column; gap: 4px;
  background: var(--card); border: 1px solid var(--secondary); border-radius: var(--radius);
  padding: var(--space-sm);
}
.cml-card-title { font-family: var(--font-display); font-size: var(--text-md); color: var(--foreground); margin: 0; font-weight: 500; }
.cml-card-body { font-size: var(--text-sm); color: var(--muted-foreground); margin: 0; }
</style>
