import { dump, load } from 'js-yaml'

/** Matches content-svc's parse.Frontmatter (pkg/contentengine/parse/types.go): name + tags. */
export interface Frontmatter {
  name: string
  tags: string[]
}

const FRONTMATTER_RE = /^---\n([\s\S]*?)\n---\n\n?([\s\S]*)$/

/**
 * Splits a full entry body into its YAML frontmatter and the rest (what
 * TiptapEditor manages — issue #47's editor never sees or round-trips
 * the frontmatter header). Falls back to an empty frontmatter with the
 * whole body as `rest` when no `---`-delimited block is present, or when
 * the block's YAML doesn't parse — never throws, since this runs against
 * real stored content on every entry load.
 */
export function decomposeFrontmatter(body: string): { frontmatter: Frontmatter; rest: string } {
  const match = body.match(FRONTMATTER_RE)
  if (!match) return { frontmatter: { name: '', tags: [] }, rest: body }

  try {
    const parsed = load(match[1]) as Partial<Frontmatter> | null | undefined
    return {
      frontmatter: {
        name: typeof parsed?.name === 'string' ? parsed.name : '',
        tags: Array.isArray(parsed?.tags) ? parsed.tags : [],
      },
      rest: match[2] ?? '',
    }
  } catch {
    return { frontmatter: { name: '', tags: [] }, rest: body }
  }
}

/**
 * Reassembles a full entry body from a Frontmatter and TiptapEditor's
 * current markdown output — the inverse of decomposeFrontmatter. Uses
 * js-yaml's dump so a title/tag containing YAML-significant characters
 * (colons, quotes) is correctly escaped, matching what content-svc's own
 * gopkg.in/yaml.v3 parser expects on the way back in.
 */
export function composeFrontmatter(frontmatter: Frontmatter, rest: string): string {
  const yamlText = dump({ name: frontmatter.name, tags: frontmatter.tags })
  return `---\n${yamlText}---\n\n${rest}`
}
