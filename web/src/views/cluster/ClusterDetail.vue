<template>
  <div class="cluster-detail-page">
    <RouterLink class="back-link" to="/cluster?tab=list"><ArrowLeft :size="16" />返回集群列表</RouterLink>
    <SectionTabsHeader :title="cluster?.name || '集群详情'" :tabs="tabs" :active-tab="activeTab" @select="selectTab">
      <template v-if="cluster" #actions><span class="badge" :class="statusClass(cluster.status)">{{ statusLabel(cluster.status) }}</span></template>
    </SectionTabsHeader>
    <FeedbackBanner v-if="error" tone="warning" :message="error" />
    <SurfaceCard v-if="loading && !cluster" class="cluster-detail-content" padding="none"><EmptyState variant="loading" message="加载中..." /></SurfaceCard>
    <SurfaceCard v-else-if="cluster && activeTab === 'topology'" class="cluster-detail-content" padding="none">
      <div class="cluster-deployment-toolbar"><strong>集群拓扑</strong><div v-if="cluster.distribution === 'k3s'" class="cluster-toolbar-actions"><button class="btn" type="button" :disabled="runningDeployment || topologyLoading" @click="openAdoption">{{ deploymentRole === 'control-plane' ? '纳管已有 K3s' : '纳管已有节点' }}</button><button class="btn btn-primary" type="button" :disabled="runningDeployment || topologyLoading" @click="openDeployment">{{ deploymentRole === 'control-plane' ? '部署 K3s' : '添加工作节点' }}</button></div><span v-else class="cell-secondary">K8s 部署暂未开放</span></div>
      <FeedbackBanner v-if="topologyError" class="topology-error" tone="warning" :message="topologyError" />
      <div v-if="clusterNodes.length" class="cluster-topology" aria-label="集群拓扑">
        <div v-if="controlNode" class="topology-node topology-node--control">
          <span class="topology-node-icon"><ServerCog :size="20" /></span>
          <div class="topology-node-copy"><span class="topology-node-role">控制平面</span><strong>{{ nodeDisplayName(controlNode) }}</strong><span v-if="nodeDisplayName(controlNode) !== controlNode.host" class="cell-secondary">{{ controlNode.host }}</span></div>
          <div class="topology-node-actions">
            <button class="icon-button icon-button--table" type="button" :title="workerNodes.length ? '请先卸载工作节点' : '卸载控制平面'" :aria-label="`卸载 ${nodeDisplayName(controlNode)}`" :disabled="workerNodes.length > 0 || runningDeployment" @click="openUninstall(controlNode)"><Trash2 :size="14" /></button>
          </div>
        </div>
        <div v-if="workerNodes.length" class="topology-branch" aria-hidden="true"><span /></div>
        <div v-if="workerNodes.length" class="topology-workers">
          <div class="topology-workers-heading">工作节点 <span>{{ workerNodes.length }}</span></div>
          <div class="topology-worker-list"><div v-for="server in workerNodes" :key="server.id" class="topology-node topology-node--worker">
            <span class="topology-node-icon"><Server :size="18" /></span>
            <div class="topology-node-copy"><span class="topology-node-role">工作节点</span><strong>{{ nodeDisplayName(server) }}</strong><span v-if="nodeDisplayName(server) !== server.host" class="cell-secondary">{{ server.host }}</span></div>
            <button class="icon-button icon-button--table topology-node-action" type="button" :title="`卸载 ${nodeDisplayName(server)}`" :aria-label="`卸载 ${nodeDisplayName(server)}`" :disabled="runningDeployment" @click="openUninstall(server)"><Trash2 :size="14" /></button>
          </div></div>
        </div>
      </div>
      <EmptyState v-if="!clusterNodes.length && !topologyLoading" :message="runningDeployment ? '正在部署，完成后显示拓扑' : cluster.distribution === 'k3s' ? '尚未部署 K3s 集群' : '暂无集群节点'" />
    </SurfaceCard>
    <SurfaceCard v-else-if="cluster && activeTab === 'tasks'" class="cluster-detail-content" padding="none">
      <div class="cluster-deployment-toolbar"><strong>集群任务</strong><button class="icon-button" type="button" title="刷新任务" aria-label="刷新任务" :disabled="topologyLoading" @click="loadTopology"><RefreshCw :size="15" :class="{ 'is-spinning': topologyLoading }" /></button></div>
      <FeedbackBanner v-if="topologyError" class="topology-error" tone="warning" :message="topologyError" />
      <div v-if="deployments.length" class="table-wrap"><table class="data-table"><thead><tr><th>任务</th><th>服务器</th><th>状态</th><th>开始时间</th><th>操作</th></tr></thead><tbody><tr v-for="task in deployments" :key="task.id"><td><span class="cell-primary">#{{ task.id }} · {{ taskLabel(task) }}</span><span class="cell-secondary">{{ taskDescription(task) }}</span></td><td>{{ serverName(task.server_id) }}</td><td><span class="badge" :class="deploymentStatusClass(task.status)">{{ deploymentStatusLabel(task.status) }}</span></td><td>{{ formatTime(task.created_at) }}</td><td><button class="btn btn-sm" type="button" :aria-label="`查看任务 #${task.id} 日志`" aria-haspopup="dialog" @click="selectDeployment(task)">查看日志</button></td></tr></tbody></table></div>
      <EmptyState v-else-if="!topologyLoading" message="暂无集群任务" />
      <div v-if="taskTotal" class="pagination task-pagination"><button class="btn btn-sm" type="button" :disabled="taskOffset === 0 || topologyLoading" @click="changeTaskPage(-1)">上一页</button><span>{{ taskPage }} / {{ taskPageCount }} · 共 {{ taskTotal }} 条</span><button class="btn btn-sm" type="button" :disabled="taskOffset + taskPageSize >= taskTotal || topologyLoading" @click="changeTaskPage(1)">下一页</button></div>
    </SurfaceCard>
    <ClusterAgentGate v-else-if="cluster && agentRequiredTab" :key="`${props.clusterID}-${activeTab}`" :cluster-id="props.clusterID"><component :is="scopedComponent" :cluster-id="props.clusterID" class="cluster-detail-content cluster-scoped-view" /></ClusterAgentGate>
    <component v-else-if="cluster && scopedComponent" :is="scopedComponent" :cluster-id="props.clusterID" :cluster="cluster" :control-node="controlNode" :deployments="latestDeployments" :running-deployment="runningDeployment" class="cluster-detail-content cluster-scoped-view" @operation-started="handleAgentOperationStarted" />
    <BaseModal :open="Boolean(selectedDeployment)" :title="selectedDeployment ? `任务 #${selectedDeployment.id} · 执行日志` : '执行日志'" size="large" dialog-class="cluster-log-modal" @close="closeLogs">
      <div v-if="selectedDeployment" class="deployment-log-meta"><span>{{ taskLabel(selectedDeployment) }} · {{ serverName(selectedDeployment.server_id) }}</span><span class="badge" :class="deploymentStatusClass(selectedDeployment.status)">{{ deploymentStatusLabel(selectedDeployment.status) }}</span></div>
      <div v-if="selectedDeployment" class="deployment-log-content">
        <FeedbackBanner v-if="logError" tone="warning" :message="logError" />
        <div class="deployment-log" role="log" aria-live="polite"><div v-for="entry in logs" :key="entry.id">{{ formatLogTime(entry.created_at) }} {{ entry.message }}</div><span v-if="!logs.length" class="cell-secondary">{{ selectedDeployment.status === 'running' ? '等待执行日志...' : '暂无执行日志' }}</span></div>
      </div>
      <template #actions><button class="btn" type="button" @click="closeLogs">关闭</button></template>
    </BaseModal>
    <BaseModal :open="adoptionOpen" :title="deploymentRole === 'control-plane' ? '纳管已有 K3s 集群' : '纳管已有工作节点'" @close="closeAdoption">
      <div class="cluster-deployment-form">
        <div class="deployment-form-field"><label for="adoption-server">{{ deploymentRole === 'control-plane' ? '选择已有控制平面服务器' : '选择已有工作节点服务器' }}</label><SelectMenu id="adoption-server" v-model="adoptServerID" :options="serverOptions" placeholder="请选择已有服务器" aria-label="纳管服务器" /></div>
        <div v-if="deploymentRole === 'control-plane'" class="deployment-form-field"><label for="adoption-address">控制平面连接地址</label><input id="adoption-address" v-model.trim="adoptAddress" class="form-input" :placeholder="adoptSelectedServer?.host || 'IP 或域名'" /></div>
      </div>
      <p class="deployment-hint">将通过 SSH 核对 K3s 服务、主机名和集群节点列表，不会安装或修改目标机器。先纳管控制平面，再逐个纳管工作节点。</p>
      <template #actions><button class="btn" type="button" :disabled="adopting" @click="closeAdoption">取消</button><button class="btn btn-primary" type="button" :disabled="!adoptServerID || adopting" @click="startAdoption">{{ adopting ? '启动中...' : '开始纳管' }}</button></template>
    </BaseModal>
    <BaseModal :open="Boolean(uninstallTarget)" title="卸载 K3s 节点" size="small" @close="closeUninstall">
      <p>将在“{{ nodeDisplayName(uninstallTarget || {}) }}”上通过 SSH 执行 K3s 官方卸载脚本。该操作会删除节点上的 K3s 服务和本地集群数据。</p>
      <template #actions><button class="btn" type="button" :disabled="uninstalling" @click="closeUninstall">取消</button><button class="btn btn-danger" type="button" :disabled="uninstalling" @click="confirmUninstall">{{ uninstalling ? '启动中...' : '确认卸载' }}</button></template>
    </BaseModal>
    <BaseModal :open="deploymentOpen" :title="deploymentRole === 'control-plane' ? '部署 K3s 控制平面' : '添加 K3s 工作节点'" :show-close="!submitting" :close-on-overlay="!submitting" :close-on-escape="!submitting" @close="closeDeployment">
      <div class="cluster-deployment-form">
        <div class="deployment-form-field"><label for="deployment-server">{{ deploymentRole === 'control-plane' ? '选择控制平面服务器' : '选择工作节点服务器' }}</label><SelectMenu id="deployment-server" v-model="selectedServerID" :options="serverOptions" placeholder="请选择已有服务器" aria-label="部署服务器" /></div>
        <div v-if="deploymentRole === 'control-plane'" class="deployment-form-field"><label for="deployment-address">控制平面连接地址</label><input id="deployment-address" v-model.trim="controlAddress" class="form-input" :placeholder="selectedServer?.host || 'IP 或域名'" /></div>
        <div class="deployment-form-field"><label for="deployment-mirror">安装源</label><SelectMenu id="deployment-mirror" v-model="mirror" :options="mirrorOptions" aria-label="安装源" /></div>
      </div>
      <p class="deployment-hint">将在选中的服务器上通过 SSH 安装 K3s。服务器只能归属一个集群。<RouterLink v-if="!serverOptions.length" to="/servers">前往添加服务器</RouterLink></p>
      <template #actions><button class="btn" type="button" :disabled="submitting" @click="closeDeployment">取消</button><button class="btn btn-primary" type="button" :disabled="!selectedServerID || submitting" @click="startDeployment">{{ submitting ? '启动中...' : deploymentRole === 'control-plane' ? '部署控制平面' : '添加工作节点' }}</button></template>
    </BaseModal>
    <ErrorNoticeModal :open="Boolean(operationError)" :message="operationError" teleport @close="clearOperationErrors" />
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { defineAsyncComponent } from 'vue'
import { ArrowLeft, RefreshCw, Server, ServerCog, Trash2 } from 'lucide-vue-next'
import { adoptClusterNode, getCluster, getClusterDeploymentLogs, getClusterDeployments, startClusterDeployment, uninstallClusterNode } from '../../api/clusters.js'
import { getServers } from '../../api/servers.js'
import { useAsyncResource } from '../../composables/useAsyncResource.js'
import { useRoutedTab } from '../../composables/useRoutedTab.js'
import BaseModal from '../../components/BaseModal.vue'
import EmptyState from '../../components/EmptyState.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import FeedbackBanner from '../../components/FeedbackBanner.vue'
import SectionTabsHeader from '../../components/SectionTabsHeader.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import SurfaceCard from '../../components/SurfaceCard.vue'
import { formatShortDateTime as formatTime } from '../../utils/formatters.js'
import AgentManagement from './AgentManagement.vue'
import ClusterAgentGate from '../../components/ClusterAgentGate.vue'

const props = defineProps({ clusterID: { type: String, required: true } })
const NodeRegistryMirrors = defineAsyncComponent(() => import('./NodeRegistryMirrors.vue'))
const ClusterDNS = defineAsyncComponent(() => import('./ClusterDNS.vue'))
const ChartRepositories = defineAsyncComponent(() => import('./ChartRepositories.vue'))
const SystemComponents = defineAsyncComponent(() => import('./SystemComponents.vue'))
const tabs = [
  { id: 'topology', label: '拓扑' },
  { id: 'agent', label: 'Agent 管理', component: AgentManagement },
  { id: 'tasks', label: '集群任务' },
  { id: 'registry-mirrors', label: '节点镜像源', component: NodeRegistryMirrors },
  { id: 'dns', label: '集群 DNS', component: ClusterDNS },
  { id: 'chart-repositories', label: 'Chart 仓库', component: ChartRepositories },
  { id: 'system-components', label: '系统组件', component: SystemComponents },
]
const { activeTab, selectTab } = useRoutedTab({ tabs, defaultTab: 'topology' })
const agentRequiredTab = computed(() => ['dns', 'system-components'].includes(activeTab.value))
const cluster = ref(null)
const error = ref('')
const resource = useAsyncResource(({ signal }, id) => getCluster(id, { signal }), null)
const loading = resource.loading
const servers = ref([])
const deployments = ref([])
const latestDeployments = ref([])
const taskRunning = ref(false)
const taskPageSize = 10
const taskOffset = ref(0)
const taskTotal = ref(0)
const taskPage = computed(() => Math.floor(taskOffset.value / taskPageSize) + 1)
const taskPageCount = computed(() => Math.max(1, Math.ceil(taskTotal.value / taskPageSize)))
const selectedServerID = ref('')
const controlAddress = ref('')
const selectedDeploymentID = ref('')
const mirror = ref('official')
const mirrorOptions = [{ value: 'official', label: '官方源' }, { value: 'cn', label: '国内镜像' }]
const logs = ref([])
const topologyLoading = ref(false)
const topologyError = ref('')
const logError = ref('')
const submitError = ref('')
const submitting = ref(false)
const deploymentOpen = ref(false)
const adoptionOpen = ref(false)
const adoptServerID = ref('')
const adoptAddress = ref('')
const adoptError = ref('')
const adopting = ref(false)
const uninstallTarget = ref(null)
const uninstallError = ref('')
const operationError = computed(() => adoptError.value || uninstallError.value || submitError.value)
function clearOperationErrors() { adoptError.value = ''; uninstallError.value = ''; submitError.value = '' }
const uninstalling = ref(false)
let pollingTimer
let topologyRequestID = 0
let logRequestID = 0
const deploymentRole = computed(() => cluster.value?.control_server_id ? 'worker' : 'control-plane')
const clusterNodes = computed(() => servers.value.filter(server => Number(server.cluster_id) === Number(props.clusterID) && server.cluster_role))
const controlNode = computed(() => clusterNodes.value.find(server => server.cluster_role === 'control-plane'))
const workerNodes = computed(() => clusterNodes.value.filter(server => server.cluster_role === 'worker'))
const serverOptions = computed(() => servers.value.filter(server => !server.cluster_id && !server.cluster_role && !server.k8s_node_name).map(server => ({ value: String(server.id), label: serverDisplayLabel(server) })))
const selectedServer = computed(() => servers.value.find(server => String(server.id) === selectedServerID.value))
const adoptSelectedServer = computed(() => servers.value.find(server => String(server.id) === adoptServerID.value))
const selectedDeployment = computed(() => deployments.value.find(item => String(item.id) === String(selectedDeploymentID.value)))
const runningDeployment = computed(() => taskRunning.value)
const scopedComponent = computed(() => tabs.find(item => item.id === activeTab.value)?.component || null)

watch(() => props.clusterID, async id => {
  stopPolling()
  topologyRequestID += 1
  logRequestID += 1
  topologyLoading.value = false
  cluster.value = null
  servers.value = []
  deployments.value = []
  latestDeployments.value = []
  taskRunning.value = false
  taskOffset.value = 0
  taskTotal.value = 0
  logs.value = []
  selectedServerID.value = ''
  controlAddress.value = ''
  selectedDeploymentID.value = ''
  deploymentOpen.value = false
  adoptionOpen.value = false
  uninstallTarget.value = null
  error.value = ''
  const result = await resource.refresh(id)
  if (result && id === props.clusterID) { cluster.value = result; void loadTopology() }
  else if (resource.error.value) error.value = resource.error.value.message || '加载集群失败'
}, { immediate: true })

watch(activeTab, tab => {
  taskOffset.value = 0
  if (tab !== 'tasks') closeLogs()
  if (['tasks', 'topology', 'agent'].includes(tab) && cluster.value) {
    topologyRequestID += 1
    topologyLoading.value = false
    void loadTopology()
  }
})

onBeforeUnmount(() => { stopPolling(); topologyRequestID += 1; logRequestID += 1 })

function stopPolling() { if (pollingTimer) clearTimeout(pollingTimer); pollingTimer = undefined }
function schedulePoll() { stopPolling(); if (activeTab.value === 'tasks' || runningDeployment.value) pollingTimer = setTimeout(() => { void loadTopology() }, activeTab.value === 'tasks' ? 5000 : 1500) }

async function changeTaskPage(direction) {
  closeLogs()
  taskOffset.value = Math.max(0, taskOffset.value + direction * taskPageSize)
  await loadTopology()
}

async function loadTopology() {
  if (!cluster.value || topologyLoading.value) return
  const requestedClusterID = props.clusterID
  const requestID = ++topologyRequestID
  topologyLoading.value = true
  topologyError.value = ''
  try {
    const [clusterResult, serverResult, deploymentResult] = await Promise.all([getCluster(requestedClusterID), getServers(), getClusterDeployments(requestedClusterID, { limit: taskPageSize, offset: taskOffset.value })])
    if (requestID !== topologyRequestID) return
    cluster.value = clusterResult
    servers.value = serverResult || []
    deployments.value = deploymentResult?.items || []
    taskTotal.value = deploymentResult?.total || 0
    latestDeployments.value = deploymentResult?.latest_agent_task ? [deploymentResult.latest_agent_task] : []
    taskRunning.value = Boolean(deploymentResult?.running)
    if (activeTab.value === 'tasks') {
      if (selectedDeploymentID.value && !deployments.value.some(item => String(item.id) === selectedDeploymentID.value)) {
        closeLogs()
      }
      if (selectedDeploymentID.value) await loadLogs()
    }
  } catch (err) { if (requestID === topologyRequestID) topologyError.value = err.message || '加载部署状态失败' }
  finally { if (requestID === topologyRequestID) { topologyLoading.value = false; schedulePoll() } }
}

async function loadLogs() {
  if (!selectedDeploymentID.value) return
  const requestedClusterID = props.clusterID
  const requestedDeploymentID = selectedDeploymentID.value
  const requestID = ++logRequestID
  const allEntries = []
  let after = 0
  logError.value = ''
  try {
    let entries
    do {
      entries = await getClusterDeploymentLogs(requestedClusterID, requestedDeploymentID, after)
      if (requestID !== logRequestID || requestedClusterID !== props.clusterID || requestedDeploymentID !== selectedDeploymentID.value) return
      if (entries?.length) { allEntries.push(...entries); after = allEntries.at(-1).id }
    } while (entries?.length === 300)
    logs.value = allEntries
  } catch (err) { if (requestID === logRequestID) logError.value = err.message || '加载部署日志失败' }
}

function selectDeployment(task) {
  selectedDeploymentID.value = String(task.id)
  logs.value = []
  void loadLogs()
}
function closeLogs() {
  selectedDeploymentID.value = ''
  logRequestID += 1
  logs.value = []
  logError.value = ''
}
function openUninstall(server) { uninstallTarget.value = server; uninstallError.value = '' }
function closeUninstall() { if (!uninstalling.value) uninstallTarget.value = null }
async function confirmUninstall() {
  if (!uninstallTarget.value || uninstalling.value) return
  uninstalling.value = true
  uninstallError.value = ''
  try {
    await uninstallClusterNode(props.clusterID, uninstallTarget.value.id)
    uninstallTarget.value = null
    await loadTopology()
    selectTab('tasks')
  } catch (err) { uninstallError.value = err.message || '启动卸载失败' }
  finally { uninstalling.value = false }
}
async function handleAgentOperationStarted() {
  taskOffset.value = 0
  await selectTab('tasks')
  await loadTopology()
}
function openDeployment() { selectedServerID.value = ''; controlAddress.value = ''; mirror.value = 'official'; submitError.value = ''; deploymentOpen.value = true }
function closeDeployment() { if (!submitting.value) deploymentOpen.value = false }

function openAdoption() { adoptServerID.value = ''; adoptAddress.value = ''; adoptError.value = ''; adoptionOpen.value = true }
function closeAdoption() { if (!adopting.value) adoptionOpen.value = false }
async function startAdoption() {
  if (!adoptServerID.value || adopting.value) return
  adopting.value = true
  adoptError.value = ''
  try {
    await adoptClusterNode(props.clusterID, adoptServerID.value, deploymentRole.value, adoptAddress.value)
    adoptionOpen.value = false
    adoptServerID.value = ''
    closeLogs()
    await selectTab('tasks')
    await loadTopology()
  } catch (err) { adoptError.value = err.message || '启动纳管失败' }
  finally { adopting.value = false }
}

async function startDeployment() {
  if (!selectedServerID.value || submitting.value) return
  submitting.value = true
  submitError.value = ''
  try {
    await startClusterDeployment(props.clusterID, selectedServerID.value, deploymentRole.value, mirror.value, controlAddress.value)
    selectedServerID.value = ''
    controlAddress.value = ''
    closeLogs()
    deploymentOpen.value = false
    await selectTab('tasks')
    await loadTopology()
  } catch (err) { submitError.value = err.message || '启动部署失败' }
  finally { submitting.value = false }
}

function roleLabel(role) { return role === 'control-plane' ? '部署控制平面' : '添加工作节点' }
function taskLabel(task) {
  const agentLabels = { 'agent-install': '部署 Agent', 'agent-upgrade': '升级 Agent', 'agent-restart': '重启 Agent', 'agent-stop': '停止 Agent', 'agent-uninstall': '卸载 Agent', 'agent-test': '测试 Agent 连接' }
  if (agentLabels[task.operation]) return agentLabels[task.operation]
  return task.operation === 'uninstall' ? `卸载${task.role === 'control-plane' ? '控制平面' : '工作节点'}` : task.operation === 'adopt' ? `纳管${task.role === 'control-plane' ? '控制平面' : '工作节点'}` : roleLabel(task.role)
}
function taskDescription(task) { return task.operation?.startsWith('agent-') ? 'cluster-agent 运维' : `K3s ${task.operation === 'uninstall' ? '卸载' : task.operation === 'adopt' ? '纳管' : '部署'}` }
function deploymentStatusLabel(status) { return ({ running: '执行中', success: '成功', failed: '失败' })[status] || status }
function deploymentStatusClass(status) { return ({ running: 'badge-deploying', success: 'badge-online', failed: 'badge-danger' })[status] || 'badge-offline' }
function serverDisplayLabel(server) {
  const host = server.host?.trim() || ''
  const name = [server.hostname, server.name].map(value => value?.trim()).find(value => value && value !== host)
  return name && host ? `${name} · ${host}` : name || host || `服务器 #${server.id}`
}
function serverName(id) { const server = servers.value.find(item => Number(item.id) === Number(id)); return server?.hostname || server?.name || `服务器 #${id}` }
function nodeDisplayName(server) { return server.hostname || server.name || server.host }
function formatLogTime(value) { return value ? new Date(value).toLocaleTimeString('zh-CN', { hour12: false }) : '' }

function statusLabel(value) {
  return ({ draft: '待部署', deploying: '部署中', ready: '运行中', failed: '异常' })[value] || value || '-'
}

function statusClass(value) {
  return ({ deploying: 'badge-deploying', ready: 'badge-online', failed: 'badge-danger' })[value] || 'badge-offline'
}
</script>

<style scoped>
.cluster-detail-page { min-width: 0; }
.back-link { display: inline-flex; width: fit-content; align-items: center; gap: 6px; margin-bottom: var(--space-16); color: var(--text-secondary); font-size: 13px; font-weight: 600; text-decoration: none; }
.back-link:hover { color: var(--action-primary); }
.back-link:focus-visible { outline: 2px solid var(--focus); outline-offset: 3px; }
.cluster-detail-content { display: block; margin-top: var(--tabbed-page-content-gap); }
.cluster-detail-content > .table-wrap { overflow-x: auto; padding: var(--space-12) var(--space-20); }
.cluster-detail-content .data-table { min-width: 680px; }
.cluster-detail-content .cell-secondary { display: block; }
.cluster-deployment-toolbar { display: flex; min-height: 62px; align-items: center; justify-content: space-between; gap: 12px; padding: 12px var(--space-20); }
.cluster-toolbar-actions { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; }
.cluster-deployment-toolbar strong { font-size: 13px; }
.topology-error { margin: 0 var(--space-20) var(--space-12); }
.cluster-topology { display: flex; min-height: 280px; align-items: center; justify-content: center; flex-direction: column; padding: 28px var(--space-20) 34px; border-top: 1px solid var(--border-muted); background: var(--surface-subtle); }
.topology-node { display: flex; width: min(320px, 100%); min-height: 84px; align-items: center; gap: 14px; border: 1px solid var(--border); border-radius: var(--radius-control); padding: 14px 16px; background: var(--surface-raised); box-shadow: var(--shadow-soft); }
.topology-node--control { border-color: var(--action-primary); }
.topology-node-icon { display: grid; width: 40px; height: 40px; flex: 0 0 40px; place-items: center; border-radius: var(--radius-control); background: var(--surface-hover); color: var(--action-primary); }
.topology-node-copy { display: grid; min-width: 0; gap: 4px; }
.topology-node-action { margin-left: auto; flex: 0 0 auto; }
.topology-node-role { color: var(--action-primary); font-size: 10px; font-weight: 700; }
.topology-node-copy strong { color: var(--text-primary); font-size: 13px; line-height: 1.3; overflow-wrap: anywhere; }
.topology-node-copy .cell-secondary { overflow-wrap: anywhere; }
.topology-node-actions { display: flex; align-items: center; justify-content: flex-end; flex-wrap: wrap; gap: 6px; margin-left: auto; }
.topology-node--worker { width: min(280px, 100%); }
.topology-node--worker .topology-node-icon, .topology-node--worker .topology-node-role { color: var(--text-secondary); }
.topology-branch { position: relative; width: 1px; height: 28px; background: var(--border); }
.topology-branch::after { position: absolute; bottom: -3px; left: -3px; width: 7px; height: 7px; border-radius: 50%; background: var(--action-primary); content: ''; }
.topology-workers { display: grid; width: 100%; justify-items: center; gap: 12px; }
.topology-workers-heading { display: flex; align-items: center; gap: 7px; color: var(--text-secondary); font-size: 11px; font-weight: 700; }
.topology-workers-heading span { display: inline-grid; min-width: 18px; height: 18px; place-items: center; border-radius: 5px; background: var(--surface-hover); color: var(--action-primary); font: 10px var(--font-mono); }
.topology-worker-list { display: flex; width: 100%; justify-content: center; flex-wrap: wrap; gap: 12px; }
.cluster-deployment-form { display: grid; gap: 14px; }
.deployment-form-field { display: grid; gap: 6px; }
.deployment-form-field label { color: var(--text-muted); font-size: 11px; }
.deployment-hint { margin: 12px 0 0; color: var(--text-secondary); font-size: 11px; line-height: 1.6; }
.deployment-log-meta { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 12px; color: var(--text-secondary); font-size: 12px; }
.deployment-log-content { display: grid; gap: 12px; }
.deployment-log { min-height: 180px; max-height: min(58dvh, 580px); overflow: auto; border: 1px solid var(--border-muted); border-radius: var(--radius-control); padding: 12px; background: var(--surface-subtle); color: var(--text-secondary); font: 11px/1.6 var(--font-mono); white-space: pre-wrap; overflow-wrap: anywhere; }
.task-pagination { padding: 0 var(--space-20) var(--space-16); color: var(--text-secondary); font-size: 12px; }
@media (max-width: 640px) {
  .cluster-detail-content > .table-wrap { padding-right: 14px; padding-left: 14px; }
  .cluster-deployment-toolbar, .cluster-topology { padding-right: 14px; padding-left: 14px; }
  .topology-error { margin-right: 14px; margin-left: 14px; }
}
</style>
