import type { Entry, Annotation } from '../types'

export function filterForPlayer(
  entries: Entry[],
  annotations: Annotation[],
  playerId: string,
  campaignId: string | null
): { visibleEntries: Entry[]; visibleAnnotations: Annotation[] } {
  const visibleEntries = entries.filter(e =>
    (e.visibility ?? 'private') === 'public' &&
    (campaignId == null || (e.campaignIds?.includes(campaignId) ?? false))
  )
  const visibleAnnotations = annotations.filter(a => {
    if (a.visibility === 'player') return true
    if (a.visibility === 'shared') return a.sharedWith?.includes(playerId)
    return false
  })
  return { visibleEntries, visibleAnnotations }
}

export function stripGmMarks(html: string): string {
  if (!html) return ''
  const doc = new DOMParser().parseFromString(html, 'text/html')
  doc.querySelectorAll('.annotation-mark--gm').forEach(el => {
    el.replaceWith(...el.childNodes)
  })
  return doc.body.innerHTML
}
