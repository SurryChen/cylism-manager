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

  it('forwards an abort signal to fetch', async () => {
    const signal = new AbortController().signal
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ code: 0, data: {} }), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    }))
    vi.stubGlobal('fetch', fetchMock)

    await api.get('/dashboard', { signal })

    expect(fetchMock).toHaveBeenCalledWith('/api/dashboard', expect.objectContaining({ signal }))
  })
})
