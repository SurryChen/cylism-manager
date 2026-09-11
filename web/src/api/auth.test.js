import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './index.js'
import { login, temporaryLogin } from './auth.js'

vi.mock('./index.js', () => ({ api: { post: vi.fn() } }))

describe('auth api', () => {
  afterEach(() => vi.clearAllMocks())

  it('posts credential login payloads', () => {
    const options = { signal: new AbortController().signal }
    login('admin', 'secret', options)
    expect(api.post).toHaveBeenCalledWith('/auth/login', { username: 'admin', password: 'secret' }, options)
  })

  it('posts temporary login tokens', () => {
    temporaryLogin('one-time-token')
    expect(api.post).toHaveBeenCalledWith('/auth/temporary-login', { token: 'one-time-token' })
  })
})
