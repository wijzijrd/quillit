<template>
  <div class="project-view">
    <header class="pv-header">
      <div class="pv-title-row">
        <div>
          <h1 class="pv-name">{{ project?.name ?? '…' }}</h1>
          <span class="pv-type-badge">{{ typeLabel }}</span>
        </div>
        <RouterLink :to="`/projects/${projectId}/notes`" class="pv-open-btn">
          Open Notes
        </RouterLink>
      </div>
    </header>

    <!-- Members -->
    <section class="pv-section">
      <h2 class="pv-section-title">Members</h2>
      <div class="member-list">
        <div v-for="m in members" :key="m.id" class="member-row">
          <span class="member-identity">{{ m.username || m.userId }}</span>
          <span class="member-role">{{ m.role }}</span>
          <button
            v-if="isEditor && m.userId !== auth.user?.sub"
            class="member-remove"
            @click="removeMember(m.userId)"
          >Remove</button>
        </div>
        <p class="pv-empty" v-if="!members.length">No members yet.</p>
      </div>
    </section>

    <!-- Add member via user search -->
    <section class="pv-section" v-if="isEditor">
      <h2 class="pv-section-title">Add Member</h2>
      <div class="search-wrap">
        <input
          class="search-input"
          v-model="searchQuery"
          placeholder="Search by username or email…"
          @input="onSearch"
          autocomplete="off"
        />
        <div class="search-dropdown" v-if="searchResults.length">
          <button
            v-for="u in searchResults"
            :key="u.id"
            class="search-result"
            @click="addUser(u)"
          >
            <span class="sr-username">{{ u.username }}</span>
            <span class="sr-email">{{ u.email }}</span>
          </button>
        </div>
      </div>
      <p class="pv-note">In a future update, invitations will require the user to accept.</p>
    </section>

    <!-- Token invite link -->
    <section class="pv-section" v-if="isEditor">
      <h2 class="pv-section-title">Invite Link</h2>
      <button class="pv-btn" @click="generateLink">Generate invite link</button>
      <div v-if="inviteLink" class="invite-link-row">
        <input class="invite-link-input" readonly :value="inviteLink" />
        <button class="pv-btn" @click="copyLink">Copy</button>
      </div>
    </section>

    <!-- Live Session -->
    <LiveSessionPanel :project-id="String(projectId)" />

    <p class="pv-error" v-if="error">{{ error }}</p>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRoute, RouterLink } from 'vue-router'
import { useProjectStore } from '../stores/useProjectStore'
import { useMemberStore } from '../stores/useMemberStore'
import { useAuthStore } from '../stores/useAuthStore'
import LiveSessionPanel from '../components/LiveSessionPanel.vue'

const route = useRoute()
const projectStore = useProjectStore()
const memberStore = useMemberStore()
const auth = useAuthStore()

const projectId = computed(() => route.params.projectId)
const project = computed(() => projectStore.projects.find(p => p.id === projectId.value) ?? null)
const members = ref([])
const searchQuery = ref('')
const searchResults = ref([])
const inviteLink = ref('')
const error = ref('')
let searchTimer = null

const typeLabel = computed(() => {
  if (!project.value) return ''
  return project.value.type === 'campaign' ? 'Campaign' : 'Book'
})

const isEditor = computed(() => {
  if (!project.value || !auth.user) return false
  const editorRole = project.value.roleLabels?.[0]?.toLowerCase()
  return project.value.myRole === editorRole || auth.user.role === 'admin'
})

onMounted(async () => {
  await Promise.all([projectStore.fetchProjects(), projectStore.fetchTypes()])
  members.value = await projectStore.fetchMembers(projectId.value)
})

function onSearch() {
  clearTimeout(searchTimer)
  searchResults.value = []
  if (searchQuery.value.trim().length < 2) return
  searchTimer = setTimeout(async () => {
    searchResults.value = await memberStore.searchUsers(searchQuery.value.trim())
  }, 280)
}

async function addUser(user) {
  error.value = ''
  const memberRole = project.value?.roleLabels?.[1]?.toLowerCase() ?? 'member'
  try {
    const m = await projectStore.addMember(projectId.value, user.id, memberRole, user.username)
    members.value.push(m)
    searchQuery.value = ''
    searchResults.value = []
  } catch (e) {
    error.value = e?.data?.error ?? 'Could not add member'
  }
}

async function removeMember(userId) {
  error.value = ''
  try {
    await projectStore.removeMember(projectId.value, userId)
    members.value = members.value.filter(m => m.userId !== userId)
  } catch (e) {
    error.value = e?.data?.error ?? 'Could not remove member'
  }
}

async function generateLink() {
  error.value = ''
  try {
    const memberRole = project.value?.roleLabels?.[1]?.toLowerCase() ?? 'member'
    const inv = await projectStore.generateInvite(projectId.value, memberRole)
    inviteLink.value = `${window.location.origin}/register?invite=${inv.token}`
  } catch (e) {
    error.value = e?.data?.error ?? 'Could not generate invite link'
  }
}

function copyLink() {
  navigator.clipboard.writeText(inviteLink.value).catch(() => {})
}
</script>

<style scoped>
.project-view {
  padding: var(--space-2xl) var(--space-xl);
  display: flex;
  flex-direction: column;
  gap: var(--space-2xl);
}
.pv-header { display: flex; flex-direction: column; gap: var(--space-sm); }
.pv-title-row { display: flex; align-items: flex-start; justify-content: space-between; gap: var(--space-lg); }
.pv-name { font-family: var(--font-display); font-size: var(--text-2xl); color: var(--foreground); font-weight: 400; margin: 0; }
.pv-type-badge {
  display: inline-block; margin-top: var(--space-xs);
  font-size: var(--text-xs); text-transform: uppercase; letter-spacing: 0.08em;
  color: var(--primary); background: color-mix(in srgb, var(--primary) 12%, transparent);
  border: 1px solid var(--secondary); border-radius: var(--radius);
  padding: 2px 8px;
}
.pv-open-btn {
  height: var(--h-md); padding: 0 var(--space-md);
  background: var(--secondary); border-radius: var(--radius);
  color: var(--primary); text-decoration: none; font-size: var(--text-md);
  display: flex; align-items: center; white-space: nowrap;
  transition: background var(--transition);
}
.pv-open-btn:hover { background: var(--primary); color: var(--background); }
.pv-section { display: flex; flex-direction: column; gap: var(--space-md); }
.pv-section-title { font-family: var(--font-display); font-size: var(--text-lg); font-weight: 400; color: var(--foreground); margin: 0; }
.member-list { display: flex; flex-direction: column; gap: var(--space-xs); }
.member-row {
  display: flex; align-items: center; gap: var(--space-sm);
  padding: var(--space-xs) var(--space-sm);
  background: var(--muted); border-radius: var(--radius);
}
.member-identity { flex: 1; font-size: var(--text-md); color: var(--foreground); }
.member-role { font-size: var(--text-xs); text-transform: capitalize; color: var(--muted-foreground); }
.member-remove {
  font-size: var(--text-xs); color: var(--destructive); background: none; border: none;
  cursor: pointer; padding: 2px 6px; border-radius: var(--radius);
}
.member-remove:hover { background: color-mix(in srgb, var(--destructive) 12%, transparent); }
.pv-empty { font-size: var(--text-sm); color: var(--muted-foreground); margin: 0; }
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
.search-result:hover { background: var(--muted); }
.sr-username { font-size: var(--text-md); color: var(--foreground); }
.sr-email { font-size: var(--text-xs); color: var(--muted-foreground); }
.pv-note { font-size: var(--text-xs); color: var(--muted-foreground); margin: 0; }
.invite-link-row { display: flex; gap: var(--space-sm); }
.invite-link-input {
  flex: 1; background: var(--muted); border: 1px solid var(--border);
  border-radius: var(--radius); color: var(--muted-foreground); font-family: var(--font-body);
  font-size: var(--text-sm); height: var(--h-md); padding: 0 var(--space-sm); outline: none;
}
.pv-btn {
  height: var(--h-md); padding: 0 var(--space-md); background: var(--muted);
  border: 1px solid var(--border); border-radius: var(--radius);
  color: var(--foreground); font-family: var(--font-body); font-size: var(--text-md);
  cursor: pointer; transition: background var(--transition); white-space: nowrap;
}
.pv-btn:hover { background: var(--muted); }
.pv-error { font-size: var(--text-sm); color: var(--destructive); margin: 0; }
</style>
