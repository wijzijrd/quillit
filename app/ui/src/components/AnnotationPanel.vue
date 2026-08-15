<template>
  <aside class="annotation-panel">
    <!-- New annotation form (shown when text is selected) -->
    <div class="annotation-form" v-if="pendingSelection">
      <div class="selected-text">"{{ pendingSelection.text }}"</div>
      <textarea
        class="annotation-input"
        v-model="draftText"
        placeholder="Write your GM note…"
        rows="4"
        autofocus
      />
      <div class="visibility-row">
        <label
          v-for="v in VISIBILITIES"
          :key="v.value"
          class="vis-option"
          :class="{ selected: draftVisibility === v.value, [`vis-${v.value}`]: true }"
        >
          <input type="radio" :value="v.value" v-model="draftVisibility" hidden />
          {{ v.label }}
        </label>
      </div>
      <div class="form-actions">
        <button class="btn-save" @click="saveAnnotation" :disabled="!draftText.trim()">Save</button>
        <button class="btn-cancel" @click="emit('cancel')">Cancel</button>
      </div>
    </div>

    <!-- Empty state -->
    <div class="panel-empty" v-else-if="visibleAnnotations.length === 0">
      <p>Select text in the editor to add an annotation.</p>
    </div>

    <!-- Annotation list -->
    <ul class="annotation-list" v-if="visibleAnnotations.length > 0">
      <li
        v-for="ann in visibleAnnotations"
        :key="ann.id"
        class="annotation-item"
        :class="`ann-${ann.visibility}`"
      >
        <div class="ann-header">
          <span class="vis-badge" :class="`vis-${ann.visibility}`">
            {{ VISIBILITY_LABELS[ann.visibility] }}
          </span>
          <div class="ann-actions">
            <button class="ann-btn" @click="startEdit(ann)" title="Edit">✎</button>
            <button class="ann-btn danger" @click="remove(ann.id)" title="Delete">✕</button>
          </div>
        </div>
        <div v-if="editingId === ann.id" class="edit-form">
          <textarea class="annotation-input" v-model="editText" rows="3" />
          <div class="visibility-row">
            <label
              v-for="v in VISIBILITIES"
              :key="v.value"
              class="vis-option"
              :class="{ selected: editVisibility === v.value, [`vis-${v.value}`]: true }"
            >
              <input type="radio" :value="v.value" v-model="editVisibility" hidden />
              {{ v.label }}
            </label>
          </div>
          <div class="form-actions">
            <button class="btn-save" @click="saveEdit(ann.id)">Save</button>
            <button class="btn-cancel" @click="editingId = null">Cancel</button>
          </div>
        </div>
        <p v-else class="ann-text">{{ ann.text }}</p>
      </li>
    </ul>
  </aside>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useAnnotationsStore } from '../stores/useAnnotationsStore'

const props = defineProps<{
  entryId?: string
  previewMode?: boolean
  pendingSelection: { from: number; to: number; text: string } | null
}>()

const emit = defineEmits(['apply-annotation', 'remove-annotation', 'cancel'])

const annotations = useAnnotationsStore()

const VISIBILITIES = [
  { value: 'gm',     label: '🔒 Private' },
  { value: 'shared', label: '👥 Shared' },
]

const VISIBILITY_LABELS: Record<string, string> = { gm: 'Private', shared: 'Shared', player: 'Shared' }

const draftText = ref('')
const draftVisibility = ref('gm')
const editingId = ref<string | null>(null)
const editText = ref('')
const editVisibility = ref('gm')

const visibleAnnotations = computed(() => {
  const list = annotations.getByEntry(props.entryId)
  return props.previewMode ? list.filter(a => a.visibility !== 'gm') : list
})

function saveAnnotation() {
  if (!draftText.value.trim()) return
  const id = annotations.createAnnotation(props.entryId, {
    text: draftText.value.trim(),
    visibility: draftVisibility.value,
  })
  emit('apply-annotation', { id, visibility: draftVisibility.value })
  draftText.value = ''
  draftVisibility.value = 'gm'
}

function startEdit(ann: { id: string; text: string; visibility: string }) {
  editingId.value = ann.id
  editText.value = ann.text
  editVisibility.value = ann.visibility
}

function saveEdit(id: string) {
  annotations.updateAnnotation(id, {
    text: editText.value.trim(),
    visibility: editVisibility.value,
  })
  editingId.value = null
}

function remove(id: string) {
  annotations.deleteAnnotation(id)
  emit('remove-annotation', id)
}
</script>

<style scoped>
.annotation-panel {
  display: flex;
  flex-direction: column;
  overflow-y: auto;
  flex: 1;
}

.annotation-form, .edit-form {
  padding: 12px 14px;
  border-bottom: 1px solid var(--border);
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.selected-text {
  font-size: 0.82em;
  color: var(--muted-foreground);
  font-style: italic;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.annotation-input {
  background: var(--muted);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  color: var(--foreground);
  font-family: var(--font-body);
  font-size: 0.88em;
  padding: 8px 10px;
  resize: vertical;
  width: 100%;
  line-height: 1.5;
}
.annotation-input:focus { outline: none; border-color: var(--secondary); }

.visibility-row {
  display: flex;
  gap: 4px;
}

.vis-option {
  flex: 1;
  text-align: center;
  font-size: 0.75em;
  padding: 4px 2px;
  border-radius: var(--radius);
  border: 1px solid var(--border);
  cursor: pointer;
  color: var(--muted-foreground);
  transition: background var(--transition), color var(--transition), border-color var(--transition);
  user-select: none;
}
.vis-option.selected.vis-gm     { background: rgba(220,80,80,0.2);  border-color: rgba(220,80,80,0.5);  color: #e88; }
.vis-option.selected.vis-shared { background: rgba(80,160,220,0.2); border-color: rgba(80,160,220,0.5); color: #8ae; }

.form-actions {
  display: flex;
  gap: 6px;
}

.btn-save {
  flex: 1;
  background: var(--secondary);
  border: none;
  border-radius: var(--radius);
  color: var(--primary);
  font-family: var(--font-body);
  font-size: 0.84em;
  padding: 5px 10px;
  cursor: pointer;
  transition: background var(--transition);
}
.btn-save:hover:not(:disabled) { background: var(--primary); color: var(--background); }
.btn-save:disabled { opacity: 0.4; cursor: default; }

.btn-cancel {
  background: none;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  color: var(--muted-foreground);
  font-family: var(--font-body);
  font-size: 0.84em;
  padding: 5px 10px;
  cursor: pointer;
  transition: background var(--transition);
}
.btn-cancel:hover { background: var(--muted); }

.panel-empty {
  padding: 20px 16px;
  color: var(--muted-foreground);
  font-size: 0.84em;
  line-height: 1.5;
}

.annotation-list {
  list-style: none;
  flex: 1;
}

.annotation-item {
  padding: 10px 14px;
  border-bottom: 1px solid var(--border);
}

.ann-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 6px;
}

.vis-badge {
  font-size: 0.72em;
  padding: 2px 6px;
  border-radius: var(--radius);
  font-weight: 600;
}
.vis-badge.vis-gm     { background: rgba(220,80,80,0.18);  color: #e88; }
.vis-badge.vis-shared { background: rgba(80,160,220,0.18); color: #8ae; }

.ann-actions { display: flex; gap: 4px; }

.ann-btn {
  background: none; border: none;
  color: var(--muted-foreground); font-size: 0.85em;
  cursor: pointer; padding: 2px 5px; border-radius: var(--radius);
  transition: color var(--transition), background var(--transition);
}
.ann-btn:hover { color: var(--foreground); background: var(--muted); }
.ann-btn.danger:hover { color: var(--destructive); background: rgba(220, 38, 38, 0.1); }

.ann-text {
  font-size: 0.88em;
  color: var(--muted-foreground);
  line-height: 1.5;
  white-space: pre-wrap;
}
</style>
