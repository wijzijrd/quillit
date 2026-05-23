<template>
  <div class="admin-categories">
    <header class="admin-header">
      <h1>Manage Categories</h1>
      <button class="btn-primary" @click="showCreateForm = true">+ New Category</button>
    </header>

    <!-- Create form -->
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

    <!-- Category list -->
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

        <!-- Inline edit form -->
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

        <!-- Default tags -->
        <div class="default-tags">
          <span class="dt-label">Default tags:</span>
          <span
            v-for="tag in cat.defaultTags"
            :key="tag.id"
            class="dt-chip"
          >
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
  </div>
</template>

<script setup>
import { ref, nextTick } from 'vue'
import { useCategoriesStore } from '../stores/useCategoriesStore.js'
import { resolveIcon, AVAILABLE_ICONS } from '../utils/categoryIcons.js'

const cats = useCategoriesStore()
cats.init()

const showCreateForm = ref(false)
const newCat = ref({ name: '', icon: 'BookMarked', color: '#888888' })
const editingId = ref(null)
const editForm = ref({ name: '', icon: '', color: '' })
const addingTagTo = ref(null)
const newTagLabel = ref('')
const tagInputRef = ref(null)

function resetNewCat() {
  newCat.value = { name: '', icon: 'BookMarked', color: '#888888' }
}

async function createCategory() {
  if (!newCat.value.name.trim()) return
  await cats.createCategory({ ...newCat.value })
  showCreateForm.value = false
  resetNewCat()
}

async function deleteCategory(id) {
  if (!confirm('Delete this category? Existing entries will keep the category name as text.')) return
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

async function removeTag(catId, tagId) {
  await cats.removeDefaultTag(catId, tagId)
}
</script>

<style scoped>
.admin-categories { padding: 40px 48px; max-width: 720px; margin: 0 auto; }
.admin-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 32px; }
.admin-header h1 { font-family: var(--font-display); font-size: 1.6em; color: var(--accent); }
.admin-form, .cat-edit-form {
  background: var(--bg-raised); border: 1px solid var(--border-light);
  border-radius: var(--radius); padding: 16px; margin-bottom: 20px;
  display: flex; flex-wrap: wrap; gap: 10px; align-items: center;
}
.admin-form h3 { width: 100%; margin: 0 0 4px; font-size: 0.9em; color: var(--text-muted); }
.admin-form input, .cat-edit-form input {
  background: var(--bg-surface); border: 1px solid var(--border-light);
  border-radius: var(--radius); padding: 6px 10px; color: var(--text-primary);
  font-family: var(--font-body); font-size: 0.9em;
}
.form-actions { display: flex; gap: 8px; }
.cat-list { display: flex; flex-direction: column; gap: 12px; }
.cat-row {
  background: var(--bg-surface); border: 1px solid var(--border);
  border-radius: var(--radius); padding: 14px 16px;
}
.cat-row-header { display: flex; align-items: center; gap: 10px; margin-bottom: 10px; }
.cat-row-icon { flex-shrink: 0; }
.icon-select-wrap {
  display: flex; align-items: center; gap: 6px;
  background: var(--bg-surface); border: 1px solid var(--border-light);
  border-radius: var(--radius); padding: 4px 8px;
}
.icon-preview { color: var(--text-muted); flex-shrink: 0; }
.icon-select {
  background: none; border: none; color: var(--text-primary);
  font-family: var(--font-body); font-size: 0.88em; cursor: pointer; outline: none;
}
.cat-row-name { font-weight: 600; flex: 1; }
.cat-row-actions { display: flex; gap: 6px; }
.default-tags { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; }
.dt-label { font-size: 0.75em; color: var(--text-faint); text-transform: uppercase; letter-spacing: 0.06em; }
.dt-chip {
  display: inline-flex; align-items: center; gap: 4px;
  background: var(--accent-dim); color: var(--accent);
  border-radius: 12px; padding: 2px 8px; font-size: 0.8em;
}
.dt-remove { background: none; border: none; cursor: pointer; color: var(--accent); padding: 0; line-height: 1; }
.dt-add { display: flex; gap: 6px; align-items: center; }
.dt-add input {
  background: var(--bg-surface); border: 1px solid var(--border-light);
  border-radius: var(--radius); padding: 3px 8px; font-size: 0.82em;
  color: var(--text-primary); font-family: var(--font-body);
}
.dt-add-btn { font-size: 0.8em; }
.btn-primary {
  background: var(--accent); color: var(--bg-base); border: none;
  border-radius: var(--radius); padding: 8px 16px; cursor: pointer; font-weight: 600;
  font-size: 0.88em; transition: opacity var(--transition);
}
.btn-primary:hover { opacity: 0.88; }
.btn-ghost {
  background: var(--bg-raised); color: var(--text-muted); border: 1px solid var(--border-light);
  border-radius: var(--radius); padding: 8px 14px; cursor: pointer;
  font-size: 0.88em; transition: background var(--transition);
}
.btn-ghost:hover { background: var(--bg-hover); color: var(--text-primary); }
.btn-danger {
  background: none; color: var(--danger, #e06c75); border: 1px solid var(--danger, #e06c75);
  border-radius: var(--radius); padding: 8px 14px; cursor: pointer; font-size: 0.88em;
  transition: background var(--transition);
}
.btn-danger:hover { background: rgba(224, 108, 117, 0.1); }
.btn-sm { padding: 4px 10px !important; font-size: 0.8em !important; }
</style>
