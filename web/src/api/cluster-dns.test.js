import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import {
  deleteClusterDNS,
  getClusterDNS,
  rollbackClusterDNS,
  updateClusterDNS,
} from './cluster-dns.js'

vi.mock('./index.js', () => ({
  api: { get: vi.fn(), post: vi.fn(), delete: vi.fn() },
}))

describe('cluster dns api', () => {
  afterEach(() => vi.clearAllMocks())

  it('forwards reads and mutations with their exact request contract', () => {
    const options = { signal: new AbortController().signal }
    const body = { resolvers: ['223.5.5.5'] }

    getClusterDNS(options)
    updateClusterDNS(body)
    deleteClusterDNS(options)
    rollbackClusterDNS('revision/1', options)

    expect(api.get).toHaveBeenCalledWith('/cluster-dns', options)
    expect(api.post).toHaveBeenNthCalledWith(1, '/cluster-dns', body)
    expect(api.delete).toHaveBeenCalledWith('/cluster-dns', undefined, options)
    expect(api.post).toHaveBeenNthCalledWith(2, '/cluster-dns/history/revision%2F1/rollback', undefined, options)
  })
})
