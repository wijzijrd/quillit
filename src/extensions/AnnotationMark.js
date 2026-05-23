import { Mark, mergeAttributes } from '@tiptap/core'

export const AnnotationMark = Mark.create({
  name: 'annotation',

  addAttributes() {
    return {
      annotationId: {
        default: null,
        parseHTML: el => el.getAttribute('data-annotation-id'),
        renderHTML: attrs => ({ 'data-annotation-id': attrs.annotationId }),
      },
      visibility: {
        default: 'gm',
        parseHTML: el => el.getAttribute('data-visibility'),
        renderHTML: attrs => ({
          'data-visibility': attrs.visibility,
          class: `annotation-mark annotation-mark--${attrs.visibility}`,
        }),
      },
      annotationIndex: {
        default: null,
        parseHTML: el => el.getAttribute('data-annotation-index') ? Number(el.getAttribute('data-annotation-index')) : null,
        renderHTML: attrs => attrs.annotationIndex != null ? { 'data-annotation-index': String(attrs.annotationIndex) } : {},
      },
    }
  },

  parseHTML() {
    return [{ tag: 'mark[data-annotation-id]' }]
  },

  renderHTML({ HTMLAttributes }) {
    return ['mark', mergeAttributes(HTMLAttributes), 0]
  },
})
