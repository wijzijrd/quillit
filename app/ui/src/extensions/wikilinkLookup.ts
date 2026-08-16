import { api } from '../api/client'

/** Shape returned by content's #43 search/lookup endpoint (app/content
 * internal/handler/search.go SearchResult) — enough to build a
 * "[[directoryPath/slug|title]]" wikilink without a follow-up request. */
export interface WikilinkSearchResult {
  id: string
  projectId: string
  slug: string
  directoryPath: string
  title: string
  tags: string[]
}

/** Builds the path half of a wikilink from a search result — path is
 * relative to the project root, directoryPath/slug joined, or just slug
 * when the entry lives at the project root (empty directoryPath). */
export function pathForResult(r: WikilinkSearchResult): string {
  return r.directoryPath ? `${r.directoryPath}/${r.slug}` : r.slug
}

/** Backs both the "@" wikilink autocomplete and (via resolveWikilinkPath
 * below) dangling-link detection — one query implementation, per this
 * issue's instruction not to add a third lookup mechanism alongside
 * entry_links (#40). */
export async function searchProjectEntries(
  projectId: string,
  query: string,
  signal?: AbortSignal,
): Promise<WikilinkSearchResult[]> {
  if (!projectId || !query.trim()) return []
  return api(`/content/projects/${encodeURIComponent(projectId)}/search`, {
    query: { mode: 'lookup', q: query },
    signal,
  })
}

// Per-project cache of path -> resolved search result, or null once we've
// confirmed the path doesn't resolve. Keeps repeated renders of the same
// wikilink (or many links to the same target) from re-querying every time.
const resolutionCache = new Map<string, Map<string, WikilinkSearchResult | null>>()

/** Does `path` resolve to a real entry in projectId? Reuses the lookup
 * endpoint (searching on the path's final segment, then matching the full
 * computed path exactly) rather than a separate resolution API. */
export async function resolveWikilinkPath(projectId: string, path: string): Promise<WikilinkSearchResult | null> {
  if (!projectId || !path) return null
  let projectCache = resolutionCache.get(projectId)
  if (!projectCache) {
    projectCache = new Map()
    resolutionCache.set(projectId, projectCache)
  }
  if (projectCache.has(path)) return projectCache.get(path) ?? null

  const slug = path.split('/').pop() || path
  let match: WikilinkSearchResult | null = null
  try {
    const results = await searchProjectEntries(projectId, slug)
    match = results.find(r => pathForResult(r) === path) ?? null
  } catch {
    // Network/API failure — don't cache a negative result for a transient error.
    return null
  }
  projectCache.set(path, match)
  return match
}

/** Clears cached resolutions for a project — call after a save so newly
 * created/renamed entries are picked up rather than staying "dangling". */
export function invalidateWikilinkCache(projectId: string): void {
  resolutionCache.delete(projectId)
}

/** Synchronous cache read: `undefined` means "not resolved yet" (caller
 * should fall back to resolveWikilinkPath), otherwise the cached result (or
 * null for a confirmed-dangling path). Lets a node view paint immediately
 * when a resolution has already been warmed — see warmWikilinkCache. */
export function getCachedResolution(projectId: string, path: string): WikilinkSearchResult | null | undefined {
  return resolutionCache.get(projectId)?.get(path)
}

const WIKILINK_SCAN_RE = /\[\[([^\]|]+)(?:\|[^\]]+)?\]\]/g

/** Extracts every `[[path]]`/`[[path|label]]` target path out of raw
 * markdown text, without needing a full Tiptap/ProseMirror parse. */
export function extractWikilinkPaths(markdown: string): string[] {
  const paths = new Set<string>()
  for (const m of markdown.matchAll(WIKILINK_SCAN_RE)) {
    paths.add(m[1].trim())
  }
  return [...paths]
}

/** Resolves every wikilink path found in `markdown` and waits for them all,
 * warming the cache so a subsequent synchronous render (e.g. the headless
 * Editor useMarkdownRender.ts constructs for getHTML()) can paint
 * resolved/dangling state immediately instead of missing the update because
 * it happened after getHTML() already ran. */
export async function warmWikilinkCache(projectId: string, markdown: string): Promise<void> {
  if (!projectId) return
  const paths = extractWikilinkPaths(markdown)
  await Promise.all(paths.map(p => resolveWikilinkPath(projectId, p)))
}
