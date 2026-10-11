import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ServerTailscale from './ServerTailscale.vue'
import { checkTailscaleConnections, diagnoseTailscaleConnection, getTailscaleStatus, getTailscaleStatuses, installTailscale, joinTailscale } from '../../api/servers.js'

vi.mock('../../api/servers.js', () => ({
  getTailscaleStatuses: vi.fn(),
  getTailscaleStatus: vi.fn(),
  installTailscale: vi.fn(),
  joinTailscale: vi.fn(),
  leaveTailscale: vi.fn(),
  checkTailscaleConnections: vi.fn(),
  diagnoseTailscaleConnection: vi.fn(),
}))

afterEach(() => { document.body.innerHTML = ''; vi.clearAllMocks() })

describe('ServerTailscale', () => {
  it('compares a K3s node IP with its current Tailscale IP', async () => {
    getTailscaleStatus.mockResolvedValue({ server_id: 1, status: 'online', ip: '100.81.23.123' })
    const wrapper = mount(ServerTailscale, { global: { stubs: { RouterLink: true } }, props: {
      servers: [{ id: 1, name: 'control', host: '203.0.113.1', k8s_node_name: 'control-node', cluster_id: 9 }],
      cluster: { id: 9, network_mode: 'tailscale' },
      nodeInfos: [{ name: 'control-node', internal_ip: '100.81.23.123' }],
    } })
    await flushPromises()
    expect(wrapper.text()).toContain('部署网络：Tailscale')
    expect(wrapper.findAll('thead th').map(cell => cell.text())).toContain('K3s 节点 IP')
    expect(wrapper.get('tbody tr').text()).toContain('IP 一致')
    expect(wrapper.find('.tailscale-toolbar-actions .btn').exists()).toBe(false)
    wrapper.unmount()
  })

  it('explains an empty Kubernetes node response instead of showing a generic pending state', async () => {
    getTailscaleStatus.mockResolvedValue({ server_id: 1, status: 'online', ip: '100.81.23.123' })
    const wrapper = mount(ServerTailscale, { global: { stubs: { RouterLink: true } }, props: {
      servers: [{ id: 1, name: 'control', host: '203.0.113.1', k8s_node_name: 'control-node' }],
      cluster: { id: 9, network_mode: 'tailscale' },
      nodeInfos: [],
    } })
    await flushPromises()
    const check = wrapper.get('.tailscale-check')
    expect(check.text()).toBe('无节点数据')
    await check.trigger('click')
    expect(document.body.querySelector('.base-modal')?.textContent).toContain('Kubernetes API 未返回节点')
    wrapper.unmount()
  })

  it('requests Kubernetes node refresh from the cluster networking page', async () => {
    getTailscaleStatus.mockResolvedValue({ server_id: 1, status: 'online', ip: '100.81.23.123' })
    const wrapper = mount(ServerTailscale, { global: { stubs: { RouterLink: true } }, props: {
      servers: [{ id: 1, name: 'control', host: '203.0.113.1' }],
      cluster: { id: 9, network_mode: 'tailscale' },
      nodesError: 'Agent 连接超时',
    } })
    await flushPromises()
    expect(wrapper.get('.tailscale-check').text()).toBe('读取失败')
    await wrapper.get('[aria-label="刷新组网与节点状态"]').trigger('click')
    expect(wrapper.emitted('refresh-nodes')).toHaveLength(1)
    wrapper.unmount()
  })

  it('installs and joins a server from its management dialog', async () => {
    getTailscaleStatus.mockResolvedValue({ server_id: 1, status: 'not_installed' })
    installTailscale.mockResolvedValue({ server_id: 1, status: 'offline' })
    joinTailscale.mockResolvedValue({ server_id: 1, status: 'online', ip: '100.81.23.123', device_name: 'worker.tailnet.ts.net' })
    const wrapper = mount(ServerTailscale, { props: { servers: [{ id: 1, name: 'worker', host: '203.0.113.1' }] } })
    await flushPromises()
    expect(wrapper.text()).toContain('未安装')
    expect(getTailscaleStatuses).not.toHaveBeenCalled()

    await wrapper.get('[aria-label="管理 worker 的组网"]').trigger('click')
    const dialog = document.body.querySelector('.base-modal')
    expect(dialog.textContent).toContain('安装 Tailscale')
    dialog.querySelector('.btn-primary').click()
    await flushPromises()
    expect(installTailscale).toHaveBeenCalledWith(1)

    const key = dialog.querySelector('#tailscale-auth-key')
    key.value = 'tskey-auth-example'
    key.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()
    dialog.querySelector('form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await flushPromises()

    expect(joinTailscale).toHaveBeenCalledWith(1, 'tskey-auth-example')
    expect(wrapper.text()).toContain('100.81.23.123')
    expect(dialog.textContent).toContain('worker.tailnet.ts.net')
    wrapper.unmount()
  })

  it('shows server rows immediately and updates each status as it arrives', async () => {
    let resolveFirst
    let resolveSecond
    getTailscaleStatus.mockImplementation(id => new Promise(resolve => {
      if (id === 1) resolveFirst = resolve
      else resolveSecond = resolve
    }))
    const wrapper = mount(ServerTailscale, { props: { servers: [
      { id: 1, name: 'first', host: '203.0.113.1' },
      { id: 2, name: 'second', host: '203.0.113.2' },
    ] } })
    expect(wrapper.findAll('tbody tr')).toHaveLength(2)
    expect(wrapper.text()).toContain('0 / 2 台已检测')
    expect(wrapper.findAll('tbody tr')[0].text()).toContain('检测中')
    expect(getTailscaleStatus).toHaveBeenCalledTimes(2)

    resolveFirst({ server_id: 1, status: 'online', ip: '100.81.23.123' })
    await flushPromises()
    expect(wrapper.text()).toContain('1 / 2 台已检测 · 1 台在线')
    expect(wrapper.findAll('tbody tr')[0].text()).toContain('100.81.23.123')
    expect(wrapper.findAll('tbody tr')[1].text()).toContain('检测中')

    resolveSecond({ server_id: 2, status: 'not_installed' })
    await flushPromises()
    expect(wrapper.text()).toContain('1 / 2 台在线')
    expect(wrapper.findAll('tbody tr')[1].text()).toContain('未安装')
    wrapper.unmount()
  })

  it('shows the specific Tailscale failure instead of a generic unreachable state', async () => {
    const message = 'SSH 已连接，Tailscale 已安装，但 tailscaled 服务未运行或不可用'
    getTailscaleStatus.mockResolvedValue({ server_id: 2, status: 'unreachable', error_type: 'tailscale_daemon', error: message })
    const wrapper = mount(ServerTailscale, { props: { servers: [{ id: 2, name: 'node', host: '203.0.113.2' }] } })
    await flushPromises()

    const status = wrapper.get('tbody tr td:nth-child(3) .badge')
    expect(status.text()).toBe('tailscaled 不可用')
    expect(status.attributes('title')).toBe(message)
    await wrapper.get('[aria-label="管理 node 的组网"]').trigger('click')
    const dialog = document.body.querySelector('.base-modal')
    expect(dialog.textContent).toContain(message)
    expect(dialog.querySelector('#tailscale-auth-key')).toBeNull()
    const startButton = dialog.querySelector('.btn-primary')
    expect(startButton.textContent).toContain('启动 Tailscale 服务')
    installTailscale.mockResolvedValue({ server_id: 2, status: 'offline' })
    startButton.click()
    await flushPromises()
    expect(installTailscale).toHaveBeenCalledWith(2)
    expect(dialog.textContent).toContain('待登录')
    wrapper.unmount()
  })

  it('shows relay connectivity and explains both endpoints on demand', async () => {
    getTailscaleStatus.mockImplementation(id => Promise.resolve({ server_id: id, status: 'online', ip: `100.64.0.${id}` }))
    checkTailscaleConnections.mockResolvedValue([
      { from_id: 1, to_id: 2, ok: true, path: 'relay', relay: 'sfo' },
      { from_id: 2, to_id: 1, ok: true, path: 'direct' },
    ])
    diagnoseTailscaleConnection.mockResolvedValue({
      connection: { from_id: 1, to_id: 2, ok: true, path: 'relay', relay: 'sfo' },
      nodes: [
        { server_id: 1, udp: false, mapping_varies_by_dest_ip: false },
        { server_id: 2, udp: true, mapping_varies_by_dest_ip: true },
      ],
      findings: ['来源服务器无法使用出站 UDP', '目标服务器的 NAT 映射随目标变化'],
    })
    const wrapper = mount(ServerTailscale, { props: { servers: [
      { id: 1, name: '来源服务器', host: '203.0.113.1' },
      { id: 2, name: '目标服务器', host: '203.0.113.2' },
    ] } })
    await flushPromises()
    await wrapper.get('.tailscale-toolbar-actions .btn').trigger('click')
    await flushPromises()
    const dialog = document.body.querySelector('.base-modal')
    expect(dialog.textContent).toContain('DERP sfo')
    expect(dialog.textContent).toContain('直连')
    expect(dialog.textContent).not.toContain('不可达')
    dialog.querySelector('.btn-sm').click()
    await flushPromises()
    expect(diagnoseTailscaleConnection).toHaveBeenCalledWith(1, 2)
    expect(dialog.textContent).toContain('来源服务器无法使用出站 UDP')
    expect(dialog.textContent).toContain('随目标变化')
    wrapper.unmount()
  })

  it('treats a ping without a reply as inconclusive and offers diagnosis', async () => {
    getTailscaleStatus.mockImplementation(id => Promise.resolve({ server_id: id, status: 'online', ip: `100.64.0.${id}` }))
    checkTailscaleConnections.mockResolvedValue([
      { from_id: 1, to_id: 2, ok: false, path: 'inconclusive', error: '本次探测未收到回应，可重新检测或分析连接' },
    ])
    diagnoseTailscaleConnection.mockResolvedValue({
      connection: { from_id: 1, to_id: 2, ok: true, path: 'relay', relay: 'sfo' },
      nodes: [],
      findings: ['两端可通过 DERP 中继通信'],
    })
    const wrapper = mount(ServerTailscale, { props: { servers: [
      { id: 1, name: 'first', host: '203.0.113.1' },
      { id: 2, name: 'second', host: '203.0.113.2' },
    ] } })
    await flushPromises()
    await wrapper.get('.tailscale-toolbar-actions .btn').trigger('click')
    await flushPromises()
    const dialog = document.body.querySelector('.base-modal')
    expect(dialog.textContent).toContain('未确认')
    expect(dialog.textContent).not.toContain('不可达')
    expect(dialog.textContent).toContain('分析连接')
    dialog.querySelector('.btn-sm').click()
    await flushPromises()
    expect(diagnoseTailscaleConnection).toHaveBeenCalledWith(1, 2)
    expect(dialog.textContent).toContain('DERP sfo')
    wrapper.unmount()
  })
})
