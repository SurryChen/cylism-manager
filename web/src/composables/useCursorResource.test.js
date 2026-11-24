import { describe, expect, it, vi } from 'vitest'
import { effectScope } from 'vue'
import { useCursorResource } from './useCursorResource.js'

function setup(loader) { const scope = effectScope(); const resource = scope.run(() => useCursorResource(loader)); return { resource, stop: () => scope.stop() } }

describe('useCursorResource', () => {
  it('preserves rows and cursor on failure, revisits previous cursors, and resets size', async () => {
    const loader = vi.fn(async query => ({ items: [query.token || 'first'], nextToken: query.token ? '' : 'second' }))
    const { resource, stop } = setup(loader)
    await resource.load({ bucket: 'one' })
    await resource.changePage(2)
    expect(resource.items.value).toEqual(['second'])
    expect(resource.page.value).toBe(2)
    expect(resource.hasNext.value).toBe(false)
    loader.mockRejectedValueOnce(new Error('offline'))
    await resource.changePage(1)
    expect(resource.items.value).toEqual(['second'])
    expect(resource.page.value).toBe(2)
    await resource.changePage(1)
    expect(resource.items.value).toEqual(['first'])
    await resource.changePageSize(20)
    expect(loader).toHaveBeenLastCalledWith({ bucket: 'one', limit: 20, token: '' }, expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(resource.pageSize.value).toBe(20)
    stop()
  })

  it('discards an obsolete scope request after reset', async () => {
    let resolveOld
    const loader = vi.fn().mockImplementationOnce(() => new Promise(resolve => { resolveOld = resolve })).mockResolvedValue({ items: ['new'], nextToken: '' })
    const { resource, stop } = setup(loader)
    const old = resource.load({ bucket: 'old' })
    resource.reset()
    expect(resource.items.value).toEqual([])
    await resource.load({ bucket: 'new' })
    resolveOld({ items: ['old'], nextToken: 'old-token' })
    await old
    expect(resource.items.value).toEqual(['new'])
    expect(resource.hasNext.value).toBe(false)
    stop()
  })
})
