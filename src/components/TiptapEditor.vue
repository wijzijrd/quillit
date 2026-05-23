<template>
  <editor-content :editor="editor" />
</template>

<script setup>
import { watch, onBeforeUnmount } from 'vue'
import { useEditor, EditorContent } from '@tiptap/vue-3'
import StarterKit from '@tiptap/starter-kit'
import Placeholder from '@tiptap/extension-placeholder'
import TextAlign from '@tiptap/extension-text-align'
import Link from '@tiptap/extension-link'
import { AnnotationMark } from '../extensions/AnnotationMark.js'
import { createEntryMention } from '../extensions/EntryMention.js'
import { useEntriesStore } from '../stores/useEntriesStore.js'

const props = defineProps({ modelValue: String })
const emit = defineEmits(['update:modelValue', 'selectionUpdate'])

const entriesStore = useEntriesStore()

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
  ],
  onUpdate({ editor }) {
    emit('update:modelValue', editor.getHTML())
  },
  onSelectionUpdate({ editor }) {
    emit('selectionUpdate', { empty: editor.state.selection.empty })
  }
})

watch(() => props.modelValue, (val) => {
  if (editor.value && val !== editor.value.getHTML()) {
    editor.value.commands.setContent(val || '', false)
  }
})

onBeforeUnmount(() => editor.value?.destroy())

defineExpose({ editor })
</script>
