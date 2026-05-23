<template>
  <div class="qv-panel">
    <div class="qv-header">
      <span class="qv-heading">Quick Info</span>
      <button class="qv-edit-toggle" @click="editingTemplate = !editingTemplate" :title="editingTemplate ? 'Done editing' : 'Edit template fields'">
        {{ editingTemplate ? 'Done' : 'Edit Template' }}
      </button>
    </div>

    <!-- Template field editor -->
    <div class="qv-template-editor" v-if="editingTemplate">
      <div class="te-field" v-for="field in template" :key="field.key">
        <span class="te-label">{{ field.label }}</span>
        <button class="te-remove" @click="quickView.removeField(entry.category, field.key)" title="Remove field">
          <X :size="11" />
        </button>
      </div>
      <div class="te-add-row">
        <input class="te-add-input" v-model="newFieldLabel" placeholder="New field name…" @keydown.enter="addField" />
        <button class="te-add-btn" @click="addField" :disabled="!newFieldLabel.trim()"><Plus :size="13" /></button>
      </div>
      <button class="te-reset" @click="quickView.resetCategory(entry.category)">Reset to defaults</button>
    </div>

    <!-- Field value form -->
    <div class="qv-fields" v-else>
      <div class="qv-field" v-for="field in template" :key="field.key">
        <label class="qv-field-label">{{ field.label }}</label>
        <textarea
          v-if="field.type === 'textarea'"
          class="qv-textarea"
          :value="localData[field.key] ?? ''"
          @input="e => updateField(field.key, e.target.value)"
          @blur="save"
          rows="2"
        />
        <select
          v-else-if="field.type === 'select'"
          class="qv-select"
          :value="localData[field.key] ?? ''"
          @change="e => { updateField(field.key, e.target.value); save() }"
        >
          <option value="">—</option>
          <option v-for="opt in field.options" :key="opt" :value="opt">{{ opt }}</option>
        </select>
        <input
          v-else
          class="qv-input"
          type="text"
          :value="localData[field.key] ?? ''"
          @input="e => updateField(field.key, e.target.value)"
          @blur="save"
        />
      </div>
      <p class="qv-empty" v-if="template.length === 0">
        No fields defined. Click "Edit Template" to add fields.
      </p>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { X, Plus } from 'lucide-vue-next'
import { useEntriesStore } from '../stores/useEntriesStore.js'
import { useQuickViewStore } from '../stores/useQuickViewStore.js'

const props = defineProps({ entryId: String })

const entries = useEntriesStore()
const quickView = useQuickViewStore()

const editingTemplate = ref(false)
const newFieldLabel = ref('')

const entry = computed(() => entries.getById(props.entryId))
const template = computed(() => entry.value ? quickView.getTemplate(entry.value.category) : [])
const localData = ref({})

watch(() => props.entryId, () => {
  localData.value = { ...(entry.value?.quickViewData ?? {}) }
}, { immediate: true })

function updateField(key, value) {
  localData.value = { ...localData.value, [key]: value }
}

function save() {
  if (!entry.value) return
  entries.updateEntry(entry.value.id, { quickViewData: { ...localData.value } })
}

function addField() {
  if (!newFieldLabel.value.trim() || !entry.value) return
  quickView.addField(entry.value.category, newFieldLabel.value.trim())
  newFieldLabel.value = ''
}
</script>

<style scoped>
.qv-panel {
  display: flex;
  flex-direction: column;
  overflow-y: auto;
  flex: 1;
}

.qv-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--space-sm) var(--space-md);
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
}

.qv-heading {
  font-size: var(--text-xs);
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--text-faint);
}

.qv-edit-toggle {
  background: none;
  border: 1px solid var(--border-light);
  border-radius: var(--radius);
  color: var(--text-muted);
  font-family: var(--font-body);
  font-size: var(--text-xs);
  height: var(--h-xs);
  padding: 0 var(--space-sm);
  cursor: pointer;
  transition: background var(--transition), color var(--transition);
}
.qv-edit-toggle:hover { background: var(--bg-hover); color: var(--text-primary); }

.qv-fields { padding: var(--space-sm) var(--space-md); display: flex; flex-direction: column; gap: var(--space-sm); }

.qv-field { display: flex; flex-direction: column; gap: 3px; }
.qv-field-label {
  font-size: var(--text-xs);
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: var(--text-faint);
}
.qv-input, .qv-select {
  background: var(--bg-raised);
  border: 1px solid var(--border-light);
  border-radius: var(--radius);
  color: var(--text-primary);
  font-family: var(--font-body);
  font-size: var(--text-md);
  height: var(--h-sm);
  padding: 0 var(--space-sm);
  outline: none;
  transition: border-color var(--transition);
}
.qv-input:focus, .qv-select:focus { border-color: var(--accent-dim); }
.qv-textarea {
  background: var(--bg-raised);
  border: 1px solid var(--border-light);
  border-radius: var(--radius);
  color: var(--text-primary);
  font-family: var(--font-body);
  font-size: var(--text-md);
  padding: var(--space-xs) var(--space-sm);
  outline: none;
  resize: vertical;
  transition: border-color var(--transition);
}
.qv-textarea:focus { border-color: var(--accent-dim); }

.qv-empty {
  font-size: var(--text-sm);
  color: var(--text-faint);
  padding: var(--space-sm) 0;
}

/* Template editor */
.qv-template-editor { padding: var(--space-sm) var(--space-md); display: flex; flex-direction: column; gap: var(--space-xs); }
.te-field {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: var(--h-sm);
  padding: 0 var(--space-sm);
  background: var(--bg-raised);
  border-radius: var(--radius);
}
.te-label { font-size: var(--text-sm); color: var(--text-muted); }
.te-remove {
  display: inline-flex;
  align-items: center;
  background: none;
  border: none;
  color: var(--text-faint);
  cursor: pointer;
  transition: color var(--transition);
}
.te-remove:hover { color: var(--danger); }
.te-add-row { display: flex; gap: var(--space-xs); }
.te-add-input {
  flex: 1;
  background: var(--bg-raised);
  border: 1px solid var(--border-light);
  border-radius: var(--radius);
  color: var(--text-primary);
  font-family: var(--font-body);
  font-size: var(--text-sm);
  height: var(--h-sm);
  padding: 0 var(--space-sm);
  outline: none;
}
.te-add-input:focus { border-color: var(--accent-dim); }
.te-add-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: var(--h-sm);
  width: var(--h-sm);
  background: var(--accent-dim);
  border: none;
  border-radius: var(--radius);
  color: var(--accent);
  cursor: pointer;
}
.te-add-btn:disabled { opacity: 0.4; cursor: default; }
.te-reset {
  align-self: flex-start;
  background: none;
  border: none;
  color: var(--text-faint);
  font-family: var(--font-body);
  font-size: var(--text-xs);
  cursor: pointer;
  padding: var(--space-xs) 0;
  transition: color var(--transition);
}
.te-reset:hover { color: var(--danger); }
</style>
