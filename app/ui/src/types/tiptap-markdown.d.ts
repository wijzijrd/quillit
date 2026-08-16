// tiptap-markdown ships MarkdownStorage as a named type but doesn't merge
// it into @tiptap/core's ambient Storage interface, so `editor.storage.markdown`
// is untyped out of the box. This augmentation is the standard fix.
import type { MarkdownStorage } from 'tiptap-markdown'

declare module '@tiptap/core' {
  interface Storage {
    markdown: MarkdownStorage
  }
}

export {}
