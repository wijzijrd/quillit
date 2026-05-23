<template>
  <div class="share-panel">
    <div class="share-section">
      <p class="share-hint">Search users by name or email to share this note.</p>
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
  </div>
</template>

<script setup>
import { ref, watch, onMounted } from 'vue'
import { useMemberStore } from '../stores/useMemberStore.js'

const props = defineProps({ entryId: String })
const member = useMemberStore()

const query = ref('')
const results = ref([])
const shares = ref([])
const searched = ref(false)
let searchTimer = null

onMounted(loadShares)

watch(() => props.entryId, () => {
  query.value = ''
  results.value = []
  searched.value = false
  loadShares()
})

async function loadShares() {
  if (!props.entryId) return
  shares.value = await member.fetchEntryShares(props.entryId)
}

function onSearch() {
  clearTimeout(searchTimer)
  if (query.value.length < 2) { results.value = []; searched.value = false; return }
  searchTimer = setTimeout(runSearch, 280)
}

async function runSearch() {
  results.value = await member.searchUsers(query.value)
  searched.value = true
}

function alreadyShared(userId) {
  return shares.value.some(s => s.userId === userId)
}

async function addUser(u) {
  await member.addShares(props.entryId, [u.id])
  shares.value = await member.fetchEntryShares(props.entryId)
}

async function removeUser(userId) {
  await member.removeShare(props.entryId, userId)
  shares.value = shares.value.filter(s => s.userId !== userId)
}
</script>

<style scoped>
.share-panel {
  display: flex; flex-direction: column; gap: var(--space-lg);
  padding: var(--space-md); overflow-y: auto; flex: 1;
}

.share-section { display: flex; flex-direction: column; gap: var(--space-xs); }

.share-label {
  font-size: var(--text-xs); text-transform: uppercase; letter-spacing: 0.08em;
  color: var(--text-faint); margin: 0;
}
.share-hint {
  font-size: var(--text-xs); color: var(--text-faint); margin: 0;
}

.search-row { display: flex; gap: var(--space-xs); }
.share-input {
  flex: 1; background: var(--bg-raised); border: 1px solid var(--border-light);
  border-radius: var(--radius); color: var(--text-primary);
  font-family: var(--font-body); font-size: var(--text-sm);
  height: var(--h-sm); padding: 0 var(--space-sm); outline: none;
  transition: border-color var(--transition);
}
.share-input:focus { border-color: var(--accent-dim); }

.search-results { display: flex; flex-direction: column; gap: 2px; }
.result-row {
  display: flex; align-items: center; gap: var(--space-sm);
  padding: 6px var(--space-sm); border-radius: var(--radius);
  background: none; border: none; cursor: pointer; text-align: left; width: 100%;
  transition: background var(--transition);
}
.result-row:hover:not(:disabled) { background: var(--bg-hover); }
.result-row:disabled { opacity: 0.5; cursor: default; }
.result-name { font-size: var(--text-sm); color: var(--text-primary); flex: 1; }
.result-email { font-size: var(--text-xs); color: var(--text-faint); }
.result-added { font-size: var(--text-xs); color: var(--accent); }

.share-list { display: flex; flex-direction: column; gap: 2px; }
.share-row {
  display: flex; align-items: center; justify-content: space-between;
  padding: 5px var(--space-sm); border-radius: var(--radius);
  background: var(--bg-raised);
}
.share-username { font-size: var(--text-sm); color: var(--text-primary); }
.remove-btn {
  background: none; border: none; color: var(--text-faint); cursor: pointer;
  font-size: 1em; line-height: 1; padding: 0 4px;
  transition: color var(--transition);
}
.remove-btn:hover { color: var(--danger); }

.empty-hint { font-size: var(--text-xs); color: var(--text-faint); margin: 0; }
</style>
