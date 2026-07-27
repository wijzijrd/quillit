<template>
  <div class="admin-view">
    <header class="admin-header">
      <h1>Admin</h1>
      <div class="tab-bar">
        <button class="tab" :class="{ active: tab === 'users' }" @click="tab = 'users'">Users</button>
        <button class="tab" :class="{ active: tab === 'projects' }" @click="tab = 'projects'">Projects</button>
        <button class="tab" :class="{ active: tab === 'categories' }" @click="tab = 'categories'">Categories</button>
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
                  @click="admin.setUserActive(u.id, !u.active)"
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

    <!-- ── Categories tab (lifted from AdminCategoriesView) ── -->
    <section v-if="tab === 'categories'" class="tab-content">
      <div class="cat-section-header">
        <button class="btn-primary" @click="showCreateForm = true">+ New Category</button>
      </div>

      <div class="admin-form" v-if="showCreateForm">
        <h3>New Category</h3>
        <input v-model="newCat.name" placeholder="Name (e.g. Rumours)" />
        <div class="icon-select-wrap">
          <component :is="resolveIcon(newCat.icon)" :size="16" class="icon-preview" />
          <select v-model="newCat.icon" class="icon-select">
            <option v-for="name in AVAILABLE_ICONS" :key="name" :value="name">{{ name }}</option>
          </select>
        </div>
        <input v-model="newCat.color" type="color" title="Colour" />
        <div class="form-actions">
          <button class="btn-primary" @click="createCategory">Create</button>
          <button class="btn-ghost" @click="showCreateForm = false; resetNewCat()">Cancel</button>
        </div>
      </div>

      <div class="cat-list">
        <div class="cat-row" v-for="cat in cats.categories" :key="cat.id">
          <div class="cat-row-header">
            <component :is="resolveIcon(cat.icon)" :size="16" :style="{ color: cat.color }" class="cat-row-icon" />
            <span class="cat-row-name">{{ cat.name }}</span>
            <div class="cat-row-actions">
              <button class="btn-ghost btn-sm" @click="startEdit(cat)">Edit</button>
              <button class="btn-danger btn-sm" @click="deleteCategory(cat.id)">Delete</button>
            </div>
          </div>

          <div class="cat-edit-form" v-if="editingId === cat.id">
            <input v-model="editForm.name" placeholder="Name" />
            <div class="icon-select-wrap">
              <component :is="resolveIcon(editForm.icon)" :size="16" class="icon-preview" />
              <select v-model="editForm.icon" class="icon-select">
                <option v-for="name in AVAILABLE_ICONS" :key="name" :value="name">{{ name }}</option>
              </select>
            </div>
            <input v-model="editForm.color" type="color" title="Colour" />
            <div class="form-actions">
              <button class="btn-primary btn-sm" @click="saveEdit(cat.id)">Save</button>
              <button class="btn-ghost btn-sm" @click="editingId = null">Cancel</button>
            </div>
          </div>

          <div class="default-tags">
            <span class="dt-label">Default tags:</span>
            <span v-for="tag in cat.defaultTags" :key="tag.id" class="dt-chip">
              {{ tag.label }}
              <button class="dt-remove" @click="removeTag(cat.id, tag.id)">×</button>
            </span>
            <div class="dt-add" v-if="addingTagTo === cat.id">
              <input
                v-model="newTagLabel"
                placeholder="Tag label"
                @keydown.enter="addTag(cat.id)"
                @keydown.escape="addingTagTo = null"
                ref="tagInputRef"
              />
              <button class="btn-ghost btn-sm" @click="addTag(cat.id)">Add</button>
            </div>
            <button v-else class="btn-ghost btn-sm dt-add-btn" @click="startAddTag(cat.id)">+ tag</button>
          </div>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, nextTick } from 'vue'
import { useAdminStore } from '../stores/useAdminStore'
import { useCategoriesStore } from '../stores/useCategoriesStore'
import { resolveIcon, AVAILABLE_ICONS } from '../utils/categoryIcons'
import { formatDate } from '../utils/date'

const admin = useAdminStore()
const cats = useCategoriesStore()

const tab = ref('users')
const userQuery = ref('')
const projectQuery = ref('')
const usersLoaded = ref(false)
const projectsLoaded = ref(false)
const expandedProject = ref(null)

// Categories state
const showCreateForm = ref(false)
const newCat = ref({ name: '', icon: 'BookMarked', color: '#888888' })
const editingId = ref(null)
const editForm = ref({ name: '', icon: '', color: '' })
const addingTagTo = ref(null)
const newTagLabel = ref('')
const tagInputRef = ref(null)

onMounted(async () => {
  await Promise.all([admin.fetchUsers(), cats.init()])
  usersLoaded.value = true
})

let userTimer = null
function searchUsers() {
  clearTimeout(userTimer)
  userTimer = setTimeout(async () => {
    await admin.fetchUsers(userQuery.value)
    usersLoaded.value = true
  }, 300)
}

let projectTimer = null
async function searchProjects() {
  clearTimeout(projectTimer)
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

async function toggleProjectMembers(id) {
  if (expandedProject.value === id) {
    expandedProject.value = null
    return
  }
  expandedProject.value = id
  if (!admin.projectMembers[id]) await admin.fetchProjectMembers(id)
}


async function confirmDeleteUser(u) {
  if (!confirm(`Delete user "${u.username}" (${u.email})? This cannot be undone.`)) return
  await admin.deleteUser(u.id)
}

// Categories
function resetNewCat() { newCat.value = { name: '', icon: 'BookMarked', color: '#888888' } }
async function createCategory() {
  if (!newCat.value.name.trim()) return
  await cats.createCategory({ ...newCat.value })
  showCreateForm.value = false
  resetNewCat()
}
async function deleteCategory(id) {
  if (!confirm('Delete this category?')) return
  await cats.deleteCategory(id)
}
function startEdit(cat) {
  editingId.value = cat.id
  editForm.value = { name: cat.name, icon: cat.icon, color: cat.color }
}
async function saveEdit(id) {
  await cats.updateCategory(id, { ...editForm.value })
  editingId.value = null
}
async function startAddTag(catId) {
  addingTagTo.value = catId
  newTagLabel.value = ''
  await nextTick()
  tagInputRef.value?.focus()
}
async function addTag(catId) {
  if (!newTagLabel.value.trim()) return
  await cats.addDefaultTag(catId, newTagLabel.value.trim())
  newTagLabel.value = ''
  addingTagTo.value = null
}
async function removeTag(catId, tagId) { await cats.removeDefaultTag(catId, tagId) }
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

/* Category section (mirrors AdminCategoriesView) */
.cat-section-header { display: flex; justify-content: flex-end; margin-bottom: 20px; }
.admin-form, .cat-edit-form {
  background: var(--muted); border: 1px solid var(--border);
  border-radius: var(--radius); padding: 16px; margin-bottom: 20px;
  display: flex; flex-wrap: wrap; gap: 10px; align-items: center;
}
.admin-form h3 { width: 100%; margin: 0 0 4px; font-size: 0.9em; color: var(--muted-foreground); }
.admin-form input, .cat-edit-form input {
  background: var(--card); border: 1px solid var(--border);
  border-radius: var(--radius); padding: 6px 10px; color: var(--foreground);
  font-family: var(--font-body); font-size: 0.9em;
}
.form-actions { display: flex; gap: 8px; }
.cat-list { display: flex; flex-direction: column; gap: 12px; }
.cat-row { background: var(--card); border: 1px solid var(--border); border-radius: var(--radius); padding: 14px 16px; }
.cat-row-header { display: flex; align-items: center; gap: 10px; margin-bottom: 10px; }
.cat-row-icon { flex-shrink: 0; }
.icon-select-wrap { display: flex; align-items: center; gap: 6px; background: var(--card); border: 1px solid var(--border); border-radius: var(--radius); padding: 4px 8px; }
.icon-preview { color: var(--muted-foreground); flex-shrink: 0; }
.icon-select { background: none; border: none; color: var(--foreground); font-family: var(--font-body); font-size: 0.88em; cursor: pointer; outline: none; }
.cat-row-name { font-weight: 600; flex: 1; }
.cat-row-actions { display: flex; gap: 6px; }
.default-tags { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; }
.dt-label { font-size: 0.75em; color: var(--muted-foreground); text-transform: uppercase; letter-spacing: 0.06em; }
.dt-chip { display: inline-flex; align-items: center; gap: 4px; background: var(--secondary); color: var(--primary); border-radius: 12px; padding: 2px 8px; font-size: 0.8em; }
.dt-remove { background: none; border: none; cursor: pointer; color: var(--primary); padding: 0; line-height: 1; }
.dt-add { display: flex; gap: 6px; align-items: center; }
.dt-add input { background: var(--card); border: 1px solid var(--border); border-radius: var(--radius); padding: 3px 8px; font-size: 0.82em; color: var(--foreground); font-family: var(--font-body); }
.dt-add-btn { font-size: 0.8em; }

.btn-primary { background: var(--primary); color: var(--background); border: none; border-radius: var(--radius); padding: 8px 16px; cursor: pointer; font-weight: 600; font-size: 0.88em; transition: opacity var(--transition); }
.btn-primary:hover { opacity: 0.88; }
.btn-ghost { background: var(--muted); color: var(--muted-foreground); border: 1px solid var(--border); border-radius: var(--radius); padding: 8px 14px; cursor: pointer; font-size: 0.88em; transition: background var(--transition); }
.btn-ghost:hover { background: var(--muted); color: var(--foreground); }
.btn-danger { background: none; color: var(--destructive); border: 1px solid var(--destructive); border-radius: var(--radius); padding: 8px 14px; cursor: pointer; font-size: 0.88em; transition: background var(--transition); }
.btn-danger:hover { background: rgba(220,38,38,0.1); }
.btn-sm { padding: 4px 10px !important; font-size: 0.8em !important; }
</style>
