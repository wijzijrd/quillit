/** Registration link that redeems a project invite token. */
export function inviteLink(token: string): string {
  return `${window.location.origin}/register?invite=${token}`
}

/** In-app route to an entry, preserving project scope when known. */
export function entryPath(projectId: string | null | undefined, entryId: string): string {
  return projectId ? `/projects/${projectId}/entries/${entryId}` : `/entries/${entryId}`
}
