import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import { getServerStats, getServers } from './servers.js'

vi.mock('./index.js', () => ({ api: { get: vi.fn() } }))

describe('servers API', () => {
  afterEach(() => vi.clearAllMocks())

  it('passes the resource abort signal to server reads', () => {
    const signal = new AbortController().signal

    getServers({ signal })
    getServerStats(42, { signal })

    expect(api.get).toHaveBeenNthCalledWith(1, '/servers', { signal })
    expect(api.get).toHaveBeenNthCalledWith(2, '/servers/42/stats', { signal })
  })
})
