import { dump, load } from 'js-yaml'

/** Matches content-svc's parse.Frontmatter (pkg/contentengine/parse/types.go): name + tags. */
export interface Frontmatter {
  name: string
  tags: string[]
}

/**
 * Splits a full entry body into its YAML frontmatter and the rest (what
 * TiptapEditor manages — issue #47's editor never sees or round-trips
 * the frontmatter header). Falls back to an empty frontmatter with the
 * whole body as `rest` when no `---`-delimited block is present, or when
 * the block's YAML doesn't parse — never throws, since this runs against
 * real stored content on every entry load.
 *
 * Ports pkg/contentengine/parse's extractFrontmatter algorithm (parse.go)
 * rather than using a regex: CRLF-normalize first, then a line-based,
 * TrimSpace-tolerant fence match. A regex anchored on `\n---\n` cannot
 * correctly express this — content-svc itself produces frontmatter blocks
 * with zero trailing newlines (app/svc/internal/migrate/convert.go's
 * ConvertEntry, when an entry has no body sections) and CRLF-authored
 * content arrives byte-for-byte from blob storage (importer.go doesn't
 * normalize line endings), neither of which a `\n`-requiring regex matches.
 */
export function decomposeFrontmatter(body: string): { frontmatter: Frontmatter; rest: string } {
  const text = body.replace(/\r\n/g, '\n')
  const lines = text.split('\n')
  if (lines[0]?.trim() !== '---') {
    return { frontmatter: { name: '', tags: [] }, rest: body }
  }
  for (let i = 1; i < lines.length; i++) {
    if (lines[i].trim() !== '---') continue
    try {
      const parsed = load(lines.slice(1, i).join('\n')) as Partial<Frontmatter> | null | undefined
      return {
        frontmatter: {
          name: typeof parsed?.name === 'string' ? parsed.name : '',
          tags: Array.isArray(parsed?.tags) ? parsed.tags.map(String) : [],
        },
        rest: lines.slice(i + 1).join('\n').replace(/^\n/, ''),
      }
    } catch {
      return { frontmatter: { name: '', tags: [] }, rest: body }
    }
  }
  return { frontmatter: { name: '', tags: [] }, rest: body }
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
