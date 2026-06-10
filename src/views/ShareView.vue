<template>
  <!-- Invalid token -->
  <div class="share-invalid" v-if="loaded && !player">
    <div class="invalid-box">
      <Lock :size="32" style="opacity:0.5" />
      <h2>Link not found</h2>
      <p>This share link is invalid or has been removed.</p>
    </div>
  </div>

  <!-- Loading -->
  <div class="share-loading" v-else-if="!loaded">
    <p>Loading…</p>
  </div>

  <!-- Player view -->
  <div class="share-shell" v-else>
    <header class="share-header">
      <span class="share-campaign">{{ activeCampaignName }}</span>
      <div class="share-badges">
        <span class="player-badge">{{ player.name }}</span>
        <button class="btn-export" @click="exportHtml" title="Download player view as HTML">
          Export
        </button>
      </div>
    </header>

    <div class="share-body" id="share-content">
      <!-- Sidebar -->
      <aside class="share-sidebar">
        <!-- GM entries section -->
        <p class="sidebar-heading">Entries</p>
        <div
          v-for="entry in visibleEntries"
          :key="entry.id"
          class="share-entry-item"
          :class="{ active: activeItemId === entry.id && activeItemType === 'entry' }"
          @click="selectEntry(entry.id)"
        >
          <span class="entry-cat" :style="{ color: `var(--cat-${entry.category.toLowerCase()})` }">
            {{ entry.category }}
          </span>
          <span class="entry-title">{{ entry.title }}</span>
        </div>
        <p class="sidebar-empty" v-if="visibleEntries.length === 0">
          No entries shared yet.
        </p>

        <!-- Player notes section -->
        <div class="sidebar-notes-header">
          <p class="sidebar-heading">My Notes</p>
          <button class="sidebar-add-btn" @click="createNote" title="New note">
            <Plus :size="12" />
          </button>
        </div>
        <div
          v-for="note in playerNotes.notes.value"
          :key="note.id"
          class="share-entry-item"
          :class="{ active: activeItemId === note.id && activeItemType === 'note' }"
          @click="selectNote(note.id)"
        >
          <div class="note-item-row">
            <span class="entry-cat" :style="{ color: `var(--cat-${note.category.toLowerCase()})` }">
              {{ note.category }}
            </span>
            <span class="note-vis-badge" :class="note.visibility">
              {{ note.visibility === 'public' ? 'Public' : 'Private' }}
            </span>
          </div>
          <span class="entry-title">{{ note.title }}</span>
        </div>
        <p class="sidebar-empty" v-if="playerNotes.notes.value.length === 0">
          No notes yet.
        </p>
      </aside>

      <!-- GM entry detail -->
      <main class="share-detail" v-if="activeItemType === 'entry' && activeEntry">
        <div class="detail-header">
          <span class="detail-cat" :style="{ color: `var(--cat-${activeEntry.category.toLowerCase()})` }">
            {{ activeEntry.category }}
          </span>
          <h1 class="detail-title">{{ activeEntry.title }}</h1>
        </div>
        <div class="detail-body tiptap" v-html="cleanBody"></div>

        <div class="see-also" v-if="linkedPublicEntries.length > 0">
          <h3 class="see-also-heading">See also</h3>
          <div class="see-also-list">
            <span
              v-for="linked in linkedPublicEntries"
              :key="linked.id"
              class="see-also-item"
              @click="selectEntry(linked.id)"
            >
              <span class="see-also-cat" :style="{ color: `var(--cat-${linked.category.toLowerCase()})` }">
                {{ linked.category }}
              </span>
              {{ linked.title }}
            </span>
          </div>
        </div>

        <div class="entry-notes" v-if="entryAnnotations.length > 0">
          <h3 class="notes-heading">Notes</h3>
          <div class="note-item" v-for="ann in entryAnnotations" :key="ann.id">
            <span class="note-badge" :class="`vis-${ann.visibility}`">
              {{ ann.visibility === 'player' ? 'Lore' : 'Shared' }}
            </span>
            <p class="note-text">{{ ann.text }}</p>
          </div>
        </div>
      </main>

      <!-- Player note editor -->
      <main class="share-detail" v-else-if="activeItemType === 'note' && activeNote">
        <div class="note-editor">
          <div class="ne-header">
            <input class="ne-title" v-model="activeNote.title" @blur="saveNote" placeholder="Note title…" />
            <div class="ne-controls">
              <select class="ne-select" v-model="activeNote.category" @change="saveNote">
                <option v-for="c in categories" :key="c">{{ c }}</option>
              </select>
              <button
                class="ne-vis-toggle"
                :class="activeNote.visibility"
                @click="toggleNoteVisibility"
                :title="activeNote.visibility === 'public' ? 'Public — visible to others' : 'Private — only you'"
              >
                <Globe v-if="activeNote.visibility === 'public'" :size="13" />
                <Lock v-else :size="13" />
                {{ activeNote.visibility === 'public' ? 'Public' : 'Private' }}
              </button>
              <button class="ne-delete" @click="deleteNote" title="Delete note">
                <Trash2 :size="13" />
              </button>
            </div>
          </div>
          <textarea
            class="ne-body"
            v-model="activeNote.body"
            @blur="saveNote"
            placeholder="Write your note here…"
          ></textarea>
        </div>
      </main>

      <main class="share-detail share-detail--empty" v-else>
        <p>Select an entry or note from the list.</p>
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, reactive } from 'vue'
import { useRoute } from 'vue-router'
import { Lock, Plus, Globe, Trash2 } from 'lucide-vue-next'
import { useCampaignStore } from '../stores/useCampaignStore'
import { useEntriesStore } from '../stores/useEntriesStore'

const CATEGORIES = ['Note', 'Characters', 'Location', 'Quest', 'Lore', 'Other']
import { useAnnotationsStore } from '../stores/useAnnotationsStore'
import { filterForPlayer, stripGmMarks } from '../composables/useAnnotationVisibility'
import { usePlayerNotes } from '../composables/usePlayerNotes'

const route = useRoute()
const campaign = useCampaignStore()
const entries = useEntriesStore()
const annotations = useAnnotationsStore()
const playerNotes = usePlayerNotes(route.params.token)
const categories = CATEGORIES

const loaded = ref(false)
const player = ref(null)
const activeCampaignId = ref(null)
const activeCampaignName = ref('')
const activeItemId = ref(null)
const activeItemType = ref('entry')

onMounted(async () => {
  await Promise.all([campaign.init(), entries.init(), annotations.init(), playerNotes.init()])
  const result = campaign.getByToken(route.params.token)
  if (result) {
    player.value = result.player
    activeCampaignId.value = result.campaign.id
    activeCampaignName.value = result.campaign.name
  }
  loaded.value = true
  if (player.value && visibleEntries.value.length > 0) {
    selectEntry(visibleEntries.value[0].id)
  }
})

const visibleEntries = computed(() => {
  if (!player.value) return []
  const { visibleEntries } = filterForPlayer(entries.entries, annotations.annotations, player.value.id, activeCampaignId.value)
  return visibleEntries
})

const activeEntry = computed(() =>
  activeItemType.value === 'entry' && activeItemId.value
    ? entries.getById(activeItemId.value)
    : null
)

const activeNote = computed(() => {
  if (activeItemType.value !== 'note' || !activeItemId.value) return null
  return playerNotes.notes.value.find(n => n.id === activeItemId.value) ?? null
})

const cleanBody = computed(() =>
  activeEntry.value ? stripGmMarks(activeEntry.value.body) : ''
)

const entryAnnotations = computed(() => {
  if (!activeEntry.value || !player.value) return []
  const { visibleAnnotations } = filterForPlayer([], annotations.annotations, player.value.id, activeCampaignId.value)
  return visibleAnnotations.filter(a => a.entryId === activeEntry.value.id)
})

const linkedPublicEntries = computed(() => {
  if (!activeEntry.value) return []
  return (activeEntry.value.linkedEntries ?? [])
    .map(id => entries.getById(id))
    .filter(e => e && (e.visibility ?? 'private') === 'public')
})

function selectEntry(id) {
  activeItemId.value = id
  activeItemType.value = 'entry'
}

function selectNote(id) {
  activeItemId.value = id
  activeItemType.value = 'note'
}

function createNote() {
  const note = playerNotes.createNote()
  selectNote(note.id)
}

function saveNote() {
  if (!activeNote.value) return
  playerNotes.updateNote(activeNote.value.id, {
    title: activeNote.value.title,
    body: activeNote.value.body,
    category: activeNote.value.category,
    visibility: activeNote.value.visibility,
  })
}

function toggleNoteVisibility() {
  if (!activeNote.value) return
  const newVis = activeNote.value.visibility === 'public' ? 'private' : 'public'
  playerNotes.updateNote(activeNote.value.id, { visibility: newVis })
}

function deleteNote() {
  if (!activeNote.value) return
  if (!confirm(`Delete "${activeNote.value.title}"?`)) return
  playerNotes.deleteNote(activeNote.value.id)
  activeItemId.value = null
  activeItemType.value = 'entry'
}

function exportHtml() {
  const styles = `
    body { font-family: Georgia, serif; background: #fff; color: #222; max-width: 860px; margin: 40px auto; padding: 0 24px; line-height: 1.7; }
    h1 { font-size: 1.8em; margin-bottom: 0.2em; }
    h2 { font-size: 1.3em; margin: 1.2em 0 0.3em; }
    .entry-section { border-top: 1px solid #ddd; padding-top: 32px; margin-top: 32px; }
    .entry-cat { font-size: 0.72em; text-transform: uppercase; letter-spacing: 0.1em; font-weight: 600; color: #666; }
    .notes { margin-top: 20px; padding-top: 16px; border-top: 1px solid #eee; }
    .note { margin-bottom: 10px; padding: 8px 12px; background: #f7f7f7; border-radius: 4px; }
    mark { background: none; }
  `
  const entryHtml = visibleEntries.value.map(entry => {
    const body = stripGmMarks(entry.body)
    const { visibleAnnotations } = filterForPlayer([], annotations.annotations, player.value.id, activeCampaignId.value)
    const entryNotes = visibleAnnotations.filter(a => a.entryId === entry.id)
    const notesHtml = entryNotes.length > 0
      ? `<div class="notes"><strong>Notes</strong>${entryNotes.map(n => `<div class="note"><p>${n.text}</p></div>`).join('')}</div>`
      : ''
    return `<section class="entry-section"><p class="entry-cat">${entry.category}</p><h1>${entry.title}</h1>${body}${notesHtml}</section>`
  }).join('\n')

  const html = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>${activeCampaignName.value} — ${player.value.name}'s View</title>
  <style>${styles}</style>
</head>
<body>
  <h2 style="font-size:1em;color:#999;font-weight:normal;">${activeCampaignName.value}</h2>
  <p style="color:#999;font-size:0.85em;">Player view for ${player.value.name}</p>
  ${entryHtml}
</body>
</html>`

  const blob = new Blob([html], { type: 'text/html' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `${activeCampaignName.value.replace(/\s+/g, '-').toLowerCase()}-${player.value.name.replace(/\s+/g, '-').toLowerCase()}.html`
  a.click()
  URL.revokeObjectURL(url)
}
</script>

<style scoped>
.share-invalid, .share-loading {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100vh;
  background: var(--background);
  color: var(--muted-foreground);
}

.invalid-box {
  text-align: center;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--space-sm);
}
.invalid-box h2 { font-family: var(--font-display); color: var(--foreground); }

.share-shell {
  display: flex;
  flex-direction: column;
  height: 100vh;
  background: var(--background);
  color: var(--foreground);
  font-family: var(--font-body);
}

.share-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--space-sm) var(--space-xl);
  border-bottom: 1px solid var(--border);
  background: var(--card);
  flex-shrink: 0;
  height: var(--h-lg);
}

.share-campaign {
  font-family: var(--font-display);
  color: var(--primary);
  font-size: var(--text-lg);
  letter-spacing: 0.06em;
}

.share-badges { display: flex; align-items: center; gap: var(--space-sm); }

.player-badge {
  font-size: var(--text-sm);
  background: rgba(80,200,120,0.15);
  color: #8e8;
  border: 1px solid rgba(80,200,120,0.3);
  border-radius: var(--radius);
  padding: 3px var(--space-sm);
}

.btn-export {
  background: var(--muted);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  color: var(--muted-foreground);
  font-family: var(--font-body);
  font-size: var(--text-sm);
  height: var(--h-sm);
  padding: 0 var(--space-sm);
  cursor: pointer;
  transition: background var(--transition), color var(--transition);
}
.btn-export:hover { background: var(--muted); color: var(--foreground); }

.share-body { display: flex; flex: 1; overflow: hidden; }

.share-sidebar {
  width: 220px;
  min-width: 220px;
  border-right: 1px solid var(--border);
  background: var(--card);
  overflow-y: auto;
  padding: var(--space-sm) 0;
}

.sidebar-heading {
  font-size: var(--text-xs);
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: var(--muted-foreground);
  padding: var(--space-xs) var(--space-md) var(--space-xs);
}

.sidebar-notes-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding-right: var(--space-sm);
  margin-top: var(--space-md);
  border-top: 1px solid var(--border);
  padding-top: var(--space-sm);
}

.sidebar-add-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: var(--h-xs);
  width: var(--h-xs);
  background: var(--secondary);
  border: none;
  border-radius: var(--radius);
  color: var(--primary);
  cursor: pointer;
}

.share-entry-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: var(--space-sm) var(--space-md);
  cursor: pointer;
  transition: background var(--transition);
  border-left: 2px solid transparent;
}
.share-entry-item:hover { background: var(--muted); }
.share-entry-item.active { background: var(--muted); border-left-color: var(--primary); }

.note-item-row { display: flex; align-items: center; gap: var(--space-xs); }
.note-vis-badge {
  font-size: 0.6em;
  padding: 1px 4px;
  border-radius: 2px;
  text-transform: uppercase;
  letter-spacing: 0.06em;
}
.note-vis-badge.public { background: rgba(80,200,120,0.15); color: #8e8; }
.note-vis-badge.private { background: var(--muted); color: var(--muted-foreground); }

.entry-cat { font-size: var(--text-xs); text-transform: uppercase; letter-spacing: 0.1em; font-weight: 600; }
.entry-title { font-size: var(--text-sm); color: var(--foreground); }

.sidebar-empty { padding: var(--space-md) var(--space-md); font-size: var(--text-sm); color: var(--muted-foreground); line-height: 1.5; }

.share-detail { flex: 1; padding: var(--space-2xl) var(--space-3xl); overflow-y: auto; }
.share-detail--empty {
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--muted-foreground);
  font-size: var(--text-md);
}

.detail-header { margin-bottom: var(--space-xl); }
.detail-cat { font-size: var(--text-xs); text-transform: uppercase; letter-spacing: 0.1em; font-weight: 600; }
.detail-title { font-family: var(--font-display); font-size: var(--text-3xl); color: var(--foreground); margin-top: var(--space-xs); font-weight: 400; }

.detail-body { line-height: 1.75; }

.see-also { margin-top: var(--space-xl); padding-top: var(--space-md); border-top: 1px solid var(--border); }
.see-also-heading { font-family: var(--font-display); font-size: var(--text-xs); letter-spacing: 0.08em; text-transform: uppercase; color: var(--muted-foreground); margin-bottom: var(--space-sm); }
.see-also-list { display: flex; flex-wrap: wrap; gap: var(--space-xs); }
.see-also-item {
  display: inline-flex; align-items: center; gap: var(--space-xs);
  background: var(--muted); border: 1px solid var(--border);
  border-radius: var(--radius); padding: 4px var(--space-sm);
  font-size: var(--text-sm); color: var(--foreground); cursor: pointer;
  transition: background var(--transition), border-color var(--transition);
}
.see-also-item:hover { background: var(--muted); border-color: var(--secondary); }
.see-also-cat { font-size: 0.65em; text-transform: uppercase; letter-spacing: 0.08em; font-weight: 600; }

.entry-notes { margin-top: var(--space-2xl); padding-top: var(--space-lg); border-top: 1px solid var(--border); }
.notes-heading { font-family: var(--font-display); font-size: var(--text-sm); letter-spacing: 0.08em; text-transform: uppercase; color: var(--muted-foreground); margin-bottom: var(--space-md); }
.note-item { display: flex; gap: var(--space-sm); padding: var(--space-sm) var(--space-md); background: var(--muted); border-radius: var(--radius); margin-bottom: var(--space-xs); align-items: flex-start; }
.note-badge { font-size: var(--text-xs); padding: 2px var(--space-xs); border-radius: var(--radius); white-space: nowrap; flex-shrink: 0; }
.note-badge.vis-player { background: rgba(80,200,120,0.18); color: #8e8; }
.note-badge.vis-shared { background: rgba(80,160,220,0.18); color: #8ae; }
.note-text { font-size: var(--text-sm); color: var(--muted-foreground); line-height: 1.55; margin: 0; }

/* Note editor */
.note-editor { display: flex; flex-direction: column; gap: var(--space-md); max-width: 680px; }
.ne-header { display: flex; flex-direction: column; gap: var(--space-sm); }
.ne-title {
  background: none;
  border: none;
  font-family: var(--font-display);
  font-size: var(--text-2xl);
  color: var(--foreground);
  outline: none;
  height: var(--h-xl);
}
.ne-title::placeholder { color: var(--muted-foreground); }
.ne-controls { display: flex; align-items: center; gap: var(--space-sm); }
.ne-select {
  background: var(--muted);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  color: var(--muted-foreground);
  font-family: var(--font-body);
  font-size: var(--text-sm);
  height: var(--h-sm);
  padding: 0 var(--space-sm);
  cursor: pointer;
}
.ne-vis-toggle {
  display: inline-flex;
  align-items: center;
  gap: var(--space-xs);
  height: var(--h-sm);
  padding: 0 var(--space-sm);
  background: none;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  color: var(--muted-foreground);
  font-family: var(--font-body);
  font-size: var(--text-xs);
  cursor: pointer;
  transition: background var(--transition), color var(--transition), border-color var(--transition);
}
.ne-vis-toggle.public { border-color: rgba(80,200,120,0.4); background: rgba(80,200,120,0.08); color: #8e8; }
.ne-delete {
  display: inline-flex; align-items: center; justify-content: center;
  height: var(--h-sm); width: var(--h-sm);
  background: none; border: none; color: var(--muted-foreground);
  cursor: pointer; border-radius: var(--radius);
  transition: color var(--transition);
  margin-left: auto;
}
.ne-delete:hover { color: var(--destructive); }
.ne-body {
  background: var(--muted);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  color: var(--foreground);
  font-family: var(--font-body);
  font-size: var(--text-md);
  line-height: 1.7;
  min-height: 200px;
  padding: var(--space-md);
  outline: none;
  resize: vertical;
  transition: border-color var(--transition);
}
.ne-body:focus { border-color: var(--secondary); }
</style>
