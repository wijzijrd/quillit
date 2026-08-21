<template>
  <div class="entry-editor" v-if="entry">
    <div class="popup-controls">
      <div class="popup-nav">
        <button v-if="canGoBack"    class="ctrl-btn" @click="goBack"    title="Back">    <ChevronLeft  :size="14" /></button>
        <button v-if="canGoForward" class="ctrl-btn" @click="goForward" title="Forward"> <ChevronRight :size="14" /></button>
      </div>
      <button v-if="onClose" class="ctrl-btn" @click="onClose()" title="Close">
        <X :size="14" />
      </button>
    </div>
    <div class="editor-header">
      <div class="title-row">
        <input
          class="title-input"
          v-model="localTitle"
          @blur="save"
          placeholder="Entry title…"
        />
        <button class="delete-btn icon-btn" @click="confirmDelete" title="Delete entry">
          <Trash2 :size="14" />
        </button>
      </div>
      <div class="tags-row">
        <span
          v-for="tag in localTags"
          :key="tag"
          class="tag-chip"
        >{{ tag }}<button class="tag-remove" @click="removeTag(tag)">×</button></span>
        <input
          class="tag-input"
          v-model="tagInput"
          placeholder="Add tag…"
          @keydown="onTagKeydown"
          @blur="addTag"
        />
      </div>
      <div class="toolbar">
        <button class="tbtn" @click="editor?.chain().focus().toggleBold().run()" :class="{ on: editor?.isActive('bold') }" title="Bold (Ctrl+B)"><Bold :size="14" /></button>
        <button class="tbtn" @click="editor?.chain().focus().toggleItalic().run()" :class="{ on: editor?.isActive('italic') }" title="Italic (Ctrl+I)"><Italic :size="14" /></button>
        <button class="tbtn" @click="editor?.chain().focus().toggleHeading({ level: 2 }).run()" :class="{ on: editor?.isActive('heading', { level: 2 }) }" title="Heading 2"><Heading2 :size="14" /></button>
        <button class="tbtn" @click="editor?.chain().focus().toggleHeading({ level: 3 }).run()" :class="{ on: editor?.isActive('heading', { level: 3 }) }" title="Heading 3"><Heading3 :size="14" /></button>
        <button class="tbtn" @click="editor?.chain().focus().toggleBulletList().run()" :class="{ on: editor?.isActive('bulletList') }" title="Bullet list"><List :size="14" /></button>
        <button class="tbtn" @click="editor?.chain().focus().setHorizontalRule().run()" title="Horizontal rule"><Minus :size="14" /></button>
        <button class="tbtn" :class="{ on: editor?.isActive('link') }" @click="setLink" title="Add / edit hyperlink"><ExternalLink :size="14" /></button>
        <div class="toolbar-divider"></div>
        <button class="tbtn" @click="editor?.chain().focus().setTextAlign('left').run()" :class="{ on: editor?.isActive({ textAlign: 'left' }) }" title="Align left"><AlignLeft :size="14" /></button>
        <button class="tbtn" @click="editor?.chain().focus().setTextAlign('center').run()" :class="{ on: editor?.isActive({ textAlign: 'center' }) }" title="Align center"><AlignCenter :size="14" /></button>
        <button class="tbtn" @click="editor?.chain().focus().setTextAlign('right').run()" :class="{ on: editor?.isActive({ textAlign: 'right' }) }" title="Align right"><AlignRight :size="14" /></button>
        <button class="tbtn" @click="editor?.chain().focus().setTextAlign('justify').run()" :class="{ on: editor?.isActive({ textAlign: 'justify' }) }" title="Justify"><AlignJustify :size="14" /></button>
        <div class="toolbar-divider"></div>
        <button
          class="tbtn secret-btn"
          @click="insertSecretBlock"
          title="Insert secret block (DM only, stripped from player view)"
        ><Lock :size="14" /></button>
        <button
          class="tbtn card-btn"
          :disabled="!cardFacets.length"
          @click="insertCardBlock"
          :title="cardFacets.length ? 'Insert card block' : 'No facets configured for this project'"
        ><Layers :size="14" /></button>
        <button class="tbtn" @click="printEntry" title="Print / export PDF"><Printer :size="14" /></button>
        <span class="toolbar-spacer"></span>
        <div class="view-switcher">
          <button class="view-btn" :class="{ on: viewMode === 'dm' }" @click="switchView('dm')" title="DM view (editable)">DM</button>
          <button class="view-btn" :class="{ on: viewMode === 'player' }" @click="switchView('player')" title="Player preview">Player</button>
          <button class="view-btn" :class="{ on: viewMode === 'card' }" @click="switchView('card')" title="Card preview" :disabled="!cardFacets.length">Card</button>
        </div>
        <select
          v-if="viewMode === 'card'"
          class="facet-select"
          v-model="selectedCardFacet"
          @change="fetchRender"
        >
          <option v-for="f in cardFacets" :key="f" :value="f">{{ f }}</option>
        </select>
        <span class="save-status">{{ saveStatus }}</span>
      </div>
    </div>
    <div class="editor-body">
      <div class="editor-content" @click="onEditorClick">
        <TiptapEditor
          v-if="viewMode === 'dm'"
          ref="tiptapRef"
          v-model="localBody"
          :uploadImageFn="uploadImage"
          @update:modelValue="debouncedSave"
        />
        <div v-else class="rendered-view">
          <p v-if="rendering" class="rendered-status">Rendering…</p>
          <p v-else-if="renderError" class="rendered-error">{{ renderError }}</p>
          <div v-else class="rendered-content" v-html="renderedHtml"></div>
        </div>
      </div>
    </div>
  </div>

  <div class="editor-empty" v-else>
    <p class="empty-hint">Select an entry or create a new one</p>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useRoute } from 'vue-router'
import { api, apiErrorMessage } from '../api/client'
import {
  Bold, Italic, Heading2, Heading3, List, Minus,
  AlignLeft, AlignCenter, AlignRight, AlignJustify,
  Printer, Lock, Layers, Trash2,
  ChevronLeft, ChevronRight, X, ExternalLink,
} from 'lucide-vue-next'
import TiptapEditor from './TiptapEditor.vue'
import { useEntryStore } from '../stores/useEntryStore'
import { composeFrontmatter, decomposeFrontmatter } from '../lib/frontmatter'
import { useFacetsStore } from '../stores/useFacetsStore'
import { useUIStore } from '../stores/useUIStore'
import { renderMarkdownToHtml } from '../composables/useMarkdownRender'
import { invalidateWikilinkCache } from '../extensions/wikilinkLookup'

defineProps<{ onClose?: () => void }>()

const entryStore = useEntryStore()
const facets = useFacetsStore()
const ui = useUIStore()
const route = useRoute()
// The entry's own campaignIds may list multiple projects; "push to session
// chat" cares about the project currently in context (this route), not the
// entry's full membership list.
const currentProjectId = computed(() => {
  const p = route.params.projectId
  return typeof p === 'string' ? p : undefined
})

// Effective facet vocabulary for the card-block insert button — issue #47's
// facet picker is constrained to this list (see useFacetsStore, from #38/#50).
watch(currentProjectId, (id) => {
  if (id) facets.fetchForProject(id).catch(() => {})
}, { immediate: true })
const cardFacets = computed(() => currentProjectId.value ? (facets.projectEffective[currentProjectId.value] ?? []) : [])

const entry = ref(null)
const localTitle = ref('')
const localBody = ref('')
const localTags = ref([])
const tagInput = ref('')
const saveStatus = ref('')
const viewMode = ref<'dm' | 'player' | 'card'>('dm')
const selectedCardFacet = ref<string>('')
const renderedHtml = ref('')
const renderError = ref('')
const rendering = ref(false)

const localHistory = ref<string[]>([])
const localFuture = ref<string[]>([])
const canGoBack = computed(() => localHistory.value.length > 0)
const canGoForward = computed(() => localFuture.value.length > 0)

const tiptapRef = ref(null)
const editor = computed(() => tiptapRef.value?.editor)

watch(() => ui.activeEntryId, async (id) => {
  // Switching entries (or closing the editor) always drops back to a clean
  // DM view — otherwise the header updates to the new entry while a stale
  // Player/Card render (or a selectedCardFacet the new entry/project may
  // not even have) is left on screen from the previous one.
  viewMode.value = 'dm'
  renderedHtml.value = ''
  renderError.value = ''
  selectedCardFacet.value = ''
  if (!id) { entry.value = null; return }
  try {
    const found = await entryStore.get(id)
    entry.value = found
    const { frontmatter, rest } = decomposeFrontmatter(found.body)
    localTitle.value = frontmatter.name
    localTags.value = frontmatter.tags
    localBody.value = rest
  } catch {
    entry.value = null
  }
}, { immediate: true })

async function uploadImage(file: File): Promise<string> {
  if (!entry.value) throw new Error('no entry')
  const form = new FormData()
  form.append('image', file)
  const res = await api(`/content/entries/${entry.value.id}/images`, { method: 'POST', body: form })
  return res.url
}

let saveTimer = null
function debouncedSave() {
  saveStatus.value = 'Unsaved…'
  clearTimeout(saveTimer)
  saveTimer = setTimeout(save, 800)
}

async function save() {
  if (!entry.value) return
  try {
    const body = composeFrontmatter({ name: localTitle.value, tags: localTags.value }, localBody.value)
    await entryStore.update(entry.value.id, body)
    // A save can newly resolve a wikilink that pointed at a since-created
    // entry, or dangle one whose target moved — drop the cached lookups
    // rather than have them go stale until the next reload.
    if (currentProjectId.value) invalidateWikilinkCache(currentProjectId.value)
    saveStatus.value = 'Saved'
  } catch {
    saveStatus.value = 'Save failed'
  }
  setTimeout(() => saveStatus.value = '', 2000)
}

/**
 * Switching into Player or Card view always saves first, so the preview
 * is never stale relative to the editor — there's no separate "unsaved
 * changes" indicator needed (issue #48's design doc §3). If the current
 * draft doesn't save (e.g. invalid content), the error surfaces and the
 * view stays DM rather than attempting to render a draft the server
 * just rejected.
 */
async function switchView(mode: 'dm' | 'player' | 'card') {
  if (mode === viewMode.value) return
  if (mode === 'dm') {
    viewMode.value = 'dm'
    return
  }
  clearTimeout(saveTimer)
  await save()
  if (saveStatus.value === 'Save failed') return
  viewMode.value = mode
  if (mode === 'card' && !selectedCardFacet.value && cardFacets.value.length) selectedCardFacet.value = cardFacets.value[0]
  await fetchRender()
}

async function fetchRender() {
  if (!entry.value) return
  rendering.value = true
  renderError.value = ''
  try {
    const query = viewMode.value === 'card'
      ? `card=${encodeURIComponent(selectedCardFacet.value)}`
      : 'view=player'
    renderedHtml.value = await api(`/content/entries/${entry.value.id}/render?${query}`, { responseType: 'text' })
  } catch (e) {
    renderError.value = apiErrorMessage(e, 'Could not render preview')
  } finally {
    rendering.value = false
  }
}

watch(selectedCardFacet, () => {
  if (viewMode.value === 'card') fetchRender()
})

function confirmDelete() {
  if (!confirm(`Delete "${entry.value.title}"? This cannot be undone.`)) return
  entryStore.remove(entry.value.id)
  ui.setActiveEntry(null)
}

function insertSecretBlock() {
  if (!editor.value) return
  editor.value.chain().focus().insertContent({
    type: 'secretBlock',
    content: [{ type: 'paragraph' }],
  }).run()
}

function insertCardBlock() {
  if (!editor.value || !cardFacets.value.length) return
  editor.value.chain().focus().insertContent({
    type: 'cardBlock',
    attrs: { facet: cardFacets.value[0] },
    content: [{ type: 'paragraph' }],
  }).run()
}

function addTag() {
  const tag = tagInput.value.trim().replace(/,$/, '').trim()
  if (tag && !localTags.value.includes(tag)) {
    localTags.value = [...localTags.value, tag]
    save()
  }
  tagInput.value = ''
}

function removeTag(tag) {
  localTags.value = localTags.value.filter(t => t !== tag)
  save()
}

function onTagKeydown(e) {
  if (e.key === 'Enter' || e.key === ',') {
    e.preventDefault()
    addTag()
  }
}

async function printEntry() {
  if (!entry.value) return
  const win = window.open('', '_blank')
  if (!win) return

  const style = win.document.createElement('style')
  style.textContent = [
    'body{font-family:Georgia,serif;background:#fff;color:#222;max-width:700px;margin:40px auto;padding:0 24px;line-height:1.7;}',
    'h1{font-size:1.8em;margin-bottom:0.2em;}h2{font-size:1.3em;margin:1.2em 0 0.3em;}h3{font-size:1.1em;margin:1em 0 0.2em;}',
    'mark{background:none;border-bottom:1px solid #aaa;}p{margin-bottom:0.8em;}ul,ol{padding-left:1.4em;margin-bottom:0.8em;}',
  ].join('')
  win.document.head.appendChild(style)
  win.document.title = entry.value.title

  const titleEl = win.document.createElement('h1')
  titleEl.textContent = entry.value.title
  win.document.body.appendChild(titleEl)

  // Render markdown to HTML with :::secret blocks stripped — print is always
  // a player-safe export, matching the CLI's `player` view (docs/cli-spec.md
  // §4) — then parse + transfer nodes to avoid innerHTML assignment.
  const bodyHtml = await renderMarkdownToHtml(localBody.value, { stripSecrets: true, interactiveLinks: false })
  const parsed = new DOMParser().parseFromString(bodyHtml, 'text/html')
  const content = win.document.createElement('div')
  Array.from(parsed.body.childNodes).forEach(node => {
    content.appendChild(win.document.importNode(node, true))
  })
  win.document.body.appendChild(content)

  win.print()
}

function setLink() {
  if (!editor.value) return
  const existing = editor.value.getAttributes('link').href ?? ''
  const url = prompt('URL:', existing)
  if (url === null) return
  if (url === '') {
    editor.value.chain().focus().unsetLink().run()
  } else {
    editor.value.chain().focus().setLink({ href: url }).run()
  }
}

function goBack() {
  if (!localHistory.value.length) return
  localFuture.value.unshift(ui.activeEntryId!)
  ui.setActiveEntry(localHistory.value.pop()!)
}

function goForward() {
  if (!localFuture.value.length) return
  localHistory.value.push(ui.activeEntryId!)
  ui.setActiveEntry(localFuture.value.shift()!)
}

function onEditorClick(e: MouseEvent) {
  // Wikilink's node view (Wikilink.ts) resolves the target async and stamps
  // data-resolved-id once it knows the entry id — navigation + back/forward
  // history live here rather than in the node view itself, matching how
  // the old .entry-mention delegation worked.
  const link = (e.target as HTMLElement).closest('.wikilink') as HTMLElement | null
  if (link?.dataset.resolvedId) {
    e.preventDefault()
    const nextId = link.dataset.resolvedId
    const currId = ui.activeEntryId
    if (currId) {
      localHistory.value.push(currId)
      localFuture.value = []
    }
    ui.setActiveEntry(nextId)
  }
}
</script>

<style scoped>
.entry-editor { display: flex; flex-direction: column; height: 100%; }

.popup-controls {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 10px;
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
}
.popup-nav { display: flex; gap: 2px; }
.ctrl-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: var(--radius);
  border: 1px solid var(--border);
  background: var(--muted);
  color: var(--muted-foreground);
  cursor: pointer;
  transition: color var(--transition), background var(--transition);
}
.ctrl-btn:hover { color: var(--foreground); background: var(--card); }

.editor-header {
  padding: var(--space-xl) var(--space-2xl) 0;
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
}
.title-row { display: flex; gap: var(--space-sm); align-items: center; margin-bottom: var(--space-md); }
.title-input {
  flex: 1;
  background: none;
  border: none;
  font-family: var(--font-display);
  font-size: var(--text-2xl);
  color: var(--foreground);
  outline: none;
  height: var(--h-xl);
}
.title-input::placeholder { color: var(--muted-foreground); }
.icon-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: var(--h-sm);
  width: var(--h-sm);
  background: none;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  cursor: pointer;
  color: var(--muted-foreground);
  transition: background var(--transition), color var(--transition), border-color var(--transition);
}
.icon-btn:hover { background: var(--muted); color: var(--foreground); }
.delete-btn:hover { color: var(--destructive); background: rgba(220, 38, 38, 0.1); border-color: rgba(220, 38, 38, 0.3); }

.toolbar {
  display: flex;
  gap: 1px;
  padding-bottom: var(--space-sm);
  align-items: center;
}
.tbtn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: var(--h-sm);
  width: var(--h-sm);
  background: none;
  border: none;
  color: var(--muted-foreground);
  border-radius: var(--radius);
  cursor: pointer;
  transition: background var(--transition), color var(--transition);
  flex-shrink: 0;
}
.tbtn:hover { background: var(--muted); color: var(--foreground); }
.tbtn.on { background: var(--secondary); color: var(--primary); }
.tbtn:disabled { opacity: 0.35; cursor: default; }
.secret-btn:hover { color: #e88; }
.card-btn:hover { color: var(--primary); }
.toolbar-divider {
  width: 1px; background: var(--border);
  height: 16px; margin: 0 var(--space-xs);
}
.toolbar-spacer { flex: 1; }

.view-switcher { display: inline-flex; border: 1px solid var(--border); border-radius: var(--radius); overflow: hidden; }
.view-btn {
  background: none; border: none; padding: 0 var(--space-sm);
  height: var(--h-sm); font-size: var(--text-xs); color: var(--muted-foreground);
  cursor: pointer; transition: background var(--transition), color var(--transition);
}
.view-btn:disabled { opacity: 0.35; cursor: default; }
.view-btn.on { background: var(--secondary); color: var(--primary); }
.view-btn + .view-btn { border-left: 1px solid var(--border); }
.facet-select {
  height: var(--h-sm); border: 1px solid var(--border); border-radius: var(--radius);
  background: var(--muted); color: var(--foreground); font-size: var(--text-xs); padding: 0 var(--space-xs);
}
.rendered-view { max-width: 700px; margin: 0 auto; }
.rendered-status, .rendered-error { color: var(--muted-foreground); font-size: var(--text-sm); }
.rendered-error { color: var(--destructive); }
.save-status { font-size: var(--text-xs); color: var(--muted-foreground); margin-left: var(--space-sm); }

.editor-body { display: flex; flex: 1; overflow: hidden; }
.editor-content { flex: 1; padding: var(--space-xl) var(--space-2xl); overflow-y: auto; }

.editor-empty {
  display: flex; align-items: center; justify-content: center;
  height: 100%; color: var(--muted-foreground); font-size: var(--text-md);
}

.tags-row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-xs);
  align-items: center;
  min-height: var(--h-xs);
  margin-bottom: var(--space-sm);
  padding: 0 2px;
}
.tag-chip {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  background: var(--secondary);
  color: var(--primary);
  border-radius: var(--radius);
  height: var(--h-xs);
  padding: 0 var(--space-sm);
  font-size: var(--text-xs);
}
.tag-remove {
  background: none; border: none;
  color: var(--primary); cursor: pointer;
  padding: 0 2px; font-size: 1em; line-height: 1; opacity: 0.7;
}
.tag-remove:hover { opacity: 1; }
.tag-input {
  background: none; border: none;
  color: var(--muted-foreground); font-family: var(--font-body);
  font-size: var(--text-md); outline: none; width: 100px;
  height: var(--h-xs);
}
.tag-input::placeholder { color: var(--muted-foreground); }
</style>
