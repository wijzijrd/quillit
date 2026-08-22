<template>
  <div class="admin-view">
    <header class="admin-header">
      <h1>Admin</h1>
      <div class="tab-bar">
        <button class="tab" :class="{ active: tab === 'users' }" @click="tab = 'users'">Users</button>
        <button class="tab" :class="{ active: tab === 'projects' }" @click="tab = 'projects'">Projects</button>
      </div>
    </header>

    <!-- ── Users tab ── -->
    <section v-if="tab === 'users'" class="tab-content">
      <div class="search-row">
        <input class="search-input" v-model="userQuery" placeholder="Search by email or username…" @input="searchUsers" />
      </div>

      <div class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <th>Username</th>
              <th>Email</th>
              <th>Role</th>
              <th>Status</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="u in admin.users" :key="u.id">
              <td>{{ u.username }}</td>
              <td class="email-cell">{{ u.email }}</td>
              <td><span class="role-badge" :class="u.role">{{ u.role }}</span></td>
              <td>
                <span class="status-badge" :class="u.active ? 'active' : 'disabled'">
                  {{ u.active ? 'Active' : 'Disabled' }}
                </span>
              </td>
              <td class="action-cell">
                <button
                  class="btn-ghost btn-sm"
                  @click="toggleUserActive(u)"
                >
                  {{ u.active ? 'Disable' : 'Enable' }}
                </button>
                <button
                  class="btn-danger btn-sm"
                  @click="confirmDeleteUser(u)"
                >Delete</button>
              </td>
            </tr>
          </tbody>
        </table>
        <p class="empty-msg" v-if="admin.users.length === 0 && usersLoaded">No users found.</p>
      </div>
    </section>

    <!-- ── Projects tab ── -->
    <section v-if="tab === 'projects'" class="tab-content">
      <div class="search-row">
        <input class="search-input" v-model="projectQuery" placeholder="Search by project name…" @input="searchProjects" />
      </div>

      <div class="project-list">
        <div class="project-row" v-for="p in admin.projects" :key="p.id">
          <div class="pr-header">
            <span class="pr-type">{{ p.type }}</span>
            <span class="pr-name">{{ p.name }}</span>
            <span class="pr-meta">{{ p.memberCount }} members</span>
            <button
              class="btn-ghost btn-sm"
              @click="toggleProjectMembers(p.id)"
            >{{ expandedProject === p.id ? 'Hide' : 'Members' }}</button>
          </div>

          <div class="pr-members" v-if="expandedProject === p.id">
            <p class="loading-msg" v-if="!admin.projectMembers[p.id]">Loading…</p>
            <table class="data-table" v-else>
              <thead><tr><th>User ID</th><th>Role</th><th>Joined</th></tr></thead>
              <tbody>
                <tr v-for="m in admin.projectMembers[p.id]" :key="m.id">
                  <td class="mono">{{ m.userId }}</td>
                  <td><span class="role-badge">{{ m.role }}</span></td>
                  <td class="date-cell">{{ formatDate(m.joinedAt) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
        <p class="empty-msg" v-if="admin.projects.length === 0 && projectsLoaded">No projects found.</p>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useAdminStore } from '../stores/useAdminStore'
import { formatDate } from '../utils/date'
import type { AdminUser } from '../types'

const admin = useAdminStore()

const tab = ref('users')
const userQuery = ref('')
const projectQuery = ref('')
const usersLoaded = ref(false)
const projectsLoaded = ref(false)
const expandedProject = ref<string | null>(null)

onMounted(async () => {
  await admin.fetchUsers()
  usersLoaded.value = true
})

let userTimer: ReturnType<typeof setTimeout> | null = null
function searchUsers() {
  if (userTimer) clearTimeout(userTimer)
  userTimer = setTimeout(async () => {
    await admin.fetchUsers(userQuery.value)
    usersLoaded.value = true
  }, 300)
}

let projectTimer: ReturnType<typeof setTimeout> | null = null
async function searchProjects() {
  if (projectTimer) clearTimeout(projectTimer)
  projectTimer = setTimeout(async () => {
    await admin.fetchProjects(projectQuery.value)
    projectsLoaded.value = true
  }, 300)
}

async function onTabChange() {
  if (tab.value === 'projects' && admin.projects.length === 0) {
    await admin.fetchProjects()
    projectsLoaded.value = true
  }
}

// Watch tab changes — using watcher inline
import { watch } from 'vue'
watch(tab, onTabChange)

async function toggleProjectMembers(id: string) {
  if (expandedProject.value === id) {
    expandedProject.value = null
    return
  }
  expandedProject.value = id
  if (!admin.projectMembers[id]) await admin.fetchProjectMembers(id)
}

// admin.users is AdminUser[] (auth-svc's UserResponse), which always sets
// `id` — unlike the old shared User type, AdminUser can't also mean
// AuthUser (svc's MeResponse, which has no `id`), so no guard is needed here.
function toggleUserActive(u: AdminUser) {
  admin.setUserActive(u.id, !u.active)
}

async function confirmDeleteUser(u: AdminUser) {
  if (!confirm(`Delete user "${u.username}" (${u.email})? This cannot be undone.`)) return
  await admin.deleteUser(u.id)
}
</script>

<style scoped>
.admin-view { padding: 40px 48px; }
.admin-header { margin-bottom: 28px; }
.admin-header h1 { font-family: var(--font-display); font-size: 1.8em; color: var(--primary); margin-bottom: 16px; }

.tab-bar { display: flex; gap: 4px; border-bottom: 1px solid var(--border); }
.tab {
  background: none; border: none; border-bottom: 2px solid transparent;
  color: var(--muted-foreground); font-family: var(--font-body); font-size: 0.9em;
  padding: 8px 16px; cursor: pointer; margin-bottom: -1px;
  transition: color var(--transition), border-color var(--transition);
  border-radius: var(--radius) var(--radius) 0 0;
}
.tab:hover { color: var(--foreground); }
.tab.active { color: var(--primary); border-bottom-color: var(--primary); font-weight: 600; }

.tab-content { padding-top: 24px; }
.search-row { margin-bottom: 20px; }
.search-input {
  width: 100%; background: var(--muted); border: 1px solid var(--border);
  border-radius: var(--radius); color: var(--foreground); font-family: var(--font-body);
  font-size: 0.9em; padding: 8px 12px; outline: none; transition: border-color var(--transition);
}
.search-input:focus { border-color: var(--secondary); }

.table-wrap { overflow-x: auto; }
.data-table { width: 100%; border-collapse: collapse; font-size: 0.88em; }
.data-table th { text-align: left; padding: 8px 12px; color: var(--muted-foreground); font-size: 0.78em; text-transform: uppercase; letter-spacing: 0.08em; border-bottom: 1px solid var(--border); }
.data-table td { padding: 10px 12px; border-bottom: 1px solid var(--border); color: var(--foreground); vertical-align: middle; }
.data-table tr:last-child td { border-bottom: none; }
.email-cell { color: var(--muted-foreground); }
.date-cell { color: var(--muted-foreground); }
.mono { font-family: monospace; font-size: 0.85em; color: var(--muted-foreground); }

.role-badge {
  font-size: 0.7em; text-transform: uppercase; letter-spacing: 0.08em;
  padding: 2px 6px; border-radius: 4px;
  background: var(--muted); color: var(--muted-foreground);
}
.role-badge.admin { background: var(--secondary); color: var(--primary); }
.status-badge {
  font-size: 0.75em; padding: 2px 8px; border-radius: 10px;
}
.status-badge.active { background: rgba(80,200,120,0.15); color: #8e8; }
.status-badge.disabled { background: rgba(220,38,38,0.1); color: var(--destructive); }
.action-cell { display: flex; gap: 6px; align-items: center; }

/* Project rows */
.project-list { display: flex; flex-direction: column; gap: 10px; }
.project-row { background: var(--card); border: 1px solid var(--border); border-radius: var(--radius); overflow: hidden; }
.pr-header { display: flex; align-items: center; gap: 12px; padding: 12px 16px; }
.pr-type { font-size: 0.65em; text-transform: uppercase; letter-spacing: 0.1em; background: var(--muted); color: var(--muted-foreground); border-radius: 4px; padding: 2px 6px; }
.pr-name { font-weight: 600; flex: 1; }
.pr-meta { font-size: 0.8em; color: var(--muted-foreground); }
.pr-members { border-top: 1px solid var(--border); padding: 12px 16px; }
.loading-msg { color: var(--muted-foreground); font-size: 0.88em; }
.empty-msg { color: var(--muted-foreground); font-size: 0.9em; padding: 16px 0; }

.btn-ghost { background: var(--muted); color: var(--muted-foreground); border: 1px solid var(--border); border-radius: var(--radius); padding: 8px 14px; cursor: pointer; font-size: 0.88em; transition: background var(--transition); }
.btn-ghost:hover { background: var(--muted); color: var(--foreground); }
.btn-danger { background: none; color: var(--destructive); border: 1px solid var(--destructive); border-radius: var(--radius); padding: 8px 14px; cursor: pointer; font-size: 0.88em; transition: background var(--transition); }
.btn-danger:hover { background: rgba(220,38,38,0.1); }
.btn-sm { padding: 4px 10px !important; font-size: 0.8em !important; }
</style>
