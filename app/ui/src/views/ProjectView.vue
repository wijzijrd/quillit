<template>
  <div class="project-view">
    <header class="pv-header">
      <div class="pv-title-row">
        <div>
          <h1 class="pv-name">{{ project?.name ?? '…' }}</h1>
          <span class="pv-type-badge">{{ typeLabel }}</span>
        </div>
        <RouterLink :to="`/projects/${projectId}/entries`" class="pv-open-btn">
          Open Entries
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

    <!-- Facets -->
    <section class="pv-section">
      <h2 class="pv-section-title">Facets</h2>
      <p class="pv-note">
        Facets are the card-block vocabulary used in entries (e.g. <code>:::card npc</code>).
        Global facets are available to every project; this project can add its own on top.
      </p>

      <div class="facet-group">
        <div class="facet-group-header">
          <span class="facet-group-label facet-group-label-global">Global</span>
          <span class="facet-group-hint">Available to every project</span>
        </div>
        <div class="facet-chips">
          <span v-for="f in facets.global" :key="f" class="facet-chip facet-chip-global">
            {{ f }}
            <button
              v-if="isEditor"
              class="facet-chip-remove"
              @click="removeGlobalFacet(f)"
              :title="`Remove global facet '${f}'`"
            >×</button>
          </span>
          <p class="pv-empty" v-if="!facets.global.length && facetsLoaded">No global facets yet.</p>
        </div>
        <form v-if="isEditor" class="facet-add-form" @submit.prevent="addGlobalFacet">
          <input
            class="facet-add-input"
            v-model="newGlobalFacet"
            placeholder="new-facet-name"
            @input="globalFacetError = ''"
          />
          <button class="pv-btn" type="submit">Add</button>
        </form>
        <p class="facet-error" v-if="globalFacetError">{{ globalFacetError }}</p>
      </div>

      <div class="facet-group">
        <div class="facet-group-header">
          <span class="facet-group-label facet-group-label-project">Project-specific</span>
          <span class="facet-group-hint">Only available in this project</span>
        </div>
        <div class="facet-chips">
          <span v-for="f in projectOnlyFacets" :key="f" class="facet-chip facet-chip-project">
            {{ f }}
            <button
              v-if="isEditor"
              class="facet-chip-remove"
              @click="removeProjectFacet(f)"
              :title="`Remove project facet '${f}'`"
            >×</button>
          </span>
          <p class="pv-empty" v-if="!projectOnlyFacets.length && facetsLoaded">No project-specific facets yet.</p>
        </div>
        <form v-if="isEditor" class="facet-add-form" @submit.prevent="addProjectFacet">
          <input
            class="facet-add-input"
            v-model="newProjectFacet"
            placeholder="new-facet-name"
            @input="projectFacetError = ''"
          />
          <button class="pv-btn" type="submit">Add</button>
        </form>
        <p class="facet-error" v-if="projectFacetError">{{ projectFacetError }}</p>
      </div>

      <div class="facet-notice" v-if="facetNotice">
        <span>{{ facetNotice }}</span>
        <button class="facet-notice-dismiss" @click="facetNotice = ''" title="Dismiss">×</button>
      </div>
    </section>

    <!-- CLI project import -->
    <section class="pv-section" v-if="isEditor">
      <h2 class="pv-section-title">Import from CLI</h2>
      <p class="pv-note">
        Upload a project tarball built by <code>quillit push --output</code> to bring
        entries from a local CLI project into this one.
      </p>
      <ImportPanel :project-id="String(projectId)" />
    </section>

    <!-- Game Mode -->
    <section class="pv-section">
      <h2 class="pv-section-title">Game Mode</h2>
      <RouterLink :to="`/projects/${projectId}/game`" class="pv-btn pv-game-link">
        Open Game Mode
      </RouterLink>
    </section>

    <p class="pv-error" v-if="error">{{ error }}</p>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRoute, RouterLink } from 'vue-router'
import { useProjectStore } from '../stores/useProjectStore'
import { useAuthStore } from '../stores/useAuthStore'
import { useFacetsStore } from '../stores/useFacetsStore'
import { apiErrorMessage } from '../api/client'
import ImportPanel from '../components/ImportPanel.vue'
import { inviteLink as buildInviteLink } from '../utils/links'
import { isKebabCase } from '../utils/facets'

const route = useRoute()
const projectStore = useProjectStore()
const auth = useAuthStore()
const facets = useFacetsStore()

const projectId = computed(() => route.params.projectId)
const project = computed(() => projectStore.projects.find(p => p.id === projectId.value) ?? null)
const members = ref([])
const searchQuery = ref('')
const searchResults = ref([])
const inviteLink = ref('')
const error = ref('')
let searchTimer = null

const facetsLoaded = ref(false)
const newGlobalFacet = ref('')
const newProjectFacet = ref('')
const globalFacetError = ref('')
const projectFacetError = ref('')
const facetNotice = ref('')
const projectOnlyFacets = computed(() => facets.projectOnlyFacets(projectId.value))

const typeLabel = computed(() => {
  const type = project.value?.type
  if (!type) return ''
  return type.charAt(0).toUpperCase() + type.slice(1)
})

const isEditor = computed(() => {
  if (!project.value || !auth.user) return false
  const editorRole = project.value.roleLabels?.[0]?.toLowerCase()
  return project.value.myRole === editorRole || auth.user.role === 'admin'
})

onMounted(async () => {
  await Promise.all([projectStore.fetchProjects(), projectStore.fetchTypes()])
  members.value = await projectStore.fetchMembers(projectId.value)
  await Promise.all([facets.fetchGlobal(), facets.fetchForProject(projectId.value)])
  facetsLoaded.value = true
})

function onSearch() {
  clearTimeout(searchTimer)
  searchResults.value = []
  if (searchQuery.value.trim().length < 2) return
  searchTimer = setTimeout(async () => {
    try {
      searchResults.value = await projectStore.searchUsers(searchQuery.value.trim())
    } catch (e: unknown) {
      error.value = apiErrorMessage(e, 'Could not search users')
    }
  }, 280)
}

async function addUser(user: { id: string; username: string }) {
  error.value = ''
  const memberRole = project.value?.roleLabels?.[1]?.toLowerCase() ?? 'member'
  try {
    const m = await projectStore.addMember(projectId.value, user.id, memberRole, user.username)
    members.value.push(m)
    searchQuery.value = ''
    searchResults.value = []
  } catch (e: unknown) {
    error.value = apiErrorMessage(e, 'Could not add member')
  }
}

async function removeMember(userId: string) {
  error.value = ''
  try {
    await projectStore.removeMember(projectId.value, userId)
    members.value = members.value.filter(m => m.userId !== userId)
  } catch (e: unknown) {
    error.value = apiErrorMessage(e, 'Could not remove member')
  }
}

async function generateLink() {
  error.value = ''
  try {
    const memberRole = project.value?.roleLabels?.[1]?.toLowerCase() ?? 'member'
    const inv = await projectStore.generateInvite(projectId.value, memberRole)
    inviteLink.value = buildInviteLink(inv.token)
  } catch (e: unknown) {
    error.value = apiErrorMessage(e, 'Could not generate invite link')
  }
}

function copyLink() {
  navigator.clipboard.writeText(inviteLink.value).catch(() => {})
}

function facetWarning(name: string): string {
  return `Delete facet "${name}"?\n\nEntry bodies are left untouched — any entry with a :::card block still referencing "${name}" will fail validation at its next save or render.`
}

async function addGlobalFacet() {
  const name = newGlobalFacet.value.trim()
  if (!name) return
  if (!isKebabCase(name)) {
    globalFacetError.value = 'Facet names must be lowercase letters, digits, and hyphens (kebab-case), e.g. "ancient-ruins".'
    return
  }
  globalFacetError.value = ''
  try {
    const res = await facets.createGlobal(name)
    newGlobalFacet.value = ''
    facetNotice.value = res.added ? '' : (res.message ?? '')
    // A global add changes every project's effective vocabulary — refresh so
    // projectOnlyFacets (effective minus global) stays correct, not stale.
    await facets.fetchForProject(projectId.value)
  } catch (e: unknown) {
    globalFacetError.value = apiErrorMessage(e, 'Could not add facet')
  }
}

async function removeGlobalFacet(name: string) {
  if (!confirm(facetWarning(name))) return
  try {
    const res = await facets.deleteGlobal(name)
    facetNotice.value = res.message ?? `Removed "${name}".`
    // Same staleness concern as addGlobalFacet: re-sync the effective list so
    // a facet that was only global (never this project's own) doesn't get
    // misclassified as "project-specific" once it drops out of `global`.
    await facets.fetchForProject(projectId.value)
  } catch (e: unknown) {
    facetNotice.value = apiErrorMessage(e, 'Could not remove facet')
  }
}

async function addProjectFacet() {
  const name = newProjectFacet.value.trim()
  if (!name) return
  if (!isKebabCase(name)) {
    projectFacetError.value = 'Facet names must be lowercase letters, digits, and hyphens (kebab-case), e.g. "ancient-ruins".'
    return
  }
  projectFacetError.value = ''
  try {
    const res = await facets.createForProject(projectId.value, name)
    newProjectFacet.value = ''
    facetNotice.value = res.added ? '' : (res.message ?? '')
  } catch (e: unknown) {
    projectFacetError.value = apiErrorMessage(e, 'Could not add facet')
  }
}

async function removeProjectFacet(name: string) {
  if (!confirm(facetWarning(name))) return
  try {
    const res = await facets.deleteForProject(projectId.value, name)
    facetNotice.value = res.message ?? `Removed "${name}".`
  } catch (e: unknown) {
    facetNotice.value = apiErrorMessage(e, 'Could not remove facet')
  }
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
.pv-game-link {
  display: inline-flex; align-items: center; align-self: flex-start;
  text-decoration: none;
}
.pv-error { font-size: var(--text-sm); color: var(--destructive); margin: 0; }

.facet-group { display: flex; flex-direction: column; gap: var(--space-xs); }
.facet-group-header { display: flex; align-items: baseline; gap: var(--space-sm); }
.facet-group-label {
  font-size: var(--text-xs); text-transform: uppercase; letter-spacing: 0.08em;
  font-weight: 600; color: var(--foreground);
}
.facet-group-label-global { color: var(--primary); }
.facet-group-label-project { color: var(--secondary-foreground, var(--foreground)); }
.facet-group-hint { font-size: var(--text-xs); color: var(--muted-foreground); }
.facet-chips { display: flex; flex-wrap: wrap; gap: 6px; align-items: center; min-height: 1.6em; }
.facet-chip {
  display: inline-flex; align-items: center; gap: 4px;
  background: var(--card); border: 1px solid var(--border);
  border-radius: 12px; padding: 3px 10px; font-size: var(--text-xs);
  color: var(--foreground);
}
.facet-chip-global { border-color: var(--secondary); }
.facet-chip-project {
  background: color-mix(in srgb, var(--secondary) 30%, var(--card));
}
.facet-chip-remove {
  background: none; border: none; cursor: pointer; color: var(--muted-foreground);
  padding: 0; line-height: 1; font-size: 1em; transition: color var(--transition);
}
.facet-chip-remove:hover { color: var(--destructive); }
.facet-add-form { display: flex; gap: var(--space-sm); }
.facet-add-input {
  flex: 1; max-width: 260px; background: var(--muted); border: 1px solid var(--border);
  border-radius: var(--radius); color: var(--foreground); font-family: var(--font-body);
  font-size: var(--text-sm); height: var(--h-md); padding: 0 var(--space-sm); outline: none;
  transition: border-color var(--transition); box-sizing: border-box;
}
.facet-add-input:focus { border-color: var(--secondary); }
.facet-error { font-size: var(--text-xs); color: var(--destructive); margin: 0; }
.facet-notice {
  display: flex; align-items: center; justify-content: space-between; gap: var(--space-sm);
  background: color-mix(in srgb, var(--secondary) 25%, var(--card));
  border: 1px solid var(--secondary); border-radius: var(--radius);
  padding: var(--space-xs) var(--space-sm); font-size: var(--text-xs); color: var(--foreground);
}
.facet-notice-dismiss {
  background: none; border: none; cursor: pointer; color: var(--muted-foreground);
  padding: 0; line-height: 1; font-size: 1.1em; flex-shrink: 0;
}
.facet-notice-dismiss:hover { color: var(--foreground); }
</style>
