import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick, reactive } from 'vue'
import Applications from './Applications.vue'

vi.mock('../../api/index.js', () => ({
  api: { get: vi.fn().mockResolvedValue([]), post: vi.fn().mockResolvedValue({ id: 1 }), delete: vi.fn().mockResolvedValue({}) }
}))
const route = reactive({ query: { project_id: '1', environment_id: '2' } })
vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }), useRoute: () => route }))

beforeEach(async () => {
  const { api } = await import('../../api/index.js')
  route.query.project_id = '1'
  route.query.environment_id = '2'
  api.get.mockReset()
  api.get.mockResolvedValue([])
  api.post.mockReset()
  api.post.mockResolvedValue({ id: 1 })
  api.delete.mockReset()
  api.delete.mockResolvedValue({})
})

describe('Applications view', () => {
  it('switches workspace tabs across runtime, releases, configuration, and access', async () => {
    const { api } = await import('../../api/index.js')
    api.get.mockImplementation(path => {
      if (path === '/projects') return Promise.resolve([{ id: 1, name: 'commerce', environments: [{ id: 2, name: 'production', cluster_id: 7, namespace: 'commerce-prod' }] }])
      if (path.startsWith('/workspace/overview')) return Promise.resolve({
        applications: [{ id: 3, name: 'order-api', runtime: { status: 'running', ready_pods: 1, total_pods: 1 }, endpoints: [{ domain: 'api.example.com', path: '/', service_port: 80, tls_enabled: true }, { domain: 'record.example.com', path: '/', service_port: 80, ingress_enabled: false }] }],
        releases: [{ id: 10, application_id: 3, application_name: 'order-api', version: '1.0.0', image: 'orders:1.0.0', status: 'succeeded', created_at: '2026-10-10T00:00:00Z' }],
        recent_releases: [],
      })
      if (path === '/applications/3/deployment-templates') return Promise.resolve([{ id: 4, name: '生产模板', is_default: true, default_version: '1.0.0', spec: { image: 'orders', replicas: 2 } }])
      return Promise.resolve([])
    })
    const wrapper = mount(Applications)
    await flushPromises()

    expect(wrapper.find('.overview-metrics').exists()).toBe(false)
    expect(wrapper.findAll('[data-testid="workspace-list"]')).toHaveLength(1)
    expect(wrapper.get('[data-testid="workspace-tab-runtime"]').attributes('aria-current')).toBe('page')
    expect(wrapper.text()).toContain('组件就绪 1/1')
    expect(api.get).not.toHaveBeenCalledWith('/applications/3/deployment-templates', expect.anything())
    await wrapper.get('[data-testid="workspace-tab-releases"]').trigger('click')
    expect(wrapper.findAll('[data-testid="workspace-list"]')).toHaveLength(1)
    expect(wrapper.get('.workspace-count').text()).toBe('1 条记录')
    expect(wrapper.text()).toContain('orders:1.0.0')
    await wrapper.get('[data-testid="workspace-tab-config"]').trigger('click')
    await flushPromises()
    expect(wrapper.findAll('[data-testid="workspace-list"]')).toHaveLength(1)
    expect(wrapper.text()).toContain('生产模板')
    await wrapper.get('[data-testid="workspace-tab-access"]').trigger('click')
    expect(wrapper.findAll('[data-testid="workspace-list"]')).toHaveLength(1)
    expect(wrapper.text()).toContain('api.example.com')
    expect(wrapper.text()).toContain('仅记录域名')
    expect(wrapper.text()).toContain('启用 HTTP Ingress')
    wrapper.unmount()
  })

  it('shows a dismissible dialog when deleting an associated project fails', async () => {
    const { api } = await import('../../api/index.js')
    api.get.mockImplementation(path => path === '/projects' ? Promise.resolve([{ id: 1, name: '测试', slug: 'test', environments: [{ id: 2, name: 'production' }] }]) : Promise.resolve([]))
    api.delete.mockRejectedValueOnce(new Error('项目仍关联环境或应用，无法删除'))
    const wrapper = mount(Applications, { props: { section: 'projects' } })
    await flushPromises()

    await wrapper.get('[title="删除项目"]').trigger('click')
    await wrapper.get('[data-testid="confirm-dialog-confirm"]').trigger('click')
    await flushPromises()

    expect(document.body.querySelector('.error-notice')?.textContent).toContain('项目仍关联环境或应用，无法删除')
    expect(wrapper.find('[data-testid="applications-mutation-error"]').exists()).toBe(false)
    document.body.querySelector('[data-testid="dismiss-error-notice"]').click()
    await nextTick()
    expect(document.body.querySelector('.error-notice')).toBeNull()
    expect(wrapper.find('[data-testid="confirm-dialog-confirm"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('requires a project English name when creating a project', async () => {
    const { api } = await import('../../api/index.js')
    const wrapper = mount(Applications, { props: { section: 'projects' } })
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text().includes('新建项目')).trigger('click')
    await wrapper.get('input[placeholder="commerce"]').setValue('commerce')
    await wrapper.get('.modal input[title="仅支持小写英文字母、数字和连字符，最长 40 位"]').setValue('shop')
    await wrapper.get('.modal form').trigger('submit')
    await flushPromises()

    expect(api.post).toHaveBeenCalledWith('/projects', { name: 'commerce', slug: 'shop', description: '' })
    wrapper.unmount()
  })

  it('asks to bind legacy environments before loading cluster runtime', async () => {
    const { api } = await import('../../api/index.js')
    api.get.mockImplementation(path => {
      if (path === '/projects') return Promise.resolve([{ id: 1, name: 'commerce', environments: [{ id: 2, name: 'production', cluster_id: 0, namespace: 'commerce-prod' }] }])
      return Promise.resolve([])
    })
    const wrapper = mount(Applications)
    await flushPromises()

    expect(wrapper.text()).toContain('当前环境尚未绑定集群')
    expect(api.get).not.toHaveBeenCalledWith('/workspace/overview?project_id=1&environment_id=2', expect.anything())
    wrapper.unmount()
  })

  it('deletes an unreleased deployment component after confirmation', async () => {
    const { api } = await import('../../api/index.js')
    api.get.mockImplementation(path => {
      if (path === '/projects') return Promise.resolve([{ id: 1, name: '测试', environments: [{ id: 2, name: '开发', cluster_id: 7, namespace: 'test-dev' }] }])
      if (path.startsWith('/workspace/overview')) return Promise.resolve({ applications: [{ id: 9, name: 'order-api', runtime: { status: 'not_released' } }], domains: [], recent_releases: [] })
      return Promise.resolve([])
    })
    const wrapper = mount(Applications)
    await flushPromises()
    await wrapper.get('[title="删除部署组件 order-api"]').trigger('click')
    expect(wrapper.text()).toContain('其上线模板和商城导入关联也会删除')
    await wrapper.get('[data-testid="confirm-dialog-confirm"]').trigger('click')
    await flushPromises()
    expect(api.delete).toHaveBeenCalledWith('/applications/9')
    wrapper.unmount()
  })

  it('keeps the latest workspace data when the route context changes during a request', async () => {
    let resolveFirstWorkspace
    const firstWorkspace = new Promise(resolve => { resolveFirstWorkspace = resolve })
    const { api } = await import('../../api/index.js')
    route.query.project_id = '1'
    route.query.environment_id = '2'
    api.get.mockImplementation(path => {
      if (path === '/projects') return Promise.resolve([
        { id: 1, name: 'first-project', environments: [{ id: 2, name: 'first-env' }] },
        { id: 3, name: 'second-project', environments: [{ id: 4, name: 'second-env' }] },
      ])
      if (path === '/workspace/overview?project_id=1&environment_id=2') return firstWorkspace
      if (path === '/workspace/overview?project_id=3&environment_id=4') {
        return Promise.resolve({ applications: [{ id: 2, name: 'new-api' }], domains: [], recent_releases: [] })
      }
      return Promise.resolve([])
    })
    const wrapper = mount(Applications)
    await flushPromises()

    route.query.project_id = '3'
    route.query.environment_id = '4'
    await nextTick()
    await flushPromises()
    resolveFirstWorkspace({ applications: [{ id: 1, name: 'old-api' }], domains: [], recent_releases: [] })
    await flushPromises()

    expect(wrapper.text()).toContain('new-api')
    expect(wrapper.text()).not.toContain('old-api')
    wrapper.unmount()
    route.query.project_id = '1'
    route.query.environment_id = '2'
  })

  it('shows the workspace controls and empty state inside one list card', async () => {
    const wrapper = mount(Applications)
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(wrapper.find('.page-title').text()).toBe('工作台')
    expect(wrapper.text()).not.toContain('暂无应用')
    expect(wrapper.text()).not.toContain('创建应用')
    expect(wrapper.findAll('[data-testid="workspace-list"]')).toHaveLength(1)
    expect(wrapper.get('[data-testid="workspace-list"] .empty-state').text()).toContain('请选择项目与环境')
  })

  it('imports enabled catalog applications into the selected workspace environment', async () => {
    const { api } = await import('../../api/index.js')
    api.get.mockImplementation(path => {
      if (path === '/projects') return Promise.resolve([{ id: 1, name: 'commerce', environments: [{ id: 2, name: 'production', cluster_id: 7, namespace: 'commerce-prod' }] }])
      if (path === '/workspace/overview?project_id=1&environment_id=2') return Promise.resolve({ applications: [], domains: [], recent_releases: [] })
      if (path === '/application-catalog') return Promise.resolve([
        { id: 10, name: '订单系统', enabled: true, components: [{ name: 'API', application_name: 'orders-api', spec: { container_port: 8080 } }] },
        { id: 11, name: '未启用', enabled: false, components: [] },
      ])
      return Promise.resolve([])
    })
    api.post.mockResolvedValue([{ components: [{}] }])
    const wrapper = mount(Applications)
    await flushPromises()

    const workspaceCard = wrapper.get('[data-testid="workspace-list"]')
    expect(workspaceCard.findAll('.workspace-toolbar-actions button').map(button => button.text())).toEqual(['从商城导入', '创建部署组件'])
    await wrapper.findAll('button').find(button => button.text() === '从商城导入').trigger('click')
    await flushPromises()
    const modal = document.body.querySelector('.workspace-import-modal')
    expect(modal.textContent).toContain('commerce · production · commerce-prod')
    expect(modal.textContent).toContain('订单系统')
    expect(modal.textContent).not.toContain('未启用')
    modal.querySelector('input[type="checkbox"]').click()
    await nextTick()
    modal.querySelector('.btn-primary').click()
    await flushPromises()

    expect(api.post).toHaveBeenCalledWith('/application-catalog/import', { project_id: 1, environment_id: 2, item_ids: [10] })
    expect(document.body.querySelector('.workspace-import-modal')).toBeNull()
    expect(wrapper.find('.feedback-banner--success').exists()).toBe(false)
    expect(document.body.querySelector('.error-notice')).toBeNull()
    wrapper.unmount()
  })

  it('blocks catalog import when a component name is already used in the environment', async () => {
    const { api } = await import('../../api/index.js')
    api.get.mockImplementation(path => {
      if (path === '/projects') return Promise.resolve([{ id: 1, name: 'commerce', environments: [{ id: 2, name: 'production', cluster_id: 7, namespace: 'commerce-prod' }] }])
      if (path === '/workspace/overview?project_id=1&environment_id=2') return Promise.resolve({ applications: [{ id: 3, name: 'orders-api' }], domains: [], recent_releases: [] })
      if (path === '/application-catalog') return Promise.resolve([{ id: 10, name: '订单系统', enabled: true, components: [{ name: 'API', application_name: 'orders-api', spec: { container_port: 8080 } }] }])
      return Promise.resolve([])
    })
    const wrapper = mount(Applications)
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === '从商城导入').trigger('click')
    await flushPromises()
    const modal = document.body.querySelector('.workspace-import-modal')
    modal.querySelector('input[type="checkbox"]').click()
    await nextTick()

    expect(modal.textContent).toContain('名称冲突')
    expect(modal.querySelector('.btn-primary').disabled).toBe(true)
    expect(api.post).not.toHaveBeenCalledWith('/application-catalog/import', expect.anything())
    wrapper.unmount()
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

  it('shows each project once and leaves environment details to the project page', async () => {
    const { api } = await import('../../api/index.js')
    api.get.mockImplementation((path) => {
      if (path === '/projects') {
        return Promise.resolve([{ id: 1, name: 'commerce', slug: 'commerce', default_image_registry: { name: 'hidden-default' }, environments: [
          { id: 2, name: 'production', cluster_id: 7, namespace: 'commerce-prod' },
          { id: 3, name: 'staging', cluster_id: 8, namespace: 'commerce-stage' },
        ] }])
      }
      if (path === '/clusters') return Promise.resolve([{ id: 7, name: '国内' }, { id: 8, name: '海外' }])
      return Promise.resolve([{ id: 3, project_id: 1, environment_id: 2, name: 'order-api' }])
    })
    const wrapper = mount(Applications, { props: { section: 'projects' } })
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(wrapper.findAll('th').map(th => th.text())).toEqual(['项目', '英文名', '说明', '已关联应用', '操作'])
    const rows = wrapper.findAll('tbody tr')
    expect(rows).toHaveLength(1)
    expect(rows[0].findAll('td').map(cell => cell.text())).toEqual(['commerce', 'commerce', '-', '1', '编辑管理环境删除'])
    expect(wrapper.text()).not.toContain('production')
    expect(wrapper.text()).not.toContain('commerce-prod')
    expect(wrapper.text()).not.toContain('hidden-default')
    expect(wrapper.text()).toContain('编辑')
    expect(wrapper.text()).toContain('删除')
    expect(wrapper.find('.btn-danger').attributes('disabled')).toBeUndefined()
  })

  it('uses the shared select menu for the workspace context', async () => {
    const { api } = await import('../../api/index.js')
    api.get.mockImplementation((path) => {
      if (path === '/projects') return Promise.resolve([{ id: 1, name: 'commerce', environments: [{ id: 2, name: 'production', namespace: 'project-commerce-prod' }] }])
      if (path === '/workspace/overview?project_id=1&environment_id=2') return Promise.resolve({ applications: [], domains: [], recent_releases: [], failed_releases: [] })
      return Promise.resolve([])
    })
    const wrapper = mount(Applications)
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(wrapper.findAll('.context-select')).toHaveLength(0)
    expect(wrapper.findAll('.workspace-picker')).toHaveLength(2)
    const [projectPicker, environmentPicker] = wrapper.findAll('.workspace-picker')
    expect(wrapper.get('[data-testid="workspace-list"] .workspace-toolbar').exists()).toBe(true)
    expect(projectPicker.get('.context-picker-label').text()).toBe('项目')
    expect(environmentPicker.get('.context-picker-label').text()).toBe('环境')
    expect(projectPicker.get('.select-menu-trigger').text()).toContain('commerce')
    expect(environmentPicker.get('.select-menu-trigger').text()).toContain('production')

    await projectPicker.get('.select-menu-trigger').trigger('click')
    const menu = document.body.querySelector('.select-menu-options[aria-label="项目"]')
    expect(menu.textContent).toContain('commerce')
    expect(menu.querySelector('.select-menu-option-description').textContent).toContain('未设置项目说明')
    expect(menu.querySelector('.select-menu-option.is-selected').textContent).toContain('commerce')
    wrapper.unmount()
  })

  it('prioritizes application runtime and opens its detail from the workspace', async () => {
    const { api } = await import('../../api/index.js')
    api.get.mockImplementation(path => {
      if (path === '/projects') return Promise.resolve([{ id: 1, name: 'commerce', environments: [{ id: 2, name: 'production', namespace: 'commerce-prod' }] }])
      if (path === '/workspace/overview?project_id=1&environment_id=2') return Promise.resolve({ applications: [{ id: 3, name: 'order-api', workload_kind: 'deployment', endpoint_url: 'https://api.example.com', endpoint_count: 2, runtime: { status: 'running', ready_pods: 1, total_pods: 1 }, active_release: { version: '1.4.0' }, latest_release: { status: 'succeeded', created_at: '2026-08-03T10:00:00Z' } }], domains: [], recent_releases: [{ id: 9, application_id: 3, application_name: 'order-api', sequence: 4, image: 'order-api:1.4.0', status: 'succeeded', created_at: '2026-08-03T10:00:00Z' }] })
      return Promise.resolve([])
    })
    const wrapper = mount(Applications)
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(wrapper.text()).toContain('运行中')
    expect(wrapper.text()).toContain('1.4.0')
    expect(wrapper.findAll('.workspace-applications-table th').map(header => header.text())).toEqual(['部署组件', '运行状态', 'Pod 就绪', '运行版本', '最近发布', '操作'])
    expect(wrapper.findAll('.workspace-applications-table tbody tr:first-child td').map(cell => cell.text()).slice(0, 3)).toEqual(['order-api', '运行中', '1/1'])
    expect(wrapper.text()).toContain('查看运行')
    expect(wrapper.text()).not.toContain('https://api.example.com')
  })

  it('loads selectable application templates for a release', async () => {
    const { api } = await import('../../api/index.js')
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
    const { api } = await import('../../api/index.js')
    api.get.mockImplementation((path) => {
      if (path === '/projects') return Promise.resolve([{ id: 1, name: 'commerce', environments: [{ id: 2, name: 'production', namespace: 'commerce-prod' }] }])
      if (path === '/workspace/overview?project_id=1&environment_id=2') return Promise.resolve({ applications: [{ id: 3, project_id: 1, environment_id: 2, name: 'order-api', project: { id: 1, name: 'commerce' }, environment: { name: 'production', namespace: 'commerce-prod' } }], domains: [], recent_releases: [], failed_releases: [] })
      if (path === '/applications/3/deployment-templates') return Promise.resolve([{ id: 8, name: '标准生产配置', enabled: true, is_default: true, revision: 2, spec: { image: 'harbor.example.com/commerce/order-api', replicas: 2, container_port: 8080, service: { port: 80 } } }])
      return Promise.resolve([])
    })
    api.post.mockImplementation(path => Promise.resolve(path.endsWith('/releases/preflight') ? { ready: true, checks: [{ name: '密钥', ok: true, detail: '已配置' }] } : { id: 9 }))
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
    await flushPromises()
    releaseModal.querySelectorAll('button').forEach(button => { if (button.textContent === '检查配置') button.click() })
    await flushPromises()
    await releaseModal.querySelector('form').dispatchEvent(new Event('submit'))
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(api.post).toHaveBeenCalledWith('/applications/3/releases', { template_id: 8, version: '1.2.3' })
    wrapper.unmount()
  })

  it('allows an empty version when the selected template has a default', async () => {
    const { api } = await import('../../api/index.js')
    api.get.mockImplementation(path => {
      if (path === '/projects') return Promise.resolve([{ id: 1, name: 'commerce', environments: [{ id: 2, name: 'production', namespace: 'commerce-prod' }] }])
      if (path === '/workspace/overview?project_id=1&environment_id=2') return Promise.resolve({ applications: [{ id: 3, name: 'order-api' }], domains: [], recent_releases: [] })
      if (path === '/applications/3/deployment-templates') return Promise.resolve([{ id: 8, name: '默认配置', enabled: true, is_default: true, default_version: '2.3.4', revision: 1, spec: { image: 'example.com/order-api', service: { port: 80 } } }])
      return Promise.resolve([])
    })
    api.post.mockImplementation(path => Promise.resolve(path.endsWith('/releases/preflight') ? { ready: true, checks: [] } : { id: 9 }))
    const wrapper = mount(Applications)
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '发布版本').trigger('click')
    await flushPromises()

    const modal = document.body.querySelector('.release-modal')
    expect(modal.textContent).toContain('留空则使用模板默认版本 2.3.4')
    expect(modal.querySelector('input[placeholder="2.3.4"]').required).toBe(false)
    modal.querySelectorAll('button').forEach(button => { if (button.textContent === '检查配置') button.click() })
    await flushPromises()
    modal.querySelector('form').dispatchEvent(new Event('submit'))
    await flushPromises()

    expect(api.post).toHaveBeenCalledWith('/applications/3/releases', { template_id: 8, version: '' })
    wrapper.unmount()
  })

  it('keeps domain binding out of the version release dialog', async () => {
    const { api } = await import('../../api/index.js')
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
