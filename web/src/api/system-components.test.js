import { describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import { getClusterNodes, getSystemComponents } from './system-components.js'

vi.mock('./index.js', () => ({ api: { get: vi.fn() } }))

describe('system components api', () => {
  it('forwards request options', () => {
    const options = { signal: new AbortController().signal }
    getSystemComponents(options)
    getClusterNodes(options)
    expect(api.get).toHaveBeenNthCalledWith(1, '/system-components', options)
    expect(api.get).toHaveBeenNthCalledWith(2, '/nodes', options)
  })
})
