<template>
  <div class="entry-editor" v-if="entry" :class="{ 'player-preview': previewMode }">
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
        <select v-if="inProject" class="cat-select" v-model="localCategory" @change="save">
          <option v-for="c in (cats.projectCategories.length ? cats.projectCategories : cats.categories)" :key="c.id" :value="c.name">{{ c.icon }} {{ c.name }}</option>
        </select>
        <button
          class="vis-toggle icon-btn"
          :class="localVisibility === 'public' ? 'vis-public' : 'vis-private'"
          @click="toggleVisibility"
          :title="localVisibility === 'public' ? 'Public — visible to players' : 'Private — GM only'"
        >
          <Globe v-if="localVisibility === 'public'" :size="14" />
          <Lock v-else :size="14" />
        </button>
        <button class="delete-btn icon-btn" @click="confirmDelete" title="Delete entry">
          <Trash2 :size="14" />
        </button>
      </div>
      <div class="tags-row">
        <span
          v-for="tag in localTags"
          :key="tag"
          class="tag-chip"
          :style="tagColor
            ? { color: tagColor, background: hexToAlpha(tagColor, 0.12), borderColor: hexToAlpha(tagColor, 0.3) }
            : {}"
        >{{ tag }}<button class="tag-remove" @click="removeTag(tag)">×</button></span>
        <input
          class="tag-input"
          v-model="tagInput"
          placeholder="Add tag…"
          @keydown="onTagKeydown"
          @blur="addTag"
        />
      </div>
      <div class="tag-suggestions" v-if="suggestedTags.length > 0">
        <button
          v-for="tag in suggestedTags"
          :key="tag"
          class="tag-suggest-chip"
          @click="applyDefaultTag(tag)"
          type="button"
          :style="tagColor
            ? { color: tagColor, borderColor: hexToAlpha(tagColor, 0.4) }
            : {}"
        >+ {{ tag }}</button>
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
          class="tbtn annotate-btn"
          :class="{ on: !!pendingSelection }"
          :disabled="!hasSelection && !pendingSelection"
          @click="startAnnotation"
          title="Annotate selected text"
        ><Pin :size="14" /></button>
        <button class="tbtn" @click="printEntry" title="Print / export PDF"><Printer :size="14" /></button>
        <span class="toolbar-spacer"></span>
        <button
          class="tbtn preview-btn"
          :class="{ on: previewMode }"
          @click="previewMode = !previewMode"
          :title="previewMode ? 'Switch to GM view' : 'Toggle player preview'"
        >
          <EyeOff v-if="previewMode" :size="14" />
          <Eye v-else :size="14" />
        </button>
        <span class="save-status">{{ saveStatus }}</span>
      </div>
    </div>
    <div class="editor-body">
      <div class="editor-content" @click="onEditorClick">
        <TiptapEditor
          ref="tiptapRef"
          v-model="localBody"
          :uploadImageFn="uploadImage"
          @update:modelValue="debouncedSave"
          @selectionUpdate="onSelectionUpdate"
        />
      </div>
      <div class="right-panel" :class="{ collapsed: panelCollapsed }">
        <div class="panel-tabs">
          <button
            class="panel-tab"
            :class="{ active: activePanel === 'annotations' }"
            @click="activePanel = 'annotations'; panelCollapsed = false"
            title="Notes"
          ><Pin :size="13" /><span class="tab-label">Notes</span></button>
          <button
            class="panel-tab"
            :class="{ active: activePanel === 'links' }"
            @click="activePanel = 'links'; panelCollapsed = false"
            title="Links"
          ><Link :size="13" /><span class="tab-label">Links</span></button>
          <button
            class="panel-tab"
            :class="{ active: activePanel === 'quickview' }"
            @click="activePanel = 'quickview'; panelCollapsed = false"
            title="Quick Info"
          ><LayoutList :size="13" /><span class="tab-label">Info</span></button>
          <button
            class="panel-tab"
            :class="{ active: activePanel === 'share' }"
            @click="activePanel = 'share'; panelCollapsed = false"
            title="Share"
          ><Share2 :size="13" /><span class="tab-label">Share</span></button>
          <button
            class="panel-collapse-btn"
            @click="panelCollapsed = !panelCollapsed"
            :title="panelCollapsed ? 'Expand panel' : 'Collapse panel'"
          >
            <PanelRightClose v-if="!panelCollapsed" :size="13" />
            <PanelRightOpen v-else :size="13" />
          </button>
        </div>
        <AnnotationPanel
          v-if="activePanel === 'annotations'"
          :entryId="entry.id"
          :previewMode="previewMode"
          :pendingSelection="pendingSelection"
          @apply-annotation="applyAnnotation"
          @remove-annotation="removeAnnotation"
          @cancel="pendingSelection = null"
        />
        <LinkedEntriesPanel
          v-if="activePanel === 'links'"
          :entryId="entry.id"
        />
        <QuickViewPanel
          v-if="activePanel === 'quickview'"
          :entryId="entry.id"
        />
        <NoteSharePanel
          v-if="activePanel === 'share'"
          :entryId="entry.id"
        />
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
import { api } from '../api/client'
import {
  Bold, Italic, Heading2, Heading3, List, Minus,
  AlignLeft, AlignCenter, AlignRight, AlignJustify,
  Pin, Eye, EyeOff, Printer, Globe, Lock, Trash2,
  PanelRightClose, PanelRightOpen, Link, LayoutList,
  ChevronLeft, ChevronRight, X, ExternalLink, Share2,
} from 'lucide-vue-next'
import TiptapEditor from './TiptapEditor.vue'
import AnnotationPanel from './AnnotationPanel.vue'
import LinkedEntriesPanel from './LinkedEntriesPanel.vue'
import QuickViewPanel from './QuickViewPanel.vue'
import NoteSharePanel from './NoteSharePanel.vue'
import { hexToAlpha } from '../utils/color'
import { useEntriesStore } from '../stores/useEntriesStore'
import { useCategoriesStore } from '../stores/useCategoriesStore'
import { useAnnotationsStore } from '../stores/useAnnotationsStore'
import { useUIStore } from '../stores/useUIStore'
import { stripGmMarks } from '../composables/useAnnotationVisibility'

defineProps<{ onClose?: () => void }>()

const entries = useEntriesStore()
const cats = useCategoriesStore()
const annotations = useAnnotationsStore()
const ui = useUIStore()
const route = useRoute()
const inProject = computed(() => !!route.params.projectId)

const entry = ref(null)
const localTitle = ref('')
const localCategory = ref('Lore')
const localBody = ref('')
const localVisibility = ref('private')
const localTags = ref([])
const tagInput = ref('')
const saveStatus = ref('')
const previewMode = ref(false)
const hasSelection = ref(false)
const pendingSelection = ref(null)
const activePanel = ref('annotations')
const panelCollapsed = ref(false)

const localHistory = ref<string[]>([])
const localFuture = ref<string[]>([])
const canGoBack = computed(() => localHistory.value.length > 0)
const canGoForward = computed(() => localFuture.value.length > 0)

const tiptapRef = ref(null)
const editor = computed(() => tiptapRef.value?.editor)

watch(() => ui.activeEntryId, async (id) => {
  if (!id) { entry.value = null; return }
  const found = entries.getById(id)
  if (!found) return
  entry.value = found
  localTitle.value = found.title
  localCategory.value = found.category
  localBody.value = found.body
  localVisibility.value = found.visibility ?? 'private'
  localTags.value = found.tags ?? []
  pendingSelection.value = null
  hasSelection.value = false
  // Body may be empty when stored in MinIO (list response omits it).
  // Fetch the full entry to hydrate body content.
  if (!found.body) {
    try {
      const full = await api(`/entries/${id}`)
      localBody.value = full.body ?? ''
    } catch { /* non-critical — editor stays empty */ }
  }
}, { immediate: true })

async function uploadImage(file: File): Promise<string> {
  if (!entry.value) throw new Error('no entry')
  const form = new FormData()
  form.append('image', file)
  const res = await api(`/entries/${entry.value.id}/images`, { method: 'POST', body: form })
  return res.url
}

let saveTimer = null
function debouncedSave() {
  saveStatus.value = 'Unsaved…'
  clearTimeout(saveTimer)
  saveTimer = setTimeout(save, 800)
}

function toggleVisibility() {
  localVisibility.value = localVisibility.value === 'public' ? 'private' : 'public'
  save()
}

function save() {
  if (!entry.value) return
  entries.updateEntry(entry.value.id, {
    title: localTitle.value,
    category: localCategory.value,
    body: localBody.value,
    visibility: localVisibility.value,
    tags: localTags.value,
  })
  saveStatus.value = 'Saved'
  setTimeout(() => saveStatus.value = '', 2000)
}

function confirmDelete() {
  if (!confirm(`Delete "${entry.value.title}"? This cannot be undone.`)) return
  entries.deleteEntry(entry.value.id)
  ui.setActiveEntry(null)
}

function onSelectionUpdate({ empty }) {
  hasSelection.value = !empty
}

function startAnnotation() {
  if (!editor.value) return
  const { from, to } = editor.value.state.selection
  const text = editor.value.state.doc.textBetween(from, to, ' ')
  pendingSelection.value = { from, to, text: text.slice(0, 60) }
  activePanel.value = 'annotations'
}

function applyAnnotation({ id, visibility }) {
  if (!editor.value || !pendingSelection.value) return
  const { from, to } = pendingSelection.value
  // Compute annotation index for multi-word selections
  let count = 0
  editor.value.state.doc.nodesBetween(0, editor.value.state.doc.content.size, (node) => {
    node.marks.forEach(m => { if (m.type.name === 'annotation') count++ })
  })
  const isMultiWord = pendingSelection.value.text.includes(' ')
  const annotationIndex = isMultiWord ? count + 1 : null
  editor.value.chain()
    .setTextSelection({ from, to })
    .setMark('annotation', { annotationId: id, visibility, annotationIndex })
    .run()
  pendingSelection.value = null
  localBody.value = editor.value.getHTML()
  save()
}

function removeAnnotation(id) {
  if (!editor.value) return
  editor.value.commands.command(({ tr, dispatch }) => {
    tr.doc.nodesBetween(0, tr.doc.content.size, (node, pos) => {
      node.marks.forEach(mark => {
        if (mark.type.name === 'annotation' && mark.attrs.annotationId === id) {
          tr.removeMark(pos, pos + node.nodeSize, mark)
        }
      })
    })
    dispatch?.(tr)
    return true
  })
  localBody.value = editor.value.getHTML()
  save()
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

function printEntry() {
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

  const catEl = win.document.createElement('p')
  catEl.style.cssText = 'font-size:0.75em;text-transform:uppercase;letter-spacing:0.1em;color:#666;'
  catEl.textContent = entry.value.category
  win.document.body.appendChild(catEl)

  const titleEl = win.document.createElement('h1')
  titleEl.textContent = entry.value.title
  win.document.body.appendChild(titleEl)

  // Parse body through DOMParser then transfer nodes to avoid innerHTML assignment
  const bodyHtml = stripGmMarks(localBody.value)
  const parsed = new DOMParser().parseFromString(bodyHtml, 'text/html')
  const content = win.document.createElement('div')
  Array.from(parsed.body.childNodes).forEach(node => {
    content.appendChild(win.document.importNode(node, true))
  })
  win.document.body.appendChild(content)

  // Annotation footer: list player/shared annotations
  const entryAnnotations = annotations.getByEntry(entry.value.id)
    .filter(a => a.visibility !== 'gm')
  if (entryAnnotations.length > 0) {
    const hr = win.document.createElement('hr')
    hr.style.cssText = 'margin:24px 0 12px;border:none;border-top:1px solid #ccc;'
    win.document.body.appendChild(hr)

    const footer = win.document.createElement('section')
    footer.className = 'ann-print-footer'
    footer.style.cssText = 'font-size:0.85em;color:#444;'

    const heading = win.document.createElement('p')
    heading.style.cssText = 'font-weight:600;margin-bottom:8px;text-transform:uppercase;letter-spacing:0.06em;font-size:0.8em;color:#888;'
    heading.textContent = 'Notes'
    footer.appendChild(heading)

    entryAnnotations.forEach(ann => {
      const item = win.document.createElement('p')
      item.style.cssText = 'margin-bottom:6px;'
      const prefix = win.document.createElement('span')
      prefix.style.cssText = 'font-weight:600;margin-right:4px;'
      prefix.textContent = ann.annotationIndex != null ? `[${ann.annotationIndex}]` : '•'
      item.appendChild(prefix)
      item.appendChild(win.document.createTextNode(' ' + ann.text))
      footer.appendChild(item)
    })
    win.document.body.appendChild(footer)
  }

  win.print()
}

const suggestedTags = computed(() => {
  const defaults = cats.defaultTagsFor(localCategory.value)
  return defaults.filter(t => !localTags.value.includes(t))
})

const tagColor = computed(() => cats.categoryFor(localCategory.value)?.color ?? null)

function applyDefaultTag(tag) {
  if (!localTags.value.includes(tag)) {
    localTags.value = [...localTags.value, tag]
    save()
  }
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
  const mention = (e.target as HTMLElement).closest('.entry-mention') as HTMLElement | null
  if (mention?.dataset.entryId) {
    e.preventDefault()
    const nextId = mention.dataset.entryId
    const currId = ui.activeEntryId
    if (currId) {
      localHistory.value.push(currId)
      localFuture.value = []
    }
    ui.setActiveEntry(nextId)
    return
  }
  if (e.shiftKey) {
    const mark = (e.target as HTMLElement).closest('.annotation-mark') as HTMLElement | null
    if (mark) {
      activePanel.value = 'annotations'
      panelCollapsed.value = false
    }
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
.cat-select {
  background: var(--muted);
  border: 1px solid var(--border);
  color: var(--muted-foreground);
  font-family: var(--font-body);
  font-size: var(--text-sm);
  height: var(--h-md);
  padding: 0 var(--space-sm);
  border-radius: var(--radius);
  cursor: pointer;
}
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
.vis-toggle.vis-public { border-color: rgba(80,200,120,0.4); background: rgba(80,200,120,0.08); color: #8e8; }
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
.annotate-btn.on { background: rgba(200,146,42,0.15); color: var(--primary); }
.preview-btn.on { background: rgba(80,200,120,0.15); color: #8e8; }
.toolbar-divider {
  width: 1px; background: var(--border);
  height: 16px; margin: 0 var(--space-xs);
}
.toolbar-spacer { flex: 1; }
.save-status { font-size: var(--text-xs); color: var(--muted-foreground); margin-left: var(--space-sm); }

.editor-body { display: flex; flex: 1; overflow: hidden; }
.editor-content { flex: 1; padding: var(--space-xl) var(--space-2xl); overflow-y: auto; }

.right-panel {
  width: 260px;
  min-width: 260px;
  border-left: 1px solid var(--border);
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: var(--card);
  transition: width 160ms ease, min-width 160ms ease;
}
.right-panel.collapsed {
  width: var(--h-lg);
  min-width: var(--h-lg);
}

.panel-tabs {
  display: flex;
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
  align-items: center;
}
.panel-tab {
  flex: 1;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: var(--space-xs);
  background: none;
  border: none;
  border-bottom: 2px solid transparent;
  color: var(--muted-foreground);
  height: var(--h-md);
  cursor: pointer;
  transition: color var(--transition), border-color var(--transition);
  margin-bottom: -1px;
  white-space: nowrap;
  overflow: hidden;
}
.panel-tab:hover { color: var(--muted-foreground); }
.panel-tab.active { color: var(--foreground); border-bottom-color: var(--primary); }
.tab-label {
  font-family: var(--font-body);
  font-size: var(--text-xs);
}
.right-panel.collapsed .tab-label { display: none; }
.right-panel.collapsed .panel-tab { flex: none; width: var(--h-lg); }

.panel-collapse-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: var(--h-sm);
  height: var(--h-md);
  background: none;
  border: none;
  color: var(--muted-foreground);
  cursor: pointer;
  flex-shrink: 0;
  transition: color var(--transition);
}
.panel-collapse-btn:hover { color: var(--foreground); }

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


.tag-suggestions {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  padding: 4px 0 6px;
}
.tag-suggest-chip {
  font-size: 0.75em;
  padding: 2px 8px;
  border: 1px dashed var(--border);
  border-radius: 12px;
  background: none;
  color: var(--muted-foreground);
  cursor: pointer;
  transition: color var(--transition), border-color var(--transition);
}
.tag-suggest-chip:hover { color: var(--primary); border-color: var(--primary); }
</style>
