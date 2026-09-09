import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import Servers from './Servers.vue'
import { api } from '../api/index.js'

vi.mock('../api/index.js', () => ({
  api: {
    get: vi.fn().mockImplementation(url => {
	  if (url === '/servers/network-diagnostics') return Promise.resolve({
	    servers: [
	      { server_id: 1, name: 'test-srv', k8s_unit: 'k3s', network_mode: 'k3s_embedded_tailscale', tailscale: { installed: true, online: true, tailnet_ip: '100.101.102.1', udp: true, ipv4: true, nearest_derp: 'tok' } },
	      { server_id: 2, name: 'cluster-srv', network_mode: 'external_tailscale', tailscale: { installed: true, online: true, tailnet_ip: '100.101.102.2', udp: false, ipv4: true } },
	    ],
	    links: [
	      { source_server_id: 1, target_server_id: 2, path: 'direct', latency_ms: 18 },
	      { source_server_id: 2, target_server_id: 1, path: 'derp', derp_region: 'tok', latency_ms: 126 },
	      { source_server_id: 1, target_server_id: 3, path: 'unreachable', error_code: 'ping_timeout' },
	    ],
	  })
      if (url === '/servers/resource-stats') return Promise.resolve([
        { server_id: 1, status: 'ready', cpu_percent: 42.5, cpu_cores: 2, memory_used_mb: 512, memory_total_mb: 1024, disk_used_gb: 8, disk_total_gb: 40, load_1m: 0.4, uptime: '2 days', sampled_at: '2026-08-03T09:00:00Z' },
        { server_id: 2, status: 'unreachable', error: 'SSH connection timed out', sampled_at: '2026-08-03T09:00:00Z' },
      ])
      if (url.startsWith('/servers')) return Promise.resolve([
        { id: 1, name: 'test-srv', host: '10.0.0.1', ssh_user: 'root', ssh_auth_type: 'password', ssh_port: 22, cluster_role: '', k8s_node_name: '' },
        { id: 2, name: 'cluster-srv', host: '10.0.0.2', ssh_user: 'root', ssh_auth_type: 'password', ssh_port: 22, cluster_role: 'worker', k8s_node_name: 'worker-a' },
      ])
      return Promise.resolve({})
    }),
    post: vi.fn().mockResolvedValue({}),
    delete: vi.fn().mockResolvedValue({ message: 'ok' })
  }
}))

beforeEach(() => { document.body.innerHTML = '' })

describe('Servers view', () => {
  it('renders a server registry page without node tabs', () => {
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    expect(wrapper.text()).toContain('服务器')
    expect(wrapper.text()).toContain('集群节点已经拆分到“集群节点”页面')
    expect(wrapper.findAll('.tab-btn')).toHaveLength(0)
    expect(wrapper.get('.section-tabs-header').find('h1').text()).toBe('服务器')
    expect(wrapper.findAll('.section-tab')).toHaveLength(3)
    expect(wrapper.find('.server-content').exists()).toBe(true)
  })

  it('shows server table with new columns', async () => {
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    await new Promise(r => setTimeout(r, 200))
    await nextTick()
    const text = wrapper.text()
    expect(text).toContain('test-srv')
    expect(text).toContain('10.0.0.1')
    expect(text).toContain('root')
    expect(text).toContain('密码')
    expect(text).toContain('未加入')
  })

  it('shows add server modal on button click', async () => {
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    await new Promise(r => setTimeout(r, 200))
    await nextTick()
    const btn = wrapper.find('.btn-primary')
    await btn.trigger('click')
    await nextTick()
    expect(wrapper.find('.modal').exists()).toBe(true)
  })

  it('shows a batch-refreshed resource monitoring view', async () => {
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    await new Promise(r => setTimeout(r, 200))
    await wrapper.findAll('button').find(button => button.text().includes('资源监控')).trigger('click')
    await new Promise(r => setTimeout(r, 0))
    await nextTick()

    expect(api.get).toHaveBeenCalledWith('/servers/resource-stats', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(wrapper.text()).toContain('资源概览')
    expect(wrapper.text()).toContain('42.5%')
    expect(wrapper.text()).toContain('不可达')
    expect(wrapper.find('.resource-overview').classes()).toContain('card')
    wrapper.unmount()
  })

  it('stops resource polling after leaving the monitoring section', async () => {
    vi.useFakeTimers()
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    await vi.advanceTimersByTimeAsync(0)
    await wrapper.findAll('button').find(button => button.text().includes('资源监控')).trigger('click')
    await vi.advanceTimersByTimeAsync(0)

    const initialRequests = api.get.mock.calls.filter(([url]) => url === '/servers/resource-stats').length
    await wrapper.findAll('button').find(button => button.text().includes('基本配置')).trigger('click')
    await vi.advanceTimersByTimeAsync(10000)

    expect(api.get.mock.calls.filter(([url]) => url === '/servers/resource-stats')).toHaveLength(initialRequests)
    wrapper.unmount()
    vi.useRealTimers()
  })

  it('stops resource polling when the view is unmounted', async () => {
    vi.useFakeTimers()
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    await vi.advanceTimersByTimeAsync(0)
    await wrapper.findAll('button').find(button => button.text().includes('资源监控')).trigger('click')
    await vi.advanceTimersByTimeAsync(0)

    const initialRequests = api.get.mock.calls.filter(([url]) => url === '/servers/resource-stats').length
    wrapper.unmount()
    await vi.advanceTimersByTimeAsync(10000)

    expect(api.get.mock.calls.filter(([url]) => url === '/servers/resource-stats')).toHaveLength(initialRequests)
    vi.useRealTimers()
  })

  it('shows manually refreshed Tailscale diagnostics without raw remote output', async () => {
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    await new Promise(r => setTimeout(r, 200))
    await wrapper.findAll('button').find(button => button.text().includes('网络诊断')).trigger('click')
    await new Promise(r => setTimeout(r, 0))
    await nextTick()

    expect(api.get).toHaveBeenCalledWith('/servers/network-diagnostics', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(wrapper.text()).toContain('K3s 内建 Tailscale')
	    expect(wrapper.text()).toContain('UDP 直连')
    expect(wrapper.text()).toContain('DERP 中继')
    expect(wrapper.text()).toContain('不可达')
    expect(wrapper.text()).toContain('超时')
    expect(wrapper.text()).toContain('126 ms')
    expect(wrapper.text()).not.toContain('ExecStart')
    wrapper.unmount()
  })

  it('shows a concise failure state when network diagnostics cannot be collected', async () => {
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    await new Promise(r => setTimeout(r, 200))
    api.get.mockRejectedValueOnce(new Error('connection failed'))
    await wrapper.findAll('button').find(button => button.text().includes('网络诊断')).trigger('click')
    await new Promise(r => setTimeout(r, 0))
    await nextTick()

    expect(wrapper.text()).toContain('网络诊断请求失败')
    wrapper.unmount()
  })

  it('manually unbinds a server without deleting its Kubernetes node', async () => {
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(true)
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    await new Promise(r => setTimeout(r, 200))
    await nextTick()

    const unbind = wrapper.findAll('button').find(button => button.text() === '解除绑定')
    await unbind.trigger('click')

    expect(confirm).toHaveBeenCalled()
    expect(api.post).toHaveBeenCalledWith('/servers/2/unbind')
    confirm.mockRestore()
  })

  it('keeps the SSH terminal open when the overlay is clicked during text selection', async () => {
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    await new Promise(r => setTimeout(r, 200))
    await nextTick()

    const terminal = wrapper.findAll('button').find(button => button.text() === '💻')
    await terminal.trigger('click')
    await nextTick()
    const overlay = document.querySelector('.terminal-overlay')
    overlay.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()

    expect(document.querySelector('.terminal-overlay')).not.toBeNull()
    wrapper.unmount()
  })

  it('keeps the latest selected server statistics when an earlier selection resolves late', async () => {
    let resolveFirstStats
    const firstStats = new Promise(resolve => { resolveFirstStats = resolve })
    api.get.mockImplementation(url => {
      if (url === '/servers') return Promise.resolve([
        { id: 1, name: 'first-srv', host: '10.0.0.1', ssh_user: 'root', ssh_auth_type: 'password', ssh_port: 22 },
        { id: 2, name: 'second-srv', host: '10.0.0.2', ssh_user: 'root', ssh_auth_type: 'password', ssh_port: 22 },
      ])
      if (url === '/servers/resource-stats') return Promise.resolve([
        { server_id: 1, memory_used_mb: 1, memory_total_mb: 1, disk_used_gb: 1, disk_total_gb: 1 },
        { server_id: 2, memory_used_mb: 1, memory_total_mb: 1, disk_used_gb: 1, disk_total_gb: 1 },
      ])
      if (url === '/servers/1/stats') return firstStats
      if (url === '/servers/2/stats') return Promise.resolve({ uptime: 'new result' })
      return Promise.resolve({})
    })
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    await new Promise(r => setTimeout(r, 0))
    await wrapper.findAll('button').find(button => button.text().includes('资源监控')).trigger('click')
    await new Promise(r => setTimeout(r, 0))

    const rows = wrapper.findAll('.resource-row')
    await rows[0].trigger('click')
    await rows[1].trigger('click')
    resolveFirstStats({ uptime: 'stale result' })
    await new Promise(r => setTimeout(r, 0))
    await nextTick()

    expect(wrapper.text()).toContain('资源监控 — second-srv')
    expect(wrapper.text()).toContain('运行 new result')
    expect(wrapper.text()).not.toContain('运行 stale result')
    wrapper.unmount()
  })
})
