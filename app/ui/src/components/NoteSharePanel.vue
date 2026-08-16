<template>
  <div class="share-panel">
    <p class="share-error" v-if="shareError">{{ shareError }}</p>

    <div class="share-section">
      <p class="share-hint">Search users by name or email to share this entry.</p>
      <div class="search-row">
        <input
          class="share-input"
          v-model="query"
          placeholder="Search users…"
          @input="onSearch"
          autocomplete="off"
        />
      </div>
      <div class="search-results" v-if="results.length > 0">
        <button
          v-for="u in results"
          :key="u.id"
          class="result-row"
          :disabled="alreadyShared(u.id)"
          @click="addUser(u)"
        >
          <span class="result-name">{{ u.username }}</span>
          <span class="result-email">{{ u.email }}</span>
          <span class="result-added" v-if="alreadyShared(u.id)">Shared</span>
        </button>
      </div>
      <p class="empty-hint" v-else-if="searched && query.length >= 2">No users found.</p>
    </div>

    <div class="share-section">
      <p class="share-label">Shared with</p>
      <div class="share-list" v-if="shares.length > 0">
        <div class="share-row" v-for="s in shares" :key="s.userId">
          <span class="share-username">{{ s.username ?? s.userId }}</span>
          <button class="remove-btn" @click="removeUser(s.userId)" title="Remove access">×</button>
        </div>
      </div>
      <p class="empty-hint" v-else>Not shared with anyone yet.</p>
    </div>

    <div class="share-section" v-if="props.projectId">
      <p class="share-label">Session Chat</p>
      <button
        class="push-btn"
        :disabled="!sessionRunning"
        @click="pushToChat"
      >Push to session chat</button>
      <p class="empty-hint" v-if="!sessionRunning">No live session running for this project.</p>
      <p class="empty-hint push-confirm" v-else-if="pushed">Shared to the session chat.</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onMounted, computed } from 'vue'
import { useMemberStore } from '../stores/useMemberStore'
import { useLiveSessionStore } from '../stores/useLiveSessionStore'
import { apiErrorMessage } from '../api/client'

const props = defineProps<{ entryId?: string; projectId?: string }>()
const member = useMemberStore()
const liveSession = useLiveSessionStore()

const pushed = ref(false)
const sessionRunning = computed(() => liveSession.status === 'running')

watch(() => props.projectId, async (id) => {
  if (!id) return
  try {
    await liveSession.fetchStatus(id)
    // Ensure the chat socket is open so a push works even if the user never
    // visited the project's Live Session panel this session (e.g. they came
    // straight to the entry editor).
    if (liveSession.status === 'running') liveSession.connect(id)
  } catch { /* status check failed — push stays disabled */ }
}, { immediate: true })

function pushToChat() {
  if (!props.entryId || !sessionRunning.value) return
  liveSession.shareEntry(props.entryId)
  pushed.value = true
  setTimeout(() => { pushed.value = false }, 2000)
}

const query = ref('')
const results = ref<any[]>([])
const shares = ref<any[]>([])
const searched = ref(false)
const shareError = ref('')
let searchTimer: ReturnType<typeof setTimeout> | null = null

onMounted(loadShares)

watch(() => props.entryId, () => {
  query.value = ''
  results.value = []
  searched.value = false
  loadShares()
})

async function loadShares() {
  const entryId = props.entryId
  if (!entryId) return
  try {
    const fetched = await member.fetchEntryShares(entryId)
    if (props.entryId !== entryId) return
    shares.value = fetched
  } catch (e: unknown) {
    if (props.entryId !== entryId) return
    shareError.value = apiErrorMessage(e, 'Could not load shares')
  }
}

function onSearch() {
  if (searchTimer) clearTimeout(searchTimer)
  if (query.value.length < 2) { results.value = []; searched.value = false; return }
  searchTimer = setTimeout(runSearch, 280)
}

async function runSearch() {
  try {
    results.value = await member.searchUsers(query.value)
    searched.value = true
  } catch (e: unknown) {
    shareError.value = apiErrorMessage(e, 'Could not search users')
  }
}

function alreadyShared(userId: string) {
  return shares.value.some(s => s.userId === userId)
}

async function addUser(u: { id: string }) {
  try {
    await member.addShares(props.entryId, [u.id])
    shares.value = await member.fetchEntryShares(props.entryId)
  } catch (e: unknown) {
    shareError.value = apiErrorMessage(e, 'Could not add share')
  }
}

async function removeUser(userId: string) {
  await member.removeShare(props.entryId, userId)
  shares.value = shares.value.filter(s => s.userId !== userId)
}
</script>

<style scoped>
.share-panel {
  display: flex; flex-direction: column; gap: var(--space-lg);
  padding: var(--space-md); overflow-y: auto; flex: 1;
}

.share-error { font-size: var(--text-sm); color: var(--destructive); margin: 0; }

.share-section { display: flex; flex-direction: column; gap: var(--space-xs); }

.share-label {
  font-size: var(--text-xs); text-transform: uppercase; letter-spacing: 0.08em;
  color: var(--muted-foreground); margin: 0;
}
.share-hint {
  font-size: var(--text-xs); color: var(--muted-foreground); margin: 0;
}

.search-row { display: flex; gap: var(--space-xs); }
.share-input {
  flex: 1; background: var(--muted); border: 1px solid var(--border);
  border-radius: var(--radius); color: var(--foreground);
  font-family: var(--font-body); font-size: var(--text-sm);
  height: var(--h-sm); padding: 0 var(--space-sm); outline: none;
  transition: border-color var(--transition);
}
.share-input:focus { border-color: var(--secondary); }

.search-results { display: flex; flex-direction: column; gap: 2px; }
.result-row {
  display: flex; align-items: center; gap: var(--space-sm);
  padding: 6px var(--space-sm); border-radius: var(--radius);
  background: none; border: none; cursor: pointer; text-align: left; width: 100%;
  transition: background var(--transition);
}
.result-row:hover:not(:disabled) { background: var(--muted); }
.result-row:disabled { opacity: 0.5; cursor: default; }
.result-name { font-size: var(--text-sm); color: var(--foreground); flex: 1; }
.result-email { font-size: var(--text-xs); color: var(--muted-foreground); }
.result-added { font-size: var(--text-xs); color: var(--primary); }

.share-list { display: flex; flex-direction: column; gap: 2px; }
.share-row {
  display: flex; align-items: center; justify-content: space-between;
  padding: 5px var(--space-sm); border-radius: var(--radius);
  background: var(--muted);
}
.share-username { font-size: var(--text-sm); color: var(--foreground); }
.remove-btn {
  background: none; border: none; color: var(--muted-foreground); cursor: pointer;
  font-size: 1em; line-height: 1; padding: 0 4px;
  transition: color var(--transition);
}
.remove-btn:hover { color: var(--destructive); }

.empty-hint { font-size: var(--text-xs); color: var(--muted-foreground); margin: 0; }

.push-btn {
  align-self: flex-start; height: var(--h-sm); padding: 0 var(--space-sm);
  background: var(--secondary); border: none; border-radius: var(--radius);
  color: var(--primary); font-family: var(--font-body); font-size: var(--text-sm);
  cursor: pointer; transition: background var(--transition);
}
.push-btn:hover:not(:disabled) { background: var(--primary); color: var(--background); }
.push-btn:disabled { opacity: 0.5; cursor: default; }
.push-confirm { color: var(--primary); }
</style>
