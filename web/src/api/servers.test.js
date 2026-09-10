import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import {
  createServer,
  deleteServer,
  getServerStats,
  getServers,
  importServerToCluster,
  preimportServer,
  probeServer,
  unbindServer,
  updateServer,
} from './servers.js'

vi.mock('./index.js', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}))

describe('servers API', () => {
  afterEach(() => vi.clearAllMocks())

  it('passes the resource abort signal to server reads', () => {
    const signal = new AbortController().signal

    getServers({ signal })
    getServerStats(42, { signal })

    expect(api.get).toHaveBeenNthCalledWith(1, '/servers', { signal })
    expect(api.get).toHaveBeenNthCalledWith(2, '/servers/42/stats', { signal })
  })

  it('uses the existing server mutation endpoints and forwards request options', () => {
    const body = { name: 'edge-1' }
    const options = { signal: new AbortController().signal }

    createServer(body, options)
    updateServer('server/a', body, options)
    deleteServer('server/a', options)
    probeServer('server/a', options)
    preimportServer('server/a', options)
    importServerToCluster('server/a', body, options)
    unbindServer('server/a', options)

    expect(api.post).toHaveBeenNthCalledWith(1, '/servers', body, options)
    expect(api.put).toHaveBeenCalledWith('/servers/server%2Fa', body, options)
    expect(api.delete).toHaveBeenNthCalledWith(1, '/servers/server%2Fa', undefined, options)
    expect(api.post).toHaveBeenNthCalledWith(2, '/servers/server%2Fa/probe', undefined, options)
    expect(api.post).toHaveBeenNthCalledWith(3, '/nodes/server%2Fa/preimport', undefined, options)
    expect(api.post).toHaveBeenNthCalledWith(4, '/nodes/server%2Fa/import', body, options)
    expect(api.post).toHaveBeenNthCalledWith(5, '/servers/server%2Fa/unbind', undefined, options)
  })
})
