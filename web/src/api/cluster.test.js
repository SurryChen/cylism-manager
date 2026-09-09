import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import { getClusterInventory } from './cluster.js'

vi.mock('./index.js', () => ({ api: { get: vi.fn() } }))

describe('cluster api', () => {
  afterEach(() => vi.clearAllMocks())

  it('loads nodes and server inventory with one signal', () => {
    const options = { signal: new AbortController().signal }
    getClusterInventory(options)
    expect(api.get).toHaveBeenNthCalledWith(1, '/nodes', options)
    expect(api.get).toHaveBeenNthCalledWith(2, '/servers', options)
  })
})
