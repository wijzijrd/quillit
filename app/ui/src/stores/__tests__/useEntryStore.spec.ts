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

import { useEntryStore } from '../useEntryStore'

describe('useEntryStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    apiMock.mockReset()
  })

  it('get fetches the entry by id', async () => {
    apiMock.mockResolvedValue({ id: 'e1', title: 'Tom' })
    const store = useEntryStore()
    const entry = await store.get('e1')
    expect(apiMock).toHaveBeenCalledWith('/content/entries/e1')
    expect(entry).toEqual({ id: 'e1', title: 'Tom' })
  })

  it('update PATCHes the composed body', async () => {
    apiMock.mockResolvedValue({ id: 'e1', body: 'new body' })
    const store = useEntryStore()
    await store.update('e1', 'new body')
    expect(apiMock).toHaveBeenCalledWith('/content/entries/e1', { method: 'PATCH', body: { body: 'new body' } })
  })

  it('remove DELETEs the entry', async () => {
    apiMock.mockResolvedValue(undefined)
    const store = useEntryStore()
    await store.remove('e1')
    expect(apiMock).toHaveBeenCalledWith('/content/entries/e1', { method: 'DELETE' })
  })

  it('create generates a kebab-case slug from the title and posts to the project', async () => {
    apiMock.mockResolvedValue({ id: 'e2', slug: 'tom-the-innkeeper' })
    const store = useEntryStore()
    const entry = await store.create('proj-1', 'Tom the Innkeeper', '---\nname: Tom the Innkeeper\ntags: []\n---\n\n')
    expect(apiMock).toHaveBeenCalledTimes(1)
    const [url, opts] = apiMock.mock.calls[0]
    expect(url).toBe('/content/projects/proj-1/entries')
    expect(opts.method).toBe('POST')
    expect(opts.body.slug).toBe('tom-the-innkeeper')
    expect(opts.body.directoryPath).toBe('')
    expect(entry.id).toBe('e2')
  })

  it('create defaults to "untitled" for a blank title', async () => {
    apiMock.mockResolvedValue({ id: 'e3' })
    const store = useEntryStore()
    await store.create('proj-1', '', 'body')
    const [, opts] = apiMock.mock.calls[0]
    expect(opts.body.slug).toBe('untitled')
  })

  it('create retries with a numeric suffix on a 409 conflict', async () => {
    apiMock
      .mockRejectedValueOnce({ response: { status: 409 }, data: { error: 'an entry already exists at this path' } })
      .mockRejectedValueOnce({ response: { status: 409 }, data: { error: 'an entry already exists at this path' } })
      .mockResolvedValueOnce({ id: 'e4', slug: 'tom-3' })
    const store = useEntryStore()
    const entry = await store.create('proj-1', 'Tom', 'body')
    expect(apiMock).toHaveBeenCalledTimes(3)
    expect(apiMock.mock.calls[0][1].body.slug).toBe('tom')
    expect(apiMock.mock.calls[1][1].body.slug).toBe('tom-2')
    expect(apiMock.mock.calls[2][1].body.slug).toBe('tom-3')
    expect(entry.id).toBe('e4')
  })

  it('create gives up after too many 409 conflicts', async () => {
    apiMock.mockRejectedValue({ response: { status: 409 }, data: { error: 'conflict' } })
    const store = useEntryStore()
    await expect(store.create('proj-1', 'Tom', 'body')).rejects.toThrow(/too many/i)
  })

  it('create does not retry on a non-409 error', async () => {
    apiMock.mockRejectedValue({ response: { status: 500 }, data: { error: 'db error' } })
    const store = useEntryStore()
    await expect(store.create('proj-1', 'Tom', 'body')).rejects.toThrow('db error')
    expect(apiMock).toHaveBeenCalledTimes(1)
  })
})
