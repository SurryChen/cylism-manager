import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ManagedOCIRegistries from './ManagedOCIRegistries.vue'

vi.mock('../api/index.js', () => ({ api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() } }))

describe('ManagedOCIRegistries view', () => {
  it('shows the deployment action before a registry exists', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(path => Promise.resolve(path === '/servers' ? [] : path === '/managed-oci-registries/storage-preflight' ? { ready: true, storage_class_name: 'local-path', storage_classes: ['local-path'], data_nodes: ['node-a'] } : []))
    const wrapper = mount(ManagedOCIRegistries)
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(wrapper.get('[data-testid="registry-empty"]').text()).toContain('尚未部署自托管制品库')
    expect(wrapper.text()).toContain('由 Cylism 在集群中部署和管理')
    expect(wrapper.findAll('[data-testid="deploy-registry"]')).toHaveLength(1)
    expect(wrapper.text()).not.toContain('交付中心')
    await wrapper.get('[data-testid="deploy-registry"]').trigger('click')
    expect(document.body.querySelector('.registry-modal')).not.toBeNull()
    expect(document.body.querySelector('.registry-modal').textContent).toContain('local-path')
    expect(document.body.querySelector('input[placeholder="100Gi"]').value).toBe('100Gi')
    wrapper.unmount()
  })

  it('offers known Kubernetes nodes when deploying a registry', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(path => Promise.resolve(path === '/servers' ? [{ id: 7, name: 'worker-1', host: '10.0.0.7', cluster_role: 'worker', k8s_node_name: 'worker-1.cluster.local' }] : path === '/managed-oci-registries/storage-preflight' ? { ready: true, storage_class_name: 'local-path', storage_classes: ['local-path'], data_nodes: ['node-a'] } : []))
    const wrapper = mount(ManagedOCIRegistries)
    await new Promise(resolve => setTimeout(resolve, 0))
    await wrapper.get('[data-testid="deploy-registry"]').trigger('click')
    expect(document.body.querySelector('.registry-data-node-select').textContent).toContain('node-a')
    expect(document.body.querySelector('.registry-storage-class-select').textContent).toContain('local-path')
    wrapper.unmount()
  })

  it('blocks deployment when local-path storage is not ready', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(path => Promise.resolve(path === '/servers' ? [] : path === '/managed-oci-registries/storage-preflight' ? { ready: false, storage_class_name: 'local-path', storage_classes: ['local-path'], data_nodes: ['node-a'], message: 'StorageClass "local-path" 必须使用 WaitForFirstConsumer' } : []))
    const wrapper = mount(ManagedOCIRegistries)
    await new Promise(resolve => setTimeout(resolve, 0))
    await wrapper.get('[data-testid="deploy-registry"]').trigger('click')
    const modal = document.body.querySelector('.registry-modal')
    expect(modal.textContent).toContain('必须使用 WaitForFirstConsumer')
    expect([...modal.querySelectorAll('button')].find(button => button.textContent.includes('开始部署')).disabled).toBe(true)
    wrapper.unmount()
  })

  it('selects a matching platform certificate for HTTPS', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(path => Promise.resolve(path === '/servers' ? [] : path === '/managed-oci-registries/storage-preflight' ? { ready: true, storage_class_name: 'local-path', storage_classes: ['local-path'], data_nodes: ['node-a'] } : path.startsWith('/managed-oci-registries/certificates?') ? [{ name: 'registry-cert', namespace: 'cylism-system', domains: ['registry.internal'] }] : []))
    const wrapper = mount(ManagedOCIRegistries)
    await new Promise(resolve => setTimeout(resolve, 0))
    await wrapper.get('[data-testid="deploy-registry"]').trigger('click')
    const endpoint = document.body.querySelector('input[placeholder="registry.example.com:31813"]')
    endpoint.value = 'registry.internal:5443'
    endpoint.dispatchEvent(new Event('input', { bubbles: true }))
    await new Promise(resolve => setTimeout(resolve, 0))
    const certificateSelect = document.body.querySelector('.registry-certificate-select')
    expect(certificateSelect.textContent).toContain('registry-cert')
    expect(document.body.querySelector('input[placeholder="registry-tls"]')).toBeNull()
    wrapper.unmount()
  })

  it('marks an HTTP Registry as insecure without showing a credential', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(path => Promise.resolve(path === '/servers' ? [{ id: 7, name: 'worker-1', host: '10.0.0.7', cluster_role: 'worker' }] : path === '/managed-oci-registries/storage-preflight' ? { ready: true, storage_class_name: 'local-path', storage_classes: ['local-path'], data_nodes: ['node-a'] } : [{ id: 1, endpoint: 'registry.internal', insecure_http: true, status: 'degraded', data_node: 'worker-1', pvc_name: 'cylism-oci-registry-data', storage_size: '100Gi', registry_image: 'registry:2', namespace: 'cylism-system', pull_username: 'cylism-pull', node_registry_mirror_id: 2, last_error: '入口不可达' }]))
    const wrapper = mount(ManagedOCIRegistries)
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(wrapper.get('[data-testid="registry-summary"]').text()).toContain('HTTP，凭据和镜像层以明文传输')
    expect(wrapper.get('[data-testid="registry-summary"]').text()).toContain('异常')
    expect(wrapper.text()).not.toContain('registry-password')
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
})
