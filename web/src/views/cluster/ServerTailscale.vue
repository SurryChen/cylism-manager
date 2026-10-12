<template>
  <SurfaceCard as="div" padding="none" class="tailscale-card">
    <div class="tailscale-toolbar">
      <span class="cell-secondary"><template v-if="cluster">部署网络：{{ cluster.network_mode === 'tailscale' ? 'Tailscale' : '公网或待核实' }} · </template>{{ loading ? `${completedCount} / ${servers.length} 台已检测 · ${onlineCount} 台在线` : `${onlineCount} / ${servers.length} 台在线` }}</span>
      <div class="tailscale-toolbar-actions">
        <slot name="actions" />
        <button v-if="!cluster" class="btn btn-sm" :disabled="checking || onlineCount < 2" @click="checkConnections"><Network :size="15" />{{ checking ? '检测中...' : '检测节点互通' }}</button>
        <button class="icon-button" :title="cluster ? '刷新组网与节点状态' : '刷新组网状态'" :aria-label="cluster ? '刷新组网与节点状态' : '刷新组网状态'" :disabled="loading || nodesLoading" @click="refreshAll"><RefreshCw :size="16" :class="{ 'is-spinning': loading || nodesLoading }" /></button>
      </div>
    </div>
    <EmptyState v-if="!servers.length" :message="cluster ? '当前集群暂无节点' : '暂无服务器'" />
    <div v-else class="table-wrap tailscale-table-wrap">
      <table class="data-table">
        <thead><tr><th>服务器</th><th>主机</th><th>组网状态</th><th>Tailscale IP</th><th v-if="cluster">K3s 节点 IP</th><th v-if="cluster">IP 对照</th><th v-else>设备名</th><th class="action-cell">操作</th></tr></thead>
        <tbody><tr v-for="server in servers" :key="server.id">
          <td>{{ server.name }}</td>
          <td>{{ server.host }}</td>
          <td><span :class="['badge', statusClass(statusFor(server.id))]" :title="statusFor(server.id)?.error || undefined">{{ pendingIDs.has(server.id) ? '检测中' : statusLabel(statusFor(server.id)) }}</span></td>
          <td>{{ statusFor(server.id)?.ip || '-' }}</td>
          <td v-if="cluster">{{ nodeFor(server)?.internal_ip || '-' }}</td>
          <td v-if="cluster"><button type="button" class="badge tailscale-check" :class="nodeMatch(server).className" :title="nodeMatch(server).reason" :aria-label="`${server.name}：${nodeMatch(server).label}，查看原因`" @click="selectedNodeCheck = nodeMatch(server)">{{ nodeMatch(server).label }}</button></td>
          <td v-else>{{ statusFor(server.id)?.device_name || '-' }}</td>
          <td class="action-cell"><RouterLink v-if="cluster" class="icon-button icon-button--table" :to="{ path: '/servers', query: { tab: 'tailscale', return: `/cluster/${cluster.id}?tab=networking` } }" :title="`前往服务器组网管理 ${server.name}`" :aria-label="`前往服务器组网管理 ${server.name}`"><Network :size="15" /></RouterLink><button v-else class="icon-button icon-button--table" type="button" :title="`管理 ${server.name} 的组网`" :aria-label="`管理 ${server.name} 的组网`" @click="openManage(server)"><Network :size="15" /></button></td>
        </tr></tbody>
      </table>
    </div>
    <slot name="footer" />
  </SurfaceCard>

  <BaseModal :open="Boolean(selectedNodeCheck)" title="IP 对照" size="small" teleport @close="selectedNodeCheck = null">
    <p>{{ selectedNodeCheck?.reason }}</p>
    <template #actions><button class="btn" type="button" @click="selectedNodeCheck = null">关闭</button></template>
  </BaseModal>

  <BaseModal :open="Boolean(selectedServer)" :title="`组网 · ${selectedServer?.name || ''}`" teleport :show-close="!working" :close-on-overlay="!working" :close-on-escape="!working" @close="closeManage">
    <div class="tailscale-detail">
      <div><span>状态</span><strong>{{ statusLabel(selectedStatus) }}</strong></div>
      <div><span>Tailscale IP</span><strong>{{ selectedStatus?.ip || '-' }}</strong></div>
      <div><span>设备名</span><strong>{{ selectedStatus?.device_name || '-' }}</strong></div>
    </div>
    <EmptyState v-if="pendingIDs.has(selectedServer?.id)" variant="loading" message="正在读取组网状态" />
    <div v-else-if="selectedStatus?.status === 'not_installed' || selectedStatus?.error_type === 'tailscale_daemon' || selectedStatus?.error_type === 'not_running'" class="tailscale-form">
      <button class="btn btn-primary" :disabled="working" @click="install">{{ working ? (selectedStatus?.status === 'not_installed' ? '安装中...' : '启动中...') : (selectedStatus?.status === 'not_installed' ? '安装 Tailscale' : '启动 Tailscale 服务') }}</button>
    </div>
    <form v-else-if="selectedStatus?.status === 'offline' && (!selectedStatus?.error_type || selectedStatus?.error_type === 'login_required')" class="tailscale-form" @submit.prevent="join">
      <label class="form-label" for="tailscale-auth-key">一次性授权密钥</label>
      <input id="tailscale-auth-key" v-model.trim="authKey" class="form-input" type="password" autocomplete="off" placeholder="tskey-auth-..." required />
      <button class="btn btn-primary" type="submit" :disabled="working || !authKey">{{ working ? '加入中...' : '加入网络' }}</button>
    </form>
    <p v-if="selectedStatus?.error" class="tailscale-error">{{ selectedStatus.error }}</p>
    <template #actions>
      <button v-if="selectedStatus?.status === 'online'" class="btn btn-danger" :disabled="working || Boolean(selectedServer?.cluster_id)" :title="selectedServer?.cluster_id ? '请先卸载集群节点' : '退出 Tailscale 网络'" @click="confirmLeave = true">退出网络</button>
      <button class="btn" :disabled="working" @click="refreshSelected"><RefreshCw :size="15" /> 重新检测</button>
      <button class="btn" :disabled="working" @click="closeManage">关闭</button>
    </template>
  </BaseModal>

  <BaseModal :open="confirmLeave" title="退出 Tailscale 网络" size="small" teleport @close="confirmLeave = false">
    <p>确定让“{{ selectedServer?.name }}”退出组网吗？</p>
    <template #actions><button class="btn" :disabled="working" @click="confirmLeave = false">取消</button><button class="btn btn-danger" :disabled="working" @click="leave">确认退出</button></template>
  </BaseModal>

  <BaseModal :open="connectionsOpen" :title="diagnosis || diagnosing ? '直连分析' : '节点互通检测'" size="large" teleport :show-close="!diagnosing" :close-on-overlay="!diagnosing" :close-on-escape="!diagnosing" @close="closeConnections">
    <EmptyState v-if="diagnosing" variant="loading" message="正在检测两端网络与连接路径" />
    <div v-else-if="diagnosis" class="connection-diagnosis">
      <div class="connection-diagnosis-summary">
        <strong>{{ nameFor(diagnosis.connection.from_id) }} → {{ nameFor(diagnosis.connection.to_id) }}</strong>
        <span :class="['badge', connectionBadge(diagnosis.connection)]">{{ connectionPath(diagnosis.connection) }}</span>
      </div>
      <p v-if="diagnosis.connection.error" class="tailscale-error">{{ diagnosis.connection.error }}</p>
      <h3>两端网络检测</h3>
      <div class="table-wrap"><table class="data-table"><thead><tr><th>服务器</th><th>出站 UDP</th><th>NAT 映射</th><th>检测结果</th></tr></thead><tbody><tr v-for="node in diagnosis.nodes" :key="node.server_id"><td>{{ nameFor(node.server_id) }}</td><td>{{ networkBoolean(node.udp, '可用', '不可用') }}</td><td>{{ networkBoolean(node.mapping_varies_by_dest_ip, '随目标变化', '稳定') }}</td><td>{{ node.error || '-' }}</td></tr></tbody></table></div>
      <h3>排查线索</h3>
      <ul class="connection-findings"><li v-for="finding in diagnosis.findings" :key="finding">{{ finding }}</li></ul>
    </div>
    <EmptyState v-else-if="!connections.length" message="暂无检测结果" />
    <div v-else class="table-wrap"><table class="data-table"><thead><tr><th>来源</th><th>目标</th><th>结果</th><th>连接方式</th><th>原因</th><th class="action-cell">操作</th></tr></thead><tbody><tr v-for="item in connections" :key="`${item.from_id}-${item.to_id}`"><td>{{ nameFor(item.from_id) }}</td><td>{{ nameFor(item.to_id) }}</td><td><span :class="['badge', connectionBadge(item)]">{{ connectionResult(item) }}</span></td><td>{{ connectionPath(item) }}</td><td>{{ item.error || '-' }}</td><td class="action-cell"><button v-if="item.path === 'relay' || item.path === 'inconclusive'" class="btn btn-sm" type="button" @click="diagnose(item)">分析连接</button></td></tr></tbody></table></div>
    <template #actions><button v-if="diagnosis" class="btn" @click="diagnosis = null">返回列表</button><button class="btn" :disabled="diagnosing" @click="closeConnections">关闭</button></template>
  </BaseModal>
  <ErrorNoticeModal :open="Boolean(operationError)" :message="operationError" teleport @close="operationError = ''" />
</template>

<script setup>
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { Network, RefreshCw } from 'lucide-vue-next'
import { checkTailscaleConnections, diagnoseTailscaleConnection, getTailscaleStatus, installTailscale, joinTailscale, leaveTailscale } from '../../api/servers.js'
import BaseModal from '../../components/BaseModal.vue'
import EmptyState from '../../components/EmptyState.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import SurfaceCard from '../../components/SurfaceCard.vue'

const props = defineProps({ servers: { type: Array, default: () => [] }, cluster: { type: Object, default: null }, nodeInfos: { type: Array, default: () => [] }, nodesLoading: { type: Boolean, default: false }, nodesError: { type: String, default: '' } })
const emit = defineEmits(['refresh-nodes'])
const statuses = ref([])
const loading = ref(false)
const pendingIDs = ref(new Set())
const operationError = ref('')
const selectedServer = ref(null)
const selectedNodeCheck = ref(null)
const authKey = ref('')
const working = ref(false)
const checking = ref(false)
const confirmLeave = ref(false)
const connections = ref([])
const connectionsOpen = ref(false)
const diagnosis = ref(null)
const diagnosing = ref(false)
const statusMap = computed(() => new Map(statuses.value.map(status => [Number(status.server_id), status])))
const onlineCount = computed(() => statuses.value.filter(status => status.status === 'online').length)
const completedCount = computed(() => props.servers.length - pendingIDs.value.size)
const selectedStatus = computed(() => statusFor(selectedServer.value?.id))
let refreshController
let refreshGeneration = 0

function statusFor(id) { return statusMap.value.get(Number(id)) }
function nodeFor(server) { return server.k8s_node_name ? props.nodeInfos.find(node => node.name?.toLowerCase() === server.k8s_node_name.toLowerCase()) : null }
function nodeMatch(server) {
  if (props.nodesLoading) return { label: '检测中', className: 'badge-offline', reason: '正在读取 K3s 节点信息。' }
  if (props.nodesError) return { label: '读取失败', className: 'badge-danger', reason: `读取 K3s 节点失败：${props.nodesError}` }
  if (!props.nodeInfos.length) return { label: '无节点数据', className: 'badge-warn', reason: 'Kubernetes API 未返回节点，无法获取 K3s Node InternalIP。请检查集群节点和 Agent 连接。' }
  if (!server.k8s_node_name) return { label: '未关联节点', className: 'badge-warn', reason: '服务器没有关联的 Kubernetes 节点名，无法匹配 K3s 节点。' }
  if (!nodeFor(server)) return { label: '未匹配节点', className: 'badge-warn', reason: `未找到与 ${server.k8s_node_name} 同名的 Kubernetes 节点。` }
  const nodeIP = nodeFor(server)?.internal_ip
  const tailscaleIP = statusFor(server.id)?.status === 'online' ? statusFor(server.id)?.ip : ''
  if (!nodeIP) return { label: '缺少节点 IP', className: 'badge-warn', reason: `Kubernetes 节点 ${server.k8s_node_name} 没有返回 InternalIP。` }
  if (!tailscaleIP) return { label: '缺少组网 IP', className: 'badge-warn', reason: '当前服务器没有可用的 Tailscale IP，请检查组网状态。' }
  return nodeIP === tailscaleIP
    ? { label: 'IP 一致', className: 'badge-online', reason: 'K3s Node InternalIP 与当前 Tailscale IP 一致；这不能单独证明 K3s 的全部网络配置。' }
    : { label: 'IP 不一致', className: 'badge-warn', reason: `K3s Node InternalIP ${nodeIP} 与 Tailscale IP ${tailscaleIP} 不一致。` }
}
function nameFor(id) { return props.servers.find(server => Number(server.id) === Number(id))?.name || `#${id}` }
function connectionPath(item) {
  if (item?.path === 'direct') return '直连'
  if (item?.path === 'relay') return `DERP ${item.relay || '中继'}`
  return item?.ok ? '路径未知' : '-'
}
function connectionResult(item) { return item?.ok ? '可达' : item?.path === 'error' ? '检测失败' : '未确认' }
function connectionBadge(item) { return item?.ok ? (item?.path === 'relay' ? 'badge-warn' : 'badge-online') : item?.path === 'error' ? 'badge-danger' : 'badge-offline' }
function networkBoolean(value, yes, no) { return value === true ? yes : value === false ? no : '未知' }
function statusLabel(status) {
  if (status?.status === 'unreachable') {
    return ({
      ssh_auth: 'SSH 认证失败',
      ssh_timeout: 'SSH 连接超时',
      ssh_connection: 'SSH 连接失败',
      command_timeout: '状态查询超时',
      tailscale_daemon: 'tailscaled 不可用',
      tailscale_command: 'Tailscale 查询失败',
      invalid_response: '状态响应异常',
    })[status.error_type] || '无法检测'
  }
  if (status?.status === 'offline') {
    return ({ login_required: '待登录', not_running: '服务未运行', missing_ip: '无 IPv4', device_offline: '设备离线', unknown_state: '状态异常' })[status.error_type] || '待登录'
  }
  return ({ online: '在线', not_installed: '未安装' })[status?.status] || '待检测'
}
function statusClass(status) { return status?.status === 'online' ? 'badge-online' : status?.status === 'unreachable' ? 'badge-danger' : 'badge-offline' }
function refreshAll() { if (props.cluster) emit('refresh-nodes'); refresh() }
async function refresh() {
  refreshController?.abort()
  const controller = new AbortController()
  refreshController = controller
  const generation = ++refreshGeneration
  const ids = props.servers.map(server => server.id)
  statuses.value = []
  pendingIDs.value = new Set(ids)
  loading.value = ids.length > 0
  let next = 0
  async function worker() {
    while (next < ids.length && !controller.signal.aborted) {
      const id = ids[next++]
      try {
        const status = await getTailscaleStatus(id, { signal: controller.signal })
        if (generation === refreshGeneration) replaceStatus(status)
      } catch (error) {
        if (generation === refreshGeneration && error?.name !== 'AbortError') {
          replaceStatus({ server_id: id, status: 'unreachable', error: error?.message || '读取组网状态失败' })
        }
      } finally {
        if (generation === refreshGeneration) {
          const pending = new Set(pendingIDs.value)
          pending.delete(id)
          pendingIDs.value = pending
        }
      }
    }
  }
  await Promise.all(Array.from({ length: Math.min(4, ids.length) }, worker))
  if (generation === refreshGeneration) loading.value = false
}
async function refreshSelected() {
  if (!selectedServer.value) return
  try { replaceStatus(await getTailscaleStatus(selectedServer.value.id)) }
  catch (error) { operationError.value = error.message || '检测组网状态失败' }
}
function replaceStatus(status) { statuses.value = [...statuses.value.filter(item => item.server_id !== status.server_id), status] }
function openManage(server) { selectedServer.value = server; authKey.value = ''; if (!statusFor(server.id) && !pendingIDs.value.has(server.id)) refreshSelected() }
function closeManage() { if (working.value) return; selectedServer.value = null; authKey.value = ''; confirmLeave.value = false }
async function install() { await operate(() => installTailscale(selectedServer.value.id), '安装 Tailscale 失败') }
async function join() { await operate(() => joinTailscale(selectedServer.value.id, authKey.value), '加入 Tailscale 失败'); authKey.value = '' }
async function leave() { await operate(() => leaveTailscale(selectedServer.value.id), '退出 Tailscale 失败'); confirmLeave.value = false }
async function operate(action, fallback) {
  if (working.value) return
  working.value = true
  try { replaceStatus(await action()); operationError.value = '' }
  catch (error) { operationError.value = error.message || fallback }
  finally { working.value = false }
}
async function checkConnections() {
  checking.value = true
  try { connections.value = await checkTailscaleConnections() || []; diagnosis.value = null; connectionsOpen.value = true; operationError.value = '' }
  catch (error) { operationError.value = error.message || '节点互通检测失败' }
  finally { checking.value = false }
}
async function diagnose(item) {
  diagnosing.value = true
  try {
    const result = await diagnoseTailscaleConnection(item.from_id, item.to_id)
    diagnosis.value = result
    connections.value = connections.value.map(connection => connection.from_id === item.from_id && connection.to_id === item.to_id ? result.connection : connection)
    operationError.value = ''
  } catch (error) { operationError.value = error.message || '直连分析失败' }
  finally { diagnosing.value = false }
}
function closeConnections() { if (diagnosing.value) return; connectionsOpen.value = false; diagnosis.value = null }
watch(() => props.servers, refresh, { immediate: true })
onBeforeUnmount(() => { refreshGeneration++; refreshController?.abort() })
</script>

<style scoped>
.tailscale-toolbar,.tailscale-toolbar-actions{display:flex;align-items:center;gap:8px}
.tailscale-toolbar{min-height:62px;justify-content:space-between;padding:12px 20px}
.tailscale-table-wrap{padding:0 20px 12px}
.tailscale-table-wrap th:last-child,.tailscale-table-wrap td:last-child{text-align:right}
.tailscale-check{border:0;cursor:pointer}.tailscale-check:focus-visible{outline:2px solid var(--focus);outline-offset:2px}
.tailscale-detail{display:grid;gap:0}
.tailscale-detail>div{display:grid;grid-template-columns:130px minmax(0,1fr);gap:12px;padding:10px 0;border-bottom:1px solid var(--border-muted);font-size:12px}
.tailscale-detail span{color:var(--text-secondary)}.tailscale-detail strong{font-weight:500;overflow-wrap:anywhere}
.tailscale-form{display:flex;flex-direction:column;align-items:flex-start;gap:10px;margin-top:20px}.tailscale-form .form-input{width:100%}
.tailscale-error{color:var(--danger);font-size:12px}
.connection-diagnosis-summary{display:flex;align-items:center;justify-content:space-between;gap:12px;padding:4px 12px 16px;font-size:13px}
.connection-diagnosis h3{margin:16px 12px 8px;font-size:12px;font-weight:600}
.connection-findings{display:grid;gap:8px;margin:0;padding:0 12px 4px 28px;color:var(--text-secondary);font-size:12px;line-height:1.5}
@media(max-width:640px){.tailscale-toolbar{padding:12px 14px}.tailscale-table-wrap{padding:0 14px 12px}.tailscale-detail>div{grid-template-columns:100px minmax(0,1fr)}}
</style>
