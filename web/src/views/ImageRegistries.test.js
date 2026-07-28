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
        id: 12, name: 'commerce-harbor', endpoint: 'harbor.example.com', verification_image: 'harbor.example.com/commerce/order-api:latest', auth_type: 'basic', username: 'robot$commerce', credential_configured: true, enabled: true, projects: [{ id: 1, name: 'commerce' }],
      }])
    })
    const wrapper = mount(ImageRegistries)
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(wrapper.text()).toContain('commerce-harbor')
    expect(wrapper.text()).toContain('harbor.example.com')
    expect(wrapper.text()).toContain('harbor.example.com/commerce/order-api:latest')
    expect(wrapper.text()).toContain('commerce')
    expect(wrapper.text()).toContain('凭据已配置')
    expect(wrapper.text()).not.toContain('registry-password')
  })

  it('runs a connection check and shows the saved verification result', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(path => {
      if (path === '/projects') return Promise.resolve([{ id: 1, name: 'commerce' }])
      return Promise.resolve([{ id: 12, name: 'commerce-harbor', endpoint: 'harbor.example.com', verification_image: 'harbor.example.com/commerce/order-api:latest', auth_type: 'basic', enabled: true, projects: [{ id: 1, name: 'commerce' }] }])
    })
    api.post.mockResolvedValue({ id: 12, name: 'commerce-harbor', endpoint: 'harbor.example.com', verification_image: 'harbor.example.com/commerce/order-api:latest', auth_type: 'basic', enabled: true, projects: [{ id: 1, name: 'commerce' }], last_verify_status: 'succeeded', last_verified_at: '2026-07-28T11:30:00Z' })
    const wrapper = mount(ImageRegistries)
    await new Promise(resolve => setTimeout(resolve, 0))

    await wrapper.get('[title="检测镜像仓库"]').trigger('click')
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(api.post).toHaveBeenCalledWith('/image-registries/12/verify')
    expect(wrapper.text()).toContain('连通')
  })

  it('saves the real image reference used for registry verification', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(path => path === '/projects' ? Promise.resolve([{ id: 1, name: 'commerce' }]) : Promise.resolve([]))
    api.post.mockResolvedValue({ id: 12 })
    const wrapper = mount(ImageRegistries)
    await new Promise(resolve => setTimeout(resolve, 0))

    await wrapper.get('.page-header .btn-primary').trigger('click')
    await wrapper.get('input[placeholder="commerce-harbor"]').setValue('commerce-harbor')
    await wrapper.get('input[placeholder="harbor.example.com"]').setValue('harbor.example.com')
    await wrapper.get('input[placeholder="harbor.example.com/commerce/order-api:latest"]').setValue('harbor.example.com/commerce/order-api:latest')
    await wrapper.get('.project-option input').setValue(true)
    await wrapper.get('form').trigger('submit')
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(api.post).toHaveBeenLastCalledWith('/image-registries', {
      name: 'commerce-harbor', endpoint: 'harbor.example.com', verification_image: 'harbor.example.com/commerce/order-api:latest', auth_type: 'anonymous', username: '', credential: '', enabled: true, project_ids: [1],
    })
  })
})
