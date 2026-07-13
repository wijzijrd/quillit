<template>
  <div class="member-shell">
    <!-- Sidebar -->
    <aside class="member-sidebar">
      <div class="sb-section">
        <button
          class="sb-item"
          :class="{ active: activeSection === 'shared' }"
          @click="activeSection = 'shared'"
        >
          <Share2 :size="14" /> All shared notes
        </button>
        <button
          class="sb-item"
          :class="{ active: activeSection === 'session' }"
          @click="activeSection = 'session'"
        >
          <FileText :size="14" /> Session notes
        </button>
      </div>

      <div class="sb-section">
        <div class="sb-heading-row">
          <span class="sb-heading">Folders</span>
          <button class="sb-add-btn" @click="showNewFolder = !showNewFolder" title="New folder">
            <FolderPlus :size="12" />
          </button>
        </div>

        <div class="folder-create" v-if="showNewFolder">
          <input
            class="folder-input"
            v-model="newFolderName"
            placeholder="Folder name"
            @keydown.enter="createFolder"
            @keydown.escape="showNewFolder = false"
          />
          <button class="sb-add-btn" @click="createFolder"><Check :size="12" /></button>
        </div>

        <button
          v-for="folder in member.folders"
          :key="folder.id"
          class="sb-item sb-folder"
          :class="{ active: activeSection === 'folder:' + folder.id }"
          @click="activeSection = 'folder:' + folder.id"
        >
          <Folder :size="14" :style="folder.color ? { color: folder.color } : {}" />
          <span class="folder-name">{{ folder.name }}</span>
          <button class="folder-delete" @click.stop="deleteFolder(folder.id)" title="Delete folder">×</button>
        </button>
      </div>
    </aside>

    <!-- Main panel -->
    <main class="member-main">
      <!-- All shared notes -->
      <template v-if="activeSection === 'shared'">
        <div class="panel-header">
          <h2>Shared with me</h2>
          <span class="entry-count">{{ member.sharedNotes.length }} notes</span>
        </div>
        <div class="entry-list">
          <div
            v-for="entry in member.sharedNotes"
            :key="entry.id"
            class="entry-row"
            :class="{ active: activeEntryId === entry.id }"
            @click="selectEntry(entry)"
          >
            <span class="er-cat">{{ entry.category }}</span>
            <span class="er-title">{{ entry.title }}</span>
          </div>
          <p class="empty-msg" v-if="member.sharedNotes.length === 0 && loaded">
            No notes have been shared with you yet.
          </p>
        </div>
      </template>

      <!-- Session notes -->
      <template v-else-if="activeSection === 'session'">
        <div class="panel-header">
          <h2>Session Notes</h2>
          <button class="btn-sm-primary" @click="createSessionNote">+ New note</button>
        </div>
        <div class="entry-list">
          <div
            v-for="note in member.sessionNotes"
            :key="note.id"
            class="entry-row"
            :class="{ active: activeEntryId === note.id }"
            @click="selectEntry(note)"
          >
            <span class="er-cat">{{ note.category }}</span>
            <span class="er-title">{{ note.title || 'Untitled Note' }}</span>
          </div>
          <p class="empty-msg" v-if="member.sessionNotes.length === 0 && loaded">
            No session notes yet. Create one to get started.
          </p>
        </div>
      </template>

      <!-- Folder view -->
      <template v-else-if="activeSection.startsWith('folder:')">
        <div class="panel-header">
          <h2>{{ activeFolderName }}</h2>
        </div>
        <p class="empty-msg">Drag entries here or use the share panel to add them to this folder.</p>
      </template>

      <!-- Entry detail panel -->
      <div class="entry-detail" v-if="activeEntry">
        <div class="detail-top">
          <button class="back-btn" @click="activeEntry = null">← Back</button>
          <button
            v-if="isSessionNote"
            class="btn-danger-xs"
            @click="deleteSessionNote"
          >Delete</button>
        </div>

        <!-- Session note editor -->
        <template v-if="isSessionNote">
          <input
            class="detail-title-input"
            v-model="activeEntry.title"
            placeholder="Note title…"
            @blur="saveSessionNote"
          />
          <textarea
            class="detail-body-textarea"
            v-model="activeEntry.body"
            placeholder="Write your note here…"
            @blur="saveSessionNote"
          ></textarea>
        </template>

        <!-- Read-only shared note -->
        <template v-else>
          <p class="detail-cat">{{ activeEntry.category }}</p>
          <h1 class="detail-title">{{ activeEntry.title }}</h1>
          <div class="detail-body tiptap" v-html="activeEntry.body"></div>

          <!-- Pin toggle -->
          <div class="meta-bar">
            <button
              class="meta-btn"
              :class="{ pinned: entryMeta.pinned }"
              @click="togglePin"
            >
              <Pin :size="13" /> {{ entryMeta.pinned ? 'Pinned' : 'Pin' }}
            </button>
          </div>
        </template>
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { Share2, FileText, FolderPlus, Folder, Check, Pin } from 'lucide-vue-next'
import { useMemberStore } from '../stores/useMemberStore'

const member = useMemberStore()

const activeSection = ref('shared')
const activeEntryId = ref(null)
const activeEntry = ref(null)
const loaded = ref(false)
const showNewFolder = ref(false)
const newFolderName = ref('')
const entryMeta = ref({ pinned: false })

const isSessionNote = computed(() =>
  member.sessionNotes.some(n => n.id === activeEntryId.value)
)

const activeFolderName = computed(() => {
  if (!activeSection.value.startsWith('folder:')) return ''
  const fid = activeSection.value.replace('folder:', '')
  return member.folders.find(f => f.id === fid)?.name ?? 'Folder'
})

onMounted(async () => {
  await Promise.all([
    member.fetchSharedNotes(),
    member.fetchFolders(),
    member.fetchSessionNotes(),
  ])
  loaded.value = true
})

function selectEntry(entry) {
  activeEntryId.value = entry.id
  activeEntry.value = { ...entry }
  entryMeta.value = { pinned: false }
}

async function createSessionNote() {
  const note = await member.createSessionNote({ title: 'Untitled Note', category: 'Note' })
  activeSection.value = 'session'
  selectEntry(note)
}

async function saveSessionNote() {
  if (!activeEntry.value || !isSessionNote.value) return
  await member.updateSessionNote(activeEntry.value.id, {
    title: activeEntry.value.title,
    body: activeEntry.value.body,
  })
}

async function deleteSessionNote() {
  if (!activeEntry.value) return
  if (!confirm(`Delete "${activeEntry.value.title}"?`)) return
  await member.deleteSessionNote(activeEntry.value.id)
  activeEntry.value = null
  activeEntryId.value = null
}

async function createFolder() {
  if (!newFolderName.value.trim()) return
  await member.createFolder({ name: newFolderName.value.trim() })
  newFolderName.value = ''
  showNewFolder.value = false
}

async function deleteFolder(id) {
  if (!confirm('Delete this folder?')) return
  await member.deleteFolder(id)
  if (activeSection.value === 'folder:' + id) activeSection.value = 'shared'
}

async function togglePin() {
  entryMeta.value.pinned = !entryMeta.value.pinned
  await member.upsertEntryMeta(activeEntry.value.id, { pinned: entryMeta.value.pinned })
}
</script>

<style scoped>
.member-shell {
  display: flex; height: 100%; overflow: hidden;
}

.member-sidebar {
  width: 220px; min-width: 220px;
  border-right: 1px solid var(--border);
  background: var(--card);
  overflow-y: auto;
  display: flex; flex-direction: column;
  padding: var(--space-sm) 0;
}

.sb-section {
  padding: var(--space-xs) var(--space-sm);
  border-bottom: 1px solid var(--border);
  margin-bottom: var(--space-xs);
}

.sb-heading-row {
  display: flex; align-items: center; justify-content: space-between;
  padding: var(--space-xs) var(--space-xs);
}
.sb-heading {
  font-size: var(--text-xs); text-transform: uppercase; letter-spacing: 0.1em;
  color: var(--muted-foreground);
}
.sb-add-btn {
  display: inline-flex; align-items: center; justify-content: center;
  width: 20px; height: 20px; background: none; border: none;
  color: var(--muted-foreground); cursor: pointer; border-radius: var(--radius);
  transition: color var(--transition);
}
.sb-add-btn:hover { color: var(--primary); }

.sb-item {
  display: flex; align-items: center; gap: var(--space-xs);
  width: 100%; padding: 6px var(--space-sm);
  background: none; border: none; border-left: 2px solid transparent;
  color: var(--muted-foreground); font-family: var(--font-body); font-size: var(--text-sm);
  cursor: pointer; text-align: left;
  transition: background var(--transition), color var(--transition);
  border-radius: var(--radius);
}
.sb-item:hover { background: var(--muted); color: var(--foreground); }
.sb-item.active { background: var(--muted); color: var(--foreground); border-left-color: var(--primary); }
.sb-folder { position: relative; }
.folder-name { flex: 1; }
.folder-delete {
  opacity: 0; background: none; border: none; color: var(--muted-foreground);
  cursor: pointer; padding: 0; font-size: 1em; line-height: 1;
  transition: opacity var(--transition), color var(--transition);
}
.sb-folder:hover .folder-delete { opacity: 1; }
.folder-delete:hover { color: var(--destructive); }

.folder-create {
  display: flex; gap: 4px; align-items: center; margin: 4px var(--space-xs);
}
.folder-input {
  flex: 1; background: var(--muted); border: 1px solid var(--border);
  border-radius: var(--radius); padding: 4px 8px;
  color: var(--foreground); font-family: var(--font-body); font-size: var(--text-sm);
  outline: none;
}

.member-main {
  flex: 1; display: flex; overflow: hidden;
}

.panel-header {
  display: flex; align-items: center; justify-content: space-between;
  padding: var(--space-lg) var(--space-xl) var(--space-sm);
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
}
.panel-header h2 { font-family: var(--font-display); font-size: 1.1em; color: var(--foreground); font-weight: 400; }
.entry-count { font-size: var(--text-xs); color: var(--muted-foreground); }

.btn-sm-primary {
  background: var(--secondary); color: var(--primary); border: none;
  border-radius: var(--radius); padding: 5px 12px; font-size: 0.82em;
  cursor: pointer; transition: background var(--transition);
}
.btn-sm-primary:hover { background: var(--primary); color: var(--background); }

.entry-list {
  flex: 1; overflow-y: auto; padding: var(--space-xs) 0;
  display: flex; flex-direction: column;
}

.member-main > template { display: contents; }
.member-main { flex-direction: column; }

.entry-row {
  display: flex; align-items: center; gap: var(--space-sm);
  padding: 10px var(--space-xl); cursor: pointer;
  border-left: 2px solid transparent;
  transition: background var(--transition);
}
.entry-row:hover { background: var(--muted); }
.entry-row.active { background: var(--muted); border-left-color: var(--primary); }
.er-cat { font-size: 0.7em; text-transform: uppercase; letter-spacing: 0.1em; font-weight: 600; color: var(--muted-foreground); width: 72px; flex-shrink: 0; }
.er-title { font-size: var(--text-sm); color: var(--foreground); }

.empty-msg { padding: var(--space-xl); color: var(--muted-foreground); font-size: var(--text-sm); }

.entry-detail {
  position: absolute; inset: 0; background: var(--card);
  overflow-y: auto; padding: var(--space-2xl) var(--space-3xl);
  z-index: 10;
}
.detail-top { display: flex; align-items: center; justify-content: space-between; margin-bottom: var(--space-xl); }
.back-btn {
  background: none; border: none; color: var(--muted-foreground); cursor: pointer;
  font-family: var(--font-body); font-size: var(--text-sm); padding: 0;
  transition: color var(--transition);
}
.back-btn:hover { color: var(--foreground); }
.btn-danger-xs {
  background: none; color: var(--destructive); border: 1px solid var(--destructive);
  border-radius: var(--radius); padding: 3px 8px; font-size: 0.78em; cursor: pointer;
}

.detail-cat { font-size: var(--text-xs); text-transform: uppercase; letter-spacing: 0.1em; font-weight: 600; color: var(--muted-foreground); margin: 0 0 var(--space-xs); }
.detail-title { font-family: var(--font-display); font-size: var(--text-3xl); color: var(--foreground); font-weight: 400; margin: 0 0 var(--space-xl); }
.detail-body { line-height: 1.75; }

.detail-title-input {
  width: 100%; background: none; border: none; outline: none;
  font-family: var(--font-display); font-size: var(--text-2xl);
  color: var(--foreground); margin-bottom: var(--space-lg);
}
.detail-title-input::placeholder { color: var(--muted-foreground); }
.detail-body-textarea {
  width: 100%; background: var(--muted); border: 1px solid var(--border);
  border-radius: var(--radius); color: var(--foreground);
  font-family: var(--font-body); font-size: var(--text-md); line-height: 1.7;
  min-height: 280px; padding: var(--space-md); resize: vertical; outline: none;
}
.detail-body-textarea:focus { border-color: var(--secondary); }

.meta-bar { margin-top: var(--space-xl); display: flex; gap: var(--space-sm); }
.meta-btn {
  display: inline-flex; align-items: center; gap: 4px;
  background: var(--muted); border: 1px solid var(--border);
  border-radius: var(--radius); padding: 4px 10px; font-size: var(--text-xs);
  color: var(--muted-foreground); cursor: pointer; transition: background var(--transition), color var(--transition);
}
.meta-btn:hover { color: var(--foreground); }
.meta-btn.pinned { border-color: var(--secondary); color: var(--primary); background: rgba(128, 168, 255, 0.15); }
</style>
