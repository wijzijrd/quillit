import StarterKit from '@tiptap/starter-kit'
import Placeholder from '@tiptap/extension-placeholder'
import TextAlign from '@tiptap/extension-text-align'
import Link from '@tiptap/extension-link'
import Image from '@tiptap/extension-image'
import { Markdown } from 'tiptap-markdown'
import { SecretBlock } from '../extensions/SecretBlock'
import { CardBlock } from '../extensions/CardBlock'
import { Wikilink } from '../extensions/Wikilink'

export interface MarkdownExtensionsOptions {
  placeholder?: string
  /** Passed through to Wikilink — set false for throwaway headless
   * renders (print) that don't need dangling-link resolution fetched. */
  interactiveLinks?: boolean
}

/**
 * The single extension list shared by the interactive editor
 * (TiptapEditor.vue) and the read-only renderer (useMarkdownRender.ts) —
 * one schema, one markdown parser/serializer, used both ways so a saved
 * entry always parses/renders identically regardless of which surface
 * loaded it.
 *
 * Markdown is now the editor's persisted/stored artifact (issue #47): the
 * Tiptap document is an ephemeral in-memory projection — markdown in on
 * load, markdown out on save — via `tiptap-markdown`'s `Markdown`
 * extension, which grafts a default markdown serializer/parser onto every
 * StarterKit node/mark that doesn't already define its own, and leaves our
 * three custom nodes' own `addStorage().markdown` (SecretBlock, CardBlock,
 * Wikilink) untouched.
 *
 * `html: true` lets content the schema doesn't otherwise recognize survive
 * as raw HTML passthrough rather than being silently dropped — notably a
 * safety net for entries still holding pre-#47 HTML bodies until they're
 * next edited and re-saved as markdown.
 */
export function buildMarkdownExtensions(opts: MarkdownExtensionsOptions = {}) {
  return [
    StarterKit,
    Markdown.configure({
      html: true,
      transformPastedText: true,
      transformCopiedText: true,
    }),
    ...(opts.placeholder ? [Placeholder.configure({ placeholder: opts.placeholder })] : []),
    TextAlign.configure({ types: ['heading', 'paragraph'] }),
    SecretBlock,
    CardBlock,
    Wikilink.configure({ interactiveLinks: opts.interactiveLinks ?? true }),
    Link.configure({
      openOnClick: false,
      HTMLAttributes: { rel: 'noopener noreferrer', target: '_blank' },
    }),
    Image.configure({ inline: false }),
  ]
}
