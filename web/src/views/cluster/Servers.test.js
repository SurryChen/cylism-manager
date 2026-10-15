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
  getSingBoxStatus,
  probeServer,
  resolveServerHostname,
  unbindServer,
  updateServer,
} from '../../api/servers.js'

vi.mock('../../api/servers.js', () => ({
  createServer: vi.fn(),
  deleteServer: vi.fn(),
  getServerResourceStats: vi.fn(),
  getServers: vi.fn(),
  getServerStats: vi.fn(),
  getSingBoxStatus: vi.fn(),
  installSingBox: vi.fn(),
  importSingBoxConfig: vi.fn(),
  startSingBox: vi.fn(),
  stopSingBox: vi.fn(),
  probeServer: vi.fn(),
  resolveServerHostname: vi.fn(),
  unbindServer: vi.fn(),
  updateServer: vi.fn(),
}))

vi.mock('../../utils/terminalRuntime.js', () => ({
  loadTerminalRuntime: vi.fn(() => Promise.reject(new Error('终端组件加载失败'))),
}))

const servers = () => [
  { id: 1, name: 'test-srv', host: '10.0.0.1', hostname: 'cloudzone.cylism.com', ssh_user: 'root', ssh_auth_type: 'password', ssh_port: 22, cluster_role: '', k8s_node_name: '' },
  { id: 2, name: 'cluster-srv', host: '10.0.0.2', hostname: 'worker-a', ssh_user: 'root', ssh_auth_type: 'password', ssh_port: 22, cluster_role: 'worker', k8s_node_name: 'worker-a' },
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
  getSingBoxStatus.mockResolvedValue({ installed: false, configured: false, running: false })
  createServer.mockResolvedValue({})
  updateServer.mockResolvedValue({})
  deleteServer.mockResolvedValue({ message: 'ok' })
  probeServer.mockResolvedValue({ reachable: true, latency_ms: 4 })
  resolveServerHostname.mockResolvedValue({ hostname: 'cloudzone.cylism.com' })
  unbindServer.mockResolvedValue({})
})

describe('Servers view', () => {
  it('renders the server registry sections', () => {
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    expect(wrapper.text()).toContain('服务器')
    expect(wrapper.findAll('.section-tab')).toHaveLength(5)
    expect(wrapper.text()).toContain('组网')
    expect(wrapper.text()).toContain('代理')
    expect(wrapper.text()).toContain('网络测速')
    expect(wrapper.text()).not.toContain('网络诊断')
    expect(wrapper.find('.server-content').exists()).toBe(true)
    expect(wrapper.find('.section-tabs-actions').exists()).toBe(false)
    const configurationCard = wrapper.get('.server-configuration-card')
    expect(configurationCard.findComponent({ name: 'SurfaceCard' }).exists()).toBe(true)
    expect(configurationCard.get('.server-list-toolbar [title="刷新服务器列表"]').exists()).toBe(true)
    expect(configurationCard.get('[data-testid="add-server"]').text()).toBe('添加服务器')
  })

  it('loads the server table through the domain API', async () => {
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    await flush()
    await nextTick()
    expect(getServers).toHaveBeenCalledWith(expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(wrapper.text()).toContain('test-srv')
    expect(wrapper.text()).toContain('10.0.0.1')
    expect(wrapper.get('.server-configuration-card thead').text()).not.toContain('集群状态')
    expect(wrapper.findAll('.server-configuration-card tbody tr')).toHaveLength(2)
    expect(wrapper.text()).not.toContain('导入集群')
    expect(wrapper.get('.server-configuration-card thead').text()).toContain('操作')
    expect(wrapper.get('.server-configuration-card thead').text()).toContain('主机名')
    expect(wrapper.get('.server-configuration-card thead').text()).not.toContain('节点名')
    expect(wrapper.text()).toContain('cloudzone.cylism.com')
    expect(wrapper.find('[title="刷新 test-srv 的主机名"]').exists()).toBe(false)
    expect(wrapper.get('.server-configuration-card thead').text()).not.toContain('SSH 连通')
    expect(wrapper.get('.server-configuration-card tbody td').classes()).not.toContain('cell-primary')
    expect(wrapper.get('[title="编辑 test-srv"]').classes()).toContain('icon-button--table')
    expect(wrapper.get('[title="删除 test-srv"]').classes()).toContain('icon-button--table')
    expect(wrapper.get('[title="解除 cluster-srv 的集群绑定"]').classes()).toContain('icon-button--table')
    expect(wrapper.get('[title="打开 test-srv 的终端"]').classes()).toContain('icon-button--table')
    expect(wrapper.find('[title="检测 test-srv 的 SSH 连通性"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('only reloads saved hostnames when refreshing the list', async () => {
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    await flush()
    const changed = servers()
    changed[0].hostname = 'new-host.example'
    getServers.mockResolvedValueOnce(changed)
    await wrapper.get('[title="刷新服务器列表"]').trigger('click')
    await flush()

    expect(wrapper.text()).toContain('new-host.example')
    expect(getServers).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('shows a hostname lookup failure only after an explicit request', async () => {
    resolveServerHostname.mockRejectedValueOnce(new Error('SSH 认证失败'))
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    await flush()
    await wrapper.get('[title="编辑 test-srv"]').trigger('click')
    expect(wrapper.get('#server-hostname').element.value).toBe('cloudzone.cylism.com')
    expect(wrapper.get('#server-hostname').attributes('readonly')).toBeDefined()
    expect(resolveServerHostname).not.toHaveBeenCalled()
    await wrapper.get('[title="自动获取机器名"]').trigger('click')
    await flush()

    expect(wrapper.text()).toContain('SSH 认证失败')
    expect(wrapper.get('#server-hostname').element.value).toBe('cloudzone.cylism.com')
    await wrapper.get('form').trigger('submit')
    await flush()

    expect(wrapper.find('.modal').exists()).toBe(false)
    expect(updateServer).toHaveBeenCalledWith(1, expect.objectContaining({ hostname: 'cloudzone.cylism.com' }))
    expect(resolveServerHostname).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('edits a cluster-bound server without clearing its stored SSH credentials', async () => {
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    await flush()
    await wrapper.get('[title="编辑 cluster-srv"]').trigger('click')
    expect(wrapper.text()).toContain('编辑服务器')
    expect(wrapper.get('input[placeholder="留空保持原密码"]').element.value).toBe('')
    expect(wrapper.get('#server-name').element.value).toBe('cluster-srv')
    await wrapper.get('form').trigger('submit')
    await flush()

    expect(updateServer).toHaveBeenCalledWith(2, {
      name: 'cluster-srv',
      host: '10.0.0.2',
      ssh_port: 22,
      ssh_user: 'root',
      ssh_auth_type: 'password',
      hostname: 'worker-a',
      proxy_server_id: null,
    })
    wrapper.unmount()
  })

  it('saves an optional server name when creating and editing', async () => {
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    await flush()
    await wrapper.get('[data-testid="add-server"]').trigger('click')
    await wrapper.get('#server-name').setValue('  应用服务器  ')
    await wrapper.get('input[placeholder="192.168.1.100"]').setValue('10.0.0.9')
    await wrapper.get('[title="自动获取机器名"]').trigger('click')
    await flush()
    await wrapper.get('form').trigger('submit')
    await flush()
    expect(createServer).toHaveBeenCalledWith(expect.objectContaining({ name: '应用服务器', host: '10.0.0.9' }))

    await wrapper.get('[title="编辑 test-srv"]').trigger('click')
    await wrapper.get('#server-name').setValue('')
    await wrapper.get('form').trigger('submit')
    await flush()
    expect(updateServer).toHaveBeenCalledWith(1, expect.objectContaining({ name: '', host: '10.0.0.1' }))
    wrapper.unmount()
  })

  it('sends the selected gateway for proxied servers and clears it on direct access', async () => {
    const existing = servers()
    existing.push({ id: 3, name: 'office-host', host: '100.100.1.2', hostname: 'office-host', ssh_user: 'root', ssh_auth_type: 'password', ssh_port: 22, proxy_server_id: 1, proxy_server_name: 'test-srv', cluster_role: '', k8s_node_name: '' })
    getServers.mockResolvedValue(existing)
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    await flush()
    expect(wrapper.text()).toContain('代理访问 · test-srv')
    await wrapper.get('[title="编辑 office-host"]').trigger('click')
    expect(wrapper.get('select[aria-label="连接方式"]').element.value).toBe('proxy')
    expect(wrapper.get('select[aria-label="代理节点"]').element.value).toBe('1')
    await wrapper.get('select[aria-label="连接方式"]').setValue('direct')
    await wrapper.get('[title="自动获取机器名"]').trigger('click')
    await flush()
    await wrapper.get('form').trigger('submit')
    await flush()
    expect(updateServer).toHaveBeenCalledWith(3, expect.objectContaining({ proxy_server_id: null }))

    await wrapper.get('[data-testid="add-server"]').trigger('click')
    await wrapper.get('input[placeholder="192.168.1.100"]').setValue('100.100.1.3')
    await wrapper.get('select[aria-label="连接方式"]').setValue('proxy')
    await wrapper.get('select[aria-label="代理节点"]').setValue('1')
    await wrapper.get('input[placeholder="输入密码"]').setValue('target-secret')
    await wrapper.get('[title="自动获取机器名"]').trigger('click')
    await flush()
    expect(resolveServerHostname).toHaveBeenCalledWith(null, expect.objectContaining({ proxy_server_id: 1 }))
    await wrapper.get('form').trigger('submit')
    await flush()
    expect(createServer).toHaveBeenCalledWith(expect.objectContaining({ proxy_server_id: 1, host: '100.100.1.3' }))
    wrapper.unmount()
  })

  it('prompts for automatic machine-name lookup before adding', async () => {
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    await flush()
    await wrapper.get('[data-testid="add-server"]').trigger('click')
    expect(wrapper.get('#server-name').element.value).toBe('')
    await wrapper.get('input[placeholder="192.168.1.100"]').setValue('10.0.0.9')
    await wrapper.get('form').trigger('submit')
    await flush()

    expect(wrapper.text()).toContain('请先点击“自动获取”获取机器名')
    expect(createServer).not.toHaveBeenCalled()
    expect(resolveServerHostname).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('automatically gets a machine name from draft SSH settings and saves it', async () => {
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    await flush()
    await wrapper.get('[data-testid="add-server"]').trigger('click')
    await wrapper.get('input[placeholder="192.168.1.100"]').setValue('10.0.0.9')
    await wrapper.get('input[placeholder="输入密码"]').setValue('secret')
    await wrapper.get('[title="自动获取机器名"]').trigger('click')
    await flush()

    expect(resolveServerHostname).toHaveBeenCalledWith(null, expect.objectContaining({ host: '10.0.0.9', ssh_password: 'secret' }))
    expect(wrapper.get('#server-hostname').element.value).toBe('cloudzone.cylism.com')
    await wrapper.get('form').trigger('submit')
    await flush()
    expect(createServer).toHaveBeenCalledWith(expect.objectContaining({ hostname: 'cloudzone.cylism.com' }))
    expect(resolveServerHostname).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('directs duplicate hosts to their existing server record', async () => {
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    await flush()
    await wrapper.get('[data-testid="add-server"]').trigger('click')
    await wrapper.get('input[placeholder="192.168.1.100"]').setValue('10.0.0.1')
    await wrapper.get('[title="自动获取机器名"]').trigger('click')
    await flush()
    await wrapper.get('form').trigger('submit')
    await flush()

    expect(wrapper.text()).toContain('该主机已添加，请编辑已有服务器')
    expect(createServer).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('clears a resolved machine name when connection details change', async () => {
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    await flush()
    await wrapper.get('[title="编辑 test-srv"]').trigger('click')
    await wrapper.get('input[placeholder="192.168.1.100"]').setValue('10.0.0.10')
    expect(wrapper.get('#server-hostname').element.value).toBe('')
    await wrapper.get('[title="自动获取机器名"]').trigger('click')
    await flush()
    expect(resolveServerHostname).toHaveBeenCalledWith(1, expect.objectContaining({ host: '10.0.0.10' }))
    expect(wrapper.get('#server-hostname').element.value).toBe('cloudzone.cylism.com')
    await wrapper.get('input[placeholder="留空保持原密码"]').setValue('new-secret')
    expect(wrapper.get('#server-hostname').element.value).toBe('')
    await wrapper.get('form').trigger('submit')
    await flush()
    expect(updateServer).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('请先点击“自动获取”获取机器名')
    await wrapper.get('[title="自动获取机器名"]').trigger('click')
    await flush()
    await wrapper.get('form').trigger('submit')
    await flush()
    expect(updateServer).toHaveBeenCalledWith(1, expect.objectContaining({ hostname: 'cloudzone.cylism.com', ssh_password: 'new-secret' }))
    expect(resolveServerHostname).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('tests the current edit form without saving and shows the result in the form', async () => {
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    await flush()
    await wrapper.get('[title="编辑 test-srv"]').trigger('click')
    await wrapper.get('input[placeholder="192.168.1.100"]').setValue('10.0.0.9')
    await wrapper.get('input[placeholder="留空保持原密码"]').setValue('new-password')
    await wrapper.findAll('button').find(button => button.text() === '测试连接').trigger('click')
    await flush()

    expect(probeServer).toHaveBeenCalledWith(1, expect.objectContaining({ host: '10.0.0.9', ssh_password: 'new-password' }))
    expect(updateServer).not.toHaveBeenCalled()
    expect(wrapper.get('.server-probe-result').text()).toContain('连接成功')
    await wrapper.get('input[placeholder="192.168.1.100"]').setValue('10.0.0.10')
    expect(wrapper.find('.server-probe-result').exists()).toBe(false)
    wrapper.unmount()
  })

  it('keeps the server form open and shows its error when saving fails', async () => {
    createServer.mockRejectedValueOnce(new Error('SSH 凭据无效'))
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    await flush()
    await wrapper.get('[data-testid="add-server"]').trigger('click')
    await wrapper.get('input[placeholder="192.168.1.100"]').setValue('10.0.0.9')
    await wrapper.get('[title="自动获取机器名"]').trigger('click')
    await flush()
    await wrapper.get('form').trigger('submit')
    await flush()

    expect(wrapper.text()).toContain('SSH 凭据无效')
    expect(wrapper.get('#server-name').element.value).toBe('')
    wrapper.unmount()
  })

  it('shows resource monitoring and forwards an AbortSignal', async () => {
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    await flush()
    await wrapper.findAll('.section-tab').find(button => button.text().includes('资源监控')).trigger('click')
    await flush()
    await nextTick()

    expect(getServerResourceStats).toHaveBeenCalledWith(expect.objectContaining({ signal: expect.any(AbortSignal) }))
    const monitoringCard = wrapper.get('.resource-monitoring-card')
    expect(monitoringCard.findComponent({ name: 'SurfaceCard' }).exists()).toBe(true)
    expect(monitoringCard.get('.resource-server-name').classes()).not.toContain('cell-primary')
    expect(wrapper.text()).not.toContain('资源概览')
    expect(monitoringCard.get('.resource-monitoring-meta').text()).toContain('每 10 秒自动刷新')
    expect(monitoringCard.get('[title="刷新资源数据"]').exists()).toBe(true)
    expect(monitoringCard.get('thead').text()).toContain('操作')
    expect(monitoringCard.get('thead').text()).toContain('主机')
    expect(monitoringCard.get('thead').text()).not.toContain('采样时间')
    expect(monitoringCard.get('[data-testid="view-resource-stats-1"]').exists()).toBe(true)
    expect(monitoringCard.get('[data-testid="view-resource-stats-1"]').classes()).toContain('icon-button--table')
    expect(wrapper.text()).toContain('42.5%')
    expect(wrapper.text()).toContain('2天')
    expect(wrapper.text()).not.toContain('2 days')
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
    await wrapper.get('[title="解除 cluster-srv 的集群绑定"]').trigger('click')
    await flush()

    expect(confirm).toHaveBeenCalled()
    expect(unbindServer).toHaveBeenCalledWith(2)
    confirm.mockRestore()
    wrapper.unmount()
  })

  it('keeps the terminal overlay open when the backdrop is clicked', async () => {
    const wrapper = mount(Servers, { global: { stubs: { RouterLink: true } } })
    await flush()
    await wrapper.get('[title="打开 test-srv 的终端"]').trigger('click')
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
    await wrapper.get('[data-testid="view-resource-stats-1"]').trigger('click')
    await wrapper.get('[data-testid="view-resource-stats-2"]').trigger('click')
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
