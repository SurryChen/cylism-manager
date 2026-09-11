import { describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import {
  getClusterNodes,
  getSystemComponents,
  revertSystemComponent,
  updateSystemComponent,
} from './system-components.js'

vi.mock('./index.js', () => ({ api: { get: vi.fn(), post: vi.fn(), put: vi.fn() } }))

describe('system components api', () => {
  it('forwards request options', () => {
    const options = { signal: new AbortController().signal }
    getSystemComponents(options)
    getClusterNodes(options)
    expect(api.get).toHaveBeenNthCalledWith(1, '/system-components', options)
    expect(api.get).toHaveBeenNthCalledWith(2, '/nodes', options)
  })

  it('owns component mutations with encoded names and optional request options', () => {
    const options = { signal: new AbortController().signal }
    const payload = { values_content: 'replicaCount: 2' }

    updateSystemComponent('traefik/test', payload)
    revertSystemComponent('traefik/test', options)

    expect(api.put).toHaveBeenCalledWith('/system-components/traefik%2Ftest', payload)
    expect(api.post).toHaveBeenCalledWith('/system-components/traefik%2Ftest/revert', undefined, options)
  })
})
