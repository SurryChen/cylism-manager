import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './index.js'

describe('API response handling', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    localStorage.clear()
  })

  it('explains when a proxied API request receives the SPA HTML fallback', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('<!doctype html><html><body>app</body></html>', {
      status: 200,
      headers: { 'content-type': 'text/html' },
    })))

    await expect(api.get('/managed-oci-registries')).rejects.toThrow('服务端未提供此 API')
  })
})
