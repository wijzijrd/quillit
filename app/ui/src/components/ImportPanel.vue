<template>
  <Dialog v-model:open="open" @update:open="onOpenChange">
    <DialogTrigger as-child>
      <Button variant="outline">Import from CLI</Button>
    </DialogTrigger>
    <DialogContent class="sm:max-w-lg">
      <DialogHeader>
        <DialogTitle>Import from CLI</DialogTitle>
        <DialogDescription>
          Upload a project tarball built by <code>quillit push --output</code>. Preview
          runs automatically as a dry run — nothing is written until you apply.
        </DialogDescription>
      </DialogHeader>

      <div class="import-panel-body">
        <label class="import-field">
          <span class="import-field-label">Tarball</span>
          <input type="file" accept=".tar.gz,.tgz" @change="onFileChange" :disabled="isBusy" />
        </label>
        <p class="import-size-error" v-if="store.sizeError">{{ store.sizeError }}</p>

        <label class="import-field">
          <span class="import-field-label">On conflict</span>
          <select
            class="import-select"
            :value="store.onConflict"
            :disabled="isBusy"
            @change="onOnConflictChange"
          >
            <option value="fail">Fail (default) — stop on any conflict</option>
            <option value="skip">Skip — leave the existing entry alone</option>
            <option value="overwrite">Overwrite — replace the existing entry</option>
            <option value="suffix">Suffix — append -2, -3, … to the slug</option>
          </select>
        </label>

        <label class="import-checkbox-field">
          <input
            type="checkbox"
            :checked="store.createFacets"
            :disabled="isBusy"
            @change="onCreateFacetsChange"
          />
          <span>Create missing facets automatically</span>
        </label>

        <div class="import-status" v-if="store.status === 'loading' || store.status === 'applying'">
          Working…
        </div>

        <div class="import-report" v-else-if="store.status === 'success' && successResult">
          <table class="import-report-table">
            <thead>
              <tr><th>Path</th><th>Action</th><th>Detail</th></tr>
            </thead>
            <tbody>
              <tr v-for="row in successResult.report" :key="row.path" :class="rowClass(row.action)">
                <td>{{ row.path }}</td>
                <td>{{ row.action }}</td>
                <td>{{ row.detail }}</td>
              </tr>
            </tbody>
          </table>
          <p v-if="successResult.facetsCreated.length" class="import-note">
            Facets to create: {{ successResult.facetsCreated.join(', ') }}
          </p>
          <p v-if="successResult.images.length" class="import-note">
            {{ successResult.images.length }} image(s) will be uploaded.
          </p>
        </div>

        <div class="import-validation" v-else-if="store.status === 'validation' && validationResult">
          <p class="import-error">Import can't proceed until these are fixed:</p>
          <ul class="import-error-list">
            <li v-for="e in validationResult.entries" :key="e.path">{{ e.path }}: {{ e.error }}</li>
          </ul>
          <p
            v-if="validationResult.missingFacets.length && !store.createFacets"
            class="import-note"
          >
            {{ validationResult.missingFacets.length }} facet(s) not in this project's vocabulary
            ({{ validationResult.missingFacets.join(', ') }}) — check "Create missing facets
            automatically" above and re-select the file to add them.
          </p>
        </div>

        <div class="import-generic-error" v-else-if="store.status === 'error' && errorResult">
          <p class="import-error">{{ errorResult.message }}</p>
        </div>

        <div class="import-applied" v-else-if="store.status === 'applied' && appliedResult">
          <p class="import-success">
            Import applied — {{ createdCount }} entr{{ createdCount === 1 ? 'y' : 'ies' }} created,
            {{ appliedResult.facetsCreated.length }} facet(s) added,
            {{ appliedResult.images.length }} image(s) uploaded.
          </p>
        </div>
      </div>

      <DialogFooter>
        <Button v-if="store.status === 'applied'" variant="outline" @click="importAnother">
          Import another
        </Button>
        <Button v-if="store.status === 'applied'" @click="closeDialog">Done</Button>
        <Button
          v-else
          :disabled="store.status !== 'success' || isBusy"
          @click="onApply"
        >
          Apply
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref } from 'vue'
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger,
} from './ui/dialog'
import { Button } from './ui/button'
import { useImportStore, type ImportSuccess, type ImportValidationError, type ImportGenericError } from '../stores/useImportStore'
import { useFacetsStore } from '../stores/useFacetsStore'

const props = defineProps<{ projectId: string }>()

const store = useImportStore()
const facets = useFacetsStore()
const open = ref(false)

const isBusy = computed(() => store.status === 'loading' || store.status === 'applying')

const successResult = computed(() => (store.result?.kind === 'success' ? (store.result as ImportSuccess) : null))
const appliedResult = computed(() => (store.status === 'applied' && store.result?.kind === 'success' ? (store.result as ImportSuccess) : null))
const validationResult = computed(() => (store.result?.kind === 'validation' ? (store.result as ImportValidationError) : null))
const errorResult = computed(() => (store.result?.kind === 'error' ? (store.result as ImportGenericError) : null))
const createdCount = computed(() => appliedResult.value?.report.filter(r => r.action === 'create').length ?? 0)

function rowClass(action: string) {
  if (action === 'conflict' || action === 'error') return 'import-row-bad'
  if (action === 'skip') return 'import-row-neutral'
  return 'import-row-ok'
}

function onFileChange(e: Event) {
  const f = (e.target as HTMLInputElement).files?.[0]
  if (f) store.selectFile(props.projectId, f)
}

function onOnConflictChange(e: Event) {
  store.setOnConflict(props.projectId, (e.target as HTMLSelectElement).value as typeof store.onConflict)
}

function onCreateFacetsChange(e: Event) {
  store.setCreateFacets(props.projectId, (e.target as HTMLInputElement).checked)
}

async function onApply() {
  await store.apply(props.projectId)
  if (store.status === 'applied' && appliedResult.value?.facetsCreated.length) {
    await facets.fetchForProject(props.projectId)
  }
}

function importAnother() {
  store.reset()
}

/**
 * Setting `open` here (rather than relying on `@update:open`) is deliberate: reka-ui's
 * DialogRoot only emits `update:open` when *it* changes state (Escape, overlay click, its
 * own DialogClose). A parent-initiated `open.value = false` doesn't round-trip back through
 * that emit, so `onOpenChange` below wouldn't fire and the store would stay stale for next
 * open. Reset explicitly instead of depending on the emit.
 */
function closeDialog() {
  open.value = false
  store.reset()
}

function onOpenChange(isOpen: boolean) {
  if (!isOpen) store.reset()
}

onUnmounted(() => store.reset())
</script>

<style scoped>
.import-panel-body { display: flex; flex-direction: column; gap: 1rem; }
.import-field { display: flex; flex-direction: column; gap: 0.25rem; }
.import-field-label { font-size: var(--text-sm); color: var(--muted-foreground); }
.import-select { border: 1px solid var(--border); border-radius: var(--radius); padding: 0.375rem 0.5rem; background: var(--background); color: var(--foreground); }
.import-checkbox-field { display: flex; align-items: center; gap: 0.5rem; font-size: var(--text-sm); }
.import-size-error, .import-error { color: var(--destructive); font-size: var(--text-sm); }
.import-note { color: var(--muted-foreground); font-size: var(--text-sm); }
.import-success { color: var(--foreground); font-size: var(--text-sm); }
.import-error-list { font-size: var(--text-sm); color: var(--destructive); padding-left: 1.25rem; }
.import-report-table { width: 100%; border-collapse: collapse; font-size: var(--text-sm); }
.import-report-table th { text-align: left; color: var(--muted-foreground); font-weight: 500; padding: 0.25rem 0.5rem; }
.import-report-table td { padding: 0.25rem 0.5rem; border-top: 1px solid var(--border); }
.import-row-ok td:nth-child(2) { color: var(--primary); }
.import-row-bad td:nth-child(2) { color: var(--destructive); }
.import-row-neutral td:nth-child(2) { color: var(--muted-foreground); }
</style>
