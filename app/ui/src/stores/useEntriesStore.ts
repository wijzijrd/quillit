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
  const loaded = ref(false)

  async function init(projectId: string) {
    if (loaded.value) return
    loaded.value = true
    try {
      entries.value = await api(`/content/projects/${projectId}/entries`)
    } catch (e) {
      loaded.value = false
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

  function getById(id: string): ContentEntry | null {
    return entries.value.find(e => e.id === id) ?? null
  }

  function search(query: string): ContentEntry[] {
    if (!query.trim()) return []
    const q = query.toLowerCase()
    return entries.value.filter(e =>
      e.title.toLowerCase().includes(q) ||
      e.body.toLowerCase().includes(q)
    )
  }

  /**
   * Real full-text search (issue #51), replacing the naive client-side
   * `.filter()` in `search()` above (kept for now — still used by the
   * unwired SearchBar.vue). Hits content-svc's project-scoped search
   * endpoint (GET /content/projects/{id}/search?q=), which is FTS5-backed
   * and actually indexes body content, not just whatever's cached locally.
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

  return { entries, loaded, init, createEntry, updateEntry, deleteEntry, getById, search, searchRemote, allTags, byTag }
})
