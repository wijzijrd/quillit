import { beforeEach, describe, expect, it } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { Editor } from '@tiptap/core'
import { buildMarkdownExtensions } from '../markdownExtensions'

/**
 * Round-trip fidelity is issue #47's core correctness property: markdown in
 * (load) -> Tiptap doc (ephemeral, in-editor) -> markdown out (save) must be
 * semantically lossless. These are pure-function tests against the
 * serializer/parser pair, independent of any Vue component or live API —
 * exactly the "assert on the actual serialized markdown string" the issue
 * asks for, not just "the UI looks right".
 */

function markdownToMarkdown(markdown: string): string {
  const editor = new Editor({
    extensions: buildMarkdownExtensions(),
    content: markdown,
  })
  try {
    return editor.storage.markdown.getMarkdown()
  } finally {
    editor.destroy()
  }
}

// Loosely typed on purpose: these tests dig into arbitrary node shapes
// (secretBlock/cardBlock/wikilink attrs, nested content) that don't fit
// Tiptap's generic JSONContent typing cleanly, and precision here isn't
// the point — the round-trip markdown string assertions above are.
function markdownToJSON(markdown: string): any {
  const editor = new Editor({
    extensions: buildMarkdownExtensions(),
    content: markdown,
  })
  try {
    return editor.getJSON()
  } finally {
    editor.destroy()
  }
}

/** Normalizes only whitespace runs/edges — the one divergence the
 * acceptance criteria explicitly allow. */
function normalize(md: string): string {
  return md.trim().replace(/[ \t]+\n/g, '\n').replace(/\n{3,}/g, '\n\n')
}

beforeEach(() => {
  setActivePinia(createPinia())
})

describe('markdown round trip', () => {
  it('round-trips plain prose (paragraphs, headings, bold, list)', () => {
    const md = [
      '# Tom the Innkeeper',
      '',
      'Tom runs the **Gilded Goose** inn.',
      '',
      '- Friendly',
      '- Talkative',
    ].join('\n')

    const out = markdownToMarkdown(md)
    expect(normalize(out)).toBe(normalize(md))

    // Idempotency: serializing a second time from the round-tripped output
    // produces byte-identical markdown — proves the transform is stable,
    // not just "close enough on the first pass".
    expect(markdownToMarkdown(out)).toBe(out)
  })

  it('round-trips a :::secret block', () => {
    const md = ':::secret\nTom is secretly a spy for the Crimson Hand.\n:::'
    const out = markdownToMarkdown(md)
    expect(normalize(out)).toBe(normalize(md))
    expect(markdownToMarkdown(out)).toBe(out)

    const doc = markdownToJSON(md)
    const secret = doc.content?.find((n: any) => n.type === 'secretBlock')
    expect(secret).toBeTruthy()
  })

  it('round-trips a :::card <facet> block, preserving the facet', () => {
    const md = ':::card motivation\nWants to buy back his family farm.\n:::'
    const out = markdownToMarkdown(md)
    expect(normalize(out)).toBe(normalize(md))

    const doc = markdownToJSON(md)
    const card = doc.content?.find((n: any) => n.type === 'cardBlock')
    expect(card?.attrs?.facet).toBe('motivation')
  })

  it('round-trips a card block nested inside a secret block', () => {
    const md = [
      ':::secret',
      'Only the DM should know this much.',
      '',
      ':::card motivation',
      'Nested motivation card, DM-only, still a valid card view target.',
      ':::',
      ':::',
    ].join('\n')

    const out = markdownToMarkdown(md)
    expect(normalize(out)).toBe(normalize(md))
    expect(markdownToMarkdown(out)).toBe(out)

    const doc = markdownToJSON(md)
    const secret = doc.content?.find((n: any) => n.type === 'secretBlock')
    expect(secret).toBeTruthy()
    const nestedCard = secret.content?.find((n: any) => n.type === 'cardBlock')
    expect(nestedCard).toBeTruthy()
    expect(nestedCard.attrs.facet).toBe('motivation')
    // The secret's own direct prose shouldn't duplicate the nested card's text.
    const secretText = JSON.stringify(secret.content?.filter((n: any) => n.type !== 'cardBlock'))
    expect(secretText).not.toContain('Nested motivation card')
  })

  it('round-trips a bare wikilink [[path]]', () => {
    const md = 'He rarely speaks of [[characters/npcs/mary]].'
    const out = markdownToMarkdown(md)
    expect(normalize(out)).toBe(normalize(md))

    const doc = markdownToJSON(md)
    const para = doc.content?.[0]
    const link = para.content?.find((n: any) => n.type === 'wikilink')
    expect(link?.attrs?.path).toBe('characters/npcs/mary')
  })

  it('round-trips a labeled wikilink [[path|Label]]', () => {
    const md = 'He rarely speaks of [[characters/npcs/mary|Mary]].'
    const out = markdownToMarkdown(md)
    expect(normalize(out)).toBe(normalize(md))

    const doc = markdownToJSON(md)
    const para = doc.content?.[0]
    const link = para.content?.find((n: any) => n.type === 'wikilink')
    expect(link?.attrs?.path).toBe('characters/npcs/mary')
    expect(link?.attrs?.label).toBe('Mary')
  })

  it('round-trips a wikilink inside a card block inside a secret block', () => {
    const md = [
      ':::secret',
      ':::card description',
      'Spouse: [[characters/npcs/mary|Mary]]',
      ':::',
      ':::',
    ].join('\n')

    const out = markdownToMarkdown(md)
    expect(normalize(out)).toBe(normalize(md))

    const doc = markdownToJSON(md)
    const secret = doc.content?.find((n: any) => n.type === 'secretBlock')
    const card = secret.content?.find((n: any) => n.type === 'cardBlock')
    const para = card.content?.[0]
    const link = para.content?.find((n: any) => n.type === 'wikilink')
    expect(link?.attrs?.path).toBe('characters/npcs/mary')
    expect(link?.attrs?.label).toBe('Mary')
  })

  it('round-trips the full CLI spec §4 example entry body', () => {
    const md = [
      '# Tom the Innkeeper',
      '',
      'Tom runs the Gilded Goose inn. He rarely speaks of [[characters/npcs/mary|Mary]].',
      '',
      ':::secret',
      'Tom is secretly a spy for the Crimson Hand.',
      ':::',
      '',
      ':::card motivation',
      'Wants to buy back his family farm.',
      ':::',
      '',
      ':::card description',
      'Round-faced, ale-stained apron, booming laugh. Spouse: [[characters/npcs/mary|Mary]]',
      ':::',
    ].join('\n')

    const out = markdownToMarkdown(md)
    expect(normalize(out)).toBe(normalize(md))
    expect(markdownToMarkdown(out)).toBe(out)
  })

  it('soft-wraps two lines of one paragraph (no blank line between) into one line on serialize', () => {
    // This is the one place round-tripping isn't byte-identical: two source
    // lines with no blank line between them are a single CommonMark
    // paragraph, and re-serializing that paragraph writes it back out as
    // one line. The acceptance criteria calls this out as an acceptable
    // "minor whitespace normalization" divergence — the rendered content
    // (and the words themselves) are unchanged, only the author's manual
    // line-wrap point is lost, exactly like any other soft-wrap-reflowing
    // markdown editor.
    const md = 'Round-faced, ale-stained apron, booming laugh.\nSpouse: [[characters/npcs/mary|Mary]]'
    const out = markdownToMarkdown(md)
    expect(out).toBe('Round-faced, ale-stained apron, booming laugh. Spouse: [[characters/npcs/mary|Mary]]')
    // But it's now stable — no further drift on subsequent round trips.
    expect(markdownToMarkdown(out)).toBe(out)
  })

  it('round-trips a plain markdown hyperlink alongside a wikilink', () => {
    const md = 'See the [inn website](https://example.com/goose) or [[characters/npcs/mary|Mary]].'
    const out = markdownToMarkdown(md)
    expect(normalize(out)).toBe(normalize(md))
  })

  it('leaves an unclosed :::secret as literal text rather than crashing', () => {
    const md = ':::secret\nNever closed.'
    expect(() => markdownToMarkdown(md)).not.toThrow()
    const doc = markdownToJSON(md)
    expect(doc.content?.some((n: any) => n.type === 'secretBlock')).toBe(false)
  })
})
