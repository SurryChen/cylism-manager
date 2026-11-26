import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
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
      global: { stubs: { Teleport: true, RouterLink: { template: '<a><slot /></a>' } } },
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
    expect(wrapper.get('.environment-link').text()).toBe('production')
    expect(wrapper.findAll('.data-table thead th').map(cell => cell.text())).toEqual(['环境', '集群', '命名空间', '命名空间状态', '关联应用', '操作'])
    const cells = wrapper.findAll('.data-table tbody td')
    expect(cells[1].text()).toBe('生产集群')
    expect(cells[2].text()).toBe('commerce-prod')
    expect(cells[3].text()).toBe('就绪')
    expect(wrapper.get('.product-toolbar').text()).toContain('1 个环境')
    expect(wrapper.find('[aria-label="项目环境分页"]').exists()).toBe(false)
    expect(wrapper.find('.btn-danger').exists()).toBe(false)
    expect(wrapper.findAll('.data-table tbody button')).toHaveLength(1)
    await wrapper.get('[title="编辑环境"]').trigger('click')
    expect(wrapper.get('.modal-title').text()).toBe('编辑环境')
    expect(wrapper.get('.environment-delete').text()).toBe('删除环境')
    wrapper.unmount()
  })

  it('does not render an empty list frame when a project has no environments', async () => {
    const { api } = await import('../../api/index.js')
    api.get.mockImplementation((path) => {
      if (path === '/projects') return Promise.resolve([{ id: 1, name: 'commerce' }])
      return Promise.resolve([])
    })
    const wrapper = mount(ProjectEnvironments, {
      props: { projectID: '1' },
      global: { stubs: { Teleport: true, RouterLink: { template: '<a><slot /></a>' } } },
    })
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(wrapper.find('.page-title').text()).toBe('commerce 的环境')
    expect(wrapper.text()).toContain('暂无部署环境')
    expect(wrapper.find('.card').exists()).toBe(false)
  })

  it('synchronizes a missing environment namespace', async () => {
    const { api } = await import('../../api/index.js')
    api.get.mockImplementation((path) => {
      if (path === '/projects') return Promise.resolve([{ id: 1, name: 'commerce' }])
      if (path === '/projects/1/environments') return Promise.resolve([{ id: 2, name: 'development', kind: 'testing', cluster_id: 1, namespace: 'dev', namespace_status: 'missing' }])
      return Promise.resolve([])
    })
    api.post.mockResolvedValue({ id: 2, name: 'development', kind: 'testing', cluster_id: 1, namespace: 'dev', namespace_status: 'active' })
    const wrapper = mount(ProjectEnvironments, { props: { projectID: '1' }, global: { stubs: { Teleport: true, RouterLink: { template: '<a><slot /></a>' } } } })
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
    const wrapper = mount(ProjectEnvironments, { props: { projectID: '1' }, global: { stubs: { Teleport: true, RouterLink: { template: '<a><slot /></a>' } } } })
    await new Promise(resolve => setTimeout(resolve, 0))

    await wrapper.get('.product-toolbar .btn-primary').trigger('click')
    expect(wrapper.find('input[placeholder="frontend-dev"]').exists()).toBe(false)
    expect(wrapper.get('input[disabled][value="project-commerce-testing"]').exists()).toBe(true)
    await wrapper.get('input[placeholder="production"]').setValue('development')
    expect(wrapper.get('input[disabled][value="project-commerce-development"]').exists()).toBe(true)
    await wrapper.get('.modal form').trigger('submit')
    expect(api.post).toHaveBeenCalledWith('/projects/1/environments', {
      name: 'development', kind: 'testing', cluster_id: 1,
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
    const wrapper = mount(ProjectEnvironments, { props: { projectID: '1' }, global: { stubs: { Teleport: true, RouterLink: { template: '<a><slot /></a>' } } } })
    await new Promise(resolve => setTimeout(resolve, 0))

    await wrapper.get('[title="编辑环境"]').trigger('click')
    expect(wrapper.get('input[placeholder="production"]').attributes('pattern')).toBeUndefined()
    expect(wrapper.get('input[disabled][value="legacy-prod"]').exists()).toBe(true)
    await wrapper.get('.modal form').trigger('submit')
    expect(api.put).toHaveBeenCalledWith('/projects/1/environments/2', { name: '生产环境', kind: 'unclassified', cluster_id: 1 })
    wrapper.unmount()
  })

  it('cancels the active project read when the view unmounts', async () => {
    const { api } = await import('../../api/index.js')
    let signal
    api.get.mockImplementation((_path, options) => {
      signal ||= options?.signal
      return new Promise(() => {})
    })
    const wrapper = mount(ProjectEnvironments, { props: { projectID: '1' }, global: { stubs: { Teleport: true, RouterLink: { template: '<a><slot /></a>' } } } })
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
    const wrapper = mount(ProjectEnvironments, { props: { projectID: '1' }, global: { stubs: { Teleport: true, RouterLink: { template: '<a><slot /></a>' } } } })
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
    const wrapper = mount(ProjectEnvironments, { props: { projectID: '1' }, global: { stubs: { Teleport: true, RouterLink: { template: '<a><slot /></a>' } } } })
    await new Promise(resolve => setTimeout(resolve, 0))
    await wrapper.get('.product-toolbar .btn-primary').trigger('click')
    await wrapper.get('.modal form').trigger('submit')
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(wrapper.find('.modal').exists()).toBe(true)
    expect(wrapper.get('.error-notice').text()).toContain('命名空间已被占用')
    wrapper.unmount()
  })

  it('keeps environment routes and shows pagination for multiple pages', async () => {
    const { api } = await import('../../api/index.js')
    api.get.mockImplementation(path => {
      if (path === '/projects') return Promise.resolve([{ id: 1, name: 'commerce' }])
      if (path === '/projects/1/environments') return Promise.resolve(Array.from({ length: 11 }, (_, index) => ({ id: index + 2, name: 'testing-' + index, namespace: 'ns-' + index })))
      return Promise.resolve([])
    })
    const wrapper = mount(ProjectEnvironments, {
      props: { projectID: '1', embedded: true },
      global: { stubs: { Teleport: true, RouterLink: { name: 'RouterLink', props: ['to'], template: '<a><slot /></a>' } } },
    })
    await flushPromises()
    const link = wrapper.findAllComponents({ name: 'RouterLink' }).find(item => item.text() === 'testing-0')
    expect(link.props('to')).toEqual({ path: '/applications', query: { project_id: '1', environment_id: 2 } })
    expect(wrapper.find('[aria-label="项目环境分页"]').exists()).toBe(true)
    expect(wrapper.findAll('.data-table tbody tr')).toHaveLength(10)
    await wrapper.get('[aria-label="下一页"]').trigger('click')
    await flushPromises()
    expect(wrapper.findAll('.data-table tbody tr')).toHaveLength(1)
    expect(wrapper.get('.environment-link').text()).toBe('testing-10')
    await wrapper.get('select[aria-label="每页条数"]').setValue(20)
    await flushPromises()
    expect(wrapper.findAll('.data-table tbody tr')).toHaveLength(11)
    expect(wrapper.find('[aria-label="项目环境分页"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('requires confirmation after opening deletion from the environment editor', async () => {
    const { api } = await import('../../api/index.js')
    api.delete.mockClear()
    api.get.mockImplementation(path => {
      if (path === '/projects') return Promise.resolve([{ id: 1, name: 'commerce' }])
      if (path === '/projects/1/environments') return Promise.resolve([{ id: 2, name: 'testing', namespace: 'test' }])
      return Promise.resolve([])
    })
    const wrapper = mount(ProjectEnvironments, { props: { projectID: '1', embedded: true }, global: { stubs: { Teleport: true, RouterLink: { template: '<a><slot /></a>' } } } })
    await flushPromises()
    await wrapper.get('[title="编辑环境"]').trigger('click')
    await wrapper.get('.environment-delete').trigger('click')
    expect(wrapper.find('#project-environment-form').exists()).toBe(false)
    expect(wrapper.text()).toContain('确认删除环境“testing”？')
    expect(api.delete).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="confirm-dialog-cancel"]').trigger('click')
    expect(api.delete).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})

vi.mock('../../api/list-pages.js', async importOriginal => {
  const original = await importOriginal()
  const { api } = await import('../../api/index.js')
  const { listPageFixture } = await import('../../test-support/list-page-fixture.js')
  return { ...original, getListPage: async (path, query, options) => {
    const filters = new URLSearchParams()
    for (const key of ['environment_id', 'unassigned']) if (query[key]) filters.set(key, query[key])
    const url = path + (filters.size ? '?' + filters : '')
    return listPageFixture(await api.get(url, options), query)
  } }
})
