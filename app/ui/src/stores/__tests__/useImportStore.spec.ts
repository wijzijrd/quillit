import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const apiMock = vi.fn()
vi.mock('../../api/client', () => ({
  api: (...args: unknown[]) => apiMock(...args),
  apiErrorMessage: (e: unknown, fallback: string) => {
    const data = (e as { data?: { error?: string } } | undefined)?.data
    return data?.error ?? fallback
  },
}))

import { useImportStore, MAX_IMPORT_BYTES } from '../useImportStore'

function makeFile(name: string, sizeBytes: number, type = 'application/gzip'): File {
  const blob = new Blob([new Uint8Array(sizeBytes)], { type })
  return new File([blob], name, { type })
}

async function flush() {
  await vi.advanceTimersByTimeAsync(0)
}

describe('useImportStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    apiMock.mockReset()
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('selectFile builds the correct dry-run request (URL, body, headers)', async () => {
    apiMock.mockResolvedValue({ applied: false, report: [], facets: { created: [] }, images: [] })
    const store = useImportStore()
    const file = makeFile('project.tar.gz', 100)

    store.selectFile('proj-1', file)
    await flush()

    expect(apiMock).toHaveBeenCalledTimes(1)
    const [url, opts] = apiMock.mock.calls[0]
    expect(url).toBe('/content/projects/proj-1/import?mode=dry-run&onConflict=fail&createFacets=false')
    expect(opts.method).toBe('POST')
    expect(opts.body).toBe(file)
    expect(opts.headers).toEqual({ 'Content-Type': 'application/gzip' })
  })

  it('rejects an oversized file locally, without calling api', () => {
    const store = useImportStore()
    const bigFile = makeFile('huge.tar.gz', MAX_IMPORT_BYTES + 1)

    store.selectFile('proj-1', bigFile)

    expect(apiMock).not.toHaveBeenCalled()
    expect(store.file).toBeNull()
    expect(store.sizeError).toMatch(/50 MB/)
    expect(store.status).toBe('idle')
  })

  it('normalizes a 200 response into ImportSuccess with camelCased facets', async () => {
    apiMock.mockResolvedValue({
      applied: false,
      report: [{ path: 'tom', action: 'create' }],
      facets: { created: ['npc'] },
      images: [{ path: 'tom/portrait.png', uploaded: false }],
    })
    const store = useImportStore()

    store.selectFile('proj-1', makeFile('project.tar.gz', 100))
    await flush()

    expect(store.status).toBe('success')
    expect(store.result).toEqual({
      kind: 'success',
      applied: false,
      report: [{ path: 'tom', action: 'create' }],
      facetsCreated: ['npc'],
      images: [{ path: 'tom/portrait.png', uploaded: false }],
    })
  })

  it('normalizes a 422 response into ImportValidationError', async () => {
    apiMock.mockRejectedValue({
      response: { status: 422 },
      data: { error: 'validation failed', entries: [{ path: 'tom', error: 'bad slug' }], missingFacets: ['npc'] },
    })
    const store = useImportStore()

    store.selectFile('proj-1', makeFile('project.tar.gz', 100))
    await flush()

    expect(store.status).toBe('validation')
    expect(store.result).toEqual({
      kind: 'validation',
      entries: [{ path: 'tom', error: 'bad slug' }],
      missingFacets: ['npc'],
    })
  })

  it('normalizes a non-422 failure into ImportGenericError using apiErrorMessage', async () => {
    apiMock.mockRejectedValue({ response: { status: 503 }, data: { error: 'blob storage not configured' } })
    const store = useImportStore()

    store.selectFile('proj-1', makeFile('project.tar.gz', 100))
    await flush()

    expect(store.status).toBe('error')
    expect(store.result).toEqual({ kind: 'error', message: 'blob storage not configured' })
  })

  it('debounces a re-preview when onConflict changes after a file is selected', async () => {
    apiMock.mockResolvedValue({ applied: false, report: [], facets: { created: [] }, images: [] })
    const store = useImportStore()
    store.selectFile('proj-1', makeFile('project.tar.gz', 100))
    await flush()
    apiMock.mockClear()

    store.setOnConflict('proj-1', 'overwrite')
    store.setOnConflict('proj-1', 'suffix') // rapid second change before the debounce fires
    expect(apiMock).not.toHaveBeenCalled() // still debouncing

    vi.advanceTimersByTime(300)
    await flush()

    expect(apiMock).toHaveBeenCalledTimes(1) // only one request, for the final value
    const [url] = apiMock.mock.calls[0]
    expect(url).toContain('onConflict=suffix')
  })

  it('setCreateFacets with no file selected does not call api', () => {
    const store = useImportStore()
    store.setCreateFacets('proj-1', true)
    vi.advanceTimersByTime(300)
    expect(apiMock).not.toHaveBeenCalled()
  })

  it('discards a stale (superseded) preview response in favor of a newer one', async () => {
    let resolveFirst: (v: unknown) => void = () => {}
    apiMock
      .mockImplementationOnce(() => new Promise(resolve => { resolveFirst = resolve }))
      .mockResolvedValueOnce({ applied: false, report: [{ path: 'second', action: 'create' }], facets: { created: [] }, images: [] })
    const store = useImportStore()

    store.selectFile('proj-1', makeFile('first.tar.gz', 100))
    store.selectFile('proj-1', makeFile('second.tar.gz', 100)) // supersedes the first before it resolves
    await flush()

    resolveFirst({ applied: false, report: [{ path: 'first', action: 'create' }], facets: { created: [] }, images: [] })
    await flush()

    expect(store.status).toBe('success')
    expect((store.result as { report: { path: string }[] }).report[0].path).toBe('second')
  })

  it('apply is a no-op unless status is success', async () => {
    const store = useImportStore()
    await store.apply('proj-1') // no file, no prior preview
    expect(apiMock).not.toHaveBeenCalled()
  })

  it('apply calls the endpoint with mode=apply and transitions to applied on success', async () => {
    apiMock.mockResolvedValue({ applied: true, report: [{ path: 'tom', action: 'create' }], facets: { created: [] }, images: [] })
    const store = useImportStore()
    store.selectFile('proj-1', makeFile('project.tar.gz', 100))
    await flush()
    apiMock.mockClear()
    apiMock.mockResolvedValue({ applied: true, report: [{ path: 'tom', action: 'create' }], facets: { created: ['npc'] }, images: [] })

    await store.apply('proj-1')

    expect(apiMock).toHaveBeenCalledTimes(1)
    const [url] = apiMock.mock.calls[0]
    expect(url).toContain('mode=apply')
    expect(store.status).toBe('applied')
    expect((store.result as { facetsCreated: string[] }).facetsCreated).toEqual(['npc'])
  })

  it('apply cannot be called twice in a row (status leaves success after the first call)', async () => {
    apiMock.mockResolvedValue({ applied: false, report: [], facets: { created: [] }, images: [] })
    const store = useImportStore()
    store.selectFile('proj-1', makeFile('project.tar.gz', 100))
    await flush()
    apiMock.mockClear()
    apiMock.mockResolvedValue({ applied: true, report: [], facets: { created: [] }, images: [] })

    await store.apply('proj-1')
    apiMock.mockClear()
    await store.apply('proj-1') // status is now 'applied', not 'success' — must be a no-op

    expect(apiMock).not.toHaveBeenCalled()
  })

  it('reset clears all state back to idle', async () => {
    apiMock.mockResolvedValue({ applied: false, report: [{ path: 'tom', action: 'create' }], facets: { created: [] }, images: [] })
    const store = useImportStore()
    store.selectFile('proj-1', makeFile('project.tar.gz', 100))
    store.setOnConflict('proj-1', 'overwrite')
    store.setCreateFacets('proj-1', true)
    await flush()

    store.reset()

    expect(store.file).toBeNull()
    expect(store.onConflict).toBe('fail')
    expect(store.createFacets).toBe(false)
    expect(store.status).toBe('idle')
    expect(store.result).toBeNull()
    expect(store.sizeError).toBe('')
  })

  it('selecting a new file clears a previous report before the new preview resolves', async () => {
    apiMock.mockResolvedValue({ applied: false, report: [{ path: 'first', action: 'create' }], facets: { created: [] }, images: [] })
    const store = useImportStore()
    store.selectFile('proj-1', makeFile('first.tar.gz', 100))
    await flush()
    expect(store.result).not.toBeNull()

    apiMock.mockImplementation(() => new Promise(() => {})) // never resolves
    store.selectFile('proj-1', makeFile('second.tar.gz', 100))

    expect(store.result).toBeNull()
    expect(store.status).toBe('loading')
  })
})
