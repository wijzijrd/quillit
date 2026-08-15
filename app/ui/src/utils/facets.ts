/** Matches app/content/internal/handler/facets.go's kebabRe: lowercase letters, digits, and
 *  hyphens only, no leading/trailing/doubled hyphens. Kept in sync so client-side feedback
 *  matches the server's authoritative check. */
const KEBAB_RE = /^[a-z0-9]+(-[a-z0-9]+)*$/

export function isKebabCase(name: string): boolean {
  return KEBAB_RE.test(name)
}
