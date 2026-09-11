import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import {
  drainNode,
  forceDrainNode,
  getClusterInventory,
  getNodeDrainPlan,
  getNodeLabels,
  getNodeRemovalCheck,
  rejoinNode,
  removeClusterNode,
  updateNodeLabels,
} from './cluster.js'

vi.mock('./index.js', () => ({
  api: { get: vi.fn(), post: vi.fn(), patch: vi.fn(), delete: vi.fn() },
}))

describe('cluster api', () => {
  afterEach(() => vi.clearAllMocks())

  it('loads nodes and server inventory with one signal', () => {
    const options = { signal: new AbortController().signal }
    getClusterInventory(options)
    expect(api.get).toHaveBeenNthCalledWith(1, '/nodes', options)
    expect(api.get).toHaveBeenNthCalledWith(2, '/servers', options)
  })

  it('uses the existing node workflow endpoints and forwards request options', () => {
    const nodeName = 'node/a'
    const body = { force: false }
    const options = { signal: new AbortController().signal }

    getNodeDrainPlan(nodeName, options)
    drainNode(nodeName, body, options)
    forceDrainNode(nodeName, body, options)
    rejoinNode(nodeName, options)
    getNodeLabels(nodeName, options)
    updateNodeLabels(nodeName, body, options)
    getNodeRemovalCheck(nodeName, options)
    removeClusterNode(nodeName, options)

    expect(api.get).toHaveBeenNthCalledWith(1, '/nodes/node%2Fa/drain-plan', options)
    expect(api.post).toHaveBeenNthCalledWith(1, '/nodes/node%2Fa/drain', body, options)
    expect(api.post).toHaveBeenNthCalledWith(2, '/nodes/node%2Fa/force-drain', body, options)
    expect(api.post).toHaveBeenNthCalledWith(3, '/nodes/node%2Fa/rejoin', undefined, options)
    expect(api.get).toHaveBeenNthCalledWith(2, '/nodes/node%2Fa/labels', options)
    expect(api.patch).toHaveBeenCalledWith('/nodes/node%2Fa/labels', body, options)
    expect(api.get).toHaveBeenNthCalledWith(3, '/nodes/node%2Fa/removal-check', options)
    expect(api.delete).toHaveBeenCalledWith('/nodes/node%2Fa', undefined, options)
  })
})
