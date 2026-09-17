import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import { diagnoseRegistryProxy, updateRegistryProxy } from './registry-proxies.js'

vi.mock('./index.js', () => ({ api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() } }))

describe('registry proxies api', () => {
  afterEach(() => vi.clearAllMocks())

  it('encodes identifiers and forwards request options', () => {
    const options = { signal: new AbortController().signal }
    updateRegistryProxy('proxy/a', { name: 'proxy' }, options)
    diagnoseRegistryProxy('proxy/a', options)

    expect(api.put).toHaveBeenCalledWith('/registry-proxies/proxy%2Fa', { name: 'proxy' }, options)
    expect(api.post).toHaveBeenCalledWith('/registry-proxies/proxy%2Fa/diagnose', undefined, options)
  })
})
