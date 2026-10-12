import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import ClusterDetail from './ClusterDetail.vue'
import SelectMenu from '../../components/SelectMenu.vue'

const { get, post, patch } = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), patch: vi.fn() }))
vi.mock('../../api/index.js', () => ({ api: { get, post, patch } }))

async function mountDetail(clusterID = '9', query = '') {
  const router = createRouter({ history: createMemoryHistory(), routes: [
    { path: '/cluster', component: { template: '<div />' } },
    { path: '/cluster/:clusterID', component: ClusterDetail, props: true },
  ] })
  await router.push(`/cluster/${clusterID}${query}`)
  await router.isReady()
  return mount(ClusterDetail, { props: { clusterID }, global: { plugins: [router] } })
}

const cluster = { id: 9, name: '生产集群', distribution: 'k3s', status: 'draft', created_at: '2026-10-02T08:00:00Z' }
const server = { id: 3, name: '节点一', host: '10.0.0.3', ssh_user: 'root', ssh_auth_type: 'password' }

describe('ClusterDetail', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    patch.mockReset()
    get.mockImplementation(path => {
      if (path.endsWith('/agent-status')) return Promise.resolve({ state: 'ready' })
      if (path === '/clusters/9') return Promise.resolve(cluster)
      if (path === '/servers') return Promise.resolve([server])
      return Promise.resolve([])
    })
  })

  it('shows only topology and tasks tabs, with deployment settings in a modal', async () => {
    const wrapper = await mountDetail()
    await flushPromises()

    expect(get).toHaveBeenCalledWith('/clusters/9', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(wrapper.get('h1').text()).toBe('生产集群')
    expect(wrapper.findAll('.section-tab').map(tab => tab.text())).toEqual(['拓扑', '组网', 'Agent 管理', '集群任务', '节点镜像源', '集群 DNS', 'Chart 仓库', '系统组件'])
    expect(wrapper.get('.section-tab.is-active').text()).toBe('拓扑')
    expect(wrapper.text()).toContain('尚未部署 K3s 集群')
    expect(wrapper.find('#deployment-server').exists()).toBe(false)
    await wrapper.get('.cluster-deployment-toolbar .btn-primary').trigger('click')
    expect(wrapper.get('[role="dialog"]').text()).toContain('部署 K3s 控制平面')
    expect(wrapper.findComponent(SelectMenu).exists()).toBe(true)
    wrapper.unmount()
  })

  it('sets the managed no-schedule taint through the selected cluster', async () => {
    get.mockImplementation(path => {
      if (path === '/clusters/9') return Promise.resolve({ ...cluster, control_server_id: 3 })
      if (path === '/servers') return Promise.resolve([{ ...server, cluster_id: 9, cluster_role: 'control-plane', k8s_node_name: 'control' }])
      if (path === '/nodes?cluster_id=9') return Promise.resolve([{ name: 'control', default_no_schedule: false }])
      if (path === '/nodes/control/scheduling?cluster_id=9') return Promise.resolve({ name: 'control', default_no_schedule: false })
      return Promise.resolve([])
    })
    patch.mockResolvedValue({ name: 'control', default_no_schedule: true })
    const wrapper = await mountDetail()
    await flushPromises()
    await wrapper.get('[aria-label="设置 节点一 的调度"]').trigger('click')
    await flushPromises()
    await wrapper.get('[role="dialog"] input[type="checkbox"]').setValue(true)
    await wrapper.get('[role="dialog"] .btn-primary').trigger('click')
    await flushPromises()
    expect(patch).toHaveBeenCalledWith('/nodes/control/scheduling?cluster_id=9', { default_no_schedule: true })
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    expect(wrapper.get('.topology-scheduling-badge').text()).toBe('默认不调度')
    wrapper.unmount()
  })

  it('shows an error when the cluster cannot be found', async () => {
    get.mockRejectedValueOnce(new Error('集群不存在'))
    const wrapper = await mountDetail('99')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('集群不存在')
    expect(wrapper.find('.cluster-detail-content').exists()).toBe(false)
    wrapper.unmount()
  })

  it('shows networking only for K3s and compares its node IP', async () => {
    get.mockImplementation(path => {
      if (path === '/clusters/9') return Promise.resolve({ ...cluster, network_mode: 'tailscale', control_server_id: 3 })
      if (path === '/servers') return Promise.resolve([{ ...server, cluster_id: 9, cluster_role: 'control-plane', k8s_node_name: 'control' }])
      if (path === '/nodes?cluster_id=9') return Promise.resolve([{ name: 'control', internal_ip: '100.81.23.123' }])
      if (path === '/servers/3/tailscale') return Promise.resolve({ server_id: 3, status: 'online', ip: '100.81.23.123' })
      return Promise.resolve([])
    })
    const wrapper = await mountDetail('9', '?tab=networking')
    await flushPromises()
    expect(wrapper.text()).toContain('部署网络：Tailscale')
    expect(wrapper.text()).toContain('IP 一致')
    wrapper.unmount()

    get.mockImplementation(path => path === '/clusters/9'
      ? Promise.resolve({ ...cluster, distribution: 'kubernetes' })
      : Promise.resolve([]))
    const kubernetes = await mountDetail('9', '?tab=networking')
    await flushPromises()
    expect(kubernetes.findAll('.section-tab').map(tab => tab.text())).not.toContain('组网')
    expect(kubernetes.get('.section-tab.is-active').text()).toBe('拓扑')
    kubernetes.unmount()
  })

  it('shows agent connection details and operations in its own tab', async () => {
    get.mockImplementation(path => {
      if (path === '/clusters/9') return Promise.resolve({ ...cluster, control_server_id: 3, status: 'ready' })
      if (path === '/servers') return Promise.resolve([{ ...server, cluster_id: 9, cluster_role: 'control-plane', hostname: 'control.example.com' }])
      return Promise.resolve([])
    })
    post.mockResolvedValue({ id: 12, operation: 'agent-install', status: 'running' })
    const wrapper = await mountDetail()
    await flushPromises()
    await wrapper.findAll('.section-tab').find(tab => tab.text() === 'Agent 管理').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('0.0.0.0:9443')
    expect(wrapper.text()).toContain('127.0.0.1:6443')
    expect(wrapper.text()).toContain('cylism-cluster-agent')
    await wrapper.get('.agent-actions .btn-primary').trigger('click')
    await flushPromises()
    expect(post).toHaveBeenCalledWith('/clusters/9/agent-operations', { server_id: 3, operation: 'install' })
    expect(wrapper.vm.$route.query.tab).toBe('tasks')
    wrapper.unmount()
  })

  it('does not repeat an IP when the stored server name equals its host', async () => {
    get.mockImplementation(path => {
      if (path === '/clusters/9') return Promise.resolve(cluster)
      if (path === '/servers') return Promise.resolve([
        { id: 3, name: '117.55.235.170', host: '117.55.235.170' },
        { id: 4, name: '10.0.0.4', hostname: 'cloudzone.example.com', host: '10.0.0.4' },
      ])
      return Promise.resolve([])
    })
    const wrapper = await mountDetail()
    await flushPromises()
    await wrapper.get('.cluster-deployment-toolbar .btn-primary').trigger('click')
    expect(wrapper.findComponent(SelectMenu).props('options')).toEqual([
      { value: '3', label: '117.55.235.170' },
      { value: '4', label: 'cloudzone.example.com · 10.0.0.4' },
    ])
    wrapper.unmount()
  })

  it('starts deployment in the modal and keeps its task log collapsed', async () => {
    let tasks = []
    get.mockImplementation(path => {
      if (path === '/clusters/9') return Promise.resolve(cluster)
      if (path === '/servers') return Promise.resolve([server])
      if (path === '/clusters/9/deployments?limit=10&offset=0') return Promise.resolve({ items: tasks, total: tasks.length })
      if (path.includes('/logs')) return Promise.resolve([{ id: 11, created_at: '2026-10-02T08:00:00Z', message: '开始部署控制平面' }])
      return Promise.resolve([])
    })
    post.mockImplementation(() => {
      const task = { id: 7, cluster_id: 9, server_id: 3, role: 'control-plane', status: 'running', actor_name: 'admin' }
      tasks = [task]
      return Promise.resolve(task)
    })
    const wrapper = await mountDetail()
    await flushPromises()
    await wrapper.get('.cluster-deployment-toolbar .btn-primary').trigger('click')
    wrapper.findAllComponents(SelectMenu).find(menu => menu.props('id') === 'deployment-server').vm.$emit('update:modelValue', '3')
    await flushPromises()
    expect(wrapper.get('[role="dialog"]').text()).toContain('服务器地址')
    expect(wrapper.get('[role="dialog"]').text()).toContain('10.0.0.3')
    await wrapper.get('[role="dialog"] .btn-primary').trigger('click')
    await flushPromises()

    expect(post).toHaveBeenCalledWith('/clusters/9/deployments', { server_id: 3, role: 'control-plane', mirror: 'official', network_mode: 'public' })
    expect(wrapper.vm.$route.query.tab).toBe('tasks')
    expect(wrapper.findAll('.data-table thead th').map(cell => cell.text())).toEqual(['任务', '服务器', '状态', '开始时间', '操作'])
    expect(wrapper.find('[role="log"]').exists()).toBe(false)
    expect(get.mock.calls.some(([path]) => path.includes('/logs'))).toBe(false)
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('lets a K3s worker choose the control-plane IP used to join', async () => {
    get.mockImplementation(path => {
      if (path === '/clusters/9') return Promise.resolve({ ...cluster, status: 'ready', control_server_id: 3, control_address: '43.139.193.176', network_mode: 'public' })
      if (path === '/servers') return Promise.resolve([
        { ...server, host: '43.139.193.176', cluster_id: 9, cluster_role: 'control-plane' },
        { id: 4, name: 'worker', host: '100.67.232.15', ssh_user: 'root', ssh_auth_type: 'password' },
      ])
      if (path === '/servers/3/tailscale' || path === '/servers/4/tailscale') return Promise.resolve({ status: 'online', ip: path.includes('/3/') ? '100.81.23.123' : '100.67.232.15' })
      return Promise.resolve([])
    })
    post.mockResolvedValue({ id: 20, operation: 'deploy', status: 'running' })
    const wrapper = await mountDetail()
    await flushPromises()
    await wrapper.get('.cluster-deployment-toolbar .btn-primary').trigger('click')
    await flushPromises()
    const addressMenu = wrapper.findAllComponents(SelectMenu).find(menu => menu.props('id') === 'deployment-join-address')
    expect(addressMenu.props('options')).toEqual([
      { value: 'server', label: '服务器 IP · 43.139.193.176', disabled: false },
      { value: 'tailscale', label: 'Tailscale IP · 100.81.23.123', disabled: false },
    ])
    wrapper.findAllComponents(SelectMenu).find(menu => menu.props('id') === 'deployment-server').vm.$emit('update:modelValue', '4')
    addressMenu.vm.$emit('update:modelValue', 'tailscale')
    await flushPromises()
    await wrapper.get('[role="dialog"] .btn-primary').trigger('click')
    await flushPromises()
    expect(post).toHaveBeenCalledWith('/clusters/9/deployments', {
      server_id: 4, role: 'worker', mirror: 'official', network_mode: 'public', join_address_type: 'tailscale',
    })
    wrapper.unmount()
  })

  it('shows the worker Tailscale check failure returned by the API', async () => {
    get.mockImplementation(path => {
      if (path === '/clusters/9') return Promise.resolve({ ...cluster, status: 'ready', control_server_id: 3, network_mode: 'tailscale' })
      if (path === '/servers') return Promise.resolve([
        { ...server, cluster_id: 9, cluster_role: 'control-plane' },
        { id: 4, name: 'worker', host: '10.0.0.4', ssh_user: 'root', ssh_auth_type: 'password' },
      ])
      if (path === '/servers/3/tailscale') return Promise.resolve({ status: 'online', ip: '100.81.23.123' })
      if (path === '/servers/4/tailscale') return Promise.resolve({ status: 'unreachable', error_type: 'tailscale_daemon', error: 'SSH 已连接，Tailscale 已安装，但 tailscaled 服务未运行或不可用' })
      return Promise.resolve([])
    })
    const wrapper = await mountDetail()
    await flushPromises()
    await wrapper.get('.cluster-deployment-toolbar .btn-primary').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="dialog"]').text()).toContain('请先选择服务器')
    wrapper.findAllComponents(SelectMenu).find(menu => menu.props('id') === 'deployment-server').vm.$emit('update:modelValue', '4')
    await flushPromises()
    expect(wrapper.get('[role="dialog"]').text()).toContain('SSH 已连接，Tailscale 已安装，但 tailscaled 服务未运行或不可用')
    expect(wrapper.get('[role="dialog"] .btn-primary').attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })

  it('opens historical logs in a modal and closes it without changing the task list', async () => {
    get.mockImplementation(path => {
      if (path === '/clusters/9') return Promise.resolve(cluster)
      if (path === '/servers') return Promise.resolve([server])
      if (path === '/clusters/9/deployments?limit=10&offset=0') return Promise.resolve({ items: [
        { id: 8, server_id: 3, role: 'control-plane', actor_name: 'alice', status: 'failed' },
        { id: 7, server_id: 3, role: 'control-plane', actor_name: 'admin', status: 'success' },
      ], total: 2 })
      if (path.includes('/deployments/8/logs')) return Promise.resolve([{ id: 15, message: '部署失败' }])
      if (path.includes('/deployments/7/logs')) return Promise.resolve([{ id: 12, message: '安装成功' }])
      return Promise.resolve([])
    })
    const wrapper = await mountDetail('9', '?tab=tasks')
    await flushPromises()
    expect(wrapper.findAll('.data-table thead th').map(cell => cell.text())).toEqual(['任务', '服务器', '状态', '开始时间', '操作'])
    expect(wrapper.text()).not.toContain('alice')
    expect(wrapper.find('[role="log"]').exists()).toBe(false)
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    expect(get.mock.calls.some(([path]) => path.includes('/logs'))).toBe(false)
    await wrapper.get('[aria-label="查看任务 #8 日志"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="dialog"]').attributes('aria-modal')).toBe('true')
    expect(wrapper.get('[role="dialog"]').text()).toContain('任务 #8 · 执行日志')
    expect(wrapper.get('[role="log"]').text()).toContain('部署失败')
    await wrapper.get('.base-modal-actions .btn').trigger('click')
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    expect(wrapper.find('[role="log"]').exists()).toBe(false)
    await wrapper.get('[aria-label="查看任务 #7 日志"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="log"]').text()).toContain('安装成功')
    await wrapper.get('.base-modal-close').trigger('click')
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    expect(wrapper.findAll('.data-table tbody tr')).toHaveLength(2)
    wrapper.unmount()
  })

  it('loads cluster tasks one page at a time', async () => {
    const tasks = Array.from({ length: 17 }, (_, index) => ({ id: 17 - index, server_id: 3, status: 'success' }))
    get.mockImplementation(path => {
      if (path === '/clusters/9') return Promise.resolve({ ...cluster, control_server_id: 3, status: 'ready' })
      if (path === '/servers') return Promise.resolve([{ ...server, cluster_id: 9, cluster_role: 'control-plane' }])
      if (path === '/clusters/9/deployments?limit=10&offset=0') return Promise.resolve({ items: tasks.slice(0, 10), total: 17, latest_agent_task: { id: 1, operation: 'agent-install', server_id: 3, status: 'success' }, running: false })
      if (path === '/clusters/9/deployments?limit=10&offset=10') return Promise.resolve({ items: tasks.slice(10), total: 17, latest_agent_task: { id: 1, operation: 'agent-install', server_id: 3, status: 'success' }, running: false })
      return Promise.resolve([])
    })
    const wrapper = await mountDetail('9', '?tab=tasks')
    await flushPromises()
    expect(wrapper.findAll('.data-table tbody tr')).toHaveLength(10)
    expect(wrapper.get('.task-pagination').text()).toContain('1 / 2 · 共 17 条')
    await wrapper.get('.task-pagination button:last-child').trigger('click')
    await flushPromises()
    expect(get).toHaveBeenCalledWith('/clusters/9/deployments?limit=10&offset=10', undefined)
    expect(wrapper.findAll('.data-table tbody tr')).toHaveLength(7)
    expect(wrapper.text()).toContain('#1')
    expect(wrapper.get('.task-pagination').text()).toContain('2 / 2')
    await wrapper.findAll('.section-tab').find(tab => tab.text() === 'Agent 管理').trigger('click')
    await flushPromises()
    expect(wrapper.get('.agent-management-toolbar .badge').text()).toBe('已配置')
    wrapper.unmount()
  })

  it('passes the selected cluster to DNS and system component requests', async () => {
    get.mockImplementation(path => {
      if (path === '/clusters/9/agent-status') return Promise.resolve({ state: 'ready' })
      if (path === '/clusters/9') return Promise.resolve(cluster)
      if (path === '/servers') return Promise.resolve([server])
      if (path === '/cluster-dns?cluster_id=9') return Promise.resolve({ forwarding: [], pods: [], history: [], active_policy: null })
      return Promise.resolve([])
    })
    const wrapper = await mountDetail()
    await flushPromises()
    await wrapper.findAll('.section-tab').find(tab => tab.text() === '集群 DNS').trigger('click')
    await flushPromises()
    await vi.waitFor(() => expect(get).toHaveBeenCalledWith('/cluster-dns?cluster_id=9', expect.objectContaining({ signal: expect.any(AbortSignal) })))
    await wrapper.findAll('.section-tab').find(tab => tab.text() === '系统组件').trigger('click')
    await flushPromises()
    await vi.waitFor(() => expect(get).toHaveBeenCalledWith('/system-components?cluster_id=9', expect.objectContaining({ signal: expect.any(AbortSignal) })))
    expect(get).toHaveBeenCalledWith('/nodes?cluster_id=9', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    wrapper.unmount()
  })

  it('keeps Agent management available while gated tabs wait for Agent', async () => {
    get.mockImplementation(path => {
      if (path === '/clusters/9/agent-status') return Promise.resolve({ state: 'unreachable' })
      if (path === '/clusters/9') return Promise.resolve({ ...cluster, control_server_id: 3 })
      if (path === '/servers') return Promise.resolve([{ ...server, cluster_id: 9, cluster_role: 'control-plane' }])
      return Promise.resolve([])
    })
    const wrapper = await mountDetail()
    await flushPromises()
    await wrapper.findAll('.section-tab').find(tab => tab.text() === '集群 DNS').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('Agent 尚未安装或当前无法连接')
    expect(get.mock.calls.some(([path]) => path === '/cluster-dns?cluster_id=9')).toBe(false)
    await wrapper.findAll('.section-tab').find(tab => tab.text() === 'Agent 管理').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('部署 Agent')
    wrapper.unmount()
  })

  it('refreshes an open log modal when a running task receives new output', async () => {
    let status = 'running'
    let messages = [{ id: 1, message: '开始部署' }]
    get.mockImplementation(path => {
      if (path === '/clusters/9') return Promise.resolve(cluster)
      if (path === '/servers') return Promise.resolve([server])
      if (path === '/clusters/9/deployments?limit=10&offset=0') return Promise.resolve({ items: [{ id: 7, server_id: 3, role: 'control-plane', status }], total: 1 })
      if (path.includes('/logs')) return Promise.resolve(messages)
      return Promise.resolve([])
    })
    const wrapper = await mountDetail('9', '?tab=tasks')
    await flushPromises()
    await wrapper.get('[aria-label="查看任务 #7 日志"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="log"]').text()).toContain('开始部署')

    status = 'success'
    messages = [{ id: 1, message: '开始部署' }, { id: 2, message: '安装成功' }]
    await wrapper.get('[aria-label="刷新任务"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="dialog"]').text()).toContain('成功')
    expect(wrapper.get('[role="log"]').text()).toContain('安装成功')
    wrapper.unmount()
  })

  it('groups control plane and worker nodes in the topology', async () => {
    get.mockImplementation(path => {
      if (path === '/clusters/9') return Promise.resolve({ ...cluster, control_server_id: 3, status: 'ready' })
      if (path === '/servers') return Promise.resolve([
        { ...server, cluster_id: 9, cluster_role: 'control-plane', hostname: 'control.example.com' },
        { id: 4, name: 'worker-a', host: '10.0.0.4', cluster_id: 9, cluster_role: 'worker' },
        { id: 5, name: 'worker-b', host: '10.0.0.5', cluster_id: 9, cluster_role: 'worker' },
      ])
      return Promise.resolve([])
    })
    const wrapper = await mountDetail()
    await flushPromises()
    expect(wrapper.findAll('.topology-node')).toHaveLength(3)
    expect(wrapper.get('.topology-node--control').text()).toContain('control.example.com')
    expect(wrapper.get('.topology-workers-heading').text()).toContain('工作节点 2')
    expect(wrapper.get('.cluster-deployment-toolbar .btn-primary').text()).toBe('添加工作节点')
    wrapper.unmount()
  })

  it('uninstalls workers before the control plane through a confirmed task', async () => {
    get.mockImplementation(path => {
      if (path === '/clusters/9') return Promise.resolve({ ...cluster, control_server_id: 3, status: 'ready' })
      if (path === '/servers') return Promise.resolve([
        { ...server, cluster_id: 9, cluster_role: 'control-plane' },
        { id: 4, name: '工作节点', host: '10.0.0.4', cluster_id: 9, cluster_role: 'worker' },
      ])
      return Promise.resolve([])
    })
    post.mockResolvedValueOnce({ id: 11, operation: 'uninstall', status: 'running' })
    const wrapper = await mountDetail()
    await flushPromises()
    expect(wrapper.get('[aria-label="卸载 节点一"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[aria-label="卸载 工作节点"]').trigger('click')
    expect(wrapper.get('[role="dialog"]').text()).toContain('K3s 官方卸载脚本')
    await wrapper.get('[role="dialog"] .btn-danger').trigger('click')
    await flushPromises()
    expect(post).toHaveBeenCalledWith('/clusters/9/nodes/4/uninstall')
    wrapper.unmount()
  })

  it('labels uninstall records clearly in the task list', async () => {
    get.mockImplementation(path => {
      if (path === '/clusters/9') return Promise.resolve(cluster)
      if (path === '/servers') return Promise.resolve([server])
      if (path === '/clusters/9/deployments?limit=10&offset=0') return Promise.resolve({ items: [{ id: 11, operation: 'uninstall', role: 'worker', server_id: 3, status: 'success' }], total: 1 })
      return Promise.resolve([])
    })
    const wrapper = await mountDetail('9', '?tab=tasks')
    await flushPromises()
    expect(wrapper.get('.data-table tbody').text()).toContain('卸载工作节点')
    expect(wrapper.get('.data-table tbody').text()).toContain('K3s 卸载')
    wrapper.unmount()
  })

  it('starts adopting an existing control plane without installing K3s', async () => {
    post.mockResolvedValueOnce({ id: 12, operation: 'adopt', role: 'control-plane', status: 'running' })
    const wrapper = await mountDetail()
    await flushPromises()
    await wrapper.get('.cluster-toolbar-actions .btn:not(.btn-primary)').trigger('click')
    expect(wrapper.get('[role="dialog"]').text()).toContain('不会安装或修改目标机器')
    wrapper.findAllComponents(SelectMenu).find(menu => menu.props('id') === 'adoption-server').vm.$emit('update:modelValue', '3')
    await flushPromises()
    await wrapper.get('[role="dialog"] .btn-primary').trigger('click')
    await flushPromises()
    expect(post).toHaveBeenCalledWith('/clusters/9/adoptions', { server_id: 3, role: 'control-plane' })
    wrapper.unmount()
  })

  it('offers K3s service restart for the control plane and workers', async () => {
    get.mockImplementation(path => {
      if (path === '/clusters/9') return Promise.resolve({ ...cluster, status: 'ready', control_server_id: 3 })
      if (path === '/servers') return Promise.resolve([
        { ...server, cluster_id: 9, cluster_role: 'control-plane', k8s_node_name: 'control' },
        { id: 4, name: '工作节点', host: '10.0.0.4', cluster_id: 9, cluster_role: 'worker', k8s_node_name: 'worker' },
      ])
      if (path === '/clusters/9/deployments?limit=10&offset=0') return Promise.resolve({ items: [], total: 0 })
      return Promise.resolve([])
    })
    post.mockResolvedValue({ id: 13, operation: 'restart', status: 'running' })
    const wrapper = await mountDetail()
    await flushPromises()
    expect(wrapper.findAll('[title="重启 K3s 服务"]')).toHaveLength(2)
    await wrapper.get('[aria-label="重启 节点一 的 K3s 服务"]').trigger('click')
    expect(wrapper.get('[role="dialog"]').text()).toContain('k3s 服务')
    await wrapper.get('[role="dialog"] .btn-primary').trigger('click')
    await flushPromises()
    expect(post).toHaveBeenCalledWith('/clusters/9/nodes/3/restart')
    expect(wrapper.vm.$route.query.tab).toBe('tasks')
    wrapper.unmount()

    const workerView = await mountDetail()
    await flushPromises()
    await workerView.get('[aria-label="重启 工作节点 的 K3s 服务"]').trigger('click')
    expect(workerView.get('[role="dialog"]').text()).toContain('k3s-agent 服务')
    await workerView.get('[role="dialog"] .btn-primary').trigger('click')
    await flushPromises()
    expect(post).toHaveBeenCalledWith('/clusters/9/nodes/4/restart')
    workerView.unmount()
  })

  it('shows adoption tasks distinctly from deployment tasks', async () => {
    get.mockImplementation(path => {
      if (path === '/clusters/9') return Promise.resolve(cluster)
      if (path === '/servers') return Promise.resolve([server])
      if (path === '/clusters/9/deployments?limit=10&offset=0') return Promise.resolve({ items: [{ id: 12, operation: 'adopt', role: 'control-plane', server_id: 3, status: 'success' }], total: 1 })
      return Promise.resolve([])
    })
    const wrapper = await mountDetail('9', '?tab=tasks')
    await flushPromises()
    expect(wrapper.get('.data-table tbody').text()).toContain('纳管控制平面')
    expect(wrapper.get('.data-table tbody').text()).toContain('K3s 纳管')
    wrapper.unmount()
  })
})
