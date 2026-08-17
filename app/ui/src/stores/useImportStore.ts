import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api, apiErrorMessage } from '../api/client'

export interface ImportOptions {
  onConflict: 'fail' | 'skip' | 'overwrite' | 'suffix'
  createFacets: boolean
}

/** Mirrors app/content/internal/handler/importer.go's ImportReportRow. */
export interface ImportReportRow {
  path: string
  action: 'create' | 'overwrite' | 'skip' | 'conflict' | 'error'
  detail?: string
}

/** Mirrors app/content/internal/handler/importer.go's ImportImageRow. */
export interface ImportImageRow {
  path: string
  uploaded: boolean
  detail?: string
}

export interface ImportSuccess {
  kind: 'success'
  applied: boolean
  report: ImportReportRow[]
  facetsCreated: string[]
  images: ImportImageRow[]
}

export interface ImportValidationError {
  kind: 'validation'
  entries: { path: string; error: string }[]
  missingFacets: string[]
}

export interface ImportGenericError {
  kind: 'error'
  message: string
}

/** What a dry-run or apply request settles to — the endpoint's three possible response shapes, normalized. */
export type ImportResult = ImportSuccess | ImportValidationError | ImportGenericError

export type ImportStatus = 'idle' | 'loading' | 'success' | 'validation' | 'error' | 'applying' | 'applied'

/** Mirrors app/content/internal/handler/importer.go's maxImportBody (50 << 20) — reject oversized files client-side. */
export const MAX_IMPORT_BYTES = 50 * 1024 * 1024

const PREVIEW_DEBOUNCE_MS = 300

interface RawImportSuccess {
  applied: boolean
  report: ImportReportRow[]
  facets: { created: string[] }
  images: ImportImageRow[]
}

interface RawImportValidationError {
  error: string
  entries: { path: string; error: string }[]
  missingFacets: string[]
}

async function runImport(mode: 'dry-run' | 'apply', projectId: string, file: File, opts: ImportOptions): Promise<ImportResult> {
  const query = new URLSearchParams({
    mode,
    onConflict: opts.onConflict,
    createFacets: String(opts.createFacets),
  })
  try {
    const raw: RawImportSuccess = await api(`/content/projects/${projectId}/import?${query}`, {
      method: 'POST',
      body: file,
      headers: { 'Content-Type': 'application/gzip' },
    })
    return {
      kind: 'success',
      applied: raw.applied,
      report: raw.report,
      facetsCreated: raw.facets.created,
      images: raw.images,
    }
  } catch (e: unknown) {
    const status = (e as { response?: { status?: number } } | undefined)?.response?.status
    if (status === 422) {
      const data = (e as { data?: RawImportValidationError }).data
      return { kind: 'validation', entries: data?.entries ?? [], missingFacets: data?.missingFacets ?? [] }
    }
    return { kind: 'error', message: apiErrorMessage(e, 'Import failed') }
  }
}

export const useImportStore = defineStore('import', () => {
  const file = ref<File | null>(null)
  const onConflict = ref<ImportOptions['onConflict']>('fail')
  const createFacets = ref(false)
  const status = ref<ImportStatus>('idle')
  const result = ref<ImportResult | null>(null)
  const sizeError = ref('')

  let debounceTimer: ReturnType<typeof setTimeout> | undefined
  let requestSeq = 0

  function currentOptions(): ImportOptions {
    return { onConflict: onConflict.value, createFacets: createFacets.value }
  }

  async function runPreview(projectId: string) {
    if (!file.value) return
    const seq = ++requestSeq
    status.value = 'loading'
    const res = await runImport('dry-run', projectId, file.value, currentOptions())
    if (seq !== requestSeq) return // superseded by a newer request — discard
    result.value = res
    status.value = res.kind === 'success' ? 'success' : res.kind
  }

  /** Picking a file clears any prior report and starts an immediate preview. */
  function selectFile(projectId: string, f: File) {
    clearTimeout(debounceTimer)
    sizeError.value = ''
    result.value = null
    if (f.size > MAX_IMPORT_BYTES) {
      file.value = null
      sizeError.value = `File too large (${(f.size / 1024 / 1024).toFixed(1)} MB) — 50 MB max.`
      status.value = 'idle'
      return
    }
    file.value = f
    status.value = 'idle'
    void runPreview(projectId)
  }

  function scheduleRepreview(projectId: string) {
    if (!file.value) return
    clearTimeout(debounceTimer)
    debounceTimer = setTimeout(() => void runPreview(projectId), PREVIEW_DEBOUNCE_MS)
  }

  function setOnConflict(projectId: string, value: ImportOptions['onConflict']) {
    onConflict.value = value
    scheduleRepreview(projectId)
  }

  function setCreateFacets(projectId: string, value: boolean) {
    createFacets.value = value
    scheduleRepreview(projectId)
  }

  /** No-op unless a successful preview is currently showing — prevents double-apply and applying a stale/no report. */
  async function apply(projectId: string) {
    if (!file.value || status.value !== 'success') return
    clearTimeout(debounceTimer)
    const seq = ++requestSeq
    status.value = 'applying'
    const res = await runImport('apply', projectId, file.value, currentOptions())
    if (seq !== requestSeq) return
    result.value = res
    status.value = res.kind === 'success' ? 'applied' : res.kind
  }

  function reset() {
    clearTimeout(debounceTimer)
    requestSeq++ // invalidate any in-flight request
    file.value = null
    onConflict.value = 'fail'
    createFacets.value = false
    status.value = 'idle'
    result.value = null
    sizeError.value = ''
  }

  return {
    file, onConflict, createFacets, status, result, sizeError,
    selectFile, setOnConflict, setCreateFacets, apply, reset,
  }
})
