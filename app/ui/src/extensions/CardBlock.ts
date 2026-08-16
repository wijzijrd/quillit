// @ts-nocheck
import { Node, mergeAttributes } from '@tiptap/core'
import router from '../router/index'
import { useFacetsStore } from '../stores/useFacetsStore'
import { registerDirectiveRules } from './markdownDirectives'

/**
 * Flash-card block for a named facet — serializes to `:::card <facet> ...
 * :::` (CLI spec §4). `group: 'block'` + `content: 'block+'` lets a
 * CardBlock nest inside a SecretBlock (also group 'block'), per this
 * issue's requirement.
 *
 * The facet picker is a plain-DOM (non-Vue) node view, matching the style
 * already used by this file's sibling extensions (see the old
 * EntryMention.ts) — deliberately not a Vue NodeView, so this node can also
 * be constructed by the headless, unmounted Editor instance the read-only
 * renderer (useMarkdownRender.ts) uses for print/view without needing a
 * live Pinia/router app context. The node view no-ops (renders a static
 * badge, no `<select>`, no fetch) whenever `editor.isEditable` is false.
 */
export const CardBlock = Node.create({
  name: 'cardBlock',
  group: 'block',
  content: 'block+',
  defining: true,

  addAttributes() {
    return {
      facet: {
        default: '',
        parseHTML: el => el.getAttribute('data-facet') || '',
        renderHTML: attrs => ({ 'data-facet': attrs.facet }),
      },
    }
  },

  parseHTML() {
    return [{ tag: 'div[data-type="card-block"]' }]
  },

  renderHTML({ HTMLAttributes }) {
    return ['div', mergeAttributes(HTMLAttributes, { 'data-type': 'card-block', class: 'card-block' }), 0]
  },

  addNodeView() {
    return ({ node, editor, getPos }) => {
      const dom = document.createElement('div')
      dom.className = 'card-block'
      dom.setAttribute('data-type', 'card-block')

      const header = document.createElement('div')
      header.className = 'card-block-header'
      header.contentEditable = 'false'
      dom.appendChild(header)

      const icon = document.createElement('span')
      icon.className = 'card-block-icon'
      icon.textContent = '🗂'
      header.appendChild(icon)

      let select: HTMLSelectElement | null = null

      function renderBadge(facet: string) {
        header.querySelector('.card-block-badge')?.remove()
        const badge = document.createElement('span')
        badge.className = 'card-block-badge'
        badge.textContent = facet || '(choose a facet)'
        header.appendChild(badge)
      }

      if (editor.isEditable) {
        select = document.createElement('select')
        select.className = 'card-block-select'

        const facetsStore = useFacetsStore()
        const projectId = router.currentRoute.value.params.projectId
        const populate = () => {
          if (!select) return
          const vocab = typeof projectId === 'string' ? (facetsStore.projectEffective[projectId] ?? []) : []
          select.innerHTML = ''
          if (!node.attrs.facet) {
            const placeholder = document.createElement('option')
            placeholder.value = ''
            placeholder.textContent = 'Choose a facet…'
            select.appendChild(placeholder)
          }
          for (const name of vocab) {
            const opt = document.createElement('option')
            opt.value = name
            opt.textContent = name
            select.appendChild(opt)
          }
          select.value = node.attrs.facet || ''
        }
        populate()
        if (typeof projectId === 'string' && !facetsStore.projectEffective[projectId]) {
          facetsStore.fetchForProject(projectId).then(populate).catch(() => {})
        }

        select.addEventListener('change', () => {
          if (!select) return
          const pos = typeof getPos === 'function' ? getPos() : null
          if (pos == null) return
          // Native <select> only ever offers vocabulary options (plus the
          // one-time empty placeholder) — free text isn't reachable here,
          // which is what makes this "reject in the UI itself" rather
          // than a validation message after the fact.
          editor.chain().command(({ tr }) => {
            tr.setNodeMarkup(pos, undefined, { ...node.attrs, facet: select!.value })
            return true
          }).run()
        })
        header.appendChild(select)
      } else {
        renderBadge(node.attrs.facet)
      }

      const content = document.createElement('div')
      content.className = 'card-block-content'
      dom.appendChild(content)

      return {
        dom,
        contentDOM: content,
        update(updatedNode) {
          if (updatedNode.type.name !== 'cardBlock') return false
          if (select && select.value !== (updatedNode.attrs.facet || '')) {
            select.value = updatedNode.attrs.facet || ''
          } else if (!select) {
            renderBadge(updatedNode.attrs.facet)
          }
          return true
        },
      }
    }
  },

  addStorage() {
    return {
      markdown: {
        serialize(state: any, node: any) {
          state.write(':::card ' + (node.attrs.facet || '') + '\n')
          state.renderContent(node)
          state.flushClose(1)
          state.write(':::')
          state.closeBlock(node)
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
