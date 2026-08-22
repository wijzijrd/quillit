import { describe, expect, it } from 'vitest'
import { buildDirectoryTree } from '../directoryTree'
import type { ContentEntry } from '../../stores/useEntryStore'

function entry(overrides: Partial<ContentEntry> & Pick<ContentEntry, 'id' | 'title' | 'directoryPath'>): ContentEntry {
  return {
    projectId: 'proj-1',
    slug: overrides.id,
    tags: [],
    body: '',
    createdAt: 0,
    updatedAt: 0,
    ...overrides,
  }
}

describe('buildDirectoryTree', () => {
  it('puts entries with an empty directoryPath at the root', () => {
    const tree = buildDirectoryTree([
      entry({ id: 'e1', title: 'Zed', directoryPath: '' }),
      entry({ id: 'e2', title: 'Amy', directoryPath: '' }),
    ])
    expect(tree.path).toBe('')
    expect(tree.children).toEqual([])
    expect(tree.entries.map(e => e.title)).toEqual(['Amy', 'Zed'])
  })

  it('nests entries under a multi-segment directoryPath, creating intermediate directories', () => {
    const tree = buildDirectoryTree([
      entry({ id: 'e1', title: 'Tom', directoryPath: 'characters/npcs' }),
    ])
    expect(tree.children).toHaveLength(1)
    const characters = tree.children[0]
    expect(characters.path).toBe('characters')
    expect(characters.name).toBe('characters')
    expect(characters.entries).toEqual([])
    expect(characters.children).toHaveLength(1)
    const npcs = characters.children[0]
    expect(npcs.path).toBe('characters/npcs')
    expect(npcs.name).toBe('npcs')
    expect(npcs.entries.map(e => e.title)).toEqual(['Tom'])
  })

  it('includes extraPaths as empty directory nodes even with no matching entries', () => {
    const tree = buildDirectoryTree([], ['locations/taverns'])
    expect(tree.children).toHaveLength(1)
    expect(tree.children[0].path).toBe('locations')
    expect(tree.children[0].children[0].path).toBe('locations/taverns')
    expect(tree.children[0].children[0].entries).toEqual([])
  })

  it('does not duplicate a directory node when both an entry and extraPaths reference it', () => {
    const tree = buildDirectoryTree(
      [entry({ id: 'e1', title: 'Tom', directoryPath: 'characters' })],
      ['characters']
    )
    expect(tree.children).toHaveLength(1)
    expect(tree.children[0].entries.map(e => e.title)).toEqual(['Tom'])
  })

  it('sorts sibling directories alphabetically by name', () => {
    const tree = buildDirectoryTree([], ['zoo', 'archive'])
    expect(tree.children.map(c => c.name)).toEqual(['archive', 'zoo'])
  })

  // Folder names are now free text typed by the user (DirectoryNode.vue's
  // confirmCreateFolder), so a slash-containing extraPath is no longer an
  // unrealistic input to guard against here — it's rejected upstream at the
  // UI layer instead (confirmCreateFolder refuses names containing '/').
  // This test just documents that buildDirectoryTree itself stays
  // well-defined (doesn't throw) if malformed input ever reaches it anyway.
  it('handles a path with an empty segment (e.g. a trailing slash) without crashing', () => {
    const tree = buildDirectoryTree([], ['characters/'])
    expect(tree.children).toHaveLength(1)
    expect(tree.children[0].path).toBe('characters')
    expect(tree.children[0].children).toHaveLength(1)
    expect(tree.children[0].children[0].name).toBe('')
  })
})
