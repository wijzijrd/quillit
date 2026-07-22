<template>
  <section class="ls-section">
    <div class="ls-header">
      <h2 class="ls-section-title">Live Session</h2>
      <button
        v-if="liveSession.status !== 'running'"
        class="ls-btn"
        :disabled="starting"
        @click="onStart"
      >Start session</button>
      <button
        v-else
        class="ls-btn ls-btn--stop"
        :disabled="stopping"
        @click="onStop"
      >Stop session</button>
    </div>

    <p class="ls-error" v-if="loadError">{{ loadError }}</p>

    <template v-if="liveSession.status === 'running'">
      <p class="ls-frame-error" v-if="liveSession.error">
        {{ liveSession.error }}
        <button class="ls-dismiss" @click="liveSession.clearError()" title="Dismiss">×</button>
      </p>

      <div class="ls-messages" ref="messagesEl">
        <p class="ls-empty" v-if="!liveSession.messages.length">No messages yet — say hello.</p>

        <div
          v-for="m in liveSession.messages"
          :key="m.id"
          class="ls-message"
          :class="{ 'ls-message--card': m.type === 'note_card' }"
        >
          <div v-if="m.type === 'note_card'" class="ls-card">
            <p class="ls-card-title">{{ m.cardTitle }}</p>
            <p class="ls-card-body">{{ preview(m.cardBody) }}</p>
            <div class="ls-card-actions">
              <button class="ls-save-btn" @click="toggleFolderPicker(m.id)">Save to folder…</button>
              <div class="ls-folder-picker" v-if="folderPickerFor === m.id">
                <select class="ls-folder-select" v-model="selectedFolderId">
                  <option value="" disabled>Choose a folder…</option>
                  <option v-for="f in member.folders" :key="f.id" :value="f.id">{{ f.name }}</option>
                </select>
                <button
                  class="ls-save-confirm"
                  :disabled="!selectedFolderId || saving"
                  @click="saveCard(m)"
                >Save</button>
              </div>
            </div>
          </div>
          <span v-else class="ls-message-body">{{ m.body }}</span>
        </div>
      </div>

      <form class="ls-compose" @submit.prevent="onSend">
        <input
          class="ls-input"
          v-model="draft"
          placeholder="Message the table…"
          autocomplete="off"
        />
        <button class="ls-btn" type="submit" :disabled="!draft.trim()">Send</button>
      </form>
    </template>
  </section>
</template>

<script setup lang="ts">
import { ref, watch, onMounted, nextTick } from 'vue'
import { useLiveSessionStore } from '../stores/useLiveSessionStore'
import { useMemberStore } from '../stores/useMemberStore'
import { useEntriesStore } from '../stores/useEntriesStore'
import type { ChatMessage, Entry } from '../types'

const props = defineProps<{ projectId: string }>()

const liveSession = useLiveSessionStore()
const member = useMemberStore()
const entries = useEntriesStore()

const loadError = ref('')
const starting = ref(false)
const stopping = ref(false)
const draft = ref('')
const messagesEl = ref<HTMLElement | null>(null)
const folderPickerFor = ref<string | null>(null)
const selectedFolderId = ref('')
const saving = ref(false)

onMounted(async () => {
  member.fetchFolders().catch(() => {})
  await refreshSession()
})

// Deliberately no onUnmounted disconnect here: the live session socket is
// app-global (one Pinia store instance), and other views — e.g. the notes
// editor's "push to session chat" button — may still need it open after the
// user navigates away from this panel. The store itself tears the socket
// down on explicit stop() or a server-sent session_ended frame.

async function refreshSession() {
  loadError.value = ''
  try {
    await liveSession.fetchStatus(props.projectId)
    if (liveSession.status === 'running' && liveSession.sessionId) {
      await liveSession.fetchHistory(props.projectId, liveSession.sessionId)
      liveSession.connect(props.projectId)
    }
  } catch (e: any) {
    loadError.value = e?.data?.error ?? 'Could not load session status'
  }
}

watch(() => liveSession.messages.length, () => {
  nextTick(() => {
    if (messagesEl.value) messagesEl.value.scrollTop = messagesEl.value.scrollHeight
  })
})

async function onStart() {
  loadError.value = ''
  starting.value = true
  try {
    await liveSession.start(props.projectId)
    if (liveSession.sessionId) {
      await liveSession.fetchHistory(props.projectId, liveSession.sessionId)
    }
    liveSession.connect(props.projectId)
  } catch (e: any) {
    loadError.value = e?.data?.error ?? 'Could not start session'
  } finally {
    starting.value = false
  }
}

async function onStop() {
  loadError.value = ''
  stopping.value = true
  try {
    await liveSession.stop(props.projectId)
  } catch (e: any) {
    loadError.value = e?.data?.error ?? 'Could not stop session'
  } finally {
    stopping.value = false
  }
}

function onSend() {
  if (!draft.value.trim()) return
  liveSession.sendText(draft.value.trim())
  draft.value = ''
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
  loadError.value = ''

  // Copy-on-save: a fresh entry seeded from the card snapshot, not a live
  // link back to the shared entry.
  let entry: Entry | null = null
  try {
    entry = await entries.createEntry()
    await entries.updateEntry(entry.id, { title: m.cardTitle, body: m.cardBody })
    await member.addToFolder(selectedFolderId.value, entry.id)
    folderPickerFor.value = null
  } catch (e: any) {
    loadError.value = e?.data?.error ?? 'Could not save note to folder'
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
.ls-section { display: flex; flex-direction: column; gap: var(--space-md); }
.ls-header { display: flex; align-items: center; justify-content: space-between; gap: var(--space-md); }
.ls-section-title { font-family: var(--font-display); font-size: var(--text-lg); font-weight: 400; color: var(--foreground); margin: 0; }

.ls-btn {
  height: var(--h-md); padding: 0 var(--space-md); background: var(--secondary);
  border: none; border-radius: var(--radius); color: var(--primary);
  font-family: var(--font-body); font-size: var(--text-md); cursor: pointer;
  transition: background var(--transition); white-space: nowrap;
}
.ls-btn:hover:not(:disabled) { background: var(--primary); color: var(--background); }
.ls-btn:disabled { opacity: 0.5; cursor: default; }
.ls-btn--stop { background: var(--muted); color: var(--destructive); }
.ls-btn--stop:hover:not(:disabled) { background: var(--destructive); color: var(--background); }

.ls-error { font-size: var(--text-sm); color: var(--destructive); margin: 0; }
.ls-empty { font-size: var(--text-sm); color: var(--muted-foreground); margin: 0; }

.ls-frame-error {
  display: flex; align-items: center; justify-content: space-between; gap: var(--space-sm);
  font-size: var(--text-sm); color: var(--destructive); margin: 0;
  background: color-mix(in srgb, var(--destructive) 10%, transparent);
  border: 1px solid var(--destructive); border-radius: var(--radius);
  padding: var(--space-xs) var(--space-sm);
}
.ls-dismiss {
  background: none; border: none; color: var(--destructive); cursor: pointer;
  font-size: 1em; line-height: 1; padding: 0 4px;
}

.ls-messages {
  display: flex; flex-direction: column; gap: var(--space-xs);
  max-height: 360px; overflow-y: auto;
  background: var(--muted); border-radius: var(--radius);
  padding: var(--space-sm);
}

.ls-message { padding: var(--space-xs) var(--space-sm); }
.ls-message-body { font-size: var(--text-md); color: var(--foreground); white-space: pre-wrap; }

.ls-message--card { padding: 0; }
.ls-card {
  display: flex; flex-direction: column; gap: 4px;
  background: var(--card); border: 1px solid var(--secondary); border-radius: var(--radius);
  padding: var(--space-sm);
}
.ls-card-title { font-family: var(--font-display); font-size: var(--text-md); color: var(--foreground); margin: 0; font-weight: 500; }
.ls-card-body { font-size: var(--text-sm); color: var(--muted-foreground); margin: 0; }
.ls-card-actions { display: flex; flex-direction: column; gap: var(--space-xs); margin-top: 4px; }
.ls-save-btn {
  align-self: flex-start; background: none; border: 1px solid var(--border);
  border-radius: var(--radius); color: var(--primary); font-family: var(--font-body);
  font-size: var(--text-xs); padding: 4px 10px; cursor: pointer; transition: background var(--transition);
}
.ls-save-btn:hover { background: var(--muted); }
.ls-folder-picker { display: flex; gap: var(--space-xs); }
.ls-folder-select {
  flex: 1; background: var(--muted); border: 1px solid var(--border);
  border-radius: var(--radius); color: var(--foreground); font-family: var(--font-body);
  font-size: var(--text-xs); height: var(--h-sm); padding: 0 var(--space-xs);
}
.ls-save-confirm {
  height: var(--h-sm); padding: 0 var(--space-sm); background: var(--secondary);
  border: none; border-radius: var(--radius); color: var(--primary);
  font-size: var(--text-xs); cursor: pointer; transition: background var(--transition);
}
.ls-save-confirm:hover:not(:disabled) { background: var(--primary); color: var(--background); }
.ls-save-confirm:disabled { opacity: 0.5; cursor: default; }

.ls-compose { display: flex; gap: var(--space-sm); }
.ls-input {
  flex: 1; background: var(--muted); border: 1px solid var(--border);
  border-radius: var(--radius); color: var(--foreground); font-family: var(--font-body);
  font-size: var(--text-md); height: var(--h-md); padding: 0 var(--space-sm); outline: none;
  transition: border-color var(--transition); box-sizing: border-box;
}
.ls-input:focus { border-color: var(--secondary); }
</style>
