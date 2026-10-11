import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ApplicationCatalog from './ApplicationCatalog.vue'
import ApplicationCatalogDetail from './ApplicationCatalogDetail.vue'

const mocks = vi.hoisted(() => ({
  push: vi.fn(),
  create: vi.fn(),
  list: vi.fn(),
  projects: vi.fn(),
  get: vi.fn(),
  applications: vi.fn(),
  importItems: vi.fn(),
  importFromEnvironment: vi.fn(),
  update: vi.fn(),
  routeQuery: { project_id: '1', environment_id: '2' },
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: mocks.routeQuery, params: { catalogID: '7' } }),
  useRouter: () => ({ push: mocks.push }),
}))
vi.mock('../../api/applications.js', () => ({
  createApplicationCatalogItem: mocks.create,
  createApplicationCatalogFromEnvironment: mocks.importFromEnvironment,
  deleteApplicationCatalogItem: vi.fn(),
  getApplicationCatalog: mocks.list,
  getApplicationCatalogItem: mocks.get,
  getApplications: mocks.applications,
  getProjects: mocks.projects,
  importApplicationCatalogItems: mocks.importItems,
  updateApplicationCatalogItem: mocks.update,
}))

describe('ApplicationCatalog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.routeQuery.project_id = '1'
    mocks.routeQuery.environment_id = '2'
    mocks.list.mockResolvedValue([])
    mocks.projects.mockResolvedValue([])
    mocks.applications.mockResolvedValue([])
    mocks.importItems.mockResolvedValue([])
    mocks.importFromEnvironment.mockResolvedValue({ id: 10 })
    mocks.create.mockResolvedValue({ id: 7 })
    mocks.get.mockResolvedValue({ id: 7, name: '订单系统', slug: 'orders', description: '', revision: 1, enabled: false, entry_component: '', components: [] })
  })

  it('creates a draft from basic information and opens its detail page', async () => {
    const wrapper = mount(ApplicationCatalog, { global: { stubs: { Teleport: true } } })
    await flushPromises()
    await wrapper.get('.catalog-toolbar .btn-primary').trigger('click')
    expect(wrapper.text()).toContain('新增商城应用')
    expect(wrapper.text()).not.toContain('镜像路径')
    const fields = wrapper.findAll('.modal .form-input')
    await fields[0].setValue('订单系统')
    await fields[1].setValue('orders')
    await wrapper.get('.modal form').trigger('submit')
    await flushPromises()
    expect(mocks.create).toHaveBeenCalledWith({ name: '订单系统', slug: 'orders', description: '' })
    expect(mocks.push).toHaveBeenCalledWith('/applications/catalog/7')
  })

  it('keeps an empty draft disabled until components are configured', async () => {
    const wrapper = mount(ApplicationCatalogDetail, { global: { stubs: { Teleport: true } } })
    await flushPromises()
    expect(wrapper.text()).toContain('尚未配置部署组件')
    await wrapper.get('.detail-actions input[type="checkbox"]').setValue(true)
    await wrapper.get('.detail-form').trigger('submit')
    await flushPromises()
    expect(mocks.update).not.toHaveBeenCalled()
  })

  it('returns to the catalog list after saving', async () => {
    mocks.update.mockResolvedValue({ id: 7 })
    const wrapper = mount(ApplicationCatalogDetail, { global: { stubs: { Teleport: true } } })
    await flushPromises()
    await wrapper.get('.detail-form').trigger('submit')
    await flushPromises()

    expect(mocks.update).toHaveBeenCalledOnce()
    expect(mocks.push).toHaveBeenCalledWith('/applications/catalog')
    expect(wrapper.text()).not.toContain('商城应用配置已保存')
  })

  it('saves a default version for a catalog component', async () => {
    mocks.get.mockResolvedValue({ id: 7, name: '订单系统', slug: 'orders', description: '', revision: 1, enabled: false, entry_component: 'orders-api', components: [
      { name: 'API', application_name: 'orders-api', spec: { image: 'example.com/orders-api', container_port: 8080, replicas: 1, resources: { requests_cpu: '100m', requests_memory: '128Mi', limits_cpu: '500m', limits_memory: '512Mi' } } },
    ] })
    mocks.update.mockResolvedValue({ id: 7 })
    const wrapper = mount(ApplicationCatalogDetail)
    await flushPromises()

    await wrapper.get('input[placeholder="1.4.2"]').setValue('2.3.4')
    await wrapper.get('.detail-form').trigger('submit')
    await flushPromises()

    expect(mocks.update).toHaveBeenCalledWith(7, expect.objectContaining({ components: [expect.objectContaining({ default_version: '2.3.4' })] }))
    wrapper.unmount()
  })

  it('packages applications from the chosen environment as a catalog draft', async () => {
    mocks.list.mockResolvedValue([{ id: 7, name: 'karakeep', enabled: true, revision: 1, components: [] }])
    mocks.projects.mockResolvedValue([{ id: 1, name: '项目', environments: [{ id: 2, name: '开发', namespace: 'project-dev', cluster_id: 1 }] }])
    mocks.applications.mockResolvedValue([
      { id: 11, name: 'orders-api', workload_kind: 'deployment', default_deployment_template_id: 31 },
      { id: 12, name: 'orders-web', workload_kind: 'deployment', default_deployment_template_id: 32 },
    ])
    const wrapper = mount(ApplicationCatalog, { global: { stubs: { Teleport: true } } })
    await flushPromises()

    expect(wrapper.text()).not.toContain('添加到环境')
    await wrapper.findAll('.catalog-toolbar-actions .btn').find(button => button.text() === '导入应用').trigger('click')
    await flushPromises()
    expect(wrapper.findAll('.catalog-source-list .catalog-import-option')).toHaveLength(2)
    expect(wrapper.get('.catalog-source-list').text()).not.toContain('karakeep')
    await wrapper.findAll('.catalog-source-list input[type="checkbox"]')[0].setValue(true)
    expect(wrapper.vm.sourceSelectedIDs).toEqual([11])
    await wrapper.findAll('.catalog-source-list input[type="checkbox"]')[1].setValue(true)
    expect(wrapper.vm.sourceSelectedIDs).toEqual([11, 12])
    await wrapper.findAll('.catalog-source-fields .form-input')[0].setValue('订单系统')
    await wrapper.findAll('.catalog-source-fields .form-input')[1].setValue('orders')
    await wrapper.get('.catalog-source-fields .select-menu-native').setValue('11')
    expect(wrapper.get('.catalog-source-fields + .modal-actions .btn-primary').attributes('disabled')).toBeUndefined()
    await wrapper.get('.catalog-source-fields + .modal-actions .btn-primary').trigger('click')
    await flushPromises()

    expect(mocks.importFromEnvironment).toHaveBeenCalledWith({ project_id: 1, environment_id: 2, application_ids: [11, 12], entry_application_id: 11, name: '订单系统', slug: 'orders', description: '' })
    expect(mocks.push).toHaveBeenCalledWith('/applications/catalog/10')
  })
})
