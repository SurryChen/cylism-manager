import { describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import { createChartRepository, deleteChartRepository, getChartRepositories, updateChartRepository, verifyChartRepository } from './chart-repositories.js'

vi.mock('./index.js', () => ({ api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() } }))

describe('chart repositories api', () => {
  it('owns chart repository reads and mutations', () => {
    const options = { signal: new AbortController().signal }
    const body = { name: 'jetstack' }
    getChartRepositories(options)
    createChartRepository(body, options)
    updateChartRepository('repo/1', body, options)
    verifyChartRepository('repo/1', options)
    deleteChartRepository('repo/1', options)

    expect(api.get).toHaveBeenCalledWith('/chart-repositories', options)
    expect(api.post).toHaveBeenNthCalledWith(1, '/chart-repositories', body, options)
    expect(api.put).toHaveBeenCalledWith('/chart-repositories/repo%2F1', body, options)
    expect(api.post).toHaveBeenNthCalledWith(2, '/chart-repositories/repo%2F1/verify', undefined, options)
    expect(api.delete).toHaveBeenCalledWith('/chart-repositories/repo%2F1', undefined, options)
  })
})
