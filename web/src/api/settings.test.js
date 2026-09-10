import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import {
  adoptPlatformIngress,
  createPlatformRelease,
  createTemporaryToken,
  deleteTemporaryToken,
  generatePlatformWebhookSecret,
  getPlatformEndpoint,
  getPlatformStatus,
  getTemporaryTokens,
  reconcilePlatformEndpoint,
  rollbackPlatformRelease,
  updatePlatformEndpoint,
  updatePlatformImagePrefix,
} from './settings.js'

vi.mock('./index.js', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}))

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

  it('owns platform and temporary-token mutations with encoded identifiers', () => {
    const options = { signal: new AbortController().signal }
    const endpoint = { hostname: 'console.example.com', enabled: true }
    const token = { label: 'support', ttl_seconds: 3600 }

    updatePlatformEndpoint(endpoint)
    adoptPlatformIngress(options)
    reconcilePlatformEndpoint()
    createTemporaryToken(token, options)
    deleteTemporaryToken('token/1', options)
    generatePlatformWebhookSecret()
    updatePlatformImagePrefix({ image_prefix: 'ghcr.io/example/platform' }, options)
    createPlatformRelease({ image: 'ghcr.io/example/platform:1.0.0' })
    rollbackPlatformRelease('release/1', options)

    expect(api.put).toHaveBeenNthCalledWith(1, '/platform/endpoint', endpoint)
    expect(api.post).toHaveBeenNthCalledWith(1, '/platform/endpoint/adopt-ingress', undefined, options)
    expect(api.post).toHaveBeenNthCalledWith(2, '/platform/endpoint/reconcile')
    expect(api.post).toHaveBeenNthCalledWith(3, '/auth/temporary-tokens', token, options)
    expect(api.delete).toHaveBeenCalledWith('/auth/temporary-tokens/token%2F1', undefined, options)
    expect(api.post).toHaveBeenNthCalledWith(4, '/platform/webhook-secret')
    expect(api.put).toHaveBeenNthCalledWith(2, '/platform/image-prefix', { image_prefix: 'ghcr.io/example/platform' }, options)
    expect(api.post).toHaveBeenNthCalledWith(5, '/platform/releases', { image: 'ghcr.io/example/platform:1.0.0' })
    expect(api.post).toHaveBeenNthCalledWith(6, '/platform/releases/release%2F1/rollback', undefined, options)
  })
})
