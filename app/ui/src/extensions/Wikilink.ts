// @ts-nocheck
import { Node, mergeAttributes } from '@tiptap/core'
import { Suggestion } from '@tiptap/suggestion'
import { PluginKey } from '@tiptap/pm/state'
import router from '../router/index'
import { registerDirectiveRules } from './markdownDirectives'
import {
  getCachedResolution,
  pathForResult,
  resolveWikilinkPath,
  searchProjectEntries,
  type WikilinkSearchResult,
} from './wikilinkLookup'

export const wikilinkSuggestionKey = new PluginKey('wikilinkSuggestion')

/**
 * Replaces EntryMention.ts. Stores `path` (+ optional `label`) rather than
 * an entry id — path-addressed, matching the CLI's model (docs/cli-spec.md
 * §4 "Links"). Serializes to `[[path]]` / `[[path|label]]`.
 *
 * Trigger stays "@" — same character EntryMention.ts used — for UX
 * continuity, even though the serialized syntax is `[[...]]`.
 *
 * Like CardBlock, the node view is plain DOM (no Vue), so this node also
 * works inside the headless Editor the read-only renderer constructs. It
 * no-ops (skips the resolution fetch and click handler) when
 * `editor.isEditable` is false.
 */
export const Wikilink = Node.create({
  name: 'wikilink',
  group: 'inline',
  inline: true,
  atom: true,
  selectable: true,

  addOptions() {
    return {
      // Whether the node view resolves link status (dangling vs. resolved)
      // and stamps data-resolved-id for click-to-navigate. On by default
      // for both the interactive editor and EntryViewModal's read-only
      // view; turned off for the throwaway headless render print uses, so
      // a print job doesn't fire a round of lookup requests for links
      // nobody can click in a printed/new-tab document anyway.
      interactiveLinks: true,
    }
  },

  addAttributes() {
    return {
      path: {
        default: null,
        parseHTML: el => el.getAttribute('data-path'),
        renderHTML: attrs => ({ 'data-path': attrs.path }),
      },
      label: {
        default: null,
        parseHTML: el => el.getAttribute('data-label'),
        renderHTML: attrs => (attrs.label ? { 'data-label': attrs.label } : {}),
      },
    }
  },

  parseHTML() {
    return [{ tag: 'span[data-type="wikilink"]' }]
  },

  renderHTML({ HTMLAttributes }) {
    return ['span', mergeAttributes(HTMLAttributes, { 'data-type': 'wikilink', class: 'wikilink' })]
  },

  addNodeView() {
    const { interactiveLinks } = this.options
    return ({ node }) => {
      const dom = document.createElement('span')
      dom.className = 'wikilink'
      dom.setAttribute('data-type', 'wikilink')

      const paint = (attrs: { path: string; label: string | null }) => {
        dom.textContent = attrs.label || attrs.path || ''
        if (attrs.path) dom.dataset.path = attrs.path
        if (attrs.label) dom.dataset.label = attrs.label
      }
      paint(node.attrs)

      // Resolution + click-to-navigate: styling and the resolved entry id
      // are set here (async, via the same lookup endpoint search uses —
      // see wikilinkLookup.ts), but the actual navigation is handled by
      // EntryEditor.vue's editor-content click delegate (onEditorClick),
      // matching the delegation pattern the old .entry-mention handling
      // used — that's also where back/forward history gets pushed, which
      // a NodeView (outside EntryEditor.vue's component instance) can't
      // reach directly.
      if (interactiveLinks) {
        const projectId = router.currentRoute.value.params.projectId
        if (typeof projectId === 'string' && node.attrs.path) {
          const applyResolution = (match: WikilinkSearchResult | null) => {
            dom.classList.toggle('wikilink--resolved', !!match)
            dom.classList.toggle('wikilink--dangling', !match)
            if (match) {
              dom.dataset.resolvedId = match.id
              dom.title = match.title
            } else {
              delete dom.dataset.resolvedId
              dom.title = `No entry found at "${node.attrs.path}"`
            }
          }
          // Paint synchronously if a caller already warmed the cache
          // (useMarkdownRender.ts does this before constructing its
          // headless Editor, so a one-shot getHTML() call still reflects
          // resolved/dangling status rather than missing an async update
          // that would otherwise land after the DOM was already read).
          const cached = getCachedResolution(projectId, node.attrs.path)
          if (cached !== undefined) {
            applyResolution(cached)
          } else {
            resolveWikilinkPath(projectId, node.attrs.path).then(applyResolution)
          }
        }
      }

      return {
        dom,
        update(updatedNode) {
          if (updatedNode.type.name !== 'wikilink') return false
          paint(updatedNode.attrs)
          return true
        },
      }
    }
  },

  addProseMirrorPlugins() {
    return [
      Suggestion({
        editor: this.editor,
        char: '@',
        pluginKey: wikilinkSuggestionKey,
        items: async ({ query }): Promise<WikilinkSearchResult[]> => {
          const projectId = router.currentRoute.value.params.projectId
          if (typeof projectId !== 'string') return []
          try {
            return await searchProjectEntries(projectId, query)
          } catch {
            return []
          }
        },
        command: ({ editor, range, props }: { editor: any; range: any; props: WikilinkSearchResult }) => {
          editor.chain().focus().insertContentAt(range, {
            type: 'wikilink',
            attrs: { path: pathForResult(props), label: props.title },
          }).run()
        },
        render: () => {
          let el: HTMLElement | null = null
          let selectedIndex = 0
          let currentProps: any = null

          function build() {
            el = document.createElement('div')
            el.className = 'entry-suggestion-popup'
            document.body.appendChild(el)
          }

          function paint(props: any) {
            if (!el) return
            const rect = props.clientRect?.()
            if (rect) {
              el.style.cssText = [
                'position:fixed',
                `top:${rect.bottom + 4}px`,
                `left:${rect.left}px`,
                'z-index:1000',
                'display:block',
              ].join(';')
            }
            const items: WikilinkSearchResult[] = props.items ?? []
            if (!items.length) {
              el.style.display = 'none'
              return
            }
            el.textContent = ''
            items.forEach((item, i) => {
              const row = document.createElement('div')
              row.className = `esug-item${i === selectedIndex ? ' esug-item--focused' : ''}`
              row.dataset.idx = String(i)
              const path = document.createElement('span')
              path.className = 'esug-cat'
              path.textContent = item.directoryPath || '/'
              const title = document.createElement('span')
              title.className = 'esug-title'
              title.textContent = item.title
              row.appendChild(path)
              row.appendChild(title)
              row.addEventListener('mousedown', e => {
                e.preventDefault()
                props.command(item)
              })
              el!.appendChild(row)
            })
          }

          return {
            onStart(props: any) { selectedIndex = 0; currentProps = props; build(); paint(props) },
            onUpdate(props: any) { selectedIndex = 0; currentProps = props; paint(props) },
            onKeyDown({ event }: { event: KeyboardEvent }) {
              const items: WikilinkSearchResult[] = currentProps?.items ?? []
              if (!items.length) return false
              const len = items.length
              if (event.key === 'ArrowDown') { selectedIndex = (selectedIndex + 1) % len; paint(currentProps); return true }
              if (event.key === 'ArrowUp') { selectedIndex = (selectedIndex - 1 + len) % len; paint(currentProps); return true }
              if (event.key === 'Enter') { currentProps.command(items[selectedIndex]); return true }
              return false
            },
            onExit() { el?.remove(); el = null; currentProps = null },
          }
        },
      }),
    ]
  },

  addStorage() {
    return {
      markdown: {
        serialize(state: any, node: any) {
          const { path, label } = node.attrs
          if (label && label !== path) {
            state.write(`[[${path}|${label}]]`)
          } else {
            state.write(`[[${path}]]`)
          }
        },
        parse: {
          setup(markdownit: any) {
            registerDirectiveRules(markdownit)
          },
        },
      },
    }
  },
})
