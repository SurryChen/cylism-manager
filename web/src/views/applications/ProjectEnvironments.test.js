import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ProjectEnvironments from './ProjectEnvironments.vue'

vi.mock('../../api/index.js', () => ({
  api: {
    get: vi.fn().mockImplementation((path) => {
      if (path === '/projects') return Promise.resolve([{ id: 1, name: 'commerce', description: '订单服务' }])
      if (path === '/clusters') return Promise.resolve([{ id: 1, name: '生产集群' }])
      if (path === '/projects/1/environments') return Promise.resolve([{ id: 2, name: 'production', cluster_id: 1, namespace: 'commerce-prod', namespace_status: 'active' }])
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
	    expect(wrapper.text()).toContain('生产集群')
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
      if (path === '/projects/1/environments') return Promise.resolve([{ id: 2, name: 'development', cluster_id: 1, namespace: 'dev', namespace_status: 'missing' }])
      return Promise.resolve([])
    })
    api.post.mockResolvedValue({ id: 2, name: 'development', cluster_id: 1, namespace: 'dev', namespace_status: 'active' })
    const wrapper = mount(ProjectEnvironments, { props: { projectID: '1' }, global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } })
    await new Promise(resolve => setTimeout(resolve, 0))

    await wrapper.get('[title="同步命名空间"]').trigger('click')
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(api.post).toHaveBeenCalledWith('/projects/1/environments/2/sync-namespace')
    expect(wrapper.text()).toContain('就绪')
  })

  it('previews the generated namespace and submits only the environment name and cluster', async () => {
    const { api } = await import('../../api/index.js')
    api.get.mockImplementation((path) => {
      if (path === '/projects') return Promise.resolve([{ id: 1, name: '电商', slug: 'commerce' }])
      if (path === '/clusters') return Promise.resolve([{ id: 1, name: '生产集群' }])
      if (path === '/projects/1/environments') return Promise.resolve([])
      return Promise.resolve([])
    })
    const wrapper = mount(ProjectEnvironments, { props: { projectID: '1' }, global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } })
    await new Promise(resolve => setTimeout(resolve, 0))

    await wrapper.get('.page-header .btn-primary').trigger('click')
    expect(wrapper.find('input[placeholder="frontend-dev"]').exists()).toBe(false)
    expect(wrapper.get('input[disabled][value="project-commerce-production"]').exists()).toBe(true)
    await wrapper.get('input[placeholder="production"]').setValue('development')
    expect(wrapper.get('input[disabled][value="project-commerce-development"]').exists()).toBe(true)
    await wrapper.get('form').trigger('submit')
    expect(api.post).toHaveBeenCalledWith('/projects/1/environments', {
      name: 'development', cluster_id: 1,
    })
  })

  it('keeps a legacy environment namespace when its name is unchanged', async () => {
    const { api } = await import('../../api/index.js')
    api.get.mockImplementation(path => {
      if (path === '/projects') return Promise.resolve([{ id: 1, name: '旧项目' }])
      if (path === '/clusters') return Promise.resolve([{ id: 1, name: '生产集群' }])
      if (path === '/projects/1/environments') return Promise.resolve([{ id: 2, name: '生产环境', cluster_id: 1, namespace: 'legacy-prod' }])
      return Promise.resolve([])
    })
    const wrapper = mount(ProjectEnvironments, { props: { projectID: '1' }, global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } })
    await new Promise(resolve => setTimeout(resolve, 0))

    await wrapper.findAll('button').find(button => button.text() === '编辑').trigger('click')
    expect(wrapper.get('input[placeholder="production"]').attributes('pattern')).toBeUndefined()
    expect(wrapper.get('input[disabled][value="legacy-prod"]').exists()).toBe(true)
    await wrapper.get('form').trigger('submit')
    expect(api.put).toHaveBeenCalledWith('/projects/1/environments/2', { name: '生产环境', cluster_id: 1 })
    wrapper.unmount()
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
      if (path === '/projects') return Promise.resolve([{ id: 1, name: 'commerce', slug: 'commerce' }])
      if (path === '/clusters') return Promise.resolve([{ id: 1, name: '生产集群' }])
      if (path === '/projects/1/environments') return Promise.resolve([])
      return Promise.resolve([])
    })
    api.post.mockRejectedValueOnce(new Error('命名空间已被占用'))
    const wrapper = mount(ProjectEnvironments, { props: { projectID: '1' }, global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } })
    await new Promise(resolve => setTimeout(resolve, 0))
    await wrapper.get('.page-header .btn-primary').trigger('click')
    await wrapper.get('form').trigger('submit')
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(wrapper.find('.modal').exists()).toBe(true)
    expect(document.body.querySelector('.error-notice')?.textContent).toContain('命名空间已被占用')
    wrapper.unmount()
  })
})
