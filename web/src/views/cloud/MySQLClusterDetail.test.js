import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import MySQLClusterDetail from './MySQLClusterDetail.vue'
import { getClusters } from '../../api/clusters.js'

const api = vi.hoisted(() => ({
  getMySQLCluster: vi.fn(), getMySQLClusterNodes: vi.fn(), getMySQLClusterRuntime: vi.fn(),
  getMySQLClusterConfig: vi.fn(), updateMySQLClusterConfig: vi.fn(), restartMySQLClusterForConfig: vi.fn(),
  updateMySQLClusterResourcePool: vi.fn(),
  getMySQLClusterTasks: vi.fn(), getMySQLClusterTaskLogs: vi.fn(), getMySQLOperator: vi.fn(),
  installMySQLOperator: vi.fn(), cancelMySQLOperator: vi.fn(), uninstallMySQLOperator: vi.fn(), deployMySQLCluster: vi.fn(),
  switchMySQLClusterPrimary: vi.fn(),
  pauseMySQLClusterDeployment: vi.fn(),
  retryMySQLClusterShutdown: vi.fn(),
  getMySQLClusterCleanup: vi.fn(), cleanupMySQLCluster: vi.fn(),
  preflightMySQLCluster: vi.fn(), retryMySQLClusterPod: vi.fn(), pullMySQLClusterPodImages: vi.fn(),
  addMySQLClusterInstance: vi.fn(), addMySQLClusterRouter: vi.fn(), repairMySQLClusterRouterPlacement: vi.fn(), retireMySQLClusterRouter: vi.fn(),
}))
vi.mock('../../api/databases.js', () => api)
vi.mock('../../api/clusters.js', () => ({ getClusters: vi.fn().mockResolvedValue([{ id: 2, name: '国内', distribution: 'k3s' }]) }))

let wrapper
const taskResult = (items, extra = {}) => ({ items, total: items.length, operator_uninstall_pending: items.some(task => task.kind === 'uninstall_operator' && task.status === 'running'), ...extra })
beforeEach(() => {
  vi.resetAllMocks()
  getClusters.mockResolvedValue([{ id: 2, name: '国内', distribution: 'k3s' }])
  vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] })
  api.getMySQLCluster.mockResolvedValue({ id: 1, name: '自用', cluster_id: 2, status: 'draft' })
  api.getMySQLClusterNodes.mockResolvedValue([])
  api.getMySQLClusterConfig.mockResolvedValue({ mycnf: '', restart_required: false })
  api.getMySQLClusterTasks.mockResolvedValue(taskResult([]))
  api.getMySQLOperator.mockResolvedValue({ state: 'missing' })
  api.getMySQLClusterTaskLogs.mockResolvedValue([{ id: 1, message: '等待 Helm 安装 MySQL Operator' }])
  api.getMySQLClusterCleanup.mockResolvedValue({ cluster_resource: false, workloads: [], pods: [], claims: [] })
  api.preflightMySQLCluster.mockResolvedValue({ ready: true, nodes: [], storage_reason: '', images: '', disk_capacity: '' })
})

describe('MySQL configuration', () => {
  it('saves persistent configuration and confirms a rolling restart separately', async () => {
    api.getMySQLCluster.mockResolvedValue({ id: 1, name: '自用', cluster_id: 2, status: 'ready', database_instances: 1, database_nodes: ['node-a'] })
    api.getMySQLOperator.mockResolvedValue({ state: 'ready' })
    api.getMySQLClusterRuntime.mockResolvedValue({ status: 'ready', pods: [] })
    api.updateMySQLClusterConfig.mockResolvedValue({ id: 1, name: '自用', cluster_id: 2, status: 'ready', database_instances: 1, database_nodes: ['node-a'] })
    api.getMySQLClusterConfig.mockResolvedValueOnce({ mycnf: '', restart_required: false }).mockResolvedValue({ mycnf: '[mysqld]\ninnodb_buffer_pool_size=128M\n', restart_required: true })
    wrapper = mountPage()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === 'MySQL 配置').trigger('click')
    await flushPromises()
    const editor = document.body.querySelector('textarea[aria-label="MySQL my.cnf 附加配置"]')
    editor.value = '[mysqld]\ninnodb_buffer_pool_size=128M'
    editor.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()
    ;[...document.body.querySelectorAll('button')].find(button => button.textContent === '保存配置').click()
    await flushPromises()
    expect(api.updateMySQLClusterConfig).toHaveBeenCalledWith('1', { mycnf: '[mysqld]\ninnodb_buffer_pool_size=128M', cpu_limit: '', memory_limit: '' })
    expect(api.restartMySQLClusterForConfig).not.toHaveBeenCalled()
    ;[...document.body.querySelectorAll('button')].find(button => button.textContent === '滚动重启').click()
    expect(api.restartMySQLClusterForConfig).not.toHaveBeenCalled()
    await flushPromises()
    expect(document.body.querySelector('textarea[aria-label="MySQL my.cnf 附加配置"]')).toBeNull()
    expect(wrapper.text()).toContain('滚动重启 MySQL')
    await wrapper.get('[data-testid="confirm-dialog-cancel"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="confirm-dialog-confirm"]').exists()).toBe(false)
    expect(document.body.querySelector('textarea[aria-label="MySQL my.cnf 附加配置"]').value).toBe('[mysqld]\ninnodb_buffer_pool_size=128M\n')
    ;[...document.body.querySelectorAll('button')].find(button => button.textContent === '滚动重启').click()
    await flushPromises()
    await wrapper.get('[data-testid="confirm-dialog-confirm"]').trigger('click')
    await flushPromises()
    expect(api.restartMySQLClusterForConfig).toHaveBeenCalledWith('1')
  })

  it('saves a 1C1G limit and shows the generated MySQL settings', async () => {
    api.getMySQLCluster.mockResolvedValue({ id: 1, name: '自用', cluster_id: 2, status: 'ready', database_instances: 1, database_nodes: ['node-a'] })
    api.getMySQLOperator.mockResolvedValue({ state: 'ready' })
    api.getMySQLClusterRuntime.mockResolvedValue({ status: 'ready', pods: [] })
    api.updateMySQLClusterConfig.mockResolvedValue({ id: 1, name: '自用', cluster_id: 2, status: 'ready', cpu_limit: '1', memory_limit: '1Gi' })
    api.getMySQLClusterConfig.mockResolvedValueOnce({ mycnf: '', restart_required: false }).mockResolvedValue({ mycnf: '', cpu_limit: '1', memory_limit: '1Gi', effective_mycnf: '[mysqld]\ninnodb_buffer_pool_size=256M\n', restart_required: true })
    wrapper = mountPage()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === 'MySQL 配置').trigger('click')
    await flushPromises()
    const cpu = document.body.querySelector('input[aria-label="每实例 CPU 上限"]')
    const memory = document.body.querySelector('input[aria-label="每实例内存上限"]')
    cpu.value = '1'
    cpu.dispatchEvent(new Event('input', { bubbles: true }))
    memory.value = '1'
    memory.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()
    ;[...document.body.querySelectorAll('button')].find(button => button.textContent === '保存配置').click()
    await flushPromises()
    expect(api.updateMySQLClusterConfig).toHaveBeenCalledWith('1', { mycnf: '', cpu_limit: '1', memory_limit: '1Gi' })
    expect(document.body.querySelector('textarea[aria-label="最终生效的 MySQL 配置"]').value).toContain('innodb_buffer_pool_size=256M')
    expect(api.restartMySQLClusterForConfig).not.toHaveBeenCalled()
  })
})

describe('MySQL deployment pause', () => {
  it.each([
    ['scale_database', ' MySQL'],
    ['scale_router', ' Router'],
  ])('confirms withdrawal of a running %s task', async (kind, component) => {
    api.getMySQLCluster.mockResolvedValue({ id: 1, name: '自用', cluster_id: 2, status: 'starting', database_instances: 2, router_instances: 1 })
    api.getMySQLOperator.mockResolvedValue({ state: 'ready' })
    api.getMySQLClusterRuntime.mockResolvedValue({ status: 'starting', pods: [] })
    api.getMySQLClusterTasks.mockResolvedValueOnce(taskResult([{ id: 2, kind, status: 'running' }])).mockResolvedValue(taskResult([{ id: 2, kind, status: 'paused' }]))
    api.pauseMySQLClusterDeployment.mockResolvedValue({ cluster: { status: 'starting' } })
    wrapper = mountPage()
    await flushPromises()
    expect(wrapper.findAll('button').some(button => button.text() === '暂停初次部署')).toBe(false)
    await wrapper.findAll('button').find(button => button.text() === '撤回本次追加').trigger('click')
    expect(wrapper.text()).toContain(`${component} 目标数量恢复到本次追加前`)
    expect(api.pauseMySQLClusterDeployment).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="confirm-dialog-confirm"]').trigger('click')
    await flushPromises()
    expect(api.pauseMySQLClusterDeployment).toHaveBeenCalledWith('1')
  })

  it('offers a confirmed Operator cleanup retry when deletion is stalled', async () => {
    api.getMySQLCluster.mockResolvedValue({ id: 1, name: '自用', cluster_id: 2, resource_name: 'mysql-1', status: 'paused' })
    api.getMySQLOperator.mockResolvedValue({ state: 'ready' })
    api.getMySQLClusterCleanup.mockResolvedValueOnce({ cluster_resource: true, deletion_started_at: '2026-10-14T02:18:29Z', finalizers: ['mysql.oracle.com/cluster'], retry_available: true, workloads: ['mysql-1'], pods: [], claims: ['datadir-mysql-1-0'] }).mockResolvedValue({ cluster_resource: true, deletion_started_at: '2026-10-14T02:18:29Z', finalizers: ['mysql.oracle.com/cluster'], retry_available: false, workloads: ['mysql-1'], pods: [], claims: ['datadir-mysql-1-0'] })
    api.retryMySQLClusterShutdown.mockResolvedValue({ task: { id: 3, kind: 'retry_shutdown', status: 'success' } })
    wrapper = mountPage()
    await flushPromises()
    expect(wrapper.text()).toContain('暂停中')
    expect(wrapper.text()).toContain('等待超时，可重试 Operator 清理')
    await wrapper.findAll('button').find(button => button.text() === '重试 Operator 清理').trigger('click')
    expect(api.retryMySQLClusterShutdown).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="confirm-dialog-confirm"]').trigger('click')
    await flushPromises()
    expect(api.retryMySQLClusterShutdown).toHaveBeenCalledWith('1')
    expect(wrapper.text()).toContain('集群任务')
    expect(wrapper.text()).toContain('mysql.oracle.com/cluster')
    expect(document.body.textContent).toContain('任务 #3 · 重试 Operator 清理')
    expect(wrapper.findAll('button').find(button => button.text() === '重试 Operator 清理')).toBeUndefined()
  })
  it('confirms pause and shows the retained deployment as paused', async () => {
    api.getMySQLCluster.mockResolvedValueOnce({ id: 1, name: '自用', cluster_id: 2, status: 'starting' }).mockResolvedValue({ id: 1, name: '自用', cluster_id: 2, status: 'paused' })
    api.getMySQLOperator.mockResolvedValue({ state: 'ready' })
    api.getMySQLClusterRuntime.mockResolvedValue({ status: 'starting', pods: [] })
    api.pauseMySQLClusterDeployment.mockResolvedValue({ cluster: { status: 'paused' } })
    api.getMySQLClusterTasks.mockResolvedValue(taskResult([{ id: 1, kind: 'deploy', status: 'running' }]))
    wrapper = mountPage()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '暂停初次部署').trigger('click')
    expect(api.pauseMySQLClusterDeployment).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="confirm-dialog-confirm"]').trigger('click')
    await flushPromises()
    expect(api.pauseMySQLClusterDeployment).toHaveBeenCalledWith('1')
    expect(wrapper.text()).toContain('部署已暂停，可重新部署')
    expect(wrapper.text()).toContain('部署 MySQL')
  })

  it('pauses an existing workload and clears retained volumes only after explicit confirmation', async () => {
    api.getMySQLCluster.mockResolvedValueOnce({ id: 1, name: '自用', cluster_id: 2, resource_name: 'mysql-1', status: 'starting' }).mockResolvedValue({ id: 1, name: '自用', cluster_id: 2, resource_name: 'mysql-1', status: 'paused' })
    api.getMySQLOperator.mockResolvedValue({ state: 'ready' })
    api.getMySQLClusterRuntime.mockResolvedValue({ status: 'starting', pods: [{ name: 'mysql-1-0', node_name: 'node-a' }] })
    api.getMySQLClusterCleanup.mockResolvedValueOnce({ cluster_resource: true, workloads: ['mysql-1'], pods: ['mysql-1-0'], claims: ['data-mysql-1-0'] }).mockResolvedValueOnce({ cluster_resource: false, workloads: ['mysql-1'], pods: ['mysql-1-0'], claims: ['data-mysql-1-0'] }).mockResolvedValueOnce({ cluster_resource: false, workloads: [], pods: [], claims: ['data-mysql-1-0'] }).mockResolvedValue({ cluster_resource: false, workloads: [], pods: [], claims: [] })
    api.pauseMySQLClusterDeployment.mockResolvedValue({ cluster: { status: 'paused' } })
    api.getMySQLClusterTasks.mockResolvedValue(taskResult([{ id: 1, kind: 'deploy', status: 'running' }]))
    api.cleanupMySQLCluster.mockResolvedValue({ cluster_resource: false, workloads: [], pods: [], claims: [] })
    wrapper = mountPage()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '暂停初次部署').trigger('click')
    await wrapper.get('[data-testid="confirm-dialog-confirm"]').trigger('click')
    await flushPromises()
    expect(api.pauseMySQLClusterDeployment).toHaveBeenCalledWith('1')
    expect(wrapper.text()).toContain('Operator 正在停止数据库负载')
    expect(wrapper.findAll('button').find(button => button.text() === '部署 MySQL')).toBeUndefined()
    await wrapper.find('button[aria-label="刷新运行状态"]').trigger('click')
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '清理残留负载').trigger('click')
    await wrapper.get('[data-testid="confirm-dialog-confirm"]').trigger('click')
    await flushPromises()
    expect(api.cleanupMySQLCluster).toHaveBeenCalledWith('1', 'mysql-1', false)
    expect(wrapper.text()).not.toContain('部署 MySQL')
    await wrapper.findAll('button').find(button => button.text() === '清理数据卷').trigger('click')
    await wrapper.get('[data-testid="confirm-dialog-confirm"]').trigger('click')
    await flushPromises()
    expect(api.cleanupMySQLCluster).toHaveBeenCalledWith('1', 'mysql-1', true)
    expect(wrapper.text()).toContain('部署 MySQL')
  })
})

describe('MySQL deployment resources', () => {
  it('shows a pull action and latest event while an init image is still pulling', async () => {
    api.getMySQLCluster.mockResolvedValue({ id: 1, name: '自用', cluster_id: 2, status: 'starting' })
    api.getMySQLOperator.mockResolvedValue({ state: 'ready' })
    api.getMySQLClusterRuntime.mockResolvedValue({ status: 'starting', pods: [{ name: 'mysql-1', node_name: 'node-b', ready: false, reason: 'PodInitializing', last_event: 'Pulling: Pulling image "server:26.7"', images: ['server:26.7'], claims: ['data-mysql-1'], retryable: false }], claims: [{ name: 'data-mysql-1', phase: 'Bound', capacity: '10Gi' }] })
    api.pullMySQLClusterPodImages.mockResolvedValue({ images: ['server:26.7'] })
    wrapper = mountPage()
    await flushPromises()
    await wrapper.findAll('.section-tab').find(button => button.text() === '部署资源').trigger('click')
    expect(wrapper.text()).not.toContain('Pod 与数据卷')
    expect(wrapper.get('tbody').text()).toContain('节点拉取')
    expect(wrapper.get('tbody').text()).toContain('拉取镜像中')
    expect(wrapper.get('tbody').text()).not.toContain('重试')
    await wrapper.get('button[aria-label="查看 mysql-1 的详情"]').trigger('click')
    expect(document.body.textContent).toContain('Pulling image "server:26.7"')
    await [...document.body.querySelectorAll('.base-modal button')].find(button => button.textContent === '关闭').click()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '节点拉取').trigger('click')
    expect(api.pullMySQLClusterPodImages).toHaveBeenCalledWith('1', 'mysql-1')
  })

  it('shows the failed Pod and PVC, pulls on its node, then confirms a Pod-only retry', async () => {
    api.getMySQLCluster.mockResolvedValue({ id: 1, name: '自用', cluster_id: 2, status: 'starting', database_nodes: ['node-a', 'node-b', 'node-c'], router_nodes: ['node-a', 'node-b'] })
    api.getMySQLOperator.mockResolvedValue({ state: 'ready' })
    api.getMySQLClusterRuntime.mockResolvedValue({ status: 'starting', stateful_set_created: true, online_instances: 0, router_ready: 0, pods: [{ name: 'mysql-0', node_name: 'node-a', component: 'mysqld', ready: false, reason: 'ImagePullBackOff', message: 'registry timeout', images: ['mysql:8.4', 'mysql:8.4'], claims: ['data-mysql-0'], retryable: true }], claims: [{ name: 'data-mysql-0', phase: 'Bound', storage_class: 'local-path', capacity: '10Gi', volume_name: 'pvc-1' }] })
    api.pullMySQLClusterPodImages.mockResolvedValue({ node_name: 'node-a', images: ['mysql:8.4'] })
    api.retryMySQLClusterPod.mockResolvedValue({ pod_name: 'mysql-0' })
    wrapper = mountPage()
    await flushPromises()
    expect(wrapper.text()).not.toContain('mysql:8.4')
    await wrapper.findAll('.section-tab').find(button => button.text() === '部署资源').trigger('click')
    expect(wrapper.findAll('.resource-table table')).toHaveLength(1)
    expect(wrapper.get('tbody').text()).toContain('详情')
    expect(wrapper.get('button[aria-label="查看数据卷 data-mysql-0"]').exists()).toBe(true)
    await wrapper.get('button[aria-label="查看数据卷 data-mysql-0"]').trigger('click')
    expect(document.body.textContent).toContain('local-path')
    expect(document.body.textContent).toContain('pvc-1')
    await [...document.body.querySelectorAll('.base-modal button')].find(button => button.textContent === '关闭').click()
    await flushPromises()
    await wrapper.get('button[aria-label="查看 mysql-0 的详情"]').trigger('click')
    expect(document.body.textContent).toContain('mysql:8.4')
    expect(document.body.textContent).toContain('registry timeout')
    await [...document.body.querySelectorAll('.base-modal button')].find(button => button.textContent === '关闭').click()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '节点拉取').trigger('click')
    await flushPromises()
    expect(api.pullMySQLClusterPodImages).toHaveBeenCalledWith('1', 'mysql-0')
    await wrapper.findAll('button').find(button => button.text() === '重试').trigger('click')
    expect(api.retryMySQLClusterPod).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="confirm-dialog-confirm"]').trigger('click')
    await flushPromises()
    expect(api.retryMySQLClusterPod).toHaveBeenCalledWith('1', 'mysql-0')
  })
})

describe('MySQL manual switchover', () => {
  const pods = [
    { name: 'mysql-0', node_name: 'node-a', component: 'mysqld', database_role: 'PRIMARY', ready: true },
    { name: 'mysql-1', node_name: 'node-b', component: 'mysqld', database_role: 'SECONDARY', ready: true },
    { name: 'mysql-2', node_name: 'node-c', component: 'mysqld', database_role: 'SECONDARY', ready: false },
  ]

  it('confirms a ready secondary and updates the displayed primary', async () => {
    api.getMySQLCluster.mockResolvedValue({ id: 1, name: '自用', cluster_id: 2, status: 'ready', database_nodes: ['node-a', 'node-b', 'node-c'], router_nodes: ['node-a', 'node-b'] })
    api.getMySQLOperator.mockResolvedValue({ state: 'ready' })
    api.getMySQLClusterRuntime.mockResolvedValue({ status: 'ready', pods: pods.map(pod => ({ ...pod })), online_instances: 3, router_ready: 2 })
    api.switchMySQLClusterPrimary.mockResolvedValue({ pod_name: 'mysql-1', task: { id: 10, kind: 'switch_primary', status: 'success' } })
    wrapper = mountPage()
    await flushPromises()
    expect(wrapper.findAll('button[title="设为主实例"]')).toHaveLength(1)
    await wrapper.get('button[aria-label="将 mysql-1 切换为主实例"]').trigger('click')
    expect(api.switchMySQLClusterPrimary).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('已有连接可能短暂中断')
    await wrapper.get('[data-testid="confirm-dialog-confirm"]').trigger('click')
    await flushPromises()
    expect(api.switchMySQLClusterPrimary).toHaveBeenCalledWith('1', 'mysql-1')
    expect(wrapper.get('button[aria-label="将 mysql-0 切换为主实例"]').exists()).toBe(true)
    expect(wrapper.find('button[aria-label="将 mysql-1 切换为主实例"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('切换主实例失败')
  })

  it('shows switch failures in the error modal', async () => {
    api.getMySQLCluster.mockResolvedValue({ id: 1, name: '自用', cluster_id: 2, status: 'ready', database_nodes: ['node-a', 'node-b', 'node-c'], router_nodes: ['node-a', 'node-b'] })
    api.getMySQLOperator.mockResolvedValue({ state: 'ready' })
    api.getMySQLClusterRuntime.mockResolvedValue({ status: 'ready', pods: pods.map(pod => ({ ...pod })), online_instances: 3, router_ready: 2 })
    api.switchMySQLClusterPrimary.mockRejectedValue(new Error('数据库成员尚未全部在线，暂不能切主'))
    wrapper = mountPage()
    await flushPromises()
    await wrapper.get('button[aria-label="将 mysql-1 切换为主实例"]').trigger('click')
    await wrapper.get('[data-testid="confirm-dialog-confirm"]').trigger('click')
    await flushPromises()
    expect(document.body.textContent).toContain('数据库成员尚未全部在线，暂不能切主')
    expect(wrapper.get('button[aria-label="将 mysql-1 切换为主实例"]').exists()).toBe(true)
  })
})

describe('MySQL incremental scaling', () => {
  it('adds a database instance, then shows the task', async () => {
    const ready = { id: 1, name: '自用', cluster_id: 2, status: 'ready', database_instances: 1, database_nodes: ['node-a', 'node-b'], router_instances: 0, router_nodes: ['node-a'] }
    api.getMySQLCluster.mockResolvedValue(ready)
    api.getMySQLClusterNodes.mockResolvedValue([{ name: 'node-a', ready: true, database_allowed: true }, { name: 'node-b', ready: true, database_allowed: true }])
    api.getMySQLOperator.mockResolvedValue({ state: 'ready' })
    api.getMySQLClusterRuntime.mockResolvedValue({ status: 'ready', online_instances: 1, router_ready: 0, pods: [{ name: 'mysql-0', component: 'mysqld', node_name: 'node-a', database_role: 'PRIMARY', ready: true, claims: ['datadir-mysql-0'] }], claims: [{ name: 'datadir-mysql-0', phase: 'Bound', capacity: '10Gi' }] })
    api.addMySQLClusterInstance.mockResolvedValue({ cluster: { ...ready, status: 'starting', database_instances: 2 }, task: { id: 11, kind: 'scale_database', status: 'running' } })
    wrapper = mountPage()
    await flushPromises()
    expect(wrapper.text()).toContain('主实例')
    expect(wrapper.text()).toContain('node-a')
    expect(wrapper.text()).toContain('datadir-mysql-0')
    await wrapper.findAll('button').find(button => button.text() === '添加数据库实例').trigger('click')
    await flushPromises()
    expect(api.addMySQLClusterInstance).not.toHaveBeenCalled()
    expect(document.body.textContent).toContain('总数从 1 个增加到 2 个')
    expect(document.body.textContent).toContain('新实例沿用现有存储配置')
    document.body.querySelector('input[aria-label="node-a 作为新增 MySQL 实例候选节点"]').click()
    await flushPromises()
    await [...document.body.querySelectorAll('.base-modal button')].find(button => button.textContent === '确认追加').click()
    await flushPromises()
    expect(api.addMySQLClusterInstance).toHaveBeenCalledWith('1', { candidate_nodes: ['node-b'] })
    expect(wrapper.get('.section-tab.is-active').text()).toBe('集群任务')
    expect(document.body.textContent).toContain('任务 #11 · 添加数据库实例')
  })

  it('deploys the first Router with its own node selection and refreshes roles', async () => {
    const ready = { id: 1, name: '自用', cluster_id: 2, status: 'ready', database_instances: 2, database_nodes: ['node-a', 'node-b'], router_instances: 0, router_nodes: [] }
    api.getMySQLCluster.mockResolvedValue(ready)
    api.getMySQLOperator.mockResolvedValue({ state: 'ready' })
    api.getMySQLClusterNodes.mockResolvedValue([{ name: 'node-a', ready: true, router_allowed: true }, { name: 'node-b', ready: true, router_allowed: false, router_blocked_reason: '禁止普通应用调度' }, { name: 'outside-pool', ready: true, router_allowed: true }])
    const pods = [{ name: 'mysql-0', component: 'mysqld', node_name: 'node-a', database_role: 'PRIMARY', ready: true, claims: ['datadir-mysql-0'] }, { name: 'mysql-1', component: 'mysqld', node_name: 'node-b', database_role: 'SECONDARY', ready: true, claims: ['datadir-mysql-1'] }]
    api.getMySQLClusterRuntime.mockResolvedValueOnce({ status: 'ready', online_instances: 2, router_ready: 0, pods, claims: [] }).mockResolvedValue({ status: 'ready', online_instances: 2, router_ready: 0, pods: [{ ...pods[0], database_role: 'SECONDARY' }, { ...pods[1], database_role: 'PRIMARY' }], claims: [] })
    api.addMySQLClusterRouter.mockResolvedValue({ cluster: { ...ready, status: 'starting', router_instances: 1, router_nodes: ['node-a'] }, task: { id: 12, kind: 'scale_router', status: 'running' } })
    wrapper = mountPage()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(5000)
    await flushPromises()
    const groups = wrapper.findAll('.topology-node')
    expect(groups.find(group => group.text().includes('node-a')).text()).toContain('副本实例')
    expect(groups.find(group => group.text().includes('node-b')).text()).toContain('主实例')
    expect(api.getMySQLClusterRuntime).toHaveBeenCalledTimes(2)
    await wrapper.findAll('button').find(button => button.text() === '部署 Router').trigger('click')
    await flushPromises()
    expect(api.addMySQLClusterRouter).not.toHaveBeenCalled()
    expect(document.body.querySelector('input[aria-label="node-b 部署 Router"]').disabled).toBe(true)
    expect(document.body.querySelector('input[aria-label="outside-pool 部署 Router"]')).toBeNull()
    const routerNode = document.body.querySelector('input[aria-label="node-a 部署 Router"]')
    routerNode.checked = true
    routerNode.dispatchEvent(new Event('change', { bubbles: true }))
    await flushPromises()
    await [...document.body.querySelectorAll('.base-modal button')].find(button => button.textContent === '确认部署').click()
    await flushPromises()
    expect(api.addMySQLClusterRouter).toHaveBeenCalledWith('1', { router_nodes: ['node-a'] })
    expect(document.body.textContent).toContain('任务 #12 · 部署 Router')
  })

  it('adds another Router within the existing candidate nodes', async () => {
    const ready = { id: 1, name: '自用', cluster_id: 2, status: 'ready', database_instances: 1, database_nodes: ['node-a'], router_instances: 1, router_nodes: ['node-a', 'node-b'] }
    api.getMySQLCluster.mockResolvedValue(ready)
    api.getMySQLOperator.mockResolvedValue({ state: 'ready' })
    api.getMySQLClusterRuntime.mockResolvedValue({ status: 'ready', online_instances: 1, router_ready: 1, pods: [], claims: [] })
    api.addMySQLClusterRouter.mockResolvedValue({ cluster: { ...ready, status: 'starting', router_instances: 2 }, task: { id: 13, kind: 'scale_router', status: 'running' } })
    wrapper = mountPage()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '添加 Router').trigger('click')
    await flushPromises()
    expect(api.addMySQLClusterRouter).not.toHaveBeenCalled()
    expect(document.body.textContent).toContain('总数从 1 个增加到 2 个')
    await [...document.body.querySelectorAll('.base-modal button')].find(button => button.textContent === '确认追加').click()
    await flushPromises()
    expect(api.addMySQLClusterRouter).toHaveBeenCalledWith('1')
    expect(document.body.textContent).toContain('任务 #13 · 部署 Router')
  })

  it('shows Router node loading immediately and waits before allowing deployment', async () => {
    const ready = { id: 1, name: '自用', cluster_id: 2, status: 'ready', database_instances: 1, database_nodes: ['node-a'], router_instances: 0, router_nodes: [] }
    api.getMySQLCluster.mockResolvedValue(ready)
    api.getMySQLOperator.mockResolvedValue({ state: 'ready' })
    api.getMySQLClusterRuntime.mockResolvedValue({ status: 'ready', online_instances: 1, pods: [] })
    let resolveNodes
    api.getMySQLClusterNodes.mockImplementation(() => new Promise(resolve => { resolveNodes = resolve }))
    wrapper = mountPage()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '部署 Router').trigger('click')
    expect(document.body.textContent).toContain('正在读取节点')
    expect([...document.body.querySelectorAll('.base-modal button')].find(button => button.textContent === '确认部署').disabled).toBe(true)
    expect(api.addMySQLClusterRouter).not.toHaveBeenCalled()
    resolveNodes([{ name: 'node-a', ready: true, router_allowed: true }])
    await flushPromises()
    expect(document.body.querySelector('input[aria-label="node-a 部署 Router"]')).not.toBeNull()
  })

  it('confirms repair only when a Router runs outside saved candidates', async () => {
    api.getMySQLCluster.mockResolvedValue({ id: 1, name: '自用', cluster_id: 2, status: 'ready', database_instances: 1, database_nodes: ['node-a', 'node-b'], router_instances: 1, router_nodes: ['node-b'] })
    api.getMySQLOperator.mockResolvedValue({ state: 'ready' })
    const snapshot = node => ({ status: 'ready', online_instances: 1, router_ready: 1, pods: [{ name: 'mysql-router-abc', component: 'mysqlrouter', node_name: node, ready: true }], claims: [] })
    api.getMySQLClusterRuntime.mockResolvedValueOnce(snapshot('node-b')).mockResolvedValueOnce(snapshot('node-a')).mockResolvedValue(snapshot('node-b'))
    api.repairMySQLClusterRouterPlacement.mockResolvedValue({ message: '已更新 Router 调度模板' })
    wrapper = mountPage()
    await flushPromises()
    expect(wrapper.find('button[aria-label="修正 Router 调度"]').exists()).toBe(false)
    await vi.advanceTimersByTimeAsync(5000)
    await flushPromises()
    await wrapper.get('button[aria-label="修正 Router 调度"]').trigger('click')
    expect(api.repairMySQLClusterRouterPlacement).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('连接可能短暂中断')
    await wrapper.get('[data-testid="confirm-dialog-confirm"]').trigger('click')
    await flushPromises()
    expect(api.repairMySQLClusterRouterPlacement).toHaveBeenCalledWith('1')
    expect(wrapper.find('button[aria-label="修正 Router 调度"]').exists()).toBe(false)
  })

  it('confirms retiring the last Router and opens its task', async () => {
    const ready = { id: 1, name: '自用', cluster_id: 2, status: 'ready', database_instances: 1, database_nodes: ['node-a'], router_instances: 1, router_nodes: ['node-a'] }
    const task = { id: 14, kind: 'scale_router_down', status: 'running', last_progress: '正在清退 Router' }
    api.getMySQLCluster.mockResolvedValueOnce(ready).mockResolvedValue({ ...ready, status: 'starting', router_instances: 0 })
    api.getMySQLOperator.mockResolvedValue({ state: 'ready' })
    api.getMySQLClusterRuntime.mockResolvedValueOnce({ status: 'ready', online_instances: 1, router_ready: 1, pods: [{ name: 'mysql-0', component: 'mysqld', node_name: 'node-a', ready: true }, { name: 'mysql-router-abc', component: 'mysqlrouter', node_name: 'node-a', ready: true }], claims: [] }).mockResolvedValue({ status: 'starting', online_instances: 1, router_ready: 1, pods: [{ name: 'mysql-0', component: 'mysqld', node_name: 'node-a', ready: true }], claims: [] })
    api.getMySQLClusterTasks.mockResolvedValue(taskResult([task]))
    api.retireMySQLClusterRouter.mockResolvedValue({ cluster: { ...ready, status: 'starting', router_instances: 0 }, task })
    wrapper = mountPage()
    await flushPromises()
    await wrapper.get('button[aria-label="清退一个 Router 实例"]').trigger('click')
    expect(api.retireMySQLClusterRouter).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('最后一个 Router，读写入口将不可用')
    await wrapper.get('[data-testid="confirm-dialog-confirm"]').trigger('click')
    await flushPromises()
    expect(api.retireMySQLClusterRouter).toHaveBeenCalledWith('1')
    expect(wrapper.get('.section-tab.is-active').text()).toBe('集群任务')
    expect(wrapper.text()).toContain('清退 Router #14')
    expect(document.body.textContent).toContain('任务 #14 · 清退 Router')
  })
})

describe('MySQL deployment', () => {
  const modalButton = text => [...document.body.querySelectorAll('.base-modal button')].find(button => button.textContent === text)
  async function openDeployment(pool = ['node-a', 'node-b']) {
    api.getMySQLCluster.mockResolvedValue({ id: 1, name: '自用', cluster_id: 2, status: 'draft', database_nodes: pool })
    api.getMySQLClusterNodes.mockResolvedValue(pool.map(name => ({ name, ready: true, database_allowed: true })))
    api.getMySQLOperator.mockResolvedValue({ state: 'ready' })
    wrapper = mountPage()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '部署 MySQL').trigger('click')
    await flushPromises()
  }

  it('checks and deploys with selected candidates from the saved resource pool', async () => {
    api.deployMySQLCluster.mockResolvedValue({ cluster: { id: 1, cluster_id: 2, status: 'draft' }, task: { id: 9, kind: 'deploy', status: 'running' } })
    await openDeployment()
    const modal = document.body.querySelector('.base-modal')
    expect(modal.querySelectorAll('input')).toHaveLength(4)
    expect(modal.textContent).toContain('资源池 2 台节点')
    expect(modal.textContent).not.toContain('优先主节点')
    expect(modal.textContent).not.toContain('选择存储类型')
    expect(api.preflightMySQLCluster).toHaveBeenCalledWith('1', { database_instances: 1, storage_size: '10Gi', candidate_nodes: ['node-a', 'node-b'] })
    modal.querySelector('input[aria-label="node-a 作为本次 MySQL 候选节点"]').click()
    const instances = modal.querySelector('input[aria-label="数据库实例数"]')
    instances.value = '2'
    instances.dispatchEvent(new Event('input', { bubbles: true }))
    const storage = modal.querySelector('input[aria-label="每个数据库实例的容量"]')
    storage.value = '20Gi'
    storage.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()
    expect(modalButton('确认部署').disabled).toBe(true)
    modalButton('检查部署条件').click()
    await flushPromises()
    expect(api.preflightMySQLCluster).toHaveBeenLastCalledWith('1', { database_instances: 2, storage_size: '20Gi', candidate_nodes: ['node-b'] })
    modalButton('确认部署').click()
    await flushPromises()
    expect(api.deployMySQLCluster).toHaveBeenCalledWith('1', { database_instances: 2, storage_size: '20Gi', candidate_nodes: ['node-b'] })
  })

  it('explains shared node risk when more instances are requested than pool nodes', async () => {
    await openDeployment(['node-a'])
    const instances = document.body.querySelector('input[aria-label="数据库实例数"]')
    instances.value = '2'
    instances.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()
    expect(document.body.textContent).toContain('单台节点故障可能使集群失去多数派')
  })

  it('requires a saved resource pool before deployment', async () => {
    api.getMySQLCluster.mockResolvedValue({ id: 1, name: '自用', cluster_id: 2, status: 'draft', database_nodes: [] })
    api.getMySQLOperator.mockResolvedValue({ state: 'ready' })
    wrapper = mountPage()
    await flushPromises()
    expect(wrapper.findAll('button').find(button => button.text() === '部署 MySQL').element.disabled).toBe(true)
    expect(api.preflightMySQLCluster).not.toHaveBeenCalled()
    expect(api.deployMySQLCluster).not.toHaveBeenCalled()
  })
})
afterEach(() => { wrapper?.unmount(); document.body.innerHTML = ''; vi.useRealTimers() })
const mountPage = () => mount(MySQLClusterDetail, { props: { id: '1' }, global: { stubs: { RouterLink: true, PodTerminal: true } } })

describe('MySQL cluster detail loading', () => {
  it('opens a database terminal for a ready MySQL instance only', async () => {
    api.getMySQLCluster.mockResolvedValue({ id: 1, name: '自用', cluster_id: 2, resource_name: 'mysql-1', namespace: 'cylism-databases', status: 'ready', database_instances: 2 })
    api.getMySQLOperator.mockResolvedValue({ state: 'ready' })
    api.getMySQLClusterRuntime.mockResolvedValue({ status: 'ready', pods: [
      { name: 'mysql-1-0', component: 'mysqld', node_name: 'node-a', ready: true },
      { name: 'mysql-1-1', component: 'mysqld', node_name: 'node-b', ready: false },
    ] })
    wrapper = mountPage()
    await flushPromises()
    expect(wrapper.findAll('button[title="打开数据库终端"]')).toHaveLength(1)
    await wrapper.get('button[aria-label="打开 mysql-1-0 的数据库终端"]').trigger('click')
    const terminal = wrapper.getComponent({ name: 'PodTerminal' })
    expect(terminal.props('mysqlClusterId')).toBe('1')
    expect(terminal.props('pod')).toEqual({ name: 'mysql-1-0', namespace: 'cylism-databases', containers: ['mysql'] })
    terminal.vm.$emit('close')
    await flushPromises()
    expect(wrapper.findComponent({ name: 'PodTerminal' }).exists()).toBe(false)
  })

  it('pages cluster tasks and keeps the selected page on refresh', async () => {
    const allTasks = Array.from({ length: 23 }, (_, index) => ({ id: 23 - index, kind: 'deploy', status: 'success' }))
    api.getMySQLClusterTasks.mockImplementation((_, { limit, offset }) => Promise.resolve(taskResult(allTasks.slice(offset, offset + limit), { total: allTasks.length })))
    wrapper = mountPage()
    await flushPromises()
    await wrapper.findAll('.section-tab').find(button => button.text() === '集群任务').trigger('click')
    expect(wrapper.findAll('.task-table tbody tr')).toHaveLength(10)
    expect(wrapper.get('.task-pagination').text()).toContain('1 / 3 · 共 23 条')
    await wrapper.get('.task-pagination button:last-child').trigger('click')
    await flushPromises()
    expect(api.getMySQLClusterTasks).toHaveBeenLastCalledWith('1', { limit: 10, offset: 10 })
    expect(wrapper.get('.task-pagination').text()).toContain('2 / 3 · 共 23 条')
    expect(wrapper.get('.task-table tbody').text()).toContain('部署 MySQL #13')
    await wrapper.get('button[aria-label="刷新任务"]').trigger('click')
    await flushPromises()
    expect(api.getMySQLClusterTasks).toHaveBeenLastCalledWith('1', { limit: 10, offset: 10 })
    expect(wrapper.get('.task-pagination').text()).toContain('2 / 3 · 共 23 条')
  })

  it('edits the resource pool of a running cluster without changing existing workloads', async () => {
    const running = { id: 1, name: '自用', cluster_id: 2, status: 'ready', database_nodes: ['node-a'], database_instances: 1 }
    api.getMySQLCluster.mockResolvedValue(running)
    api.getMySQLOperator.mockResolvedValue({ state: 'ready' })
    api.getMySQLClusterRuntime.mockResolvedValue({ status: 'ready', pods: [], claims: [] })
    api.getMySQLClusterNodes.mockResolvedValue([
      { name: 'node-a', internal_ip: '10.0.0.1', database_allowed: true },
      { name: 'node-b', internal_ip: '10.0.0.2', database_allowed: true },
    ])
    api.updateMySQLClusterResourcePool.mockResolvedValue({ ...running, database_nodes: ['node-a', 'node-b'] })
    wrapper = mountPage()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '编辑资源池').trigger('click')
    await flushPromises()
    expect(document.body.textContent).toContain('现有实例不会迁移')
    const input = document.body.querySelector('input[aria-label="10.0.0.2 加入资源池"]')
    expect(input.disabled).toBe(false)
    input.checked = true
    input.dispatchEvent(new Event('change', { bubbles: true }))
    await flushPromises()
    Array.from(document.body.querySelectorAll('button')).find(button => button.textContent === '保存资源池').click()
    await flushPromises()
    expect(api.updateMySQLClusterResourcePool).toHaveBeenCalledWith('1', ['node-a', 'node-b'])
    expect(api.addMySQLClusterInstance).not.toHaveBeenCalled()
  })

  it('saves schedulable IPs to the resource pool on a paused cluster', async () => {
    api.getMySQLCluster.mockResolvedValue({ id: 1, name: '自用', cluster_id: 2, status: 'paused', database_nodes: ['node-a'] })
    api.getMySQLClusterNodes.mockResolvedValue([
      { name: 'node-a', internal_ip: '10.0.0.1', database_allowed: true },
      { name: 'node-b', internal_ip: '10.0.0.2', database_allowed: true },
      { name: 'node-c', internal_ip: '10.0.0.3', database_allowed: false, blocked_reason: '节点未就绪' },
    ])
    api.updateMySQLClusterResourcePool.mockResolvedValue({ id: 1, name: '自用', cluster_id: 2, status: 'paused', database_nodes: ['node-a', 'node-b'] })
    wrapper = mountPage()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '编辑资源池').trigger('click')
    await flushPromises()
    expect(document.body.textContent).toContain('10.0.0.2')
    expect(document.body.querySelector('input[aria-label="10.0.0.3 加入资源池"]').disabled).toBe(true)
    const input = document.body.querySelector('input[aria-label="10.0.0.2 加入资源池"]')
    input.checked = true
    input.dispatchEvent(new Event('change', { bubbles: true }))
    await flushPromises()
    Array.from(document.body.querySelectorAll('button')).find(button => button.textContent === '保存资源池').click()
    await flushPromises()
    expect(api.updateMySQLClusterResourcePool).toHaveBeenCalledWith('1', ['node-a', 'node-b'])
    expect(wrapper.text()).toContain('编辑资源池')
  })

  it('shows Router before MySQL and keeps connection addresses in the dialog', async () => {
    api.getMySQLCluster.mockResolvedValue({ id: 1, name: '自用', cluster_id: 2, resource_name: 'mysql-1', namespace: 'cylism-databases', status: 'ready', database_instances: 1, router_instances: 1 })
    api.getMySQLOperator.mockResolvedValue({ state: 'ready' })
    api.getMySQLClusterRuntime.mockResolvedValue({ status: 'ready', online_instances: 1, router_ready: 1, router_read_write: 'mysql-1.cylism-databases.svc.cluster.local:6446', router_read_only: 'mysql-1.cylism-databases.svc.cluster.local:6447', pods: [
      { name: 'mysql-1-0', component: 'mysqld', node_name: 'node-a', database_role: 'PRIMARY', ready: true, address: 'mysql-1-0.mysql-1-instances.cylism-databases.svc.cluster.local:3306', claims: ['datadir-mysql-1-0'] },
      { name: 'mysql-1-router-0', component: 'mysqlrouter', node_name: 'node-b', ready: true },
    ] })
    wrapper = mountPage()
    await flushPromises()
    const stages = wrapper.findAll('.topology-stage')
    expect(stages).toHaveLength(2)
    expect(stages[0].text()).toContain('Router')
    expect(stages[0].text()).not.toContain('mysql-1.cylism-databases.svc.cluster.local:6446')
    expect(stages[0].text()).not.toContain('mysql-1.cylism-databases.svc.cluster.local:6447')
    expect(stages[1].text()).toContain('MySQL 实例')
    expect(stages[1].text()).not.toContain('mysql-1-0.mysql-1-instances.cylism-databases.svc.cluster.local:3306')
    expect(stages[1].text()).toContain('主实例')
    expect(wrapper.find('.mysql-topology .topology-flow').exists()).toBe(true)
    expect(wrapper.findAll('.topology-node-router')).toHaveLength(1)
    expect(wrapper.findAll('.topology-node-database')).toHaveLength(1)
    expect(wrapper.find('.topology-node-router').text()).toContain('node-b')
    expect(wrapper.find('.topology-node-database').text()).toContain('datadir-mysql-1-0')
    expect(wrapper.find('.topology-root').exists()).toBe(false)
    await wrapper.findAll('button').find(button => button.text() === '连接信息').trigger('click')
    const dialog = document.body.querySelector('.base-modal[aria-modal="true"]')
    expect(dialog.textContent).toContain('mysql-1.cylism-databases.svc.cluster.local:6446')
    expect(dialog.textContent).toContain('mysql-1.cylism-databases.svc.cluster.local:6447')
    expect(dialog.textContent).toContain('mysql-1-0.mysql-1-instances.cylism-databases.svc.cluster.local:3306')
  })

  it('shows live Pod and container usage for MySQL and Router', async () => {
    api.getMySQLCluster.mockResolvedValue({ id: 1, name: '自用', cluster_id: 2, status: 'ready', database_instances: 1, router_instances: 1 })
    api.getMySQLOperator.mockResolvedValue({ state: 'ready' })
    const pods = [
      { name: 'mysql-1-0', component: 'mysqld', node_name: 'node-a', ready: true, cpu_millicores: 12, memory_bytes: 588 * 1024 * 1024, container_usage: [{ name: 'mysql', cpu_millicores: 11, memory_bytes: 439 * 1024 * 1024 }, { name: 'sidecar', cpu_millicores: 1, memory_bytes: 149 * 1024 * 1024 }] },
      { name: 'mysql-1-router-0', component: 'mysqlrouter', node_name: 'node-b', ready: true, cpu_millicores: 3, memory_bytes: 64 * 1024 * 1024 },
    ]
    api.getMySQLClusterRuntime.mockResolvedValueOnce({ status: 'ready', online_instances: 1, router_ready: 1, pods }).mockResolvedValue({ status: 'ready', online_instances: 1, router_ready: 1, pods: [{ ...pods[0], memory_bytes: 600 * 1024 * 1024 }, pods[1]] })
    wrapper = mountPage()
    await flushPromises()
    expect(wrapper.find('.topology-node-router').text()).toContain('CPU 3 m · 内存 64 MiB')
    expect(wrapper.find('.topology-node-database').text()).toContain('CPU 12 m · 内存 588 MiB')
    await wrapper.findAll('.section-tab').find(button => button.text() === '部署资源').trigger('click')
    expect(wrapper.text()).toContain('588 MiB')
    await wrapper.get('button[aria-label="查看 mysql-1-0 的详情"]').trigger('click')
    expect(document.body.querySelector('.resource-modal-content').textContent).toContain('mysqlCPU 11 m · 内存 439 MiB')
    expect(document.body.querySelector('.resource-modal-content').textContent).toContain('sidecarCPU 1 m · 内存 149 MiB')
    await vi.advanceTimersByTimeAsync(5000)
    await flushPromises()
    expect(document.body.querySelector('.resource-modal-content').textContent).toContain('600 MiB')
  })

  it('keeps the topology available when resource metrics are unavailable', async () => {
    api.getMySQLCluster.mockResolvedValue({ id: 1, name: '自用', cluster_id: 2, status: 'ready', database_instances: 1, router_instances: 0 })
    api.getMySQLOperator.mockResolvedValue({ state: 'ready' })
    api.getMySQLClusterRuntime.mockResolvedValue({ status: 'ready', online_instances: 1, router_ready: 0, pods: [{ name: 'mysql-1-0', component: 'mysqld', node_name: 'node-a', ready: true }] })
    wrapper = mountPage()
    await flushPromises()
    expect(wrapper.find('.topology-node-database').text()).toContain('实时指标暂不可用')
  })

  it('shows pending addresses without inventing a Router service', async () => {
    api.getMySQLCluster.mockResolvedValue({ id: 1, name: '自用', cluster_id: 2, status: 'starting', database_instances: 1, router_instances: 1 })
    api.getMySQLOperator.mockResolvedValue({ state: 'ready' })
    api.getMySQLClusterRuntime.mockResolvedValue({ status: 'starting', online_instances: 0, router_ready: 0, connection_details_checked: true, pods: [{ name: 'mysql-1-0', component: 'mysqld', ready: false }] })
    wrapper = mountPage()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '连接信息').trigger('click')
    expect(document.body.querySelector('.connection-details').textContent).toContain('Router Service 尚未创建')
    expect(document.body.querySelector('.connection-details').textContent).toContain('等待地址分配')
  })

  it('does not call an unreported address missing while the API is being updated', async () => {
    api.getMySQLCluster.mockResolvedValue({ id: 1, name: '自用', cluster_id: 2, status: 'starting', database_instances: 1, router_instances: 1 })
    api.getMySQLClusterRuntime.mockResolvedValue({ status: 'starting', pods: [{ name: 'mysql-1-0', component: 'mysqld', ready: true }] })
    wrapper = mountPage()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '连接信息').trigger('click')
    expect(document.body.querySelector('.connection-details').textContent).toContain('正在读取 Router Service')
    expect(document.body.querySelector('.connection-details').textContent).toContain('正在读取实例地址')
  })

  it('shows saved cluster data without requesting deployment nodes on entry', async () => {
    api.getMySQLClusterNodes.mockRejectedValue(new Error('节点查询超时'))
    wrapper = mountPage()
    await flushPromises()
    expect(wrapper.text()).toContain('自用')
    expect(wrapper.text()).toContain('尚未部署 MySQL 集群')
    expect(api.getMySQLClusterNodes).not.toHaveBeenCalled()
    expect(document.body.textContent).not.toContain('节点查询超时')
  })

  it('keeps topology and tasks visible when the Operator status request times out', async () => {
    api.getMySQLCluster.mockResolvedValue({ id: 1, name: '自用', cluster_id: 2, status: 'ready', database_instances: 1 })
    api.getMySQLOperator.mockRejectedValue(new Error('Operator 状态超时'))
    api.getMySQLClusterRuntime.mockResolvedValue({ status: 'ready', online_instances: 1, pods: [{ name: 'mysql-1-0', node_name: 'node-a', component: 'mysqld', ready: true, database_role: 'PRIMARY' }] })
    api.getMySQLClusterTasks.mockResolvedValue(taskResult([{ id: 8, kind: 'deploy', status: 'success' }]))
    wrapper = mountPage()
    await flushPromises()
    expect(wrapper.text()).toContain('mysql-1-0')
    expect(wrapper.text()).toContain('主实例')
    expect(document.body.textContent).toContain('Operator 状态超时')
    await wrapper.findAll('.section-tab').find(button => button.text() === '集群任务').trigger('click')
    expect(wrapper.text()).toContain('部署 MySQL #8')
  })

  it('renders ready Pods before a slow Operator status request finishes', async () => {
    api.getMySQLCluster.mockResolvedValue({ id: 1, name: '自用', cluster_id: 2, status: 'ready', database_instances: 1 })
    let failOperator
    api.getMySQLOperator.mockReturnValue(new Promise((resolve, reject) => { failOperator = reject }))
    api.getMySQLClusterRuntime.mockResolvedValue({ status: 'ready', online_instances: 1, pods: [{ name: 'mysql-1-0', node_name: 'node-a', component: 'mysqld', ready: true, database_role: 'PRIMARY' }] })
    wrapper = mountPage()
    await flushPromises()
    expect(wrapper.text()).toContain('mysql-1-0')
    expect(wrapper.text()).toContain('主实例')
    failOperator(new Error('Operator 状态超时'))
    await flushPromises()
    expect(document.body.textContent).toContain('Operator 状态超时')
  })

  it('does not reopen an error dialog on every failed background poll, but reports manual refresh failures', async () => {
    api.getMySQLCluster.mockResolvedValue({ id: 1, name: '自用', cluster_id: 2, status: 'ready', database_instances: 1 })
    api.getMySQLClusterRuntime.mockResolvedValue({ status: 'ready', online_instances: 1, pods: [] })
    wrapper = mountPage()
    await flushPromises()
    api.getMySQLOperator.mockRejectedValue(new Error('Operator 状态超时'))
    await vi.advanceTimersByTimeAsync(5000)
    await flushPromises()
    expect(document.body.textContent).not.toContain('Operator 状态超时')
    await wrapper.get('button[aria-label="刷新运行状态"]').trigger('click')
    await flushPromises()
    expect(document.body.textContent).toContain('Operator 状态超时')
  })

  it('reports a failed poll when an explicit status check joins the pending request', async () => {
    api.getMySQLOperator.mockResolvedValue({ state: 'installing' })
    wrapper = mountPage()
    await flushPromises()
    let failPoll
    api.getMySQLOperator.mockReturnValue(new Promise((resolve, reject) => { failPoll = reject }))
    await vi.advanceTimersByTimeAsync(5000)
    await wrapper.findAll('button').find(button => button.text() === '检查安装状态').trigger('click')
    failPoll(new Error('Operator 状态超时'))
    await flushPromises()
    expect(document.body.textContent).toContain('Operator 状态超时')
  })
})

describe('MySQL Operator installation tasks', () => {
  it.each([
    ['starting', null],
    ['ready', null],
    ['paused', { cluster_resource: true, workloads: [], pods: [], claims: [] }],
    ['paused', { cluster_resource: false, workloads: ['mysql-1'], pods: [], claims: [] }],
    ['paused', { cluster_resource: false, workloads: [], pods: ['mysql-1-0'], claims: [] }],
  ])('hides uninstall while the %s cluster still has resources', async (status, cleanup) => {
    api.getMySQLCluster.mockResolvedValue({ id: 1, name: '自用', cluster_id: 2, status })
    api.getMySQLOperator.mockResolvedValue({ state: 'ready', can_uninstall: true })
    api.getMySQLClusterRuntime.mockResolvedValue({ status, pods: [] })
    if (cleanup) api.getMySQLClusterCleanup.mockResolvedValue(cleanup)
    wrapper = mountPage()
    await flushPromises()
    for (const tab of ['拓扑', '集群任务']) {
      await wrapper.findAll('.section-tab').find(button => button.text() === tab).trigger('click')
      expect(wrapper.findAll('button').some(button => button.text() === '卸载 Operator')).toBe(false)
    }
  })

  it('shows uninstall after workload cleanup even when PVCs are retained', async () => {
    api.getMySQLCluster.mockResolvedValue({ id: 1, name: '自用', cluster_id: 2, status: 'paused' })
    api.getMySQLOperator.mockResolvedValue({ state: 'ready', can_uninstall: true })
    api.getMySQLClusterCleanup.mockResolvedValue({ cluster_resource: false, workloads: [], pods: [], claims: ['datadir-mysql-1-0'] })
    wrapper = mountPage()
    await flushPromises()
    expect(wrapper.get('.detail-toolbar > button').text()).toBe('卸载 Operator')
    await wrapper.findAll('.section-tab').find(button => button.text() === '集群任务').trigger('click')
    expect(wrapper.find('.detail-toolbar > button').exists()).toBe(false)
    expect(wrapper.findAll('.detail-toolbar button').map(button => button.attributes('aria-label'))).toEqual(['刷新任务'])
  })

  it('confirms uninstall from a paused cluster and tracks its own task', async () => {
    api.getMySQLCluster.mockResolvedValue({ id: 1, name: '自用', cluster_id: 2, status: 'paused' })
    api.getMySQLOperator.mockResolvedValue({ state: 'ready', can_uninstall: true })
    const task = { id: 8, kind: 'uninstall_operator', status: 'running', last_progress: '已请求 Helm 卸载 MySQL Operator，等待清理资源' }
    api.uninstallMySQLOperator.mockImplementation(async () => {
      api.getMySQLOperator.mockResolvedValue({ state: 'cancelling', can_uninstall: false })
      api.getMySQLClusterTasks.mockResolvedValue(taskResult([task]))
      return { task }
    })
    wrapper = mountPage()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '卸载 Operator').trigger('click')
    expect(api.uninstallMySQLOperator).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('数据库 PVC 不会删除')
    await wrapper.get('[data-testid="confirm-dialog-confirm"]').trigger('click')
    await flushPromises()
    expect(api.uninstallMySQLOperator).toHaveBeenCalledWith('1')
    expect(wrapper.get('.section-tab.is-active').text()).toBe('集群任务')
    expect(wrapper.get('tbody').text()).toContain('卸载 MySQL Operator #8')
    expect(wrapper.findAll('button').some(button => button.text() === '卸载 Operator')).toBe(false)
    expect(wrapper.findAll('.detail-toolbar button').map(button => button.attributes('aria-label'))).toEqual(['刷新任务'])
  })

  it('shows rejected uninstall in the shared error modal', async () => {
    api.getMySQLCluster.mockResolvedValue({ id: 1, name: '自用', cluster_id: 2, status: 'paused' })
    api.getMySQLOperator.mockResolvedValue({ state: 'ready', can_uninstall: true })
    api.uninstallMySQLOperator.mockRejectedValue(new Error('该 Kubernetes 集群仍有 MySQL 集群资源'))
    wrapper = mountPage()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '卸载 Operator').trigger('click')
    await wrapper.get('[data-testid="confirm-dialog-confirm"]').trigger('click')
    await flushPromises()
    expect(document.body.textContent).toContain('该 Kubernetes 集群仍有 MySQL 集群资源')
  })

  it('shows progress immediately when checking while a background poll is pending', async () => {
    const task = { id: 7, kind: 'install_operator', status: 'running' }
    api.getMySQLOperator.mockResolvedValue({ state: 'installing', can_cancel: true, progress: '等待安装 Pod 拉取镜像' })
    api.getMySQLClusterTasks.mockResolvedValue(taskResult([task]))
    wrapper = mountPage()
    await flushPromises()
    let completeCheck
    api.getMySQLOperator.mockReturnValue(new Promise(resolve => { completeCheck = resolve }))
    await vi.advanceTimersByTimeAsync(5000)
    await wrapper.findAll('button').find(button => button.text() === '检查安装状态').trigger('click')
    expect(document.body.textContent).toContain('正在检查安装状态...')
    expect(document.body.textContent).toContain('等待安装 Pod 拉取镜像')
    api.getMySQLClusterTasks.mockResolvedValue(taskResult([{ ...task, status: 'success', last_progress: 'MySQL Operator 已就绪' }]))
    completeCheck({ state: 'ready', progress: 'MySQL Operator 已就绪' })
    await flushPromises()
    expect(document.body.textContent).toContain('MySQL Operator 已就绪')
    expect(document.body.textContent).not.toContain('正在检查安装状态...')
    expect(wrapper.get('tbody').text()).toContain('成功')
  })

  it('confirms cancellation and shows the cancelled task without a success notice', async () => {
    api.getMySQLOperator.mockResolvedValue({ state: 'installing', can_cancel: true })
    api.getMySQLClusterTasks.mockResolvedValue(taskResult([{ id: 7, kind: 'install_operator', status: 'running' }]))
    api.cancelMySQLOperator.mockImplementation(async () => {
      api.getMySQLOperator.mockResolvedValue({ state: 'missing', can_cancel: false })
      api.getMySQLClusterTasks.mockResolvedValue(taskResult([{ id: 7, kind: 'install_operator', status: 'cancelled' }]))
      return { state: 'missing', progress: '安装已中止' }
    })
    wrapper = mountPage()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '中止安装').trigger('click')
    expect(api.cancelMySQLOperator).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="confirm-dialog-confirm"]').trigger('click')
    await flushPromises()
    expect(api.cancelMySQLOperator).toHaveBeenCalledWith('1')
    expect(wrapper.get('tbody').text()).toContain('已中止')
    expect(wrapper.find('[data-testid="confirm-dialog-confirm"]').exists()).toBe(false)
    expect(wrapper.findAll('.detail-toolbar button').map(button => button.attributes('aria-label'))).toEqual(['刷新任务'])
  })

  it('shows a cancellation failure in the error modal and retains the running task', async () => {
    api.getMySQLOperator.mockResolvedValue({ state: 'installing', can_cancel: true })
    api.getMySQLClusterTasks.mockResolvedValue(taskResult([{ id: 7, kind: 'install_operator', status: 'running' }]))
    api.cancelMySQLOperator.mockRejectedValue(new Error('集群已有 MySQL 集群使用 Operator，不能中止安装'))
    wrapper = mountPage()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '中止安装').trigger('click')
    await wrapper.get('[data-testid="confirm-dialog-confirm"]').trigger('click')
    await flushPromises()
    expect(document.body.querySelector('[role="dialog"]').textContent).toContain('不能中止安装')
    expect(wrapper.find('[data-testid="confirm-dialog-confirm"]').exists()).toBe(false)
  })

  it('reports a failed status check in the error modal', async () => {
    api.getMySQLOperator.mockResolvedValue({ state: 'installing' })
    wrapper = mountPage()
    await flushPromises()
    api.getMySQLOperator.mockRejectedValue(new Error('访问 Kubernetes API 超时'))
    await wrapper.findAll('button').find(button => button.text() === '检查安装状态').trigger('click')
    await flushPromises()
    expect(document.body.textContent).toContain('访问 Kubernetes API 超时')
    expect(document.body.textContent).not.toContain('正在检查安装状态...')
  })

  it('opens the task list after installing and exposes installation logs', async () => {
    const task = { id: 7, kind: 'install_operator', status: 'running' }
    api.installMySQLOperator.mockImplementation(async () => {
      api.getMySQLOperator.mockResolvedValue({ state: 'installing' })
      api.getMySQLClusterTasks.mockResolvedValue(taskResult([task]))
      return { state: 'installing', task }
    })
    wrapper = mountPage()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '安装 MySQL Operator').trigger('click')
    await flushPromises()
    expect(api.installMySQLOperator).toHaveBeenCalledWith('1')
    expect(wrapper.get('.section-tab.is-active').text()).toBe('集群任务')
    expect(wrapper.get('tbody').text()).toContain('安装 MySQL Operator #7')
    expect(wrapper.get('tbody').text()).not.toContain('部署 MySQL 集群 #7')
    await wrapper.findAll('button').find(button => button.text() === '查看日志').trigger('click')
    await flushPromises()
    expect(api.getMySQLClusterTaskLogs).toHaveBeenCalledWith('1', 7)
    expect(document.body.textContent).toContain('等待 Helm 安装 MySQL Operator')
  })

  it('polls an existing installation on a draft cluster and enables deployment when ready', async () => {
    api.getMySQLCluster.mockResolvedValue({ id: 1, name: '自用', cluster_id: 2, status: 'draft', database_nodes: ['node-a'] })
    api.getMySQLOperator.mockResolvedValue({ state: 'installing' })
    api.getMySQLClusterTasks.mockResolvedValue(taskResult([{ id: 7, kind: 'install_operator', status: 'running' }]))
    wrapper = mountPage()
    await flushPromises()
    const deployButton = () => wrapper.findAll('button').find(button => button.text() === '部署 MySQL')
    expect(deployButton().attributes('disabled')).toBeDefined()
    api.getMySQLOperator.mockResolvedValue({ state: 'ready' })
    api.getMySQLClusterTasks.mockResolvedValue(taskResult([{ id: 7, kind: 'install_operator', status: 'success' }]))
    await vi.advanceTimersByTimeAsync(5000)
    await flushPromises()
    expect(deployButton().attributes('disabled')).toBeUndefined()
    expect(api.getMySQLClusterRuntime).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('尚未部署 MySQL 集群')
  })
})
