import { describe, expect, it } from 'vitest'
import { composeFrontmatter, decomposeFrontmatter } from '../frontmatter'

describe('decomposeFrontmatter', () => {
  it('splits a well-formed frontmatter block from the rest of the body', () => {
    const body = '---\nname: Tom the Innkeeper\ntags:\n  - npc\n  - waterdeep\n---\n\n# Tom\n\nRuns the inn.\n'
    const { frontmatter, rest } = decomposeFrontmatter(body)
    expect(frontmatter).toEqual({ name: 'Tom the Innkeeper', tags: ['npc', 'waterdeep'] })
    expect(rest).toBe('# Tom\n\nRuns the inn.\n')
  })

  it('defaults tags to an empty array when absent', () => {
    const body = '---\nname: Solo Entry\n---\n\nBody text.\n'
    const { frontmatter } = decomposeFrontmatter(body)
    expect(frontmatter).toEqual({ name: 'Solo Entry', tags: [] })
  })

  it('falls back to an empty frontmatter and the whole body as rest when no frontmatter block is present', () => {
    const body = 'Just prose, no frontmatter at all.\n'
    const { frontmatter, rest } = decomposeFrontmatter(body)
    expect(frontmatter).toEqual({ name: '', tags: [] })
    expect(rest).toBe(body)
  })

  it('falls back gracefully on malformed YAML inside the frontmatter block', () => {
    const body = '---\nname: [unterminated\n---\n\nBody.\n'
    const { frontmatter, rest } = decomposeFrontmatter(body)
    expect(frontmatter).toEqual({ name: '', tags: [] })
    expect(rest).toBe(body)
  })

  it('handles frontmatter with only a single newline before body (Go code compatibility)', () => {
    const body = '---\nname: Solo\n---\nBody immediately here.\n'
    const { frontmatter, rest } = decomposeFrontmatter(body)
    expect(frontmatter).toEqual({ name: 'Solo', tags: [] })
    expect(rest).toBe('Body immediately here.\n')
  })
})

describe('composeFrontmatter', () => {
  it('produces a frontmatter block followed by the rest, parseable back to the same values', () => {
    const composed = composeFrontmatter({ name: 'Tom', tags: ['npc'] }, '# Tom\n\nRuns the inn.\n')
    expect(composed.startsWith('---\n')).toBe(true)
    const { frontmatter, rest } = decomposeFrontmatter(composed)
    expect(frontmatter).toEqual({ name: 'Tom', tags: ['npc'] })
    expect(rest).toBe('# Tom\n\nRuns the inn.\n')
  })
})

describe('round-trip fidelity', () => {
  it('survives a title containing a colon, a quote, and other YAML-significant characters', () => {
    const original = { name: `Tom: The Innkeeper's "Best" Friend`, tags: ['npc', 'has: colon'] }
    const composed = composeFrontmatter(original, 'Body.\n')
    const { frontmatter } = decomposeFrontmatter(composed)
    expect(frontmatter).toEqual(original)
  })

  it('survives an empty title and no tags', () => {
    const original = { name: '', tags: [] }
    const composed = composeFrontmatter(original, 'Body.\n')
    const { frontmatter } = decomposeFrontmatter(composed)
    expect(frontmatter).toEqual(original)
  })
})
