import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import ClusterDetail from './ClusterDetail.vue'
import SelectMenu from '../../components/SelectMenu.vue'

const { get, post } = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn() }))
vi.mock('../../api/index.js', () => ({ api: { get, post } }))

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
    get.mockImplementation(path => {
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
    expect(wrapper.findAll('.section-tab').map(tab => tab.text())).toEqual(['拓扑', 'Agent 管理', '集群任务', '节点镜像源', '集群 DNS', 'Chart 仓库', '系统组件'])
    expect(wrapper.get('.section-tab.is-active').text()).toBe('拓扑')
    expect(wrapper.text()).toContain('尚未部署 K3s 集群')
    expect(wrapper.find('#deployment-server').exists()).toBe(false)
    await wrapper.get('.cluster-deployment-toolbar .btn-primary').trigger('click')
    expect(wrapper.get('[role="dialog"]').text()).toContain('部署 K3s 控制平面')
    expect(wrapper.findComponent(SelectMenu).exists()).toBe(true)
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

  it('shows agent connection details and operations in its own tab', async () => {
    get.mockImplementation(path => {
      if (path === '/clusters/9') return Promise.resolve({ ...cluster, control_server_id: 3, status: 'ready' })
      if (path === '/servers') return Promise.resolve([{ ...server, cluster_id: 9, cluster_role: 'control-plane', hostname: 'control.example.com' }])
      return Promise.resolve([])
    })
    post.mockResolvedValue({ id: 12, operation: 'agent-install', status: 'running' })
    const wrapper = await mountDetail()
    await flushPromises()
    await wrapper.get('.section-tab:nth-child(2)').trigger('click')
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
      if (path === '/clusters/9/deployments') return Promise.resolve(tasks)
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
    await wrapper.get('#deployment-address').setValue('100.81.23.123')
    await wrapper.get('[role="dialog"] .btn-primary').trigger('click')
    await flushPromises()

    expect(post).toHaveBeenCalledWith('/clusters/9/deployments', { server_id: 3, role: 'control-plane', mirror: 'official', address: '100.81.23.123' })
    expect(wrapper.vm.$route.query.tab).toBe('tasks')
    expect(wrapper.findAll('.data-table thead th').map(cell => cell.text())).toEqual(['任务', '服务器', '状态', '开始时间', '操作'])
    expect(wrapper.find('[role="log"]').exists()).toBe(false)
    expect(get.mock.calls.some(([path]) => path.includes('/logs'))).toBe(false)
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('opens historical logs in a modal and closes it without changing the task list', async () => {
    get.mockImplementation(path => {
      if (path === '/clusters/9') return Promise.resolve(cluster)
      if (path === '/servers') return Promise.resolve([server])
      if (path === '/clusters/9/deployments') return Promise.resolve([
        { id: 8, server_id: 3, role: 'control-plane', actor_name: 'alice', status: 'failed' },
        { id: 7, server_id: 3, role: 'control-plane', actor_name: 'admin', status: 'success' },
      ])
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

  it('refreshes an open log modal when a running task receives new output', async () => {
    let status = 'running'
    let messages = [{ id: 1, message: '开始部署' }]
    get.mockImplementation(path => {
      if (path === '/clusters/9') return Promise.resolve(cluster)
      if (path === '/servers') return Promise.resolve([server])
      if (path === '/clusters/9/deployments') return Promise.resolve([{ id: 7, server_id: 3, role: 'control-plane', status }])
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
      if (path === '/clusters/9/deployments') return Promise.resolve([{ id: 11, operation: 'uninstall', role: 'worker', server_id: 3, status: 'success' }])
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
    expect(post).toHaveBeenCalledWith('/clusters/9/adoptions', { server_id: 3, role: 'control-plane', address: '' })
    wrapper.unmount()
  })

  it('shows adoption tasks distinctly from deployment tasks', async () => {
    get.mockImplementation(path => {
      if (path === '/clusters/9') return Promise.resolve(cluster)
      if (path === '/servers') return Promise.resolve([server])
      if (path === '/clusters/9/deployments') return Promise.resolve([{ id: 12, operation: 'adopt', role: 'control-plane', server_id: 3, status: 'success' }])
      return Promise.resolve([])
    })
    const wrapper = await mountDetail('9', '?tab=tasks')
    await flushPromises()
    expect(wrapper.get('.data-table tbody').text()).toContain('纳管控制平面')
    expect(wrapper.get('.data-table tbody').text()).toContain('K3s 纳管')
    wrapper.unmount()
  })
})
