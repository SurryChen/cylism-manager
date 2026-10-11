import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ClusterNetworking from './ClusterNetworking.vue'
import { checkClusterNetworkMigration, startClusterNetworkMigration } from '../../api/clusters.js'
import { getClusterNodes } from '../../api/cluster.js'

vi.mock('../../api/cluster.js', () => ({ getClusterNodes: vi.fn().mockResolvedValue([]) }))
vi.mock('../../api/clusters.js', () => ({
  checkClusterNetworkMigration: vi.fn(),
  startClusterNetworkMigration: vi.fn(),
}))

afterEach(() => { document.body.innerHTML = ''; vi.clearAllMocks() })

const props = {
  clusterId: '9',
  cluster: { id: 9, distribution: 'k3s', network_mode: 'public' },
  servers: [{ id: 3, name: 'control', cluster_role: 'control-plane' }],
}
const tailscaleCardStub = { template: '<div class="tailscale-card"><div class="tailscale-toolbar-actions"><slot name="actions" /></div><slot name="footer" /></div>' }

describe('ClusterNetworking migration', () => {
  it('shows a server networking link for nodes that are not on Tailscale', async () => {
    checkClusterNetworkMigration.mockResolvedValue({ ready: false, message: '预检未通过', nodes: [
      { server_id: 3, name: 'control', role: 'control-plane', ready: false, reason: '请先到服务器组网页安装并接入 Tailscale' },
    ] })
    const wrapper = mount(ClusterNetworking, { props, global: { stubs: { ServerTailscale: tailscaleCardStub, RouterLink: { template: '<a><slot /></a>' } } } })
    await wrapper.get('.tailscale-card .tailscale-toolbar-actions .btn-primary').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('请先到服务器组网页安装并接入 Tailscale')
    expect(wrapper.text()).toContain('配置组网')
    expect(wrapper.findComponent({ name: 'FeedbackBanner' }).exists()).toBe(false)
    expect(document.body.querySelector('.base-modal')?.textContent).toContain('预检未通过')
    expect(wrapper.find('.network-migration-footer').exists()).toBe(false)
    wrapper.unmount()
  })

  it('shows a node read failure in a modal instead of a banner', async () => {
    getClusterNodes.mockRejectedValueOnce(new Error('读取节点超时'))
    const wrapper = mount(ClusterNetworking, { props, global: { stubs: { ServerTailscale: tailscaleCardStub } } })
    await flushPromises()
    expect(wrapper.findComponent({ name: 'FeedbackBanner' }).exists()).toBe(false)
    expect(document.body.querySelector('.base-modal')?.textContent).toContain('读取节点超时')
    wrapper.unmount()
  })

  it('passes the node read failure to the comparison table', async () => {
    getClusterNodes.mockRejectedValueOnce(new Error('Agent 连接超时'))
    const wrapper = mount(ClusterNetworking, { props, global: { stubs: { RouterLink: true } } })
    await flushPromises()
    const check = wrapper.get('tbody tr .tailscale-check')
    expect(check.text()).toBe('读取失败')
    expect(check.attributes('title')).toContain('Agent 连接超时')
    wrapper.unmount()
  })

  it('starts the task only after a successful precheck and confirmation', async () => {
    checkClusterNetworkMigration.mockResolvedValue({ ready: true, nodes: [
      { server_id: 3, name: 'control', role: 'control-plane', ready: true, tailscale_ip: '100.81.23.123' },
    ] })
    startClusterNetworkMigration.mockResolvedValue({ id: 12, operation: 'network-migrate' })
    const wrapper = mount(ClusterNetworking, { props, global: { stubs: { ServerTailscale: tailscaleCardStub } } })
    await wrapper.get('.tailscale-card .tailscale-toolbar-actions .btn-primary').trigger('click')
    await flushPromises()
    expect(startClusterNetworkMigration).not.toHaveBeenCalled()
    await wrapper.get('.network-migration-footer .btn-primary').trigger('click')
    await flushPromises()
    document.body.querySelector('.base-modal .btn-primary').click()
    await flushPromises()
    expect(startClusterNetworkMigration).toHaveBeenCalledWith('9')
    expect(wrapper.emitted('operation-started')).toHaveLength(1)
    wrapper.unmount()
  })
})
