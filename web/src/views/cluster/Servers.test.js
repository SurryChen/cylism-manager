import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import Servers from './Servers.vue'
import {
  createServer,
  deleteServer,
  getServerResourceStats,
  getServers,
  getServerStats,
  importServerToCluster,
  preimportServer,
  probeServer,
  unbindServer,
  updateServer,
} from '../../api/servers.js'

vi.mock('../../api/servers.js', () => ({
  createServer: vi.fn(),
  deleteServer: vi.fn(),
  getServerResourceStats: vi.fn(),
  getServers: vi.fn(),
  getServerStats: vi.fn(),
  importServerToCluster: vi.fn(),
  preimportServer: vi.fn(),
  probeServer: vi.fn(),
  unbindServer: vi.fn(),
  updateServer: vi.fn(),
}))

vi.mock('../../utils/terminalRuntime.js', () => ({
  loadTerminalRuntime: vi.fn(() => Promise.reject(new Error('终端组件加载失败'))),
}))

const servers = () => [
  { id: 1, name: 'test-srv', host: '10.0.0.1', ssh_user: 'root', ssh_auth_type: 'password', ssh_port: 22, cluster_role: '', k8s_node_name: '' },
  { id: 2, name: 'cluster-srv', host: '10.0.0.2', ssh_user: 'root', ssh_auth_type: 'password', ssh_port: 22, cluster_role: 'worker', k8s_node_name: 'worker-a' },
]

const resourceStats = () => [
  { server_id: 1, status: 'ready', cpu_percent: 42.5, cpu_cores: 2, memory_used_mb: 512, memory_total_mb: 1024, disk_used_gb: 8, disk_total_gb: 40, load_1m: 0.4, uptime: '2 days', sampled_at: '2026-08-03T09:00:00Z' },
  { server_id: 2, status: 'unreachable', error: 'SSH connection timed out', sampled_at: '2026-08-03T09:00:00Z' },
]

function flush() {
  return new Promise(resolve => setTimeout(resolve, 0))
}

beforeEach(() => {
  document.body.innerHTML = ''
  Object.defineProperty(document, 'hidden', { configurable: true, value: false })
  vi.clearAllMocks()
  getServers.mockResolvedValue(servers())
  getServerResourceStats.mockResolvedValue(resourceStats())
  getServerStats.mockResolvedValue({})
  createServer.mockResolvedValue({})
  updateServer.mockResolvedValue({})
  deleteServer.mockResolvedValue({ message: 'ok' })
  preimportServer.mockResolvedValue({ node_name: 'worker-a', role: 'worker' })
  importServerToCluster.mockResolvedValue({})
  probeServer.mockResolvedValue({ reachable: true, latency_ms: 4 })
  unbindServer.mockResolvedValue({})
})

describe('Servers view', () => {
  it('renders the server registry sections', () => {
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    expect(wrapper.text()).toContain('服务器')
    expect(wrapper.text()).toContain('集群节点已经拆分到“集群节点”页面')
    expect(wrapper.findAll('.section-tab')).toHaveLength(2)
    expect(wrapper.text()).not.toContain('网络诊断')
    expect(wrapper.find('.server-content').exists()).toBe(true)
  })

  it('loads the server table through the domain API', async () => {
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    await flush()
    await nextTick()
    expect(getServers).toHaveBeenCalledWith(expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(wrapper.text()).toContain('test-srv')
    expect(wrapper.text()).toContain('10.0.0.1')
    expect(wrapper.text()).toContain('未加入')
    wrapper.unmount()
  })

  it('keeps the server form open and shows its error when saving fails', async () => {
    createServer.mockRejectedValueOnce(new Error('SSH 凭据无效'))
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    await flush()
    await wrapper.find('.btn-primary').trigger('click')
    await wrapper.get('input[placeholder="我的服务器"]').setValue('edge-a')
    await wrapper.get('input[placeholder="192.168.1.100"]').setValue('10.0.0.9')
    await wrapper.get('form').trigger('submit')
    await flush()

    expect(wrapper.text()).toContain('SSH 凭据无效')
    expect(wrapper.find('input[placeholder="我的服务器"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('shows resource monitoring and forwards an AbortSignal', async () => {
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    await flush()
    await wrapper.findAll('.section-tab').find(button => button.text().includes('资源监控')).trigger('click')
    await flush()
    await nextTick()

    expect(getServerResourceStats).toHaveBeenCalledWith(expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(wrapper.text()).toContain('资源概览')
    expect(wrapper.text()).toContain('42.5%')
    expect(wrapper.text()).toContain('不可达')
    wrapper.unmount()
  })

  it('stops resource polling after leaving monitoring and after unmount', async () => {
    vi.useFakeTimers()
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    await nextTick()
    await wrapper.findAll('.section-tab').find(button => button.text().includes('资源监控')).trigger('click')
    await nextTick()
    const initialRequests = getServerResourceStats.mock.calls.length

    await wrapper.findAll('.section-tab').find(button => button.text().includes('基本配置')).trigger('click')
    await vi.advanceTimersByTimeAsync(10000)
    expect(getServerResourceStats.mock.calls).toHaveLength(initialRequests)

    await wrapper.findAll('.section-tab').find(button => button.text().includes('资源监控')).trigger('click')
    await vi.advanceTimersByTimeAsync(0)
    const mountedRequests = getServerResourceStats.mock.calls.length
    wrapper.unmount()
    await vi.advanceTimersByTimeAsync(10000)
    expect(getServerResourceStats.mock.calls).toHaveLength(mountedRequests)
    vi.useRealTimers()
  })

  it('manually unbinds a server without deleting its Kubernetes node', async () => {
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(true)
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    await flush()
    await wrapper.findAll('button').find(button => button.text() === '解除绑定').trigger('click')
    await flush()

    expect(confirm).toHaveBeenCalled()
    expect(unbindServer).toHaveBeenCalledWith(2)
    confirm.mockRestore()
    wrapper.unmount()
  })

  it('keeps the terminal overlay open when the backdrop is clicked', async () => {
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    await flush()
    await wrapper.findAll('button').find(button => button.text() === '💻').trigger('click')
    await nextTick()
    const overlay = document.querySelector('.terminal-overlay')
    expect(overlay).not.toBeNull()
    overlay.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()
    expect(document.querySelector('.terminal-overlay')).not.toBeNull()
    wrapper.unmount()
  })

  it('keeps the latest selected server statistics when an earlier request resolves late', async () => {
    let resolveFirstStats
    const firstStats = new Promise(resolve => { resolveFirstStats = resolve })
    getServerStats.mockImplementation(serverID => serverID === 1 ? firstStats : Promise.resolve({ uptime: 'new result' }))
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    await flush()
    await wrapper.findAll('.section-tab').find(button => button.text().includes('资源监控')).trigger('click')
    await flush()
    const rows = wrapper.findAll('.resource-row')
    await rows[0].trigger('click')
    await rows[1].trigger('click')
    resolveFirstStats({ uptime: 'stale result' })
    await flush()
    await nextTick()

    expect(wrapper.text()).toContain('资源监控 — cluster-srv')
    expect(wrapper.text()).toContain('运行 new result')
    expect(wrapper.text()).not.toContain('运行 stale result')
    wrapper.unmount()
  })

  it('retains loaded servers and exposes a refresh error after a failed reload', async () => {
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    await flush()
    getServers.mockRejectedValueOnce(new Error('服务器列表刷新失败'))
    await wrapper.find('.icon-button[title="刷新服务器列表"]').trigger('click')
    await flush()

    expect(wrapper.text()).toContain('test-srv')
    expect(wrapper.text()).toContain('服务器列表刷新失败')
    wrapper.unmount()
  })
})
