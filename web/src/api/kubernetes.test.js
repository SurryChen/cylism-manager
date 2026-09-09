import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import { getResourceInventory, getResourceService } from './kubernetes.js'

vi.mock('./index.js', () => ({ api: { get: vi.fn() } }))

describe('kubernetes api', () => {
  afterEach(() => vi.clearAllMocks())

  it('encodes namespace filters for the resource inventory', () => {
    const options = { signal: new AbortController().signal }
    getResourceInventory('team/a', options)
    expect(api.get).toHaveBeenNthCalledWith(1, '/k8s/pods?namespace=team%2Fa', options)
    expect(api.get).toHaveBeenNthCalledWith(2, '/k8s/services?namespace=team%2Fa', options)
    expect(api.get).toHaveBeenNthCalledWith(3, '/k8s/deployments?namespace=team%2Fa', options)
  })

  it('loads service details with the same request options', () => {
    const options = { signal: new AbortController().signal }
    getResourceService('team/a', 'api/service', options)
    expect(api.get).toHaveBeenCalledWith('/k8s/services/team%2Fa/api%2Fservice', options)
  })
})
