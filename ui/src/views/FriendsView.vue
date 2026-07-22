<template>
  <div class="friends-view">
    <header class="fv-header">
      <h1 class="fv-name">Friends</h1>
    </header>

    <!-- Find someone to add -->
    <section class="fv-section">
      <h2 class="fv-section-title">Add a Friend</h2>
      <p class="fv-hint">Search users by name or email to send a friend request.</p>
      <div class="search-wrap">
        <input
          class="search-input"
          v-model="query"
          placeholder="Search users…"
          @input="onSearch"
          autocomplete="off"
        />
        <div class="search-dropdown" v-if="results.length">
          <button
            v-for="u in results"
            :key="u.id"
            class="search-result"
            :disabled="alreadyRequested(u.id)"
            @click="addFriend(u)"
          >
            <span class="sr-username">{{ u.username }}</span>
            <span class="sr-email">{{ u.email }}</span>
            <span class="sr-status" v-if="alreadyRequested(u.id)">{{ requestStatusLabel(u.id) }}</span>
          </button>
        </div>
      </div>
      <p class="fv-empty" v-if="searched && query.length >= 2 && !results.length">No users found.</p>
    </section>

    <!-- Incoming requests -->
    <section class="fv-section">
      <h2 class="fv-section-title">Incoming Requests</h2>
      <div class="fv-list" v-if="friends.incoming.length">
        <div class="fv-row" v-for="r in friends.incoming" :key="r.id">
          <span class="fv-identity">{{ r.requesterUsername }}</span>
          <div class="fv-actions">
            <button class="fv-btn fv-btn-accept" @click="accept(r.id)">Accept</button>
            <button class="fv-btn fv-btn-decline" @click="remove(r.id)">Decline</button>
          </div>
        </div>
      </div>
      <p class="fv-empty" v-else>No incoming requests.</p>
    </section>

    <!-- Outgoing requests -->
    <section class="fv-section">
      <h2 class="fv-section-title">Outgoing Requests</h2>
      <div class="fv-list" v-if="friends.outgoing.length">
        <div class="fv-row" v-for="r in friends.outgoing" :key="r.id">
          <span class="fv-identity">{{ r.addresseeUsername }}</span>
          <div class="fv-actions">
            <button class="fv-btn" @click="remove(r.id)">Cancel</button>
          </div>
        </div>
      </div>
      <p class="fv-empty" v-else>No outgoing requests.</p>
    </section>

    <!-- Friends -->
    <section class="fv-section">
      <h2 class="fv-section-title">Friends</h2>
      <div class="fv-list" v-if="friends.friends.length">
        <div class="fv-row" v-for="f in friends.friends" :key="f.id">
          <span class="fv-identity">{{ f.username }}</span>
          <div class="fv-actions">
            <button class="fv-btn" @click="remove(f.id)">Unfriend</button>
          </div>
        </div>
      </div>
      <p class="fv-empty" v-else>No friends yet.</p>
    </section>

    <div v-if="errorMessage" class="fv-toast">{{ errorMessage }}</div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useFriendsStore } from '../stores/useFriendsStore'
import { useMemberStore } from '../stores/useMemberStore'

const friends = useFriendsStore()
const member = useMemberStore()

const query = ref('')
const results = ref<any[]>([])
const searched = ref(false)
const errorMessage = ref('')
let searchTimer: ReturnType<typeof setTimeout> | null = null
let errorTimer: ReturnType<typeof setTimeout> | null = null

onMounted(() => {
  friends.init().catch(() => showError('Could not load friends. Please try again.'))
})

function onSearch() {
  if (searchTimer) clearTimeout(searchTimer)
  if (query.value.length < 2) { results.value = []; searched.value = false; return }
  searchTimer = setTimeout(runSearch, 280)
}

async function runSearch() {
  results.value = await member.searchUsers(query.value)
  searched.value = true
}

function alreadyRequested(userId: string) {
  return friends.outgoing.some(r => r.addresseeId === userId) || friends.friends.some(f => f.userId === userId)
}

function requestStatusLabel(userId: string) {
  if (friends.friends.some(f => f.userId === userId)) return 'Friends'
  if (friends.outgoing.some(r => r.addresseeId === userId)) return 'Pending'
  return ''
}

async function addFriend(u: { id: string; username: string }) {
  try {
    await friends.sendRequest(u.id, u.username)
  } catch (e: any) {
    if (e?.status === 502 || e?.response?.status === 502) {
      showError('Something went wrong resolving your identity. Please try again.')
    } else {
      showError(e?.data?.error ?? 'Could not send friend request.')
    }
  }
}

async function accept(id: string) {
  try {
    await friends.acceptRequest(id)
  } catch (e: any) {
    showError(e?.data?.error ?? 'Could not accept request.')
  }
}

async function remove(id: string) {
  try {
    await friends.removeRequest(id)
  } catch (e: any) {
    showError(e?.data?.error ?? 'Could not complete that action.')
  }
}

function showError(message: string) {
  errorMessage.value = message
  if (errorTimer) clearTimeout(errorTimer)
  errorTimer = setTimeout(() => { errorMessage.value = '' }, 3000)
}
</script>

<style scoped>
.friends-view {
  padding: var(--space-2xl) var(--space-xl);
  display: flex;
  flex-direction: column;
  gap: var(--space-2xl);
}
.fv-header { display: flex; flex-direction: column; gap: var(--space-sm); }
.fv-name { font-family: var(--font-display); font-size: var(--text-2xl); color: var(--foreground); font-weight: 400; margin: 0; }

.fv-section { display: flex; flex-direction: column; gap: var(--space-md); }
.fv-section-title { font-family: var(--font-display); font-size: var(--text-lg); font-weight: 400; color: var(--foreground); margin: 0; }
.fv-hint { font-size: var(--text-xs); color: var(--muted-foreground); margin: 0; }

.search-wrap { position: relative; }
.search-input {
  width: 100%; background: var(--muted); border: 1px solid var(--border);
  border-radius: var(--radius); color: var(--foreground); font-family: var(--font-body);
  font-size: var(--text-md); height: var(--h-md); padding: 0 var(--space-sm); outline: none;
  transition: border-color var(--transition); box-sizing: border-box;
}
.search-input:focus { border-color: var(--secondary); }
.search-dropdown {
  position: absolute; top: 100%; left: 0; right: 0; z-index: 10;
  background: var(--card); border: 1px solid var(--border);
  border-radius: var(--radius); margin-top: 2px; overflow: hidden;
}
.search-result {
  display: flex; align-items: center; justify-content: space-between;
  width: 100%; padding: var(--space-xs) var(--space-sm); gap: var(--space-sm);
  background: none; border: none; cursor: pointer; text-align: left;
  transition: background var(--transition);
}
.search-result:hover:not(:disabled) { background: var(--muted); }
.search-result:disabled { opacity: 0.5; cursor: default; }
.sr-username { font-size: var(--text-md); color: var(--foreground); }
.sr-email { font-size: var(--text-xs); color: var(--muted-foreground); }
.sr-status { font-size: var(--text-xs); color: var(--primary); }

.fv-list { display: flex; flex-direction: column; gap: var(--space-xs); }
.fv-row {
  display: flex; align-items: center; justify-content: space-between; gap: var(--space-sm);
  padding: var(--space-xs) var(--space-sm);
  background: var(--muted); border-radius: var(--radius);
}
.fv-identity { flex: 1; font-size: var(--text-md); color: var(--foreground); }
.fv-actions { display: flex; gap: var(--space-xs); }
.fv-btn {
  font-size: var(--text-xs); color: var(--foreground); background: none;
  border: 1px solid var(--border); cursor: pointer; padding: 2px 8px; border-radius: var(--radius);
  transition: background var(--transition);
}
.fv-btn:hover { background: var(--card); }
.fv-btn-accept { color: var(--primary); border-color: var(--secondary); }
.fv-btn-decline { color: var(--destructive); }
.fv-empty { font-size: var(--text-sm); color: var(--muted-foreground); margin: 0; }

.fv-toast {
  position: fixed;
  bottom: var(--space-xl);
  left: 50%;
  transform: translateX(-50%);
  background: var(--muted);
  border: 1px solid var(--destructive);
  border-radius: var(--radius);
  color: var(--foreground);
  font-size: var(--text-sm);
  padding: var(--space-xs) var(--space-md);
  pointer-events: none;
}
</style>
