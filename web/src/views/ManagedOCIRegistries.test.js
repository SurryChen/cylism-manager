import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ManagedOCIRegistries from './ManagedOCIRegistries.vue'

vi.mock('../api/index.js', () => ({ api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() } }))

describe('ManagedOCIRegistries view', () => {
  it('shows the deployment action before a registry exists', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(path => Promise.resolve(path === '/servers' ? [] : []))
    const wrapper = mount(ManagedOCIRegistries)
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(wrapper.get('[data-testid="registry-empty"]').text()).toContain('尚未部署自托管制品库')
    expect(wrapper.text()).toContain('由 Cylism 在集群中部署和管理')
    expect(wrapper.findAll('[data-testid="deploy-registry"]')).toHaveLength(1)
    expect(wrapper.text()).not.toContain('交付中心')
  })

  it('marks an HTTP Registry as insecure without showing a credential', async () => {
    const { api } = await import('../api/index.js')
    api.get.mockImplementation(path => Promise.resolve(path === '/servers' ? [{ id: 7, name: 'worker-1', host: '10.0.0.7', cluster_role: 'worker' }] : [{ id: 1, endpoint: 'registry.internal', insecure_http: true, status: 'degraded', data_node: 'worker-1', data_path: '/data/registry', registry_image: 'registry:2', namespace: 'cylism-system', pull_username: 'cylism-pull', node_registry_mirror_id: 2, last_error: '入口不可达' }]))
    const wrapper = mount(ManagedOCIRegistries)
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(wrapper.get('[data-testid="registry-summary"]').text()).toContain('HTTP，凭据和镜像层以明文传输')
    expect(wrapper.text()).not.toContain('registry-password')
  })
})
