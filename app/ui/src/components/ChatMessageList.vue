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
        <div class="cml-card-actions">
          <button class="cml-save-btn" @click="toggleFolderPicker(m.id)">Save to folder…</button>
          <div class="cml-folder-picker" v-if="folderPickerFor === m.id">
            <select class="cml-folder-select" v-model="selectedFolderId">
              <option value="" disabled>Choose a folder…</option>
              <option v-for="f in member.folders" :key="f.id" :value="f.id">{{ f.name }}</option>
            </select>
            <button
              class="cml-save-confirm"
              :disabled="!selectedFolderId || saving"
              @click="saveCard(m)"
            >Save</button>
          </div>
        </div>
      </div>
      <span v-else class="cml-message-body">{{ m.body }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onMounted, nextTick } from 'vue'
import { useMemberStore } from '../stores/useMemberStore'
import { useEntriesStore } from '../stores/useEntriesStore'
import { apiErrorMessage } from '../api/client'
import type { ChatMessage, Entry } from '../types'

const props = defineProps<{
  messages: ChatMessage[]
  /** Maps senderId → display name; unmapped senders render without a name line. */
  senderNames?: Record<string, string>
  emptyText?: string
}>()

const emit = defineEmits<{ error: [message: string] }>()

const member = useMemberStore()
const entries = useEntriesStore()

const messagesEl = ref<HTMLElement | null>(null)
const folderPickerFor = ref<string | null>(null)
const selectedFolderId = ref('')
const saving = ref(false)

onMounted(() => {
  member.fetchFolders().catch(() => {})
})

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

function toggleFolderPicker(messageId: string) {
  folderPickerFor.value = folderPickerFor.value === messageId ? null : messageId
  selectedFolderId.value = ''
}

async function saveCard(m: ChatMessage) {
  if (!selectedFolderId.value) return
  saving.value = true

  // Copy-on-save: a fresh entry seeded from the card snapshot, not a live
  // link back to the shared entry.
  let entry: Entry | null = null
  try {
    entry = await entries.createEntry()
    await entries.updateEntry(entry.id, { title: m.cardTitle, body: m.cardBody })
    await member.addToFolder(selectedFolderId.value, entry.id)
    folderPickerFor.value = null
  } catch (e: unknown) {
    emit('error', apiErrorMessage(e, 'Could not save note to folder'))
    // createEntry already succeeded but a later step failed — roll back the
    // orphaned blank/partial entry so it doesn't silently persist. Failure
    // to roll back shouldn't mask the original error above.
    if (entry) {
      try {
        await entries.deleteEntry(entry.id)
      } catch (rollbackErr) {
        console.error('Failed to roll back orphaned entry after save-to-folder failure', rollbackErr)
      }
    }
  } finally {
    saving.value = false
  }
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
.cml-card-actions { display: flex; flex-direction: column; gap: var(--space-xs); margin-top: 4px; }
.cml-save-btn {
  align-self: flex-start; background: none; border: 1px solid var(--border);
  border-radius: var(--radius); color: var(--primary); font-family: var(--font-body);
  font-size: var(--text-xs); padding: 4px 10px; cursor: pointer; transition: background var(--transition);
}
.cml-save-btn:hover { background: var(--muted); }
.cml-folder-picker { display: flex; gap: var(--space-xs); }
.cml-folder-select {
  flex: 1; background: var(--muted); border: 1px solid var(--border);
  border-radius: var(--radius); color: var(--foreground); font-family: var(--font-body);
  font-size: var(--text-xs); height: var(--h-sm); padding: 0 var(--space-xs);
}
.cml-save-confirm {
  height: var(--h-sm); padding: 0 var(--space-sm); background: var(--secondary);
  border: none; border-radius: var(--radius); color: var(--primary);
  font-size: var(--text-xs); cursor: pointer; transition: background var(--transition);
}
.cml-save-confirm:hover:not(:disabled) { background: var(--primary); color: var(--background); }
.cml-save-confirm:disabled { opacity: 0.5; cursor: default; }
</style>
