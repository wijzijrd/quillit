import { Editor } from '@tiptap/core'
import { buildMarkdownExtensions } from '../lib/markdownExtensions'
import router from '../router/index'
import { warmWikilinkCache } from '../extensions/wikilinkLookup'

export interface RenderMarkdownOptions {
  /** Strip :::secret blocks (and everything nested inside them) entirely —
   * used for the always-player-safe print export, matching the CLI's
   * `player` view semantics (docs/cli-spec.md §4). */
  stripSecrets?: boolean
  /** Resolve wikilink dangling/resolved status and enable click-to-navigate
   * data attributes. Default true (EntryViewModal wants this); print
   * passes false since a printed/new-tab document can't be clicked and
   * shouldn't fire a round of lookup requests for links nobody can use. */
  interactiveLinks?: boolean
}

/**
 * Renders a markdown entry body to read-only HTML, for surfaces that don't
 * need a live editable Tiptap instance (EntryViewModal.vue, print).
 *
 * Uses the exact same extension list — same schema, same markdown parser —
 * as the interactive editor (TiptapEditor.vue), via a headless
 * (non-editable, unmounted) Editor instance: `editable: false` is what
 * makes CardBlock's plain node view skip its interactive chrome (the facet
 * <select>) and render a static badge instead.
 *
 * Async because, when `interactiveLinks` is on, it pre-warms the wikilink
 * resolution cache (wikilinkLookup.ts) *before* constructing the editor —
 * a one-shot `getHTML()` call is synchronous, so without pre-warming, the
 * node view's resolved/dangling class would land (via its own async
 * lookup) after the HTML had already been read out and the editor
 * destroyed.
 */
export async function renderMarkdownToHtml(markdown: string, opts: RenderMarkdownOptions = {}): Promise<string> {
  if (!markdown) return ''

  const interactiveLinks = opts.interactiveLinks ?? true
  if (interactiveLinks) {
    const projectId = router.currentRoute.value.params.projectId
    if (typeof projectId === 'string') {
      await warmWikilinkCache(projectId, markdown)
    }
  }

  const editor = new Editor({
    editable: false,
    extensions: buildMarkdownExtensions({ interactiveLinks }),
    content: markdown,
  })

  try {
    if (opts.stripSecrets) {
      const positions: Array<{ from: number; to: number }> = []
      editor.state.doc.descendants((node, pos) => {
        if (node.type.name === 'secretBlock') {
          positions.push({ from: pos, to: pos + node.nodeSize })
          return false
        }
        return true
      })
      if (positions.length) {
        editor.commands.command(({ tr, dispatch }) => {
          for (const { from, to } of [...positions].reverse()) tr.delete(from, to)
          dispatch?.(tr)
          return true
        })
      }
    }
    return editor.getHTML()
  } finally {
    editor.destroy()
  }
}
