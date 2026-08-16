import { Node, mergeAttributes } from '@tiptap/core'
import { registerDirectiveRules } from './markdownDirectives'

/**
 * DM-only block content — serializes to `:::secret ... :::` (CLI spec §4).
 * Replaces the old inline `annotation` mark (AnnotationMark.ts, deleted
 * alongside this): visibility is now block-level content, not an inline
 * highlight or a whole-entry flag.
 *
 * `content: 'block+'` and `group: 'block'` together are what let a
 * CardBlock (also group 'block') nest inside a SecretBlock, per the CLI
 * spec's "card blocks may sit inside secret blocks" and this issue's
 * requirement that the schema allow it.
 */
export const SecretBlock = Node.create({
  name: 'secretBlock',
  group: 'block',
  content: 'block+',
  defining: true,

  parseHTML() {
    return [{ tag: 'div[data-type="secret-block"]' }]
  },

  renderHTML({ HTMLAttributes }) {
    return ['div', mergeAttributes(HTMLAttributes, { 'data-type': 'secret-block', class: 'secret-block' }), 0]
  },

  addStorage() {
    return {
      markdown: {
        serialize(state: any, node: any) {
          state.write(':::secret\n')
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
