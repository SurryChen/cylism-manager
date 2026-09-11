import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ProjectEnvironments from './ProjectEnvironments.vue'

vi.mock('../../api/index.js', () => ({
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
    const { api } = await import('../../api/index.js')
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
    const { api } = await import('../../api/index.js')
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

  it('uses a fixed project prefix while the operator enters the namespace suffix', async () => {
    const { api } = await import('../../api/index.js')
    api.get.mockImplementation((path) => {
      if (path === '/projects') return Promise.resolve([{ id: 1, name: 'commerce' }])
      if (path === '/projects/1/environments') return Promise.resolve([])
      return Promise.resolve([])
    })
    const wrapper = mount(ProjectEnvironments, { props: { projectID: '1' }, global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } })
    await new Promise(resolve => setTimeout(resolve, 0))

    await wrapper.get('.page-header .btn-primary').trigger('click')
    const namespace = wrapper.get('input[placeholder="frontend-dev"]')
    expect(wrapper.find('.namespace-prefix').text()).toBe('project-')
    expect(namespace.element.value).toBe('')

    await namespace.setValue('frontend-dev')
    await wrapper.get('form').trigger('submit')
    expect(api.post).toHaveBeenCalledWith('/projects/1/environments', {
      name: 'production', namespace: 'project-frontend-dev', namespace_mode: 'create',
    })
  })

  it('cancels the active project read when the view unmounts', async () => {
    const { api } = await import('../../api/index.js')
    let signal
    api.get.mockImplementation((_path, options) => {
      signal ||= options?.signal
      return new Promise(() => {})
    })
    const wrapper = mount(ProjectEnvironments, { props: { projectID: '1' }, global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } })
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(signal).toBeDefined()
    expect(signal.aborted).toBe(false)
    wrapper.unmount()
    expect(signal.aborted).toBe(true)
  })

  it('does not let an older project response overwrite the newer project', async () => {
    const { api } = await import('../../api/index.js')
    let resolveFirstEnvironments
    let projectReadCount = 0
    api.get.mockImplementation((path) => {
      if (path === '/projects') {
        projectReadCount += 1
        return Promise.resolve(projectReadCount === 1 ? [{ id: 1, name: 'commerce' }] : [{ id: 2, name: 'billing' }])
      }
      if (path === '/projects/1/environments') return new Promise(resolve => { resolveFirstEnvironments = resolve })
      if (path === '/projects/2/environments') return Promise.resolve([{ id: 4, name: 'production', namespace: 'billing-prod', namespace_status: 'active' }])
      return Promise.resolve([])
    })
    const wrapper = mount(ProjectEnvironments, { props: { projectID: '1' }, global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } })
    await new Promise(resolve => setTimeout(resolve, 0))
    await wrapper.setProps({ projectID: '2' })
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(wrapper.find('.page-title').text()).toBe('billing 的环境')
    expect(wrapper.text()).toContain('billing-prod')

    resolveFirstEnvironments([{ id: 2, name: 'production', namespace: 'commerce-prod', namespace_status: 'active' }])
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(wrapper.find('.page-title').text()).toBe('billing 的环境')
    expect(wrapper.text()).not.toContain('commerce-prod')
  })

  it('keeps an environment form open when saving fails', async () => {
    const { api } = await import('../../api/index.js')
    api.get.mockImplementation((path) => {
      if (path === '/projects') return Promise.resolve([{ id: 1, name: 'commerce' }])
      if (path === '/projects/1/environments') return Promise.resolve([])
      return Promise.resolve([])
    })
    api.post.mockRejectedValueOnce(new Error('命名空间已被占用'))
    const wrapper = mount(ProjectEnvironments, { props: { projectID: '1' }, global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } })
    await new Promise(resolve => setTimeout(resolve, 0))
    await wrapper.get('.page-header .btn-primary').trigger('click')
    await wrapper.get('input[placeholder="frontend-dev"]').setValue('commerce-prod')
    await wrapper.get('form').trigger('submit')
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(wrapper.find('.modal').exists()).toBe(true)
    expect(wrapper.text()).toContain('命名空间已被占用')
  })
})
