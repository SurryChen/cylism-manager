import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { reactive } from 'vue'
import Applications from './Applications.vue'

vi.mock('../api/index.js', () => ({
  api: { get: vi.fn().mockResolvedValue([]), post: vi.fn().mockResolvedValue({ id: 1 }) }
}))
const route = reactive({ query: { project_id: '1', environment_id: '2' } })
vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }), useRoute: () => route }))

describe('Applications view', () => {
  it('keeps the application entry free of a list frame when there is no data', async () => {
    const wrapper = mount(Applications)
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(wrapper.find('.page-title').text()).toBe('工作台')
    expect(wrapper.text()).not.toContain('暂无应用')
    expect(wrapper.text()).not.toContain('创建应用')
    expect(wrapper.findAll('.page-actions > .btn')).toHaveLength(0)
    expect(wrapper.find('.card').exists()).toBe(false)
  })

  it('shows project and environment management when opened from its secondary navigation item', async () => {
    const wrapper = mount(Applications, { props: { section: 'projects' } })
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(wrapper.find('.page-title').text()).toBe('项目与环境')
    expect(wrapper.text()).toContain('新建项目')
    expect(wrapper.text()).not.toContain('暂无项目')
    expect(wrapper.text()).not.toContain('的环境')
    expect(wrapper.find('.card').exists()).toBe(false)
  })

  it('shows a cross-project global overview section', async () => {
    const wrapper = mount(Applications, { props: { section: 'overview' } })
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(wrapper.find('.page-title').text()).toBe('全局概览')
    expect(wrapper.text()).not.toContain('暂无发布记录')
    expect(wrapper.find('.card').exists()).toBe(false)
  })

  it('shows each project with its environments and namespaces', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation((path) => {
      if (path === '/projects') {
        return Promise.resolve([{ id: 1, name: 'commerce', environments: [{ id: 2, name: 'production', namespace: 'commerce-prod' }] }])
      }
      return Promise.resolve([{ id: 3, project_id: 1, environment_id: 2, name: 'order-api' }])
    })
    const wrapper = mount(Applications, { props: { section: 'projects' } })
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(wrapper.text()).toContain('production')
    expect(wrapper.text()).toContain('commerce-prod')
    expect(wrapper.text()).toContain('编辑')
    expect(wrapper.text()).toContain('删除')
    expect(wrapper.find('.btn-danger').attributes('disabled')).toBeUndefined()
  })

  it('uses styled project and environment menus for the workspace context', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation((path) => {
      if (path === '/projects') return Promise.resolve([{ id: 1, name: 'commerce', environments: [{ id: 2, name: 'production', namespace: 'project-commerce-prod' }] }])
      if (path === '/workspace/overview?project_id=1&environment_id=2') return Promise.resolve({ applications: [], domains: [], recent_releases: [], failed_releases: [] })
      return Promise.resolve([])
    })
    const wrapper = mount(Applications)
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(wrapper.findAll('.context-select')).toHaveLength(0)
    expect(wrapper.findAll('.workspace-picker')).toHaveLength(2)
    expect(wrapper.get('[data-testid="workspace-project-trigger"]').text()).toContain('commerce')
    expect(wrapper.get('[data-testid="workspace-environment-trigger"]').text()).toContain('production')

    await wrapper.get('[data-testid="workspace-project-trigger"]').trigger('click')
    expect(wrapper.get('[data-testid="workspace-project-menu"]').text()).toContain('commerce')
    expect(wrapper.get('[data-testid="workspace-project-menu"] .is-selected').text()).toContain('commerce')
  })

  it('shows application runtime summary and opens only the endpoint in a new tab', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(path => {
      if (path === '/projects') return Promise.resolve([{ id: 1, name: 'commerce', environments: [{ id: 2, name: 'production', namespace: 'commerce-prod' }] }])
      if (path === '/workspace/overview?project_id=1&environment_id=2') return Promise.resolve({ applications: [{ id: 3, name: 'order-api', workload_kind: 'deployment', endpoint_url: 'https://api.example.com', endpoint_count: 2, runtime: { status: 'running', ready_pods: 1, total_pods: 1 }, active_release: { version: '1.4.0' }, latest_release: { status: 'succeeded', created_at: '2026-08-03T10:00:00Z' } }], domains: [], recent_releases: [{ id: 9, application_id: 3, application_name: 'order-api', sequence: 4, image: 'order-api:1.4.0', status: 'succeeded', created_at: '2026-08-03T10:00:00Z' }] })
      return Promise.resolve([])
    })
    const wrapper = mount(Applications)
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(wrapper.text()).toContain('运行中')
    expect(wrapper.text()).toContain('1.4.0')
    const endpoint = wrapper.find('.endpoint-link')
    expect(endpoint.attributes('href')).toBe('https://api.example.com')
    expect(endpoint.attributes('target')).toBe('_blank')
    expect(wrapper.text()).toContain('另有 1 个地址')
    expect(wrapper.text()).toContain('2026')
  })

  it('loads selectable application templates for a release', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation((path) => {
      if (path === '/projects') return Promise.resolve([{ id: 1, name: 'commerce', environments: [{ id: 2, name: 'production', namespace: 'commerce-prod' }] }])
      if (path === '/workspace/overview?project_id=1&environment_id=2') return Promise.resolve({ applications: [{ id: 3, project_id: 1, environment_id: 2, name: 'order-api', project: { id: 1, name: 'commerce', default_image_registry_id: 12 }, environment: { name: 'production', namespace: 'commerce-prod' } }], domains: [], recent_releases: [], failed_releases: [] })
      if (path === '/applications/3/deployment-templates') return Promise.resolve([{ id: 8, name: '标准生产配置', enabled: true, is_default: true, revision: 1, spec: { image: 'harbor.example.com/commerce/order-api', replicas: 2, container_port: 8080, service: { port: 80 } } }])
      return Promise.resolve([])
    })
    const wrapper = mount(Applications)
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(wrapper.text()).toContain('commerce')
    await wrapper.findAll('button').find(button => button.text() === '发布版本').trigger('click')
    await new Promise(resolve => setTimeout(resolve, 0))

    const releaseModal = document.body.querySelector('.release-modal')
    expect(releaseModal).not.toBeNull()
    expect(releaseModal.textContent).toContain('标准生产配置')
    expect(releaseModal.querySelector('select').value).toBe('8')
    wrapper.unmount()
  })

  it('publishes an application with an existing template by version only', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation((path) => {
      if (path === '/projects') return Promise.resolve([{ id: 1, name: 'commerce', environments: [{ id: 2, name: 'production', namespace: 'commerce-prod' }] }])
      if (path === '/workspace/overview?project_id=1&environment_id=2') return Promise.resolve({ applications: [{ id: 3, project_id: 1, environment_id: 2, name: 'order-api', project: { id: 1, name: 'commerce' }, environment: { name: 'production', namespace: 'commerce-prod' } }], domains: [], recent_releases: [], failed_releases: [] })
      if (path === '/applications/3/deployment-templates') return Promise.resolve([{ id: 8, name: '标准生产配置', enabled: true, is_default: true, revision: 2, spec: { image: 'harbor.example.com/commerce/order-api', replicas: 2, container_port: 8080, service: { port: 80 } } }])
      return Promise.resolve([])
    })
    api.post.mockResolvedValue({ id: 9 })
    const wrapper = mount(Applications)
    await new Promise(resolve => setTimeout(resolve, 0))
    await wrapper.findAll('button').find(button => button.text() === '发布版本').trigger('click')
    await new Promise(resolve => setTimeout(resolve, 0))

    const releaseModal = document.body.querySelector('.release-modal')
    expect(releaseModal.textContent).toContain('模板 v2')
    expect(releaseModal.querySelector('input[type="number"]')).toBeNull()
    const version = releaseModal.querySelector('input[placeholder="1.4.2"]')
    version.value = '1.2.3'
    version.dispatchEvent(new Event('input'))
    await releaseModal.querySelector('form').dispatchEvent(new Event('submit'))
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(api.post).toHaveBeenCalledWith('/applications/3/releases', { template_id: 8, version: '1.2.3' })
    wrapper.unmount()
  })

  it('keeps domain binding out of the version release dialog', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation((path) => {
      if (path === '/projects') return Promise.resolve([{ id: 1, name: 'commerce', environments: [{ id: 2, name: 'production', namespace: 'commerce-prod' }] }])
      if (path === '/workspace/overview?project_id=1&environment_id=2') return Promise.resolve({ applications: [{ id: 3, project_id: 1, environment_id: 2, name: 'order-api', project: { id: 1, name: 'commerce' }, environment: { name: 'production', namespace: 'commerce-prod' } }], domains: [], recent_releases: [], failed_releases: [] })
      if (path === '/applications/3/deployment-templates') return Promise.resolve([{ id: 8, name: '标准生产配置', enabled: true, is_default: true, revision: 1, spec: { image: 'harbor.example.com/commerce/order-api', replicas: 1, container_port: 8080, service: { port: 80 } } }])
      return Promise.resolve([])
    })
    const wrapper = mount(Applications)
    await new Promise(resolve => setTimeout(resolve, 0))
    await wrapper.findAll('button').find(button => button.text() === '发布版本').trigger('click')
    await new Promise(resolve => setTimeout(resolve, 0))
    const modalText = document.body.querySelector('.release-modal').textContent
    expect(modalText).not.toContain('公网域名')
    expect(modalText).not.toContain('受管域名')
    expect(modalText).not.toContain('HTTPS')
    wrapper.unmount()
  })
})
