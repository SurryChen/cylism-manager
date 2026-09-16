import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ManagedOCIRegistries from './ManagedOCIRegistries.vue'

vi.mock('../api/index.js', () => ({ api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() } }))

describe('ManagedOCIRegistries view', () => {
  it('shows the deployment action before a registry exists', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(path => Promise.resolve(path === '/servers' ? [] : path === '/managed-oci-registries/storage-preflight' ? { ready: true, storage_class_name: 'local-path', storage_classes: ['local-path'], data_nodes: ['node-a'] } : path === '/managed-oci-registries/pvcs' ? [{ name: 'registry-data', namespace: 'cylism-system', storage: '100Gi', storage_class_name: 'local-path', phase: 'Pending', access_modes: ['ReadWriteOnce'] }] : []))
    const wrapper = mount(ManagedOCIRegistries)
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(wrapper.get('[data-testid="registry-empty"]').text()).toContain('尚未部署自托管制品库')
    expect(wrapper.text()).not.toContain('由 Cylism 在集群中部署和管理')
    expect(wrapper.findAll('[data-testid="deploy-registry"]')).toHaveLength(1)
    expect(wrapper.text()).not.toContain('交付中心')
    await wrapper.get('[data-testid="deploy-registry"]').trigger('click')
    expect(document.body.querySelector('.registry-modal')).not.toBeNull()
    expect(document.body.querySelector('.registry-modal').textContent).toContain('local-path')
    expect(document.body.querySelector('.registry-pvc-select').textContent).toContain('registry-data')
    expect(document.body.querySelector('input[readonly]').value).toBe('cylism-system')
    wrapper.unmount()
  })

  it('opens Registry Proxy management from its delivery workspace URL', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(path => Promise.resolve(path === '/registry-proxies' ? [{ id: 2, name: 'Kubernetes Registry 代理', registry: 'registry.k8s.io', upstream_url: 'https://registry.k8s.io', endpoint_host: '100.64.0.8', node_port: 30501, node_name: 'worker-a', cache_limit_gi: 2, cleanup_interval_hours: 24, status: 'ready' }] : path === '/servers' ? [] : path === '/managed-oci-registries/storage-preflight' ? { ready: true, data_nodes: [] } : []))
    const originalHash = window.location.hash
    window.location.hash = '#/delivery/registry?tab=registry-proxy'
    const wrapper = mount(ManagedOCIRegistries)
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(wrapper.text()).toContain('Kubernetes Registry 代理')
    expect(api.get).toHaveBeenCalledWith('/registry-proxies', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    wrapper.unmount()
    window.location.hash = originalHash
  })

  it('offers known Kubernetes nodes when deploying a registry', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(path => Promise.resolve(path === '/servers' ? [{ id: 7, name: 'worker-1', host: '10.0.0.7', cluster_role: 'worker', k8s_node_name: 'worker-1.cluster.local' }] : path === '/managed-oci-registries/storage-preflight' ? { ready: true, storage_class_name: 'local-path', storage_classes: ['local-path'], data_nodes: ['node-a'] } : path === '/managed-oci-registries/pvcs' ? [{ name: 'registry-data', namespace: 'cylism-system', storage: '100Gi', storage_class_name: 'local-path', phase: 'Pending', access_modes: ['ReadWriteOnce'] }] : []))
    const wrapper = mount(ManagedOCIRegistries)
    await new Promise(resolve => setTimeout(resolve, 0))
    await wrapper.get('[data-testid="deploy-registry"]').trigger('click')
    expect(document.body.querySelector('.registry-data-node-select').textContent).toContain('node-a')
    expect(document.body.querySelector('.registry-pvc-select').textContent).toContain('local-path')
    wrapper.unmount()
  })

  it('blocks deployment when local-path storage is not ready', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(path => Promise.resolve(path === '/servers' ? [] : path === '/managed-oci-registries/storage-preflight' ? { ready: false, storage_class_name: 'local-path', storage_classes: ['local-path'], data_nodes: ['node-a'], message: 'StorageClass "local-path" 必须使用 WaitForFirstConsumer' } : path === '/managed-oci-registries/pvcs' ? [{ name: 'registry-data', namespace: 'cylism-system', storage: '100Gi', storage_class_name: 'local-path', phase: 'Pending', access_modes: ['ReadWriteOnce'] }] : []))
    const wrapper = mount(ManagedOCIRegistries)
    await new Promise(resolve => setTimeout(resolve, 0))
    await wrapper.get('[data-testid="deploy-registry"]').trigger('click')
    const modal = document.body.querySelector('.registry-modal')
    expect([...modal.querySelectorAll('button')].find(button => button.textContent.includes('开始部署')).disabled).toBe(true)
    wrapper.unmount()
  })

  it('lists ready certificates before an endpoint is entered and fills the certificate domain', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(path => Promise.resolve(path === '/servers' ? [] : path === '/managed-oci-registries/storage-preflight' ? { ready: true, storage_class_name: 'local-path', storage_classes: ['local-path'], data_nodes: ['node-a'] } : path === '/managed-oci-registries/pvcs' ? [{ name: 'registry-data', namespace: 'cylism-system', storage: '100Gi', storage_class_name: 'local-path', phase: 'Pending', access_modes: ['ReadWriteOnce'] }] : path.startsWith('/managed-oci-registries/certificates?') ? [{ name: 'registry-cert', namespace: 'cylism-system', domains: ['registry.internal'] }] : []))
    const wrapper = mount(ManagedOCIRegistries)
    await new Promise(resolve => setTimeout(resolve, 0))
    await wrapper.get('[data-testid="deploy-registry"]').trigger('click')
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
    const wrapper = mount(ManagedOCIRegistries)
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(wrapper.get('[data-testid="registry-summary"]').text()).toContain('HTTP，凭据和镜像层以明文传输')
    expect(wrapper.get('[data-testid="registry-summary"]').text()).toContain('异常')
    expect(wrapper.text()).not.toContain('registry-password')
    wrapper.unmount()
  })

  it('keeps infrastructure status inline and configuration in the edit modal without duplicating node mirror controls', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(path => Promise.resolve(path === '/servers' ? [{ id: 7, name: 'worker-1', host: '10.0.0.7', cluster_role: 'worker', k8s_node_name: 'node-a' }] : path === '/managed-oci-registries/storage-preflight' ? { ready: true, storage_class_name: 'local-path', storage_classes: ['local-path'], data_nodes: ['node-a'] } : path === '/managed-oci-registries/pvcs' ? [{ name: 'registry-data', namespace: 'cylism-system', storage: '10Gi', storage_class_name: 'local-path', phase: 'Bound', access_modes: ['ReadWriteOnce'] }] : [{ id: 1, endpoint: 'registry.internal', status: 'ready', data_node: 'node-a', pvc_name: 'registry-data', storage_size: '10Gi', storage_class_name: 'local-path', registry_image: 'registry:2', namespace: 'cylism-system', pull_username: 'cylism-pull', certificate_name: 'registry-cert', node_registry_mirror_id: 2 }]))
    const wrapper = mount(ManagedOCIRegistries)
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(wrapper.findAll('.registry-overview .metric')).toHaveLength(3)
    expect(wrapper.find('[data-testid="registry-configuration"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="edit-registry"]').text()).toContain('配置')
    expect(wrapper.text()).not.toContain('节点镜像配置')
    expect(wrapper.find('[data-testid="registry-node-table"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('repairs an unhealthy Registry through the platform endpoint', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(path => Promise.resolve(path === '/servers' ? [] : path === '/managed-oci-registries/storage-preflight' ? { ready: true, storage_class_name: 'local-path', storage_classes: ['local-path'], data_nodes: ['node-a'] } : path === '/managed-oci-registries/pvcs' ? [{ name: 'registry-data', namespace: 'cylism-system', storage: '10Gi', storage_class_name: 'local-path', phase: 'Bound', access_modes: ['ReadWriteOnce'] }] : [{ id: 1, endpoint: 'registry.internal', status: 'degraded', data_node: 'node-a', pvc_name: 'registry-data', storage_size: '10Gi', registry_image: 'registry:2', namespace: 'cylism-system', pull_username: 'cylism-pull', last_error: 'Registry 尚未就绪' }]))
    api.post.mockResolvedValueOnce({ id: 1, status: 'deploying' })
    const wrapper = mount(ManagedOCIRegistries)
    await new Promise(resolve => setTimeout(resolve, 0))
    await wrapper.get('[data-testid="repair-registry"]').trigger('click')
    expect(api.post).toHaveBeenCalledWith('/managed-oci-registries/1/repair')
    wrapper.unmount()
  })

  it('shows an API error without rendering a registry form', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockRejectedValueOnce(new Error('无法读取集群状态'))
    const wrapper = mount(ManagedOCIRegistries)
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(wrapper.get('[role="alert"]').text()).toContain('无法读取集群状态')
    expect(wrapper.find('[data-testid="registry-empty"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('shows a modal and keeps the deployment form open when deployment fails', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(path => Promise.resolve(path === '/servers' ? [] : path === '/managed-oci-registries/storage-preflight' ? { ready: true, storage_class_name: 'local-path', storage_classes: ['local-path'], data_nodes: ['node-a'] } : path === '/managed-oci-registries/pvcs' ? [{ name: 'registry-data', namespace: 'cylism-system', storage: '100Gi', storage_class_name: 'local-path', phase: 'Pending', access_modes: ['ReadWriteOnce'] }] : []))
    api.post.mockRejectedValueOnce(new Error('保存制品库配置失败，请检查名称、地址和项目授权'))
    const wrapper = mount(ManagedOCIRegistries)
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
    const wrapper = mount(ManagedOCIRegistries)
    await new Promise(resolve => setTimeout(resolve, 0))
    await wrapper.get('[data-testid="edit-registry"]').trigger('click')
    const input = document.body.querySelector('input[placeholder="registry.internal:5443/cylism-manager:1.0.0"]')
    expect(input).not.toBeNull()
    expect(input.value).toBe('registry.internal:5443/cylism-manager:1.0.0')
    document.body.querySelector('.registry-modal form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(api.put).toHaveBeenCalledWith('/managed-oci-registries/1', expect.objectContaining({ verification_image: 'registry.internal:5443/cylism-manager:1.0.0' }))
    wrapper.unmount()
  })

  it('loads repository and tag metadata in the self-hosted Registry workspace without a nested tab', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(path => Promise.resolve(
      path === '/managed-oci-registries/storage-preflight' ? { ready: true, storage_class_name: 'local-path', data_nodes: ['node-a'] }
        : path === '/managed-oci-registries/pvcs' ? []
          : path === '/managed-oci-registries/1/catalog' ? { repositories: ['team/orders'], next: '' }
            : path === '/managed-oci-registries/1/catalog/tags?repository=team%2Forders' ? { repository: 'team/orders', tags: [{ name: 'v1', digest: 'sha256:abc123', pull_reference: 'team/orders:v1', platforms: ['linux/amd64'] }], next: '' }
              : [{ id: 1, endpoint: 'registry.internal:5443', status: 'ready', data_node: 'node-a', pvc_name: 'registry-data', storage_size: '10Gi', registry_image: 'registry:2', namespace: 'cylism-system', pull_username: 'cylism-pull' }]
    ))
    const wrapper = mount(ManagedOCIRegistries)
    await new Promise(resolve => setTimeout(resolve, 0))
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(wrapper.get('[data-testid="registry-catalog"]').text()).toContain('team/orders')
    expect(wrapper.find('[data-testid="managed-registry-tab-images"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="registry-catalog"]').text()).not.toContain('镜像仓库')
    expect(wrapper.get('.registry-catalog-count').text()).toBe('1 个仓库')
    expect(wrapper.find('.registry-catalog .sr-only').exists()).toBe(false)
    await wrapper.get('.registry-repository-select').trigger('click')
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(wrapper.text()).toContain('v1')
    expect(wrapper.text()).toContain('linux/amd64')
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
    const wrapper = mount(ManagedOCIRegistries)
    await new Promise(resolve => setTimeout(resolve, 0))
    await new Promise(resolve => setTimeout(resolve, 0))
    const error = wrapper.get('[data-testid="registry-catalog-error"]')
    expect(error.classes()).toContain('card')
    expect(error.get('.feedback-banner--warning').text()).toContain('暂时无法读取镜像目录')
    expect(error.text()).not.toContain('API 路径不存在')
    expect(error.text()).toContain('镜像目录内容暂不可展示')
    expect(error.get('button').text()).toContain('重新尝试')
    wrapper.unmount()
  })
})
