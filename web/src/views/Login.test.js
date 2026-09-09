import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import Login from './Login.vue'
import { api, setTokens } from '../api/index.js'

const push = vi.fn()

vi.mock('vue-router', () => ({ useRouter: () => ({ push }) }))
vi.mock('../api/index.js', () => ({
  api: { post: vi.fn() },
  setTokens: vi.fn(),
}))

describe('Login view', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.post.mockResolvedValue({ access_token: 'access', refresh_token: 'refresh' })
  })

  it('releases the submitting state and displays a failed login', async () => {
    api.post.mockRejectedValueOnce(new Error('用户名或密码错误'))
    const wrapper = mount(Login)

    await wrapper.get('input[placeholder="admin"]').setValue('admin')
    await wrapper.get('input[type="password"]').setValue('wrong-password')
    await wrapper.get('form').trigger('submit')
    await nextTick()

    expect(wrapper.text()).toContain('用户名或密码错误')
    expect(wrapper.get('button[type="submit"]').text()).toBe('登录')
    expect(setTokens).not.toHaveBeenCalled()
    expect(push).not.toHaveBeenCalled()
  })

  it('uses the temporary-token endpoint when a temporary token is supplied', async () => {
    const wrapper = mount(Login)

    await wrapper.get('input[placeholder="粘贴临时登录秘钥"]').setValue('temporary-token')
    await wrapper.get('form').trigger('submit')

    expect(api.post).toHaveBeenCalledWith('/auth/temporary-login', { token: 'temporary-token' })
    expect(setTokens).toHaveBeenCalledWith('access', 'refresh')
    expect(push).toHaveBeenCalledWith('/')
  })
})
