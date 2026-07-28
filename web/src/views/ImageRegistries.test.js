import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ImageRegistries from './ImageRegistries.vue'

vi.mock('../api/index.js', () => ({
  api: {
    get: vi.fn().mockImplementation(path => {
      if (path === '/projects') return Promise.resolve([{ id: 1, name: 'commerce' }])
      return Promise.resolve([])
    }),
    post: vi.fn().mockResolvedValue({ id: 1 }),
    put: vi.fn().mockResolvedValue({ id: 1 }),
    delete: vi.fn().mockResolvedValue({}),
  },
}))

describe('ImageRegistries view', () => {
  it('keeps the registry list unframed before any registry is configured', async () => {
    const wrapper = mount(ImageRegistries)
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(wrapper.find('.page-title').text()).toBe('镜像仓库')
    expect(wrapper.text()).toContain('新建镜像仓库')
    expect(wrapper.text()).not.toContain('暂无镜像仓库')
    expect(wrapper.find('.card').exists()).toBe(false)
  })

  it('shows registry authorization and never displays the credential value', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(path => {
      if (path === '/projects') return Promise.resolve([{ id: 1, name: 'commerce' }])
      return Promise.resolve([{
        id: 12, name: 'commerce-harbor', endpoint: 'harbor.example.com', auth_type: 'basic', username: 'robot$commerce', credential_configured: true, enabled: true, projects: [{ id: 1, name: 'commerce' }],
      }])
    })
    const wrapper = mount(ImageRegistries)
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(wrapper.text()).toContain('commerce-harbor')
    expect(wrapper.text()).toContain('harbor.example.com')
    expect(wrapper.text()).toContain('commerce')
    expect(wrapper.text()).toContain('凭据已配置')
    expect(wrapper.text()).not.toContain('registry-password')
  })
})
