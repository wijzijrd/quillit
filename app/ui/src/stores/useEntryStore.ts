import { defineStore } from 'pinia'
import { api, apiErrorMessage } from '../api/client'

/** content-svc's Entry shape (app/content/internal/handler/entries.go's EntryMeta + Body) — deliberately not the legacy `Entry` type in ../types, which has no content-svc equivalent for category/visibility. */
export interface ContentEntry {
  id: string
  projectId: string
  slug: string
  directoryPath: string
  title: string
  tags: string[]
  body: string
  createdAt: number
  updatedAt: number
}

const MAX_SLUG_ATTEMPTS = 20

/** Kebab-cases a title into a slug matching content-svc's CHECK constraint (lowercase letters, digits, hyphens only), defaulting to "untitled" for a blank/all-punctuation title. */
function kebabCase(input: string): string {
  const slug = input.toLowerCase().trim().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '')
  return slug || 'untitled'
}

function is409(e: unknown): boolean {
  return (e as { response?: { status?: number } } | undefined)?.response?.status === 409
}

export const useEntryStore = defineStore('entry', () => {
  async function get(id: string): Promise<ContentEntry> {
    return await api(`/content/entries/${id}`)
  }

  async function update(id: string, body: string): Promise<ContentEntry> {
    return await api(`/content/entries/${id}`, { method: 'PATCH', body: { body } })
  }

  async function remove(id: string): Promise<void> {
    await api(`/content/entries/${id}`, { method: 'DELETE' })
  }

  /**
   * Creates an entry at the project root (no directoryPath — assigning
   * into a directory is #49's job). Retries with a "-2", "-3", ...
   * suffix on a 409 slug conflict, bounded, matching the CLI's own
   * onConflict=suffix idea but client-side.
   */
  async function create(projectId: string, title: string, body: string): Promise<ContentEntry> {
    const base = kebabCase(title)
    for (let attempt = 0; attempt < MAX_SLUG_ATTEMPTS; attempt++) {
      const slug = attempt === 0 ? base : `${base}-${attempt + 1}`
      try {
        return await api(`/content/projects/${projectId}/entries`, {
          method: 'POST',
          body: { slug, directoryPath: '', body },
        })
      } catch (e: unknown) {
        if (!is409(e)) throw new Error(apiErrorMessage(e, 'Could not create entry'))
      }
    }
    throw new Error(`Could not create entry: too many slug conflicts for "${base}"`)
  }

  return { get, update, remove, create }
})
