import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ProjectEnvironments from './ProjectEnvironments.vue'

vi.mock('../api/index.js', () => ({
  api: {
    get: vi.fn().mockImplementation((path) => {
      if (path === '/projects') return Promise.resolve([{ id: 1, name: 'commerce', description: '订单服务' }])
      if (path === '/projects/1/environments') return Promise.resolve([{ id: 2, name: 'production', namespace: 'commerce-prod', namespace_status: 'active' }])
      if (path === '/applications') return Promise.resolve([{ id: 3, environment_id: 2, name: 'order-api' }])
      return Promise.resolve([])
    }),
    post: vi.fn(),
    put: vi.fn().mockResolvedValue({}),
    delete: vi.fn().mockResolvedValue({}),
  },
}))

describe('ProjectEnvironments view', () => {
  it('renders project environments on a dedicated drill-down page with a back link', async () => {
    const wrapper = mount(ProjectEnvironments, {
      props: { projectID: '1' },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } },
    })
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(wrapper.find('.back-link').text()).toContain('返回项目与环境')
    expect(wrapper.find('.page-title').text()).toBe('commerce 的环境')
    expect(wrapper.text()).toContain('production')
    expect(wrapper.text()).toContain('commerce-prod')
	    expect(wrapper.text()).toContain('就绪')
    expect(wrapper.text()).toContain('关联应用')
    expect(wrapper.text()).toContain('新建环境')
    expect(wrapper.text()).toContain('编辑')
    expect(wrapper.text()).toContain('删除')
    expect(wrapper.find('.btn-danger').attributes('disabled')).toBeUndefined()
  })

  it('does not render an empty list frame when a project has no environments', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation((path) => {
      if (path === '/projects') return Promise.resolve([{ id: 1, name: 'commerce' }])
      return Promise.resolve([])
    })
    const wrapper = mount(ProjectEnvironments, {
      props: { projectID: '1' },
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } },
    })
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(wrapper.find('.page-title').text()).toBe('commerce 的环境')
    expect(wrapper.text()).not.toContain('暂无环境')
    expect(wrapper.find('.card').exists()).toBe(false)
  })

  it('synchronizes a missing environment namespace', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation((path) => {
      if (path === '/projects') return Promise.resolve([{ id: 1, name: 'commerce' }])
      if (path === '/projects/1/environments') return Promise.resolve([{ id: 2, name: 'development', namespace: 'dev', namespace_status: 'missing' }])
      return Promise.resolve([])
    })
    api.post.mockResolvedValue({ id: 2, name: 'development', namespace: 'dev', namespace_status: 'active' })
    const wrapper = mount(ProjectEnvironments, { props: { projectID: '1' }, global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } })
    await new Promise(resolve => setTimeout(resolve, 0))

    await wrapper.get('[title="同步命名空间"]').trigger('click')
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(api.post).toHaveBeenCalledWith('/projects/1/environments/2/sync-namespace')
    expect(wrapper.text()).toContain('就绪')
  })
})
