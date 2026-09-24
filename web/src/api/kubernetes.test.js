import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import { getLightweightServiceDiscovery, getResourceInventory, getResourceService, getWorkloadDeploymentPods, getConfigMap, getConfigMapsForNamespace, getSecretMetadataPage } from './kubernetes.js'

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

  it('loads Service inventory without endpoint counts', () => {
    const options = { signal: new AbortController().signal }
    getLightweightServiceDiscovery('', options)
    expect(api.get).toHaveBeenCalledWith('/k8s/services?endpoint_count=false', options)
  })

  it('encodes workload and config resource segments', () => {
    const options = { signal: new AbortController().signal }
    getWorkloadDeploymentPods('team/a', 'api/service', options)
    getConfigMap('team/a', 'app/config', options)
    getConfigMapsForNamespace('team/a', options)
    expect(api.get).toHaveBeenNthCalledWith(1, '/k8s/deployments/team%2Fa/api%2Fservice/pods', options)
    expect(api.get).toHaveBeenNthCalledWith(2, '/k8s/configmaps/team%2Fa/app%2Fconfig', options)
    expect(api.get).toHaveBeenNthCalledWith(3, '/k8s/configmaps?namespace=team%2Fa&usage=false', options)
  })

  it('loads a paged metadata-only Secret inventory', () => {
    const options = { signal: new AbortController().signal }
    getSecretMetadataPage('team/a', { continueToken: 'page/2', ...options })
    expect(api.get).toHaveBeenCalledWith('/k8s/secrets?namespace=team%2Fa&metadata=true&limit=50&continue=page%2F2', options)
  })
})
