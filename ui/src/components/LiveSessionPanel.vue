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

      <ChatMessageList
        :messages="liveSession.messages"
        :sender-names="senderNames"
        @error="loadError = $event"
      />

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
import { ref, computed, onMounted } from 'vue'
import { useLiveSessionStore } from '../stores/useLiveSessionStore'
import { useProjectStore } from '../stores/useProjectStore'
import { apiErrorMessage } from '../api/client'
import ChatMessageList from './ChatMessageList.vue'

const props = defineProps<{ projectId: string }>()

const liveSession = useLiveSessionStore()
const projectStore = useProjectStore()

const loadError = ref('')
const starting = ref(false)
const stopping = ref(false)
const draft = ref('')

const senderNames = computed<Record<string, string>>(() => {
  const map: Record<string, string> = {}
  for (const m of projectStore.membersCache[props.projectId] ?? []) {
    map[m.userId] = m.username
  }
  return map
})

onMounted(async () => {
  projectStore.fetchMembers(props.projectId).catch(() => {})
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
  } catch (e: unknown) {
    loadError.value = apiErrorMessage(e, 'Could not load session status')
  }
}

async function onStart() {
  loadError.value = ''
  starting.value = true
  try {
    await liveSession.start(props.projectId)
    if (liveSession.sessionId) {
      await liveSession.fetchHistory(props.projectId, liveSession.sessionId)
    }
    liveSession.connect(props.projectId)
  } catch (e: unknown) {
    loadError.value = apiErrorMessage(e, 'Could not start session')
  } finally {
    starting.value = false
  }
}

async function onStop() {
  loadError.value = ''
  stopping.value = true
  try {
    await liveSession.stop(props.projectId)
  } catch (e: unknown) {
    loadError.value = apiErrorMessage(e, 'Could not stop session')
  } finally {
    stopping.value = false
  }
}

function onSend() {
  if (!draft.value.trim()) return
  liveSession.sendText(draft.value.trim())
  draft.value = ''
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

.ls-compose { display: flex; gap: var(--space-sm); }
.ls-input {
  flex: 1; background: var(--muted); border: 1px solid var(--border);
  border-radius: var(--radius); color: var(--foreground); font-family: var(--font-body);
  font-size: var(--text-md); height: var(--h-md); padding: 0 var(--space-sm); outline: none;
  transition: border-color var(--transition); box-sizing: border-box;
}
.ls-input:focus { border-color: var(--secondary); }
</style>
