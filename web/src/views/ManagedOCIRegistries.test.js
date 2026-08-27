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
    expect(wrapper.text()).toContain('由 Cylism 在集群中部署和管理')
    expect(wrapper.findAll('[data-testid="deploy-registry"]')).toHaveLength(1)
    expect(wrapper.text()).not.toContain('交付中心')
    await wrapper.get('[data-testid="deploy-registry"]').trigger('click')
    expect(document.body.querySelector('.registry-modal')).not.toBeNull()
    expect(document.body.querySelector('.registry-modal').textContent).toContain('local-path')
    expect(document.body.querySelector('.registry-pvc-select').textContent).toContain('registry-data')
    expect(document.body.querySelector('input[readonly]').value).toBe('cylism-system')
    wrapper.unmount()
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
    expect(api.get).toHaveBeenCalledWith('/managed-oci-registries/certificates?namespace=cylism-system')
    const certificateSelect = document.body.querySelector('.registry-certificate-select')
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
})
