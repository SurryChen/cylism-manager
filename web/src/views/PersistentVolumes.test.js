import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import PersistentVolumes from './PersistentVolumes.vue'

vi.mock('../api/index.js', () => ({
  api: { get: vi.fn(), post: vi.fn(), delete: vi.fn() },
}))

function mockInventory(overrides = {}) {
  return path => {
    if (path === '/projects') return Promise.resolve([{ id: 1, name: 'knowledge', environments: [{ id: 2, name: 'production', namespace: 'project-knowledge-prod' }] }])
    if (path === '/k8s/namespace-names') return Promise.resolve([{ name: 'default' }, { name: 'project-knowledge-prod' }])
    if (path === '/k8s/persistent-volume-claims') return Promise.resolve(overrides.claims || [])
    if (path === '/k8s/persistent-volume-claims/usage') return Promise.resolve(overrides.usage || [])
    if (path === '/k8s/storage-classes') return Promise.resolve([{ name: 'local-path', is_default: true, volume_binding_mode: 'WaitForFirstConsumer' }])
    if (path === '/k8s/persistent-volume-migrations') return Promise.resolve([])
    if (path === '/nodes') return Promise.resolve([])
    if (path === '/servers') return Promise.resolve(overrides.servers || [])
    if (path.includes('/imports?')) return Promise.resolve([])
    return Promise.resolve([])
  }
}

async function settle() { await new Promise(resolve => setTimeout(resolve, 0)) }

describe('PersistentVolumes view', () => {
  it('uses the shared section title bar', () => {
    const wrapper = mount(PersistentVolumes)
    expect(wrapper.get('.section-page-header').find('h1').text()).toBe('存储卷')
    expect(wrapper.find('.section-page-header .page-subtitle').exists()).toBe(false)
  })

  it('loads the cluster PVC inventory without requiring an application environment', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(mockInventory({ claims: [{ name: 'manual-data', namespace: 'default', managed: false, phase: 'Bound' }] }))
    const wrapper = mount(PersistentVolumes)
    await settle()

    expect(api.get).toHaveBeenCalledWith('/k8s/persistent-volume-claims')
    expect(wrapper.text()).toContain('manual-data')
    expect(wrapper.text()).toContain('外部创建')
  })

  it('creates a PVC directly in the selected namespace', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(mockInventory())
    api.post.mockResolvedValue({})
    const wrapper = mount(PersistentVolumes)
    await settle()

    await wrapper.get('.btn-primary').trigger('click')
    await wrapper.get('[data-testid="storage-create-namespace"]').setValue('default')
    await wrapper.get('input[placeholder="karakeep-data"]').setValue('karakeep-data')
    await wrapper.get('[data-testid="storage-capacity-value"]').setValue('8')
    await wrapper.get('[data-testid="storage-capacity-unit"]').setValue('Gi')
    await wrapper.get('form').trigger('submit')

    expect(api.post).toHaveBeenCalledWith('/k8s/persistent-volume-claims', {
      namespace: 'default', name: 'karakeep-data', storage: '8Gi', storage_class_name: '',
    })
  })

  it('uses project and environment as optional filters', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(mockInventory({ claims: [
      { name: 'manual-data', namespace: 'default', managed: false, phase: 'Bound' },
      { name: 'app-data', namespace: 'project-knowledge-prod', managed: true, environment_id: 2, project_id: 1, project_name: 'knowledge', environment_name: 'production', phase: 'Bound' },
    ] }))
    const wrapper = mount(PersistentVolumes)
    await settle()

    expect(wrapper.text()).toContain('manual-data')
    await wrapper.get('[data-testid="storage-project-filter"]').setValue('1')
    await settle()
    expect(wrapper.text()).not.toContain('manual-data')
    expect(wrapper.text()).toContain('app-data')
    await wrapper.get('[data-testid="storage-environment-filter"]').setValue('2')
    await settle()
    expect(wrapper.text()).toContain('production')
  })

  it('keeps host-directory import limited to a managed environment claim', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(mockInventory({
      claims: [{ name: 'karakeep-data', namespace: 'project-knowledge-prod', managed: true, environment_id: 2, phase: 'Bound', is_local: true, bound_node: 'node-a', bound_node_display_name: '节点 A' }],
      servers: [{ id: 8, name: '历史数据服务器', host: '100.64.0.8', ssh_auth_type: 'key' }],
    }))
    api.post.mockResolvedValue({})
    const wrapper = mount(PersistentVolumes)
    await settle()

    await wrapper.get('[data-testid="open-directory-import"]').trigger('click')
    await wrapper.get('[data-testid="import-source-server"]').setValue('8')
    await wrapper.get('[data-testid="import-source-path"]').setValue('/data/legacy/karakeep')
    await wrapper.get('[data-testid="import-confirm-replace"]').setValue(true)
    await wrapper.get('.import-form').trigger('submit')

    expect(api.post).toHaveBeenCalledWith('/k8s/persistent-volume-claims/karakeep-data/imports', {
      environment_id: 2, source_server_id: 8, source_path: '/data/legacy/karakeep', confirm_data_replace: true,
    })
  })

  it('renders infrastructure PVCs as read-only monitoring resources', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(mockInventory({ claims: [{ name: 'cylism-victoria-metrics-data', namespace: 'monitoring', managed: true, owner_type: 'infrastructure', owner_name: 'VictoriaMetrics', read_only: true, phase: 'Bound' }] }))
    const wrapper = mount(PersistentVolumes)
    await settle()

    expect(wrapper.text()).toContain('基础设施')
    expect(wrapper.text()).toContain('VictoriaMetrics')
    expect(wrapper.text()).toContain('查看监控')
    expect(wrapper.find('[title="删除存储卷"]').exists()).toBe(false)
    expect(wrapper.get('.monitoring-link').classes()).toContain('monitoring-link')
  })

  it('loads local PVC usage without blocking the inventory', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(mockInventory({
      claims: [{ name: 'karakeep-data', namespace: 'project-knowledge-prod', phase: 'Bound', storage: '5Gi' }],
      usage: [{ namespace: 'project-knowledge-prod', name: 'karakeep-data', status: 'available', used_bytes: 1073741824, capacity_bytes: 5368709120 }],
    }))
    const wrapper = mount(PersistentVolumes)
    await settle()
    await settle()

    expect(api.get).toHaveBeenCalledWith('/k8s/persistent-volume-claims/usage')
    expect(wrapper.text()).toContain('1.0 GiB')
    expect(wrapper.text()).toContain('20% / 请求容量')
  })
})
