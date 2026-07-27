/** Registration link that redeems a project invite token. */
export function inviteLink(token: string): string {
  return `${window.location.origin}/register?invite=${token}`
}

/** Read-only share link for a player token. */
export function playerShareLink(token: string): string {
  return `${window.location.origin}/share/${token}`
}
