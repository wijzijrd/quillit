/** Short date, e.g. "Jan 5, 2026". Input is unix seconds. */
export function formatDate(unixSeconds: number): string {
  return new Date(unixSeconds * 1000).toLocaleDateString(undefined, {
    month: 'short', day: 'numeric', year: 'numeric',
  })
}

/** Medium date + short time, e.g. "Jan 5, 2026, 3:45 PM". Input is unix seconds. */
export function formatDateTime(unixSeconds: number): string {
  return new Date(unixSeconds * 1000).toLocaleString(undefined, {
    dateStyle: 'medium', timeStyle: 'short',
  })
}

/** Relative time, e.g. "3m ago", falling back to a short date beyond 24h. Input is unix seconds. */
export function timeAgo(unixSeconds: number): string {
  const diffMs = Date.now() - unixSeconds * 1000
  const m = Math.floor(diffMs / 60000)
  if (m < 1) return 'just now'
  if (m < 60) return `${m}m ago`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h ago`
  return new Date(unixSeconds * 1000).toLocaleDateString()
}
