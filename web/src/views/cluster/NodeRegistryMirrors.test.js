import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import NodeRegistryMirrors from './NodeRegistryMirrors.vue'
import SurfaceCard from '../../components/SurfaceCard.vue'

const api = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), put: vi.fn() }))
vi.mock('../../api/index.js', () => ({ api }))

describe('Node registry mirrors view', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.get.mockResolvedValue([{ id: 11, name: 'node-a', host: '10.0.0.11', cluster_id: 3, cluster_role: 'worker' }])
    api.post.mockResolvedValue({ inspected_at: '2026-10-04T00:00:00Z', nodes: [{ server_id: 11, name: 'node-a', state: 'matching', config_md5: 'abc123', actual: [{ registry: 'docker.io', endpoints: ['https://mirror.example.com'], auth_configured: true, auth_username: 'robot', insecure_skip_verify: false }] }] })
    api.put.mockResolvedValue({ results: [{ server_id: 11, status: 'success' }] })
  })

  it('renders one row per node with mirror count and opens realtime configuration', async () => {
    const wrapper = mount(NodeRegistryMirrors, { props: { clusterId: 3 }, global: { stubs: { Teleport: true } } })
    await flushPromises()
    expect(wrapper.getComponent(SurfaceCard).attributes('data-testid')).toBe('node-registry-mirror-workspace')
    expect(wrapper.findAll('tbody tr')).toHaveLength(1)
    expect(wrapper.get('.node-registry-table').text()).toContain('1')
    expect(wrapper.get('.node-registry-table').text()).not.toContain('https://mirror.example.com')
    expect(wrapper.get('.node-registry-table').text()).not.toContain('个 Registry')
    expect(wrapper.text()).toContain('保存后会重启对应节点的 K3s 服务')
    expect(wrapper.get('.config-md5').text()).toBe('abc123')
    expect(api.post).toHaveBeenCalledWith('/node-registry-mirrors/inspect-actual-config', { cluster_id: 3 })
    expect(wrapper.text()).not.toContain('管理 Registry Proxy')
    expect(wrapper.text()).not.toContain('查看节点配置')
    await wrapper.get('.row-actions .btn').trigger('click')
    expect(wrapper.get('[role="dialog"]').text()).toContain('https://mirror.example.com')
    expect(wrapper.get('[role="dialog"]').text()).toContain('docker.io')
    expect(wrapper.get('[role="dialog"]').text()).toContain('账号：robot')
    wrapper.unmount()
  })

  it('opens the all-node editor when md5 values match and saves all nodes', async () => {
    api.get.mockResolvedValue([
      { id: 11, name: 'node-a', host: '10.0.0.11', cluster_id: 3, cluster_role: 'worker' },
      { id: 12, name: 'node-b', host: '10.0.0.12', cluster_id: 3, cluster_role: 'worker' },
    ])
    api.post.mockResolvedValue({ nodes: [
      { server_id: 11, name: 'node-a', state: 'matching', config_md5: 'same', actual: [{ registry: 'docker.io', endpoints: ['https://old.example.com'] }] },
      { server_id: 12, name: 'node-b', state: 'matching', config_md5: 'same', actual: [{ registry: 'docker.io', endpoints: ['https://old.example.com'] }] },
    ] })
    const wrapper = mount(NodeRegistryMirrors, { props: { clusterId: 3 }, global: { stubs: { Teleport: true } } })
    await flushPromises()
    await wrapper.get('.workspace-actions .btn-primary').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="dialog"]').text()).toContain('编辑全部节点镜像源')
    const textarea = wrapper.get('.config-editor-row textarea').element
    textarea.value = 'https://new.example.com'
    textarea.dispatchEvent(new Event('input'))
    await wrapper.get('.config-editor-row input[autocomplete="new-password"]').setValue('new-secret')
    await wrapper.get('[role="dialog"] .btn-primary').trigger('click')
    await flushPromises()
    expect(api.put).toHaveBeenCalledWith('/node-registry-mirrors/actual-config', expect.objectContaining({ cluster_id: 3, server_ids: [11, 12] }))
    expect(api.put.mock.calls[0][1].configs[0].endpoints).toEqual(['https://new.example.com'])
    expect(api.put.mock.calls[0][1].configs[0].auth_credential).toBe('new-secret')
    wrapper.unmount()
  })

  it('syncs one node configuration to selected target nodes', async () => {
    api.get.mockResolvedValue([
      { id: 11, name: 'node-a', host: '10.0.0.11', cluster_id: 3, cluster_role: 'control-plane' },
      { id: 12, name: 'node-b', host: '10.0.0.12', cluster_id: 3, cluster_role: 'worker' },
    ])
    api.post.mockResolvedValue({ nodes: [{ server_id: 11, name: 'node-a', state: 'matching', config_md5: 'a', actual: [] }, { server_id: 12, name: 'node-b', state: 'drifted', config_md5: 'b', actual: [] }] })
    const wrapper = mount(NodeRegistryMirrors, { props: { clusterId: 3 }, global: { stubs: { Teleport: true } } })
    await flushPromises()
    await wrapper.findAll('.row-actions .btn-primary')[0].trigger('click')
    await wrapper.get('.sync-node-option input').setValue(true)
    await wrapper.get('[role="dialog"] .btn-primary').trigger('click')
    await flushPromises()
    expect(api.post).toHaveBeenCalledWith('/node-registry-mirrors/sync-actual-config', { cluster_id: 3, source_server_id: 11, target_server_ids: [12] })
    wrapper.unmount()
  })

  it('renders one row per node and uses node consistency for the status', async () => {
    api.get.mockResolvedValue([
      { id: 11, name: 'node-a', host: '10.0.0.11', cluster_id: 3, cluster_role: 'worker' },
      { id: 12, name: 'node-b', host: '10.0.0.12', cluster_id: 3, cluster_role: 'worker' },
    ])
    api.post.mockResolvedValue({ nodes: [
      { server_id: 11, name: 'node-a', state: 'drifted', config_md5: 'same', actual: [
        { registry: 'docker.io', endpoints: ['https://docker.example.com'] },
        { registry: 'ghcr.io', endpoints: ['https://ghcr.example.com'] },
      ] },
      { server_id: 12, name: 'node-b', state: 'drifted', config_md5: 'same', actual: [
        { registry: 'docker.io', endpoints: ['https://docker.example.com'] },
        { registry: 'ghcr.io', endpoints: ['https://ghcr.example.com'] },
      ] },
    ] })
    const wrapper = mount(NodeRegistryMirrors, { props: { clusterId: 3 }, global: { stubs: { Teleport: true } } })
    await flushPromises()
    expect(wrapper.findAll('tbody tr')).toHaveLength(2)
    expect(wrapper.findAll('.mirror-count').map(item => item.text())).toEqual(['2', '2'])
    expect(wrapper.findAll('.row-actions .btn')).toHaveLength(6)
    expect(wrapper.findAll('.badge-online')).toHaveLength(2)
    expect(wrapper.text()).not.toContain('有差异')
    wrapper.unmount()
  })
})
