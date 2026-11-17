import { effectScope } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { usePaginatedResource } from './usePaginatedResource.js'

function createPagination(loader) {
  const scope = effectScope()
  return { pagination: scope.run(() => usePaginatedResource(loader, 'data')), scope }
}

describe('usePaginatedResource', () => {
  it('clears the previous scope and ignores its pending response after reset', async () => {
    let resolvePending
    const loader = vi.fn().mockResolvedValueOnce({ data: ['private app'], total: 31 })
      .mockResolvedValueOnce({ data: ['private app'], total: 31 })
      .mockImplementationOnce(() => new Promise(resolve => { resolvePending = resolve }))
    const { pagination: p, scope } = createPagination(loader)
    await p.load({ applicationID: 1 })
    await p.changePageSize(20)
    const pending = p.refresh()
    p.reset()
    resolvePending({ data: ['obsolete app'], total: 31 })
    await pending
    expect(p.items.value).toEqual([])
    expect(p.total.value).toBe(0)
    expect(p.filters.value).toEqual({})
    expect(p.page.value).toBe(1)
    expect(p.pageSize.value).toBe(20)
    expect(p.loading.value).toBe(false)
    scope.stop()
  })
  it('uses ten rows by default and retains applied filters when navigating or resizing', async () => {
    const loader = vi.fn(async ({ offset }) => ({ data: [offset + 1], total: 35 }))
    const { pagination: p, scope } = createPagination(loader)
    const filters = { keyword: 'applied' }
    await p.load(filters)
    filters.keyword = 'draft'
    expect(loader).toHaveBeenLastCalledWith({ keyword: 'applied', limit: 10, offset: 0 }, expect.objectContaining({ signal: expect.any(AbortSignal) }))
    await p.changePage(3)
    expect(p.items.value).toEqual([21])
    expect(loader.mock.lastCall[0]).toEqual({ keyword: 'applied', limit: 10, offset: 20 })
    await p.changePageSize(20)
    expect(p.page.value).toBe(1)
    expect(p.pageSize.value).toBe(20)
    expect(loader.mock.lastCall[0]).toEqual({ keyword: 'applied', limit: 20, offset: 0 })
    scope.stop()
  })

  it('keeps rows, page, size and applied filters together after failed requests', async () => {
    const loader = vi.fn(async ({ offset }) => ({ data: [offset + 1], total: 35 }))
    const { pagination: p, scope } = createPagination(loader)
    await p.load({ keyword: 'initial' })
    await p.changePage(2)
    loader.mockRejectedValueOnce(new Error('unavailable'))
    await p.changePage(3)
    expect(p.page.value).toBe(2)
    expect(p.items.value).toEqual([11])
    expect(p.error.value.message).toBe('unavailable')
    loader.mockRejectedValueOnce(new Error('resize failed'))
    await p.changePageSize(50)
    expect(p.page.value).toBe(2)
    expect(p.pageSize.value).toBe(10)
    loader.mockRejectedValueOnce(new Error('filter failed'))
    await p.load({ keyword: 'new filter' })
    await p.changePage(3)
    expect(loader.mock.lastCall[0].keyword).toBe('initial')
    await p.load({ keyword: 'new filter' })
    expect(p.page.value).toBe(1)
    expect(p.error.value).toBeNull()
    scope.stop()
  })

  it('refetches the last valid page when records disappear during pagination', async () => {
    const loader = vi.fn().mockResolvedValueOnce({ data: [1], total: 35 })
      .mockResolvedValueOnce({ data: [], total: 15 })
      .mockResolvedValueOnce({ data: [], total: 5 })
      .mockResolvedValueOnce({ data: [1, 2, 3, 4, 5], total: 5 })
    const { pagination: p, scope } = createPagination(loader)
    await p.load()
    await p.changePage(4)
    expect(loader.mock.calls.map(([params]) => params.offset)).toEqual([0, 30, 10, 0])
    expect(p.page.value).toBe(1)
    expect(p.total.value).toBe(5)
    expect(p.items.value).toHaveLength(5)
    scope.stop()
  })

  it('ignores superseded responses and blocks repeated navigation while loading', async () => {
    let resolveOlder
    const loader = vi.fn().mockImplementationOnce(() => new Promise(resolve => { resolveOlder = resolve }))
      .mockResolvedValueOnce({ data: ['new'], total: 30 })
    const { pagination: p, scope } = createPagination(loader)
    const older = p.load({ keyword: 'older' })
    p.changePage(2)
    p.changePageSize(20)
    expect(loader).toHaveBeenCalledTimes(1)
    await p.load({ keyword: 'newer' })
    expect(loader.mock.calls[0][1].signal.aborted).toBe(true)
    resolveOlder({ data: ['old'], total: 1 })
    await older
    expect(p.items.value).toEqual(['new'])
    expect(p.total.value).toBe(30)
    expect(p.error.value).toBeNull()
    scope.stop()
  })
})
