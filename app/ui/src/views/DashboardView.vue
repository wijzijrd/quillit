<template>
  <div class="dashboard">
    <header class="dash-header">
      <div class="dash-title-row">
        <div>
          <h1>Quillit</h1>
          <p class="dash-sub">Your workspace, organised.</p>
        </div>
      </div>

      <div class="dash-search">
        <Search :size="15" class="dash-search-icon" />
        <input
          class="dash-search-input"
          v-model="searchQuery"
          placeholder="Search entries…"
          @keydown.escape="searchQuery = ''"
        />
      </div>
    </header>

    <section class="recent-section">
      <h2>{{ searching ? 'Searching…' : searchResults ? 'Search Results' : 'Recently Updated' }}</h2>
      <div class="recent-list">
        <template v-if="searchResults">
          <RouterLink
            v-for="entry in searchResults"
            :key="entry.id"
            :to="`/entries/${entry.id}`"
            class="recent-item"
          >
            <FileText :size="14" class="ri-cat" />
            <span class="ri-title">{{ entry.title }}</span>
            <span class="ri-project-badge" v-if="projectStore.projects.length > 1">{{ projectNameFor(entry.projectId) }}</span>
            <span class="ri-match-badge" v-if="matchedInBody(entry)">Matched in content</span>
          </RouterLink>
          <p v-if="searchResults.length === 0 && !searching" class="no-recent">No entries match your search.</p>
        </template>
        <template v-else>
          <RouterLink
            v-for="entry in recent"
            :key="entry.id"
            :to="`/entries/${entry.id}`"
            class="recent-item"
          >
            <FileText :size="14" class="ri-cat" />
            <span class="ri-title">{{ entry.title }}</span>
          </RouterLink>
          <p v-if="recent.length === 0" class="no-recent">
            No entries yet — head to <RouterLink to="/entries">Entries</RouterLink> to create one.
          </p>
        </template>
      </div>
    </section>

    <!-- Projects hub -->
    <section class="projects-section">
      <div class="projects-header">
        <h2>Projects</h2>
        <button class="btn-sm-primary" @click="showNewProject = true">+ New project</button>
      </div>

      <!-- New project form -->
      <div class="project-form" v-if="showNewProject">
        <input class="pf-input" v-model="newProject.name" placeholder="Project name" />
        <select class="pf-select" v-model="newProject.type">
          <option v-for="t in projectStore.types" :key="t.type" :value="t.type">{{ t.label }}</option>
        </select>
        <div class="pf-actions">
          <button class="btn-primary" @click="createProject" :disabled="!newProject.name.trim()">Create</button>
          <button class="btn-ghost" @click="showNewProject = false; newProject = { name: '', type: 'campaign' }">Cancel</button>
        </div>
      </div>

      <!-- Join via invite (from URL param) -->
      <div class="invite-banner" v-if="pendingInvite">
        <span>You have an invite token. <strong>Join project?</strong></span>
        <button class="btn-sm-primary" @click="redeemInvite" :disabled="joiningInvite">
          {{ joiningInvite ? 'Joining…' : 'Join' }}
        </button>
        <button class="btn-ghost btn-xs" @click="pendingInvite = null">Dismiss</button>
      </div>

      <div class="project-grid" v-if="projectStore.projects.length > 0">
        <div class="project-card" v-for="p in projectStore.projects" :key="p.id" @click="router.push('/projects/' + p.id + '/entries')">
          <div class="pc-top">
            <span class="pc-type-badge">{{ p.type }}</span>
            <span class="pc-live-badge" v-if="p.live"><span class="pc-live-dot" />Live</span>
            <span class="pc-role-badge" v-if="p.myRole">{{ displayRole(p.myRole, p.roleLabels) }}</span>
          </div>
          <h3 class="pc-name">{{ p.name }}</h3>
          <p class="pc-meta">{{ p.memberCount }} {{ p.memberCount === 1 ? 'member' : 'members' }}</p>
          <div class="pc-actions">
            <button class="btn-ghost btn-xs" @click.stop="router.push('/projects/' + p.id + '/game')">Game Mode</button>
            <button class="btn-ghost btn-xs" @click.stop="openProjectInvite(p)">Invite</button>
            <button class="btn-danger-xs" @click.stop="confirmDeleteProject(p)" v-if="isEditorOf(p)">Delete</button>
          </div>
          <!-- Invite link display -->
          <div class="pc-invite-row" v-if="activeInviteProject === p.id && pendingInviteLink">
            <input class="pc-invite-input" readonly :value="pendingInviteLink" @click="copyInvite" />
            <button class="btn-ghost btn-xs" @click="copyInvite">Copy</button>
          </div>
        </div>
      </div>

      <p class="no-projects" v-else-if="projectStore.loaded">
        No projects yet. Create one above to get started.
      </p>
    </section>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { RouterLink } from 'vue-router'
import { FileText, Search } from 'lucide-vue-next'
import { useEntriesStore } from '../stores/useEntriesStore'
import type { EntrySearchResult } from '../stores/useEntriesStore'
import type { ContentEntry } from '../stores/useEntryStore'
import { useProjectStore } from '../stores/useProjectStore'
import { useAuthStore } from '../stores/useAuthStore'
import { api } from '../api/client'
import { inviteLink } from '../utils/links'
import type { Project } from '../types'

/** Debounce for the dashboard search input — matches SearchOverlay.vue. */
const SEARCH_DEBOUNCE_MS = 300

const route = useRoute()
const router = useRouter()
const entries = useEntriesStore()
const projectStore = useProjectStore()
const auth = useAuthStore()

const searchQuery = ref('')
const showNewProject = ref(false)
const newProject = ref({ name: '', type: 'campaign' })
const pendingInvite = ref<string | null>(null)
const joiningInvite = ref(false)
const activeInviteProject = ref<string | null>(null)
const pendingInviteLink = ref('')

// Recents feed, populated below by loadRecents(). Deliberately NOT routed
// through useEntriesStore's `entries`/`init()` — that store's `loaded` guard
// and cache are designed around a single active project (see its doc
// comment / QuillitView's usage), not multi-project accumulation. This
// mirrors searchRemote()'s own approach: direct per-project api() calls,
// merged client-side.
const recentEntries = ref<ContentEntry[]>([])

async function loadRecents(projectIds: string[]) {
  const settled = await Promise.allSettled(
    projectIds.map(id => api(`/content/projects/${id}/entries`))
  )
  recentEntries.value = settled.flatMap(r => (r.status === 'fulfilled' ? r.value : []))
}

onMounted(async () => {
  await Promise.all([projectStore.fetchProjects(), projectStore.fetchTypes()])
  await loadRecents(projectStore.projects.map(p => p.id))

  // Handle ?invite= query param
  const inviteToken = route.query.invite
  if (typeof inviteToken === 'string') pendingInvite.value = inviteToken
})

const recent = computed(() =>
  [...recentEntries.value].sort((a, b) => b.updatedAt - a.updatedAt).slice(0, 8)
)

// Real full-text search against content-svc (issue #51), replacing the old
// client-side `.filter()` over the cached entry list. The search endpoint is
// project-scoped, but this dashboard search has always covered every
// project the user belongs to, so useEntriesStore.searchRemote() fans out
// one request per project and merges the results — see its doc comment.
const searchResults = ref<EntrySearchResult[] | null>(null)
const searching = ref(false)
const appliedSearchQuery = ref('')
let searchDebounceTimer: ReturnType<typeof setTimeout> | null = null
let searchSeq = 0

watch(searchQuery, (value) => {
  if (searchDebounceTimer) clearTimeout(searchDebounceTimer)
  const trimmed = value.trim()
  if (!trimmed) {
    searchResults.value = null
    searching.value = false
    return
  }
  searching.value = true
  searchDebounceTimer = setTimeout(() => runSearch(trimmed), SEARCH_DEBOUNCE_MS)
})

async function runSearch(q: string) {
  const seq = ++searchSeq
  const projectIds = projectStore.projects.map(p => p.id)
  const results = await entries.searchRemote(q, projectIds)
  if (seq !== searchSeq) return // superseded by a newer search
  searchResults.value = results.slice(0, 20)
  appliedSearchQuery.value = q
  searching.value = false
}

function projectNameFor(projectId: string): string {
  return projectStore.projects.find(p => p.id === projectId)?.name ?? ''
}

/**
 * The search endpoint returns no snippet/match-offset (see SearchResult in
 * app/content/internal/handler/search.go — id/projectId/slug/directoryPath/
 * title/tags only, no body), so a real highlighted snippet isn't available
 * without an extra per-result fetch of the full entry body. Rather than pay
 * that N+1 cost, this infers "matched in body" from what IS on the result:
 * if the query text doesn't appear in the title or tags, FTS must have
 * matched it in the body.
 */
function matchedInBody(entry: EntrySearchResult): boolean {
  const q = appliedSearchQuery.value.toLowerCase()
  if (!q) return false
  const inTitle = entry.title.toLowerCase().includes(q)
  const inTags = entry.tags?.some(t => t.toLowerCase().includes(q))
  return !inTitle && !inTags
}

function displayRole(role: string, roleLabels: [string, string]) {
  if (role === roleLabels[0]?.toLowerCase() || role === roleLabels[0]) return roleLabels[0]
  if (role === roleLabels[1]?.toLowerCase() || role === roleLabels[1]) return roleLabels[1]
  return role
}

function isEditorOf(project: Project) {
  if (!project.myRole) return false
  return project.myRole === project.roleLabels[0]?.toLowerCase() ||
    project.myRole === project.roleLabels[0] ||
    auth.user?.role === 'admin'
}

async function createProject() {
  if (!newProject.value.name.trim()) return
  await projectStore.createProject({ name: newProject.value.name, type: newProject.value.type })
  showNewProject.value = false
  newProject.value = { name: '', type: 'campaign' }
}

async function confirmDeleteProject(p: Project) {
  if (!confirm(`Delete project "${p.name}"? This cannot be undone.`)) return
  await projectStore.deleteProject(p.id)
}

async function openProjectInvite(p: Project) {
  if (activeInviteProject.value === p.id) {
    activeInviteProject.value = null
    pendingInviteLink.value = ''
    return
  }
  activeInviteProject.value = p.id
  const memberRole = p.roleLabels?.[1]?.toLowerCase() ?? 'member'
  const inv = await projectStore.generateInvite(p.id, memberRole)
  pendingInviteLink.value = inviteLink(inv.token)
}

function copyInvite() {
  navigator.clipboard.writeText(pendingInviteLink.value).catch(() => {})
}

async function redeemInvite() {
  const token = pendingInvite.value
  if (!token) return
  joiningInvite.value = true
  try {
    await projectStore.join(token)
    pendingInvite.value = null
    alert('You have joined the project!')
  } catch {
    alert('Could not join — the invite may have expired or already been used.')
  } finally {
    joiningInvite.value = false
  }
}
</script>

<style scoped>
.dashboard { padding: 40px 48px; }
.dash-header { margin-bottom: 0; }
.dash-title-row { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; }
.dash-header h1 { font-family: var(--font-display); font-size: 2em; color: var(--primary); letter-spacing: 0.06em; }
.dash-sub { color: var(--muted-foreground); margin-top: 4px; }

.dash-search {
  position: relative; display: flex; align-items: center;
  gap: 8px; margin-top: 16px; margin-bottom: 28px;
  background: var(--muted); border: 1px solid var(--border);
  border-radius: var(--radius); padding: 6px 12px;
}
.dash-search-icon { color: var(--muted-foreground); flex-shrink: 0; }
.dash-search-input {
  flex: 1; background: none; border: none; outline: none;
  color: var(--foreground); font-family: var(--font-body); font-size: 0.9em;
}
.dash-search-input::placeholder { color: var(--muted-foreground); }

.recent-section h2 { font-family: var(--font-display); font-size: 0.9em; letter-spacing: 0.08em; color: var(--muted-foreground); text-transform: uppercase; margin-bottom: 12px; }
.recent-list { display: flex; flex-direction: column; gap: 2px; }
.recent-item {
  display: flex; align-items: center; gap: 14px;
  padding: 10px 14px; border-radius: var(--radius);
  text-decoration: none; transition: background var(--transition);
}
.recent-item:hover { background: var(--muted); }
.ri-cat { width: 20px; flex-shrink: 0; display: flex; align-items: center; justify-content: center; }
.ri-title { color: var(--foreground); font-size: 0.95em; }
.ri-project-badge {
  font-size: 0.68em; text-transform: uppercase; letter-spacing: 0.06em;
  background: var(--muted); color: var(--muted-foreground);
  border-radius: 4px; padding: 2px 6px; margin-left: auto; flex-shrink: 0;
}
.ri-match-badge {
  font-size: 0.68em; letter-spacing: 0.02em;
  background: var(--secondary); color: var(--primary);
  border-radius: 4px; padding: 2px 6px; flex-shrink: 0;
}
.no-recent { color: var(--muted-foreground); font-size: 0.9em; padding: 8px 14px; }
.no-recent a { color: var(--primary); }

/* Projects section */
.projects-section { margin-top: 48px; }
.projects-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 16px; }
.projects-header h2 { font-family: var(--font-display); font-size: 0.9em; letter-spacing: 0.08em; color: var(--muted-foreground); text-transform: uppercase; }
.btn-sm-primary {
  background: var(--secondary); color: var(--primary); border: none;
  border-radius: var(--radius); padding: 5px 12px; font-size: 0.82em;
  cursor: pointer; transition: background var(--transition);
}
.btn-sm-primary:hover { background: var(--primary); color: var(--background); }

.project-form {
  background: var(--muted); border: 1px solid var(--border);
  border-radius: var(--radius); padding: 16px; margin-bottom: 20px;
  display: flex; flex-wrap: wrap; gap: 10px; align-items: center;
}
.pf-input, .pf-select {
  background: var(--card); border: 1px solid var(--border);
  border-radius: var(--radius); padding: 6px 10px;
  color: var(--foreground); font-family: var(--font-body); font-size: 0.9em;
}
.pf-input { flex: 1; min-width: 160px; }
.pf-actions { display: flex; gap: 8px; }

.invite-banner {
  display: flex; align-items: center; gap: 12px;
  background: color-mix(in srgb, var(--primary) 10%, var(--muted));
  border: 1px solid var(--secondary); border-radius: var(--radius);
  padding: 10px 16px; margin-bottom: 16px; font-size: 0.88em; color: var(--foreground);
}
.btn-xs { padding: 3px 8px !important; font-size: 0.78em !important; }

.project-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(240px, 1fr)); gap: 14px; }
.project-card {
  background: var(--card); border: 1px solid var(--border);
  border-radius: var(--radius); padding: 16px;
  display: flex; flex-direction: column; gap: 8px;
  transition: border-color var(--transition);
  cursor: pointer;
}
.project-card:hover { border-color: var(--primary); }
.pc-top { display: flex; align-items: center; gap: 6px; }
.pc-type-badge {
  font-size: 0.65em; text-transform: uppercase; letter-spacing: 0.1em;
  background: var(--muted); color: var(--muted-foreground);
  border-radius: 4px; padding: 2px 6px;
}
.pc-role-badge {
  font-size: 0.65em; text-transform: uppercase; letter-spacing: 0.08em;
  background: var(--secondary); color: var(--primary);
  border-radius: 4px; padding: 2px 6px;
}
.pc-live-badge {
  display: inline-flex; align-items: center; gap: 4px;
  font-size: 0.65em; text-transform: uppercase; letter-spacing: 0.1em;
  color: var(--primary); border: 1px solid var(--primary);
  border-radius: 4px; padding: 2px 6px;
}
.pc-live-dot {
  width: 6px; height: 6px; border-radius: 50%; background: var(--primary);
  animation: pc-live-pulse 1.6s ease-in-out infinite;
}
@keyframes pc-live-pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.35; }
}
.pc-name { font-family: var(--font-display); font-size: 1em; color: var(--foreground); margin: 0; font-weight: 400; }
.pc-meta { font-size: 0.78em; color: var(--muted-foreground); margin: 0; }
.pc-actions { display: flex; gap: 6px; margin-top: 4px; }
.btn-ghost {
  background: var(--muted); color: var(--muted-foreground); border: 1px solid var(--border);
  border-radius: var(--radius); padding: 6px 12px; cursor: pointer;
  font-size: 0.85em; transition: background var(--transition);
}
.btn-ghost:hover { background: var(--muted); color: var(--foreground); }
.btn-primary {
  background: var(--primary); color: var(--background); border: none;
  border-radius: var(--radius); padding: 6px 14px; cursor: pointer;
  font-weight: 600; font-size: 0.88em; transition: opacity var(--transition);
}
.btn-primary:hover { opacity: 0.88; }
.btn-primary:disabled { opacity: 0.4; cursor: default; }
.btn-danger-xs {
  background: none; color: var(--destructive); border: 1px solid var(--destructive);
  border-radius: var(--radius); padding: 3px 8px; font-size: 0.78em; cursor: pointer;
  transition: background var(--transition);
}
.btn-danger-xs:hover { background: rgba(220,38,38,0.1); }
.pc-invite-row { display: flex; gap: 6px; align-items: center; margin-top: 4px; }
.pc-invite-input {
  flex: 1; background: var(--muted); border: 1px solid var(--border);
  border-radius: var(--radius); padding: 4px 8px; font-size: 0.75em;
  color: var(--muted-foreground); cursor: pointer;
}

.no-projects { color: var(--muted-foreground); font-size: 0.9em; padding: 24px 0; }
</style>
