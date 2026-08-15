/** Registration link that redeems a project invite token. */
export function inviteLink(token: string): string {
  return `${window.location.origin}/register?invite=${token}`
}
