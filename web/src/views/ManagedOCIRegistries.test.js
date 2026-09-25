import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import ManagedOCIRegistries from './ManagedOCIRegistries.vue'

vi.mock('../api/index.js', () => ({ api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() } }))

afterEach(async () => {
  await new Promise(resolve => setTimeout(resolve, 0))
  vi.clearAllMocks()
})

function registryRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/cloud-services/registry', component: ManagedOCIRegistries },
    ],
  })
}

async function mountRegistryAt(path) {
  const router = registryRouter()
  await router.push(path)
  await router.isReady()
  return { router, wrapper: mount(ManagedOCIRegistries, { global: { plugins: [router] } }) }
}

function mountRegistry() {
  return mount(ManagedOCIRegistries, { global: { plugins: [registryRouter()] } })
}

describe('ManagedOCIRegistries view', () => {
  it('shows the deployment action before a registry exists', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(path => Promise.resolve(path === '/servers' ? [] : path === '/managed-oci-registries/storage-preflight' ? { ready: true, storage_class_name: 'local-path', storage_classes: ['local-path'], data_nodes: ['node-a'] } : path === '/managed-oci-registries/pvcs' ? [{ name: 'registry-data', namespace: 'cylism-system', storage: '100Gi', storage_class_name: 'local-path', phase: 'Pending', access_modes: ['ReadWriteOnce'] }] : []))
    const wrapper = mountRegistry()
    await new Promise(resolve => setTimeout(resolve, 0))
    const emptyRegistry = wrapper.get('[data-testid="registry-empty"]')
    expect(emptyRegistry.findComponent({ name: 'SurfaceCard' }).exists()).toBe(true)
    expect(emptyRegistry.classes()).toContain('surface-card')
    expect(emptyRegistry.text()).toContain('尚未部署自托管制品库')
    expect(wrapper.text()).not.toContain('由 Cylism 在集群中部署和管理')
    expect(wrapper.findAll('[data-testid="deploy-registry"]')).toHaveLength(1)
    expect(wrapper.text()).not.toContain('交付中心')
    expect(api.get).not.toHaveBeenCalledWith('/managed-oci-registries/storage-preflight', expect.anything())
    expect(api.get).not.toHaveBeenCalledWith('/managed-oci-registries/pvcs', expect.anything())
    await wrapper.get('[data-testid="deploy-registry"]').trigger('click')
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(document.body.querySelector('.registry-modal')).not.toBeNull()
    expect(document.body.querySelector('.registry-modal').textContent).toContain('local-path')
    expect(document.body.querySelector('.registry-pvc-select').textContent).toContain('registry-data')
    expect(document.body.querySelector('input[readonly]').value).toBe('cylism-system')
    wrapper.unmount()
  })

  it('opens Registry Proxy management from its canonical cloud services workspace query URL', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(path => Promise.resolve(path === '/registry-proxies' ? [{ id: 2, name: 'Kubernetes Registry 代理', registry: 'registry.k8s.io', upstream_url: 'https://registry.k8s.io', endpoint_host: '100.64.0.8', node_port: 30501, node_name: 'worker-a', cache_limit_gi: 2, cleanup_interval_hours: 24, status: 'ready' }] : path === '/servers' ? [] : path === '/managed-oci-registries/storage-preflight' ? { ready: true, data_nodes: [] } : []))
    const { router, wrapper } = await mountRegistryAt('/cloud-services/registry?tab=registry-proxy')
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(wrapper.text()).toContain('Kubernetes Registry 代理')
    expect(router.currentRoute.value.fullPath).toBe('/cloud-services/registry?tab=registry-proxy')
    expect(api.get).toHaveBeenCalledWith('/registry-proxies', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(api.get).not.toHaveBeenCalledWith('/managed-oci-registries', expect.anything())
    wrapper.unmount()
  })

  it('navigates tabs through their canonical cloud services workspace paths', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockResolvedValue([])
    const { router, wrapper } = await mountRegistryAt('/cloud-services/registry?tab=registry')
    await new Promise(resolve => setTimeout(resolve, 0))

    await wrapper.get('[data-testid="managed-registry-workspace-registry-proxy"]').trigger('click')
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(router.currentRoute.value.fullPath).toBe('/cloud-services/registry?tab=registry-proxy')
    wrapper.unmount()
  })

  it('restores the self-hosted Registry workspace through browser history', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockResolvedValue([])
    const { router, wrapper } = await mountRegistryAt('/cloud-services/registry?tab=registry')
    await new Promise(resolve => setTimeout(resolve, 0))

    await wrapper.get('[data-testid="managed-registry-workspace-registry-proxy"]').trigger('click')
    await new Promise(resolve => setTimeout(resolve, 0))
    router.back()
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(router.currentRoute.value.fullPath).toBe('/cloud-services/registry?tab=registry')
    expect(wrapper.get('[data-testid="managed-registry-workspace-registry"]').classes()).toContain('is-active')
    wrapper.unmount()
  })

  it('offers known Kubernetes nodes when deploying a registry', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(path => Promise.resolve(path === '/servers' ? [{ id: 7, name: 'worker-1', host: '10.0.0.7', cluster_role: 'worker', k8s_node_name: 'worker-1.cluster.local' }] : path === '/managed-oci-registries/storage-preflight' ? { ready: true, storage_class_name: 'local-path', storage_classes: ['local-path'], data_nodes: ['node-a'] } : path === '/managed-oci-registries/pvcs' ? [{ name: 'registry-data', namespace: 'cylism-system', storage: '100Gi', storage_class_name: 'local-path', phase: 'Pending', access_modes: ['ReadWriteOnce'] }] : []))
    const wrapper = mountRegistry()
    await new Promise(resolve => setTimeout(resolve, 0))
    await wrapper.get('[data-testid="deploy-registry"]').trigger('click')
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(document.body.querySelector('.registry-data-node-select').textContent).toContain('node-a')
    expect(document.body.querySelector('.registry-pvc-select').textContent).toContain('local-path')
    wrapper.unmount()
  })

  it('blocks deployment when local-path storage is not ready', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(path => Promise.resolve(path === '/servers' ? [] : path === '/managed-oci-registries/storage-preflight' ? { ready: false, storage_class_name: 'local-path', storage_classes: ['local-path'], data_nodes: ['node-a'], message: 'StorageClass "local-path" 必须使用 WaitForFirstConsumer' } : path === '/managed-oci-registries/pvcs' ? [{ name: 'registry-data', namespace: 'cylism-system', storage: '100Gi', storage_class_name: 'local-path', phase: 'Pending', access_modes: ['ReadWriteOnce'] }] : []))
    const wrapper = mountRegistry()
    await new Promise(resolve => setTimeout(resolve, 0))
    await wrapper.get('[data-testid="deploy-registry"]').trigger('click')
    await new Promise(resolve => setTimeout(resolve, 0))
    const modal = document.body.querySelector('.registry-modal')
    expect([...modal.querySelectorAll('button')].find(button => button.textContent.includes('开始部署')).disabled).toBe(true)
    wrapper.unmount()
  })

  it('lists ready certificates before an endpoint is entered and fills the certificate domain', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(path => Promise.resolve(path === '/servers' ? [] : path === '/managed-oci-registries/storage-preflight' ? { ready: true, storage_class_name: 'local-path', storage_classes: ['local-path'], data_nodes: ['node-a'] } : path === '/managed-oci-registries/pvcs' ? [{ name: 'registry-data', namespace: 'cylism-system', storage: '100Gi', storage_class_name: 'local-path', phase: 'Pending', access_modes: ['ReadWriteOnce'] }] : path.startsWith('/managed-oci-registries/certificates?') ? [{ name: 'registry-cert', namespace: 'cylism-system', domains: ['registry.internal'] }] : []))
    const wrapper = mountRegistry()
    await new Promise(resolve => setTimeout(resolve, 0))
    await wrapper.get('[data-testid="deploy-registry"]').trigger('click')
    await new Promise(resolve => setTimeout(resolve, 0))
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(api.get).toHaveBeenCalledWith('/managed-oci-registries/certificates?namespace=cylism-system', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    const certificateSelect = document.body.querySelector('.registry-certificate-select .select-menu-native')
    expect(certificateSelect.textContent).toContain('registry-cert')

    certificateSelect.value = 'registry-cert'
    certificateSelect.dispatchEvent(new Event('change', { bubbles: true }))
    await new Promise(resolve => setTimeout(resolve, 0))
    const endpoint = document.body.querySelector('input[placeholder="registry.example.com:31813"]')
    expect(endpoint.value).toBe('registry.internal')
    expect(document.body.querySelector('input[placeholder="registry-tls"]')).toBeNull()
    wrapper.unmount()
  })

  it('marks an HTTP Registry as insecure without showing a credential', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(path => Promise.resolve(path === '/servers' ? [{ id: 7, name: 'worker-1', host: '10.0.0.7', cluster_role: 'worker' }] : path === '/managed-oci-registries/storage-preflight' ? { ready: true, storage_class_name: 'local-path', storage_classes: ['local-path'], data_nodes: ['node-a'] } : path === '/managed-oci-registries/pvcs' ? [{ name: 'registry-data', namespace: 'cylism-system', storage: '100Gi', storage_class_name: 'local-path', phase: 'Pending', access_modes: ['ReadWriteOnce'] }] : [{ id: 1, endpoint: 'registry.internal', insecure_http: true, status: 'degraded', data_node: 'worker-1', pvc_name: 'cylism-oci-registry-data', storage_size: '100Gi', registry_image: 'registry:2', namespace: 'cylism-system', pull_username: 'cylism-pull', node_registry_mirror_id: 2, last_error: '入口不可达' }]))
    const wrapper = mountRegistry()
    await new Promise(resolve => setTimeout(resolve, 0))
    await wrapper.get('[data-testid="open-registry-overview"]').trigger('click')
    const overview = wrapper.get('[data-testid="registry-overview-modal"]')
    expect(overview.text()).toContain('HTTP，凭据和镜像层以明文传输')
    expect(overview.text()).toContain('异常')
    expect(wrapper.text()).not.toContain('registry-password')
    wrapper.unmount()
  })

  it('moves infrastructure status into an on-demand overview modal while keeping configuration in the header', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(path => Promise.resolve(path === '/servers' ? [{ id: 7, name: 'worker-1', host: '10.0.0.7', cluster_role: 'worker', k8s_node_name: 'node-a' }] : path === '/managed-oci-registries/storage-preflight' ? { ready: true, storage_class_name: 'local-path', storage_classes: ['local-path'], data_nodes: ['node-a'] } : path === '/managed-oci-registries/pvcs' ? [{ name: 'registry-data', namespace: 'cylism-system', storage: '10Gi', storage_class_name: 'local-path', phase: 'Bound', access_modes: ['ReadWriteOnce'] }] : [{ id: 1, endpoint: 'registry.internal', status: 'ready', data_node: 'node-a', pvc_name: 'registry-data', storage_size: '10Gi', storage_class_name: 'local-path', registry_image: 'registry:2', namespace: 'cylism-system', pull_username: 'cylism-pull', certificate_name: 'registry-cert', node_registry_mirror_id: 2 }]))
    const wrapper = mountRegistry()
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(wrapper.find('[data-testid="registry-overview-section"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="registry-summary"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="open-registry-overview"]').text()).toContain('查看运行概览')
    expect(wrapper.get('[data-testid="registry-catalog"]').get('[data-testid="open-registry-overview"]').exists()).toBe(true)
    expect(wrapper.find('.section-tabs-actions [data-testid="open-registry-overview"]').exists()).toBe(false)
    await wrapper.get('[data-testid="open-registry-overview"]').trigger('click')
    const overview = wrapper.get('[data-testid="registry-overview-modal"]')
    expect(overview.text()).toContain('访问地址')
    expect(overview.text()).toContain('registry.internal')
    expect(overview.text()).toContain('运行状态')
    expect(overview.text()).toContain('持久化存储')
    expect(wrapper.get('[data-testid="edit-registry"]').text()).toContain('配置')
    expect(wrapper.text()).not.toContain('节点镜像配置')
    expect(wrapper.find('[data-testid="registry-node-table"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('repairs an unhealthy Registry through the platform endpoint', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(path => Promise.resolve(path === '/servers' ? [] : path === '/managed-oci-registries/storage-preflight' ? { ready: true, storage_class_name: 'local-path', storage_classes: ['local-path'], data_nodes: ['node-a'] } : path === '/managed-oci-registries/pvcs' ? [{ name: 'registry-data', namespace: 'cylism-system', storage: '10Gi', storage_class_name: 'local-path', phase: 'Bound', access_modes: ['ReadWriteOnce'] }] : [{ id: 1, endpoint: 'registry.internal', status: 'degraded', data_node: 'node-a', pvc_name: 'registry-data', storage_size: '10Gi', registry_image: 'registry:2', namespace: 'cylism-system', pull_username: 'cylism-pull', last_error: 'Registry 尚未就绪' }]))
    api.post.mockResolvedValueOnce({ id: 1, status: 'deploying' })
    const wrapper = mountRegistry()
    await new Promise(resolve => setTimeout(resolve, 0))
    await wrapper.get('[data-testid="repair-registry"]').trigger('click')
    expect(api.post).toHaveBeenCalledWith('/managed-oci-registries/1/repair')
    wrapper.unmount()
  })

  it('shows an API error without rendering a registry form', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockRejectedValueOnce(new Error('无法读取集群状态'))
    const wrapper = mountRegistry()
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(wrapper.get('[role="alert"]').text()).toContain('无法读取集群状态')
    expect(wrapper.find('[data-testid="registry-empty"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('shows a modal and keeps the deployment form open when deployment fails', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(path => Promise.resolve(path === '/servers' ? [] : path === '/managed-oci-registries/storage-preflight' ? { ready: true, storage_class_name: 'local-path', storage_classes: ['local-path'], data_nodes: ['node-a'] } : path === '/managed-oci-registries/pvcs' ? [{ name: 'registry-data', namespace: 'cylism-system', storage: '100Gi', storage_class_name: 'local-path', phase: 'Pending', access_modes: ['ReadWriteOnce'] }] : []))
    api.post.mockRejectedValueOnce(new Error('保存制品库配置失败，请检查名称、地址和项目授权'))
    const wrapper = mountRegistry()
    await new Promise(resolve => setTimeout(resolve, 0))
    await wrapper.get('[data-testid="deploy-registry"]').trigger('click')
    document.body.querySelector('.registry-modal form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await new Promise(resolve => setTimeout(resolve, 0))

    const notice = document.body.querySelector('.registry-notice-modal')
    expect(notice).not.toBeNull()
    expect(notice.textContent).toContain('部署失败')
    expect(notice.textContent).toContain('保存制品库配置失败，请检查名称、地址和项目授权')
    expect(document.body.querySelector('.registry-modal')).not.toBeNull()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('edits the verification image from the managed registry form', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(path => Promise.resolve(path === '/servers' ? [] : path === '/managed-oci-registries/storage-preflight' ? { ready: true, storage_class_name: 'local-path', data_nodes: ['node-a'] } : path === '/managed-oci-registries/pvcs' ? [{ name: 'registry-data', namespace: 'cylism-system', storage: '10Gi', storage_class_name: 'local-path', phase: 'Bound', access_modes: ['ReadWriteOnce'] }] : [{ id: 1, endpoint: 'registry.internal:5443', verification_image: 'registry.internal:5443/cylism-manager:1.0.0', status: 'ready', data_node: 'node-a', pvc_name: 'registry-data', storage_size: '10Gi', registry_image: 'registry:2', namespace: 'cylism-system', pull_username: 'cylism-pull', certificate_name: 'registry-cert' }]))
    const wrapper = mountRegistry()
    await new Promise(resolve => setTimeout(resolve, 0))
    await wrapper.get('[data-testid="edit-registry"]').trigger('click')
    await new Promise(resolve => setTimeout(resolve, 0))
    const input = document.body.querySelector('input[placeholder="registry.internal:5443/cylism-manager:1.0.0"]')
    expect(input).not.toBeNull()
    expect(input.value).toBe('registry.internal:5443/cylism-manager:1.0.0')
    document.body.querySelector('.registry-modal form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(api.put).toHaveBeenCalledWith('/managed-oci-registries/1', expect.objectContaining({ verification_image: 'registry.internal:5443/cylism-manager:1.0.0' }))
    wrapper.unmount()
  })

  it('flattens repository tags into one catalog table without a master-detail split', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(path => Promise.resolve(
      path === '/managed-oci-registries/storage-preflight' ? { ready: true, storage_class_name: 'local-path', data_nodes: ['node-a'] }
        : path === '/managed-oci-registries/pvcs' ? []
          : path === '/managed-oci-registries/1/catalog' ? { repositories: ['team/orders'], next: '' }
            : path === '/managed-oci-registries/1/catalog/tags?repository=team%2Forders' ? { repository: 'team/orders', tags: [{ name: 'v1', digest: 'sha256:12a24104d303c4c2c6a384a0d662d7ef', pull_reference: 'team/orders:v1', platforms: ['linux/amd64'] }], next: '' }
              : [{ id: 1, endpoint: 'registry.internal:5443', status: 'ready', data_node: 'node-a', pvc_name: 'registry-data', storage_size: '10Gi', registry_image: 'registry:2', namespace: 'cylism-system', pull_username: 'cylism-pull' }]
    ))
    const wrapper = mountRegistry()
    await new Promise(resolve => setTimeout(resolve, 0))
    await new Promise(resolve => setTimeout(resolve, 0))
    const catalog = wrapper.get('[data-testid="registry-catalog"]')
    expect(catalog.find('h2').exists()).toBe(false)
    expect(catalog.get('.registry-catalog-toolbar').exists()).toBe(true)
    expect(catalog.get('.registry-search-field').exists()).toBe(true)
    expect(catalog.text()).toContain('team/orders')
    expect(wrapper.find('[data-testid="managed-registry-tab-images"]').exists()).toBe(false)
    expect(catalog.find('.registry-catalog-grid').exists()).toBe(false)
    expect(wrapper.get('.registry-catalog-count').text()).toBe('1 个仓库')
    expect(catalog.get('.registry-search').attributes('aria-label')).toBe('搜索仓库')
    expect(catalog.findAll('.registry-flat-row')).toHaveLength(1)
    expect(catalog.get('thead').text()).toContain('仓库')
    expect(catalog.get('thead').text()).toContain('标签')
    expect(catalog.get('thead').text()).toContain('摘要')
    expect(catalog.get('thead').text()).toContain('操作')
    expect(wrapper.text()).toContain('v1')
    const digest = 'sha256:12a24104d303c4c2c6a384a0d662d7ef'
    const digestCell = catalog.get('.registry-digest')
    expect(digestCell.attributes('aria-label')).toBe(digest)
    await digestCell.trigger('mouseenter', { clientX: 80, clientY: 120 })
    expect(document.body.querySelector('.registry-digest-tooltip')?.textContent).toBe(digest)
    await digestCell.trigger('mouseleave')
    expect(document.body.querySelector('.registry-digest-tooltip')).toBeNull()
    const writeText = vi.fn().mockResolvedValue()
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
    await catalog.get(`[aria-label="复制摘要 ${digest}"]`).trigger('click')
    expect(writeText).toHaveBeenCalledWith(digest)
    expect(catalog.get('[aria-label="复制 team/orders:v1"]').exists()).toBe(true)
    expect(catalog.get('[aria-label="删除标签 v1"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('refreshes the flattened catalog after deleting a tag', async () => {
    const { api } = await import('../api/index.js')
    let catalogReads = 0
    api.get.mockImplementation(path => {
      if (path === '/managed-oci-registries/storage-preflight') return Promise.resolve({ ready: true, storage_class_name: 'local-path', data_nodes: ['node-a'] })
      if (path === '/managed-oci-registries/pvcs') return Promise.resolve([])
      if (path === '/managed-oci-registries/1/catalog') { catalogReads += 1; return Promise.resolve({ repositories: ['team/orders', 'team/payments'], next: '' }) }
      if (path === '/managed-oci-registries/1/catalog/tags?repository=team%2Forders') return Promise.resolve({ repository: 'team/orders', tags: catalogReads === 1 ? [{ name: 'v1', pull_reference: 'team/orders:v1' }] : [], next: '' })
      if (path === '/managed-oci-registries/1/catalog/tags?repository=team%2Fpayments') return Promise.resolve({ repository: 'team/payments', tags: [{ name: 'v2', pull_reference: 'team/payments:v2' }], next: '' })
      return Promise.resolve([{ id: 1, endpoint: 'registry.internal:5443', status: 'ready', data_node: 'node-a', pvc_name: 'registry-data', storage_size: '10Gi', registry_image: 'registry:2', namespace: 'cylism-system', pull_username: 'cylism-pull' }])
    })
    api.post.mockResolvedValue({ repository: 'team/orders', tag: 'v1', affected_tags: [], references: [] })
    api.delete.mockResolvedValue({})
    const wrapper = mountRegistry()
    await new Promise(resolve => setTimeout(resolve, 0))
    await new Promise(resolve => setTimeout(resolve, 0))

    await wrapper.get('[aria-label="删除标签 v1"]').trigger('click')
    await new Promise(resolve => setTimeout(resolve, 0))
    document.body.querySelector('.registry-delete-modal .btn-danger').click()
    await new Promise(resolve => setTimeout(resolve, 0))
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(wrapper.findAll('.registry-flat-row')).toHaveLength(2)
    expect(wrapper.text()).toContain('team/payments')
    expect(wrapper.text()).toContain('v2')
    wrapper.unmount()
  })

  it('shows catalog failures as a compact retryable workspace state', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(path => {
      if (path === '/managed-oci-registries/storage-preflight') return Promise.resolve({ ready: true, storage_class_name: 'local-path', data_nodes: ['node-a'] })
      if (path === '/managed-oci-registries/pvcs') return Promise.resolve([])
      if (path === '/managed-oci-registries/1/catalog') return Promise.reject(new Error('API 路径不存在'))
      return Promise.resolve([{ id: 1, endpoint: 'registry.internal:5443', status: 'ready', data_node: 'node-a', pvc_name: 'registry-data', storage_size: '10Gi', registry_image: 'registry:2', namespace: 'cylism-system', pull_username: 'cylism-pull' }])
    })
    const wrapper = mountRegistry()
    await new Promise(resolve => setTimeout(resolve, 0))
    await new Promise(resolve => setTimeout(resolve, 0))
    const error = wrapper.get('[data-testid="registry-catalog-error"]')
    expect(wrapper.get('[data-testid="registry-catalog"]').findComponent({ name: 'SurfaceCard' }).exists()).toBe(true)
    expect(error.classes()).toContain('registry-catalog-state')
    expect(error.get('.feedback-banner--warning').text()).toContain('暂时无法读取镜像目录')
    expect(error.text()).not.toContain('API 路径不存在')
    expect(error.text()).toContain('镜像目录内容暂不可展示')
    expect(error.get('button').text()).toContain('重新尝试')
    wrapper.unmount()
  })
})
