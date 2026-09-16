import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import { applyNodeRegistryMirror, getNodeRegistryMirrorApplyStatus } from './node-registry-mirrors.js'

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
