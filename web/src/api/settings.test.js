import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import { getPlatformEndpoint, getPlatformStatus, getTemporaryTokens } from './settings.js'

vi.mock('./index.js', () => ({ api: { get: vi.fn() } }))

describe('settings api', () => {
  afterEach(() => vi.clearAllMocks())

  it('exposes named settings reads with options', () => {
    const options = { signal: new AbortController().signal }
    getPlatformEndpoint(options)
    getPlatformStatus(options)
    getTemporaryTokens(options)
    expect(api.get).toHaveBeenNthCalledWith(1, '/platform/endpoint', options)
    expect(api.get).toHaveBeenNthCalledWith(2, '/platform/status', options)
    expect(api.get).toHaveBeenNthCalledWith(3, '/auth/temporary-tokens', options)
  })
})
