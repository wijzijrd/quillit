<template>
  <div class="game-view">
    <header class="gv-header">
      <div>
        <h1 class="gv-title">Game Mode</h1>
        <p class="gv-subtitle" v-if="projectName">{{ projectName }}</p>
      </div>
      <RouterLink class="gv-back" :to="`/projects/${projectId}/notes`">Open notes</RouterLink>
    </header>

    <div class="gv-columns">
      <section class="gv-main">
        <LiveSessionPanel :project-id="projectId" />

        <section class="gv-history">
          <h2 class="gv-section-title">Past sessions</h2>
          <p class="gv-error" v-if="historyError">{{ historyError }}</p>
          <p class="gv-empty" v-else-if="!pastSessions.length">No past sessions yet.</p>

          <ul class="gv-session-list">
            <li v-for="s in pastSessions" :key="s.id">
              <button
                class="gv-session-row"
                :class="{ 'gv-session-row--open': openSessionId === s.id }"
                @click="toggleSession(s)"
              >
                <span class="gv-session-date">{{ formatDateTime(s.startedAt) }}</span>
                <span class="gv-session-meta">started by {{ memberName(s.startedBy) }}</span>
              </button>
              <div v-if="openSessionId === s.id" class="gv-session-chat">
                <p class="gv-empty" v-if="loadingMessages">Loading…</p>
                <ChatMessageList
                  v-else
                  :messages="openMessages"
                  :sender-names="senderNames"
                  empty-text="No messages in this session."
                  @error="historyError = $event"
                />
                <p class="gv-readonly-hint">This session has ended — chat is read-only.</p>
              </div>
            </li>
          </ul>
        </section>
      </section>

      <aside class="gv-members">
        <h2 class="gv-section-title">
          Members
          <span class="gv-online-count" v-if="liveSession.connected">{{ onlineCount }} in session</span>
        </h2>
        <ul class="gv-member-list">
          <li v-for="m in members" :key="m.userId" class="gv-member">
            <span class="gv-dot" :class="{ 'gv-dot--online': isOnline(m.userId) }" />
            <span class="gv-member-name">{{ m.username }}</span>
            <span class="gv-member-role">{{ m.role }}</span>
          </li>
        </ul>
      </aside>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import { RouterLink } from 'vue-router'
import { api, apiErrorMessage } from '../api/client'
import { useLiveSessionStore } from '../stores/useLiveSessionStore'
import { useProjectStore } from '../stores/useProjectStore'
import LiveSessionPanel from '../components/LiveSessionPanel.vue'
import ChatMessageList from '../components/ChatMessageList.vue'
import { formatDateTime } from '../utils/date'
import type { ChatMessage, GameSession, ProjectMember } from '../types'

const route = useRoute()
const projectId = computed(() => String(route.params.projectId)).value

const liveSession = useLiveSessionStore()
const projectStore = useProjectStore()

const pastSessions = ref<GameSession[]>([])
const historyError = ref('')
const openSessionId = ref<string | null>(null)
// Local, not store.fetchHistory: loading an ended session's chat must not
// clobber the live message stream in the shared store.
const openMessages = ref<ChatMessage[]>([])
const loadingMessages = ref(false)

const members = computed<ProjectMember[]>(() => projectStore.membersCache[projectId] ?? [])
const projectName = computed(() => projectStore.projects.find(p => p.id === projectId)?.name ?? '')

const senderNames = computed<Record<string, string>>(() => {
  const map: Record<string, string> = {}
  for (const m of members.value) map[m.userId] = m.username
  return map
})

const onlineCount = computed(() => liveSession.onlineUserIds.length)

onMounted(() => {
  projectStore.fetchProjects().catch(() => {})
  projectStore.fetchMembers(projectId).catch(() => {})
  loadSessions()
})

// A session that just stopped (locally or via session_ended) belongs in the
// past-sessions list; refresh it on the transition.
watch(() => liveSession.status, (now, prev) => {
  if (prev === 'running' && now === 'stopped') loadSessions()
})

async function loadSessions() {
  historyError.value = ''
  try {
    const all = await liveSession.fetchSessions(projectId)
    pastSessions.value = all.filter(s => s.status === 'stopped')
  } catch (e: unknown) {
    historyError.value = apiErrorMessage(e, 'Could not load past sessions')
  }
}

async function toggleSession(s: GameSession) {
  if (openSessionId.value === s.id) {
    openSessionId.value = null
    openMessages.value = []
    return
  }
  openSessionId.value = s.id
  openMessages.value = []
  loadingMessages.value = true
  try {
    const messages = await api<ChatMessage[]>(`/projects/${projectId}/session/${s.id}/messages`)
    if (openSessionId.value !== s.id) return
    openMessages.value = messages
  } catch (e: unknown) {
    if (openSessionId.value !== s.id) return
    historyError.value = apiErrorMessage(e, 'Could not load session chat')
    openSessionId.value = null
  } finally {
    if (openSessionId.value === s.id) loadingMessages.value = false
  }
}

function isOnline(userId: string): boolean {
  return liveSession.connected && liveSession.onlineUserIds.includes(userId)
}

function memberName(userId: string): string {
  return senderNames.value[userId] ?? 'unknown'
}

</script>

<style scoped>
.game-view { display: flex; flex-direction: column; gap: var(--space-lg); }

.gv-header { display: flex; align-items: flex-start; justify-content: space-between; gap: var(--space-md); }
.gv-title { font-family: var(--font-display); font-size: var(--text-2xl); font-weight: 400; color: var(--foreground); margin: 0; }
.gv-subtitle { font-size: var(--text-md); color: var(--muted-foreground); margin: 4px 0 0; }
.gv-back {
  font-size: var(--text-sm); color: var(--primary); text-decoration: none;
  border: 1px solid var(--border); border-radius: var(--radius);
  padding: 6px 12px; transition: background var(--transition); white-space: nowrap;
}
.gv-back:hover { background: var(--muted); }

.gv-columns { display: flex; gap: var(--space-lg); align-items: flex-start; }
.gv-main { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: var(--space-lg); }
.gv-members { width: 240px; flex-shrink: 0; }
@media (max-width: 720px) {
  .gv-columns { flex-direction: column-reverse; }
  .gv-members { width: 100%; }
}

.gv-section-title {
  display: flex; align-items: baseline; gap: var(--space-sm);
  font-family: var(--font-display); font-size: var(--text-lg); font-weight: 400;
  color: var(--foreground); margin: 0 0 var(--space-sm);
}
.gv-online-count { font-family: var(--font-body); font-size: var(--text-xs); color: var(--muted-foreground); }

.gv-member-list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: var(--space-xs); }
.gv-member { display: flex; align-items: center; gap: var(--space-sm); }
.gv-dot {
  width: 8px; height: 8px; border-radius: 50%;
  background: var(--border); flex-shrink: 0;
}
.gv-dot--online { background: var(--primary); }
.gv-member-name { font-size: var(--text-md); color: var(--foreground); }
.gv-member-role { font-size: var(--text-xs); color: var(--muted-foreground); margin-left: auto; }

.gv-history { display: flex; flex-direction: column; }
.gv-error { font-size: var(--text-sm); color: var(--destructive); margin: 0; }
.gv-empty { font-size: var(--text-sm); color: var(--muted-foreground); margin: 0; }

.gv-session-list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: var(--space-xs); }
.gv-session-row {
  width: 100%; display: flex; align-items: baseline; gap: var(--space-sm);
  background: var(--muted); border: 1px solid transparent; border-radius: var(--radius);
  padding: var(--space-xs) var(--space-sm); cursor: pointer; text-align: left;
  font-family: var(--font-body); transition: background var(--transition);
}
.gv-session-row:hover { background: var(--secondary); }
.gv-session-row--open { border-color: var(--border); }
.gv-session-date { font-size: var(--text-md); color: var(--foreground); }
.gv-session-meta { font-size: var(--text-xs); color: var(--muted-foreground); }
.gv-session-chat { margin-top: var(--space-xs); display: flex; flex-direction: column; gap: var(--space-xs); }
.gv-readonly-hint { font-size: var(--text-xs); color: var(--muted-foreground); margin: 0; }
</style>
