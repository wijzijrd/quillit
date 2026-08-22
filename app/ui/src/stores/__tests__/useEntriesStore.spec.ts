import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const apiMock = vi.fn()
vi.mock('../../api/client', () => ({
  api: (...args: unknown[]) => apiMock(...args),
  apiErrorMessage: (e: unknown, fallback: string) => {
    const data = (e as { data?: { error?: string } } | undefined)?.data
    return data?.error ?? fallback
  },
}))

import { useEntriesStore } from '../useEntriesStore'

describe('useEntriesStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    apiMock.mockReset()
  })

  it('init lists entries for the given project', async () => {
    apiMock.mockResolvedValue([{ id: 'e1', title: 'Tom', tags: [] }])
    const store = useEntriesStore()
    await store.init('proj-1')
    expect(apiMock).toHaveBeenCalledWith('/content/projects/proj-1/entries')
    expect(store.entries).toEqual([{ id: 'e1', title: 'Tom', tags: [] }])
    expect(store.loaded).toBe(true)
  })

  it('init only fetches once per project', async () => {
    apiMock.mockResolvedValue([])
    const store = useEntriesStore()
    await store.init('proj-1')
    await store.init('proj-1')
    expect(apiMock).toHaveBeenCalledTimes(1)
  })

  it('init refetches when the requested project changes', async () => {
    apiMock.mockResolvedValueOnce([{ id: 'e1', title: 'Tom', tags: [] }])
    apiMock.mockResolvedValueOnce([{ id: 'e2', title: 'Jerry', tags: [] }])
    const store = useEntriesStore()
    await store.init('proj-1')
    expect(store.entries).toEqual([{ id: 'e1', title: 'Tom', tags: [] }])
    await store.init('proj-2')
    expect(apiMock).toHaveBeenCalledTimes(2)
    expect(apiMock).toHaveBeenLastCalledWith('/content/projects/proj-2/entries')
    expect(store.entries).toEqual([{ id: 'e2', title: 'Jerry', tags: [] }])
  })

  it('init resets loaded on failure so a retry is possible', async () => {
    apiMock.mockRejectedValue(new Error('network error'))
    const store = useEntriesStore()
    await expect(store.init('proj-1')).rejects.toThrow()
    expect(store.loaded).toBe(false)
  })

  it('createEntry delegates to useEntryStore.create and prepends the result', async () => {
    apiMock.mockResolvedValue({ id: 'e2', title: 'Untitled', tags: [] })
    const store = useEntriesStore()
    const entry = await store.createEntry('proj-1', 'Untitled')
    expect(apiMock).toHaveBeenCalledWith('/content/projects/proj-1/entries', expect.objectContaining({ method: 'POST' }))
    expect(store.entries[0]).toEqual(entry)
  })

  it('updateEntry PATCHes and updates the cached copy', async () => {
    apiMock.mockResolvedValueOnce([{ id: 'e1', title: 'Tom', tags: [] }])
    apiMock.mockResolvedValueOnce({ id: 'e1', title: 'Tomas', tags: [] })
    const store = useEntriesStore()
    await store.init('proj-1')
    await store.updateEntry('e1', { title: 'Tomas' })
    expect(apiMock).toHaveBeenCalledWith('/content/entries/e1', { method: 'PATCH', body: { title: 'Tomas' } })
    expect(store.entries[0].title).toBe('Tomas')
  })

  it('assignEntry POSTs to the assign endpoint and updates the cached copy', async () => {
    apiMock.mockResolvedValueOnce([{ id: 'e1', title: 'Tom', tags: [], directoryPath: '' }])
    apiMock.mockResolvedValueOnce({ id: 'e1', title: 'Tom', tags: [], directoryPath: 'characters/npcs' })
    const store = useEntriesStore()
    await store.init('proj-1')
    await store.assignEntry('e1', 'characters/npcs')
    expect(apiMock).toHaveBeenCalledWith('/content/entries/e1/assign', {
      method: 'POST',
      body: { directory_path: 'characters/npcs' },
    })
    expect(store.entries[0].directoryPath).toBe('characters/npcs')
  })

  it('assignEntry leaves the cached copy untouched on failure', async () => {
    apiMock.mockResolvedValueOnce([{ id: 'e1', title: 'Tom', tags: [], directoryPath: '' }])
    apiMock.mockRejectedValueOnce(new Error('conflict'))
    const store = useEntriesStore()
    await store.init('proj-1')
    await expect(store.assignEntry('e1', 'characters/npcs')).rejects.toThrow()
    expect(store.entries[0].directoryPath).toBe('')
  })

  it('deleteEntry DELETEs and drops the cached copy', async () => {
    apiMock.mockResolvedValueOnce([{ id: 'e1', title: 'Tom', tags: [] }])
    apiMock.mockResolvedValueOnce(undefined)
    const store = useEntriesStore()
    await store.init('proj-1')
    await store.deleteEntry('e1')
    expect(apiMock).toHaveBeenCalledWith('/content/entries/e1', { method: 'DELETE' })
    expect(store.entries).toHaveLength(0)
  })

  it('getById finds a cached entry', async () => {
    apiMock.mockResolvedValue([{ id: 'e1', title: 'Tom', tags: [] }])
    const store = useEntriesStore()
    await store.init('proj-1')
    expect(store.getById('e1')?.title).toBe('Tom')
    expect(store.getById('missing')).toBeNull()
  })
})
