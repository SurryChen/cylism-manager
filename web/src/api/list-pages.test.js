import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import { getApplicationReleasePage, getCertificatePage, getInventoryPage, getWorkspaceReleasePage } from './list-pages.js'
vi.mock('./index.js', () => ({ api: { get: vi.fn() } }))
beforeEach(() => vi.clearAllMocks())
describe('paginated list APIs', () => {
  it('preserves cluster scope, filters, limits, offsets and cancellation', () => {
    const options = { signal: new AbortController().signal }
    getInventoryPage('2', 'pods', { namespace: 'production', keyword: 'order api', limit: 20, offset: 40, status: '' }, options)
    expect(api.get).toHaveBeenLastCalledWith('/clusters/2/k8s/pods?namespace=production&keyword=order+api&limit=20&offset=40', options)
    getCertificatePage(2, { limit: 10, offset: 20 }, options)
    expect(api.get).toHaveBeenLastCalledWith('/clusters/2/k8s/certs?limit=10&offset=20', options)
    getApplicationReleasePage(7, { limit: 10, offset: 10 }, options)
    expect(api.get).toHaveBeenLastCalledWith('/applications/7/releases?limit=10&offset=10', options)
    getWorkspaceReleasePage({ project_id: 3, environment_id: 4, limit: 10, offset: 30 }, options)
    expect(api.get).toHaveBeenLastCalledWith('/workspace/releases?project_id=3&environment_id=4&limit=10&offset=30', options)
  })
})
