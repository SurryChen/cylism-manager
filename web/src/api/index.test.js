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

  it('shares one refresh request when concurrent API calls receive 401', async () => {
    localStorage.setItem('access_token', 'expired-access')
    localStorage.setItem('refresh_token', 'refresh-token')
    let refreshResolve
    const refreshResponse = new Promise(resolve => { refreshResolve = resolve })
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ code: 1, message: 'expired' }), { status: 401, headers: { 'content-type': 'application/json' } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ code: 1, message: 'expired' }), { status: 401, headers: { 'content-type': 'application/json' } }))
      .mockImplementationOnce(() => refreshResponse)
      .mockImplementation(() => new Response(JSON.stringify({ code: 0, data: { ok: true } }), { status: 200, headers: { 'content-type': 'application/json' } }))
    vi.stubGlobal('fetch', fetchMock)

    const first = api.get('/first')
    const second = api.get('/second')
    await Promise.resolve()
    refreshResolve(new Response(JSON.stringify({ code: 0, data: { access_token: 'fresh-access', refresh_token: 'fresh-refresh' } }), { status: 200, headers: { 'content-type': 'application/json' } }))

    await expect(Promise.all([first, second])).resolves.toEqual([{ ok: true }, { ok: true }])
    expect(fetchMock.mock.calls.filter(([url]) => url === '/api/auth/refresh')).toHaveLength(1)
    expect(localStorage.getItem('access_token')).toBe('fresh-access')
    expect(fetchMock).toHaveBeenLastCalledWith('/api/second', expect.objectContaining({ headers: expect.objectContaining({ Authorization: 'Bearer fresh-access' }) }))
  })

  it('clears the session when a shared refresh fails', async () => {
    localStorage.setItem('access_token', 'expired-access')
    localStorage.setItem('refresh_token', 'refresh-token')
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ code: 1, message: 'expired' }), { status: 401, headers: { 'content-type': 'application/json' } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ code: 1, message: 'expired' }), { status: 401, headers: { 'content-type': 'application/json' } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ code: 1, message: 'invalid refresh' }), { status: 401, headers: { 'content-type': 'application/json' } }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(Promise.allSettled([api.get('/first'), api.get('/second')])).resolves.toEqual([
      { status: 'rejected', reason: expect.any(Error) },
      { status: 'rejected', reason: expect.any(Error) },
    ])
    expect(fetchMock.mock.calls.filter(([url]) => url === '/api/auth/refresh')).toHaveLength(1)
    expect(localStorage.getItem('access_token')).toBeNull()
    expect(localStorage.getItem('refresh_token')).toBeNull()
  })

  it('preserves the session when authentication refresh is cancelled', async () => {
    localStorage.setItem('access_token', 'expired-access')
    localStorage.setItem('refresh_token', 'refresh-token')
    const controller = new AbortController()
    let resolveRefresh
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ code: 1, message: 'expired' }), { status: 401, headers: { 'content-type': 'application/json' } }))
      .mockImplementationOnce(() => new Promise(resolve => { resolveRefresh = resolve }))
    vi.stubGlobal('fetch', fetchMock)

    const request = api.get('/cancelled', { signal: controller.signal })
    controller.abort()
    await expect(request).rejects.toMatchObject({ name: 'AbortError' })
    expect(localStorage.getItem('refresh_token')).toBe('refresh-token')
    resolveRefresh(new Response(JSON.stringify({ code: 0, data: { access_token: 'fresh-access', refresh_token: 'fresh-refresh' } }), { status: 200, headers: { 'content-type': 'application/json' } }))
  })
})
