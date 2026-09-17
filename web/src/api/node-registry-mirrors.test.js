import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import { applyNodeRegistryMirror, getNodeRegistryMirrorApplyStatus, inspectActualNodeRegistryConfiguration, restartNodeK3sService } from './node-registry-mirrors.js'

vi.mock('./index.js', () => ({ api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() } }))

describe('node registry mirrors api', () => {
  afterEach(() => vi.clearAllMocks())

  it('encodes identifiers and forwards options', () => {
    const options = { signal: new AbortController().signal }
    applyNodeRegistryMirror('team/a', { server_ids: [1] }, options)
    getNodeRegistryMirrorApplyStatus('team/a', options)
    expect(api.post).toHaveBeenNthCalledWith(1, '/node-registry-mirrors/team%2Fa/apply', { server_ids: [1] }, options)
    expect(api.get).toHaveBeenCalledWith('/node-registry-mirrors/team%2Fa/apply-status', options)
  })
})

it('requests an explicit all-node actual configuration inspection', () => {
  const options = { signal: new AbortController().signal }
  inspectActualNodeRegistryConfiguration(options)
  expect(api.post).toHaveBeenCalledWith('/node-registry-mirrors/inspect-actual-config', undefined, options)
})

it('selects a managed node and requests its fixed K3s service restart', () => {
	vi.clearAllMocks()
  inspectActualNodeRegistryConfiguration({ server_id: 17 })
  restartNodeK3sService('node/a')
  expect(api.post).toHaveBeenNthCalledWith(1, '/node-registry-mirrors/inspect-actual-config', { server_id: 17 })
  expect(api.post).toHaveBeenNthCalledWith(2, '/node-registry-mirrors/nodes/node%2Fa/restart-k3s')
})
