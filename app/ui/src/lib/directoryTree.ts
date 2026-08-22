import type { InjectionKey } from 'vue'
import type { ContentEntry } from '../stores/useEntryStore'

/** One node in the directory tree grouping entries by their directoryPath (issue #49). Directories have no backing table — a node exists purely because some entry's directoryPath references it, or because it's a not-yet-populated placeholder passed via extraPaths. */
export interface DirNode {
  path: string
  name: string
  children: DirNode[]
  entries: ContentEntry[]
}

/**
 * Builds the full directory tree from a flat entry list. extraPaths lets the
 * caller include directories with no entries yet (ephemeral "+ folder"
 * placeholders created client-side — see QuillitView.vue's pendingFolders).
 */
export function buildDirectoryTree(entries: ContentEntry[], extraPaths: string[] = []): DirNode {
  const root: DirNode = { path: '', name: '', children: [], entries: [] }
  const nodesByPath = new Map<string, DirNode>([['', root]])

  function ensureNode(path: string): DirNode {
    const existing = nodesByPath.get(path)
    if (existing) return existing
    const segments = path.split('/')
    const name = segments[segments.length - 1]
    const parentPath = segments.slice(0, -1).join('/')
    const parent = ensureNode(parentPath)
    const node: DirNode = { path, name, children: [], entries: [] }
    parent.children.push(node)
    nodesByPath.set(path, node)
    return node
  }

  for (const path of extraPaths) {
    if (path) ensureNode(path)
  }
  for (const entry of entries) {
    const node = entry.directoryPath ? ensureNode(entry.directoryPath) : root
    node.entries.push(entry)
  }

  function sortTree(node: DirNode) {
    node.children.sort((a, b) => a.name.localeCompare(b.name))
    node.entries.sort((a, b) => a.title.localeCompare(b.title))
    node.children.forEach(sortTree)
  }
  sortTree(root)

  return root
}

/** Shared callbacks every DirectoryNode.vue instance needs, provided once by QuillitView.vue at the tree root and injected at every recursion depth — avoids relaying events through every intermediate level of an arbitrarily deep tree. */
export interface DirectoryTreeActions {
  isExpanded: (path: string) => boolean
  onToggle: (path: string) => void
  onMove: (entryId: string, destPath: string) => void
  onCreateFolder: (parentPath: string, name: string) => void
  onView: (entry: ContentEntry) => void
  onEdit: (entry: ContentEntry) => void
  onLinks: (entry: ContentEntry) => void
  onDelete: (entry: ContentEntry) => void
}

export const directoryTreeActionsKey: InjectionKey<DirectoryTreeActions> = Symbol('directoryTreeActions')
