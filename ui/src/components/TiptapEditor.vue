<template>
  <editor-content :editor="editor" />
</template>

<script setup lang="ts">
import { watch, onBeforeUnmount } from 'vue'
import { useEditor, EditorContent } from '@tiptap/vue-3'
import StarterKit from '@tiptap/starter-kit'
import Placeholder from '@tiptap/extension-placeholder'
import TextAlign from '@tiptap/extension-text-align'
import Link from '@tiptap/extension-link'
import Image from '@tiptap/extension-image'
import { AnnotationMark } from '../extensions/AnnotationMark'
import { createEntryMention } from '../extensions/EntryMention'
import { useEntriesStore } from '../stores/useEntriesStore'

const props = defineProps<{
  modelValue?: string
  uploadImageFn?: (file: File) => Promise<string>
}>()
const emit = defineEmits(['update:modelValue', 'selectionUpdate'])

const entriesStore = useEntriesStore()

async function handleImageFile(file: File, insertFn: (url: string) => void) {
  if (!props.uploadImageFn) return false
  if (!file.type.startsWith('image/')) return false
  try {
    const url = await props.uploadImageFn(file)
    insertFn(url)
  } catch {
    // silently fail — image stays as-is or not inserted
  }
  return true
}

const editor = useEditor({
  content: props.modelValue || '',
  extensions: [
    StarterKit,
    Placeholder.configure({ placeholder: 'Write your lore here…' }),
    TextAlign.configure({ types: ['heading', 'paragraph'] }),
    AnnotationMark,
    createEntryMention(() => entriesStore.entries),
    Link.configure({
      openOnClick: false,
      HTMLAttributes: { rel: 'noopener noreferrer', target: '_blank' },
    }),
    Image.configure({ inline: false }),
  ],
  editorProps: {
    handlePaste(view, event) {
      const items = event.clipboardData?.items
      if (!items || !props.uploadImageFn) return false
      for (const item of Array.from(items)) {
        const file = item.getAsFile()
        if (file && file.type.startsWith('image/')) {
          event.preventDefault()
          handleImageFile(file, (url) => {
            view.dispatch(view.state.tr.replaceSelectionWith(
              view.state.schema.nodes.image.create({ src: url })
            ))
          })
          return true
        }
      }
      return false
    },
    handleDrop(view, event, _slice, moved) {
      if (moved || !event.dataTransfer?.files?.length || !props.uploadImageFn) return false
      const file = Array.from(event.dataTransfer.files).find(f => f.type.startsWith('image/'))
      if (!file) return false
      event.preventDefault()
      const coords = view.posAtCoords({ left: event.clientX, top: event.clientY })
      handleImageFile(file, (url) => {
        const pos = coords?.pos ?? view.state.selection.anchor
        view.dispatch(view.state.tr.insert(pos, view.state.schema.nodes.image.create({ src: url })))
      })
      return true
    },
  },
  onUpdate({ editor }) {
    emit('update:modelValue', editor.getHTML())
  },
  onSelectionUpdate({ editor }) {
    emit('selectionUpdate', { empty: editor.state.selection.empty })
  }
})

watch(() => props.modelValue, (val) => {
  if (editor.value && val !== editor.value.getHTML()) {
    editor.value.commands.setContent(val || '')
  }
})

onBeforeUnmount(() => editor.value?.destroy())

defineExpose({ editor })
</script>
