import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { api } from '../api/client'
import { useEntryStore, type ContentEntry } from './useEntryStore'

/** One hit from #43's content-svc full-text search (see app/content/internal/handler/search.go's SearchResult). */
export interface EntrySearchResult {
  id: string
  projectId: string
  slug: string
  directoryPath: string
  title: string
  tags: string[]
}

export const useEntriesStore = defineStore('entries', () => {
  const entries = ref<ContentEntry[]>([])

  // Keyed by project id (not a plain boolean) so a fresh component mount for
  // a *different* project still triggers a fetch — see QuillitView.vue's
  // onMounted/watch, which both just call init(id) and rely on this guard to
  // no-op only when the requested project is already loaded.
  const loadedProjectId = ref<string | null>(null)
  const loaded = computed(() => loadedProjectId.value !== null)

  // Bumped on every init() call and compared on resolution so an
  // out-of-order response (e.g. rapid in-app project switching firing
  // overlapping requests — QuillitView does this) can't clobber `entries`
  // with stale data once a newer call has already superseded it.
  let initToken = 0

  async function init(projectId: string) {
    if (loadedProjectId.value === projectId) return
    const token = ++initToken
    loadedProjectId.value = projectId
    try {
      const result = await api(`/content/projects/${projectId}/entries`)
      if (token === initToken) entries.value = result
    } catch (e) {
      if (token === initToken) loadedProjectId.value = null
      throw e
    }
  }

  async function createEntry(projectId: string, title: string): Promise<ContentEntry> {
    const entry = await useEntryStore().create(projectId, title, '')
    entries.value.unshift(entry)
    return entry
  }

  async function updateEntry(id: string, patch: Partial<ContentEntry>) {
    const updated: ContentEntry = await api(`/content/entries/${id}`, { method: 'PATCH', body: patch })
    const idx = entries.value.findIndex(e => e.id === id)
    if (idx !== -1) entries.value[idx] = updated
    return updated
  }

  async function deleteEntry(id: string) {
    await api(`/content/entries/${id}`, { method: 'DELETE' })
    entries.value = entries.value.filter(e => e.id !== id)
  }

  /**
   * Drops the cached project and its entries — used when navigating out of
   * project context entirely (content-svc has no cross-project list, so
   * there's nothing to show there either). Resetting `loadedProjectId` here
   * (not just `entries`) matters: without it, navigating away and back to
   * the *same* project would see `loadedProjectId` still matching and
   * `init` would no-op, leaving the just-cleared empty list on screen.
   */
  function clear() {
    entries.value = []
    loadedProjectId.value = null
  }

  function getById(id: string): ContentEntry | null {
    return entries.value.find(e => e.id === id) ?? null
  }

  /**
   * Real full-text search (issue #51). Hits content-svc's project-scoped
   * search endpoint (GET /content/projects/{id}/search?q=), which is
   * FTS5-backed and actually indexes body content, not just whatever's
   * cached locally.
   *
   * The endpoint is per-project, but the dashboard/global search this backs
   * has always searched across every project a user belongs to — so this
   * fans out one request per project id and merges the results. Failures
   * for an individual project (e.g. one the caller just lost access to) are
   * swallowed rather than failing the whole search.
   */
  async function searchRemote(query: string, projectIds: string[]): Promise<EntrySearchResult[]> {
    const q = query.trim()
    if (!q || !projectIds.length) return []
    const settled = await Promise.allSettled(
      projectIds.map(async (id) => {
        const hits: EntrySearchResult[] = await api(`/content/projects/${id}/search?q=${encodeURIComponent(q)}`)
        return hits
      })
    )
    return settled.flatMap(r => (r.status === 'fulfilled' ? r.value : []))
  }

  const allTags = computed(() =>
    [...new Set(entries.value.flatMap(e => e.tags ?? []))].sort()
  )

  function byTag(tag: string): ContentEntry[] {
    return entries.value.filter(e => e.tags?.includes(tag))
  }

  return { entries, loaded, init, clear, createEntry, updateEntry, deleteEntry, getById, searchRemote, allTags, byTag }
})
