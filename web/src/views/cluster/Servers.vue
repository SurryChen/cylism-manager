<template>
  <div>
    <RouterLink v-if="returnToCluster" class="back-link" :to="returnToCluster"><ArrowLeft :size="16" />返回集群组网</RouterLink>
    <SectionTabsHeader title="服务器" :tabs="sections" :active-tab="activeSection" @select="activeSection = $event" />

    <main class="server-content">
    <div v-if="serverListError" class="k8s-banner k8s-banner-warn section-gap">{{ serverListError }}</div>
    <template v-if="activeSection === 'configuration'">
    <SurfaceCard as="div" padding="none" class="server-configuration-card">
      <div class="server-list-toolbar">
        <button class="icon-button" title="刷新服务器列表" aria-label="刷新服务器列表" :disabled="serversLoading" @click="fetchServers"><RefreshCw :size="16" :class="{ 'is-spinning': serversLoading }" /></button>
        <button data-testid="add-server" class="btn btn-primary" @click="showAdd = true"><Plus :size="16" /> 添加服务器</button>
      </div>
      <div v-if="servers.length === 0" class="empty-state">
        <span class="empty-icon">⬡</span><span class="empty-text">暂无服务器</span>
      </div>
      <div v-else class="table-wrap server-table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <th>名称</th><th>主机</th><th>连接方式</th><th>SSH 用户</th><th>认证方式</th><th>主机名</th><th class="action-cell">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="srv in servers" :key="srv.id">
              <td>{{ srv.name }}</td>
              <td>{{ srv.host }}</td>
              <td>{{ srv.proxy_server_id ? `代理访问 · ${srv.proxy_server_name || serverName(srv.proxy_server_id)}` : '直接访问' }}</td>
              <td>{{ srv.ssh_user || 'root' }}</td>
              <td>{{ srv.ssh_auth_type === 'key' ? '密钥' : '密码' }}</td>
              <td>{{ srv.hostname || '-' }}</td>
              <td class="action-cell">
                <div class="btn-group action-cell">
                  <button class="icon-button icon-button--table" type="button" :title="`编辑 ${srv.name}`" :aria-label="`编辑 ${srv.name}`" @click="startEdit(srv)"><Pencil :size="14" /></button>
                  <button v-if="!srv.cluster_role && !srv.cluster_id" class="icon-button icon-button--table" type="button" :title="`删除 ${srv.name}`" :aria-label="`删除 ${srv.name}`" @click="confirmDelete(srv)"><Trash2 :size="14" /></button>
                  <button v-if="!srv.cluster_id && (srv.cluster_role || srv.k8s_node_name)" class="icon-button icon-button--table" type="button" :title="`解除 ${srv.name} 的集群绑定`" :aria-label="`解除 ${srv.name} 的集群绑定`" :disabled="unbindingId === srv.id" @click="unbindServer(srv)"><LoaderCircle v-if="unbindingId === srv.id" :size="14" class="is-spinning" /><Unlink2 v-else :size="14" /></button>
                  <button class="icon-button icon-button--table" type="button" :title="`打开 ${srv.name} 的终端`" :aria-label="`打开 ${srv.name} 的终端`" @click="openTerminal(srv.id)"><Terminal :size="14" /></button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </SurfaceCard>
    </template>

    <ServerTailscale v-else-if="activeSection === 'tailscale'" :servers="servers" />

    <template v-else-if="activeSection === 'monitoring'">
      <SurfaceCard as="div" padding="none" class="resource-monitoring-card">
        <div class="resource-monitoring-toolbar">
          <span class="resource-monitoring-meta">{{ resourceSamplingLabel }}</span>
          <button class="icon-button" title="刷新资源数据" aria-label="刷新资源数据" :disabled="resourceStatsLoading" @click="refreshResourceStats"><RefreshCw :size="16" :class="{ 'is-spinning': resourceStatsLoading }" /></button>
        </div>
        <div v-if="resourceStatsError" class="k8s-banner k8s-banner-warn">{{ resourceStatsError }}</div>
        <div v-if="servers.length === 0" class="empty-state"><span class="empty-icon">⬡</span><span class="empty-text">暂无服务器</span></div>
        <div v-else-if="resourceStatsLoading && !resourceStats.length" class="empty-state"><span class="empty-text">正在采集服务器资源...</span></div>
        <div v-else class="table-wrap resource-table-wrap"><table class="data-table resource-table"><thead><tr><th>服务器</th><th>主机</th><th>采集状态</th><th>CPU</th><th>内存</th><th>磁盘 /</th><th>负载 (1 分钟)</th><th>运行时间</th><th class="action-cell">操作</th></tr></thead><tbody><tr v-for="srv in servers" :key="srv.id" class="resource-row"><td class="resource-server-name">{{ srv.name }}</td><td class="resource-server-host">{{ srv.host }}</td><td><span class="badge" :class="resourceStatusClass(resourceFor(srv.id))">{{ resourceStatusLabel(resourceFor(srv.id)) }}</span><small v-if="resourceFor(srv.id)?.error" class="resource-error">{{ resourceFor(srv.id).error }}</small></td><td><div class="resource-metric"><strong>{{ formatPercent(resourceFor(srv.id)?.cpu_percent) }}</strong><span class="resource-meter"><i :class="resourceLevelClass(resourceFor(srv.id)?.cpu_percent)" :style="{ width: `${metricPercent(resourceFor(srv.id)?.cpu_percent)}%` }" /></span></div></td><td><div class="resource-metric"><strong>{{ formatMB(resourceFor(srv.id)?.memory_used_mb) }} / {{ formatMB(resourceFor(srv.id)?.memory_total_mb) }}</strong><span class="resource-meter"><i :class="resourceLevelClass(memPercent(resourceFor(srv.id)))" :style="{ width: `${metricPercent(memPercent(resourceFor(srv.id)))}%` }" /></span></div></td><td><div class="resource-metric"><strong>{{ resourceFor(srv.id)?.disk_used_gb ?? '-' }} / {{ resourceFor(srv.id)?.disk_total_gb ?? '-' }} GB</strong><span class="resource-meter"><i :class="resourceLevelClass(diskPercent(resourceFor(srv.id)))" :style="{ width: `${metricPercent(diskPercent(resourceFor(srv.id)))}%` }" /></span></div></td><td>{{ formatLoad(resourceFor(srv.id)) }}</td><td>{{ formatUptime(resourceFor(srv.id)?.uptime) }}</td><td class="action-cell"><button class="icon-button icon-button--table" type="button" :data-testid="`view-resource-stats-${srv.id}`" :title="`查看 ${srv.name} 的资源详情`" :aria-label="`查看 ${srv.name} 的资源详情`" @click="openStats(srv.id)"><ChartNoAxesColumnIncreasing :size="14" /></button></td></tr></tbody></table></div>
      </SurfaceCard>
    </template>

    </main>

    <!-- Add Server modal -->
    <div v-if="showAdd" class="overlay" @click.self="closeForm">
      <div class="modal">
        <h2 class="modal-title">{{ editingId ? '编辑服务器' : '添加服务器' }}</h2>
        <form @submit.prevent="addServer">
          <div class="form-group"><label class="form-label" for="server-name">名称</label><input id="server-name" v-model.trim="form.name" class="form-input" placeholder="留空使用主机 IP" /></div>
          <div class="form-group"><label class="form-label">主机</label><input v-model="form.host" class="form-input" placeholder="192.168.1.100" required /></div>
          <div class="form-group"><label class="form-label">连接方式</label><SelectMenu v-model="form.connection_mode" :options="connectionModeOptions" aria-label="连接方式" /></div>
          <div v-if="form.connection_mode === 'proxy'" class="form-group"><label class="form-label">代理节点</label><SelectMenu v-model="form.proxy_server_id" :options="proxyServerOptions" placeholder="选择可直接访问的服务器" aria-label="代理节点" /><span class="cell-secondary">主机请填写代理节点可连接的目标 IP 地址</span></div>
          <div class="form-row">
            <div class="form-group"><label class="form-label">SSH 端口</label><input v-model.number="form.ssh_port" class="form-input" type="number" placeholder="22" /></div>
            <div class="form-group"><label class="form-label">SSH 用户</label><input v-model="form.ssh_user" class="form-input" placeholder="root" /></div>
          </div>
          <div class="form-group"><label class="form-label">认证方式</label><SelectMenu v-model="form.ssh_auth_type" class="form-select"><option value="password">密码</option><option value="key">密钥</option></SelectMenu></div>
          <div class="form-group" v-if="form.ssh_auth_type === 'password'"><label class="form-label">SSH 密码</label><input v-model="form.ssh_password" class="form-input" type="password" :placeholder="editingId ? '留空保持原密码' : '输入密码'" /></div>
          <div class="form-group" v-if="form.ssh_auth_type === 'key'"><label class="form-label">SSH 密钥</label><textarea v-model="form.ssh_key" class="form-input textarea-input" :placeholder="editingId ? '留空保持原密钥' : '粘贴私钥内容'" /></div>
          <div class="form-group">
            <label class="form-label" for="server-hostname">机器名</label>
            <div class="hostname-form-row">
              <input id="server-hostname" :value="form.hostname" class="form-input" placeholder="点击自动获取" readonly />
              <button type="button" class="btn" title="自动获取机器名" aria-label="自动获取机器名" :disabled="hostnameFetching || savingServer || !form.host.trim() || !form.ssh_port" @click="fetchHostname"><RefreshCw :size="15" :class="{ 'is-spinning': hostnameFetching }" />{{ hostnameFetching ? '获取中...' : '自动获取' }}</button>
            </div>
            <p v-if="hostnameError" class="form-error" role="status">{{ hostnameError }}</p>
          </div>
          <div v-if="editingId" class="server-probe">
            <button type="button" class="btn btn-sm" :disabled="probing || savingServer || !form.host.trim() || !form.ssh_port" @click="probeServer">{{ probing ? '检测中...' : '测试连接' }}</button>
            <span v-if="probeResult" class="server-probe-result" :class="probeResult.reachable ? 'status-success' : 'status-warning'" role="status">{{ probeResult.reachable ? `连接成功 (${probeResult.latency_ms}ms)` : `连接失败：${probeResult.error || 'SSH 不可达'}` }}</span>
          </div>
          <p v-if="formError" class="form-error">{{ formError }}</p>
          <div class="modal-actions"><button type="button" class="btn" :disabled="savingServer" @click="closeForm">取消</button><button type="submit" class="btn btn-primary" :disabled="savingServer || hostnameFetching">{{ savingServer ? '保存中...' : (editingId ? '保存修改' : '确认添加') }}</button></div>
        </form>
      </div>
    </div>

    <!-- Stats modal -->
    <div v-if="statsServer" class="overlay" @click.self="statsServer = null">
      <div class="modal modal-wide">
        <h2 class="modal-title">资源监控 — {{ statsServer?.name }}</h2>
        <div v-if="statsLoading" style="color:var(--text-secondary);text-align:center;padding:20px">加载中...</div>
        <div v-else-if="chartLoadError" class="empty-state"><span class="empty-text">{{ chartLoadError }}</span></div>
        <div v-else-if="!DoughnutComponent" class="empty-state"><span class="empty-text">正在加载图表...</span></div>
        <div v-else class="stats-grid">
          <div class="stats-card">
            <component :is="DoughnutComponent" :data="cpuChartData" :options="chartOptions" />
            <div class="ring-title">CPU</div>
          </div>
          <div class="stats-card">
            <component :is="DoughnutComponent" :data="memChartData" :options="chartOptions" />
            <div class="ring-title">内存</div>
            <div class="ring-detail">{{ formatMB(statsData.memory_used_mb) }} / {{ formatMB(statsData.memory_total_mb) }}</div>
          </div>
          <div class="stats-card">
            <component :is="DoughnutComponent" :data="diskChartData" :options="chartOptions" />
            <div class="ring-title">磁盘 /</div>
            <div class="ring-detail">{{ statsData.disk_used_gb || 0 }} / {{ statsData.disk_total_gb || 0 }} GB</div>
          </div>
        </div>
        <div class="stats-info" style="margin-top:8px">
          <span>⚡ 负载 {{ statsData.load_1m?.toFixed(2) || '-' }} / {{ statsData.load_5m?.toFixed(2) || '-' }} / {{ statsData.load_15m?.toFixed(2) || '-' }}</span>
          <span style="margin-left:16px">⏱ 运行 {{ statsData.uptime || '-' }}</span>
        </div>
        <div class="modal-actions" style="margin-top:12px">
          <button class="btn" @click="statsServer = null">关闭</button>
          <button class="btn btn-primary" @click="openStats(statsServer.id)">刷新</button>
        </div>
      </div>
    </div>

    <ServerTerminal v-if="terminalServer" :server="terminalServer" @close="terminalServer = null" />

    <div v-if="deleteTarget" class="overlay" @click.self="deleteTarget = null">
      <div class="modal"><h2 class="modal-title">删除服务器</h2><p class="modal-copy">确定删除 <strong>{{ deleteTarget.name }}</strong> 吗？</p><p v-if="deleteError" class="form-error">{{ deleteError }}</p><div class="modal-actions"><button class="btn" :disabled="deletingServer" @click="deleteTarget = null">取消</button><button class="btn btn-danger" :disabled="deletingServer" @click="deleteServer">{{ deletingServer ? '删除中...' : '确认删除' }}</button></div></div>
    </div>
    <ErrorNoticeModal :open="Boolean(unbindError)" :message="unbindError" teleport @close="unbindError = ''" />
  </div>
</template>

<script setup>
import { computed, ref, shallowRef, onMounted, onUnmounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import {
  createServer,
  deleteServer as deleteServerRequest,
  getServerResourceStats,
  getServers,
  getServerStats,
  probeServer as probeServerRequest,
  resolveServerHostname,
  unbindServer as unbindServerRequest,
  updateServer,
} from '../../api/servers.js'
import { useAsyncResource } from '../../composables/useAsyncResource.js'
import { usePolling } from '../../composables/usePolling.js'
import { formatClockTime as formatSampleTime } from '../../utils/formatters.js'
import { ArrowLeft, ChartNoAxesColumnIncreasing, LoaderCircle, Pencil, Plus, RefreshCw, Terminal, Trash2, Unlink2 } from 'lucide-vue-next'
import SectionTabsHeader from '../../components/SectionTabsHeader.vue'
import SurfaceCard from '../../components/SurfaceCard.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import ServerTerminal from './ServerTerminal.vue'
import ServerTailscale from './ServerTailscale.vue'

const serversResource = useAsyncResource(({ signal }) => getServers({ signal }), [])
const servers = serversResource.data
const serversLoading = serversResource.loading
const serverListError = computed(() => serversResource.error.value?.message || '')
const route = useRoute()
const activeSection = ref(route?.query?.tab === 'tailscale' ? 'tailscale' : 'configuration')
const returnToCluster = computed(() => /^\/cluster\/\d+\?tab=networking$/.test(String(route?.query?.return || '')) ? route.query.return : '')
watch(() => route?.query?.tab, tab => { if (tab === 'tailscale') activeSection.value = 'tailscale' })
const sections = [
  { id: 'configuration', label: '基本配置' },
  { id: 'tailscale', label: '组网' },
  { id: 'monitoring', label: '资源监控' },
]
const resourceStatsResource = useAsyncResource(({ signal }) => getServerResourceStats({ signal }), [])
const resourceStats = resourceStatsResource.data
const resourceStatsLoading = resourceStatsResource.loading
const resourceStatsError = computed(() => resourceStatsResource.error.value ? '资源数据刷新失败，已保留上次成功结果' : '')
const resourceStatsUpdatedAt = ref('')
const showAdd = ref(false)
const editingId = ref(null)
const deleteTarget = ref(null)
const formError = ref('')
const savingServer = ref(false)
const deleteError = ref('')
const deletingServer = ref(false)
const unbindError = ref('')
const probing = ref(false)
const hostnameFetching = ref(false)
const hostnameError = ref('')
let hostnameRequestToken = 0
const unbindingId = ref(null)
const probeResult = ref(null)
const statsServer = ref(null)
const DoughnutComponent = shallowRef(null)
const chartLoadError = ref('')
const serverStatsResource = useAsyncResource(({ signal }, serverID) => getServerStats(serverID, { signal }), {})
const statsData = serverStatsResource.data
const statsLoading = serverStatsResource.loading
const terminalServer = ref(null)
const resourcePolling = usePolling(refreshResourceStats, { interval: 10000 })

// Chart.js computed ring data
function chartRingData(percent, label) {
  const p = Math.min(100, Math.max(0, Number(percent) || 0))
  const full = percent >= 90 ? '#d84a3e' : percent >= 70 ? '#b86412' : '#22736b'
  return {
    labels: [label, ''],
    datasets: [{
      data: [p, 100 - p],
      backgroundColor: [full, 'transparent'],
      borderColor: [full, 'transparent'],
      borderWidth: 0,
      cutout: '80%',
    }],
  }
}
const cpuChartData = computed(() => chartRingData(statsData.value.cpu_percent, 'CPU'))
const memChartData = computed(() => chartRingData(memPercent(statsData.value), 'Mem'))
const diskChartData = computed(() => chartRingData(diskPercent(statsData.value), 'Disk'))
const resourceStatsByServerID = computed(() => new Map(resourceStats.value.map(stats => [Number(stats.server_id), stats])))
const resourceSamplingLabel = computed(() => {
  if (resourceStatsLoading.value) return '正在采集资源数据...'
  if (!resourceStatsUpdatedAt.value) return '进入此视图后开始采集'
  return `上次采集 ${formatSampleTime(resourceStatsUpdatedAt.value)} · 每 10 秒自动刷新`
})

const chartOptions = {
  responsive: true,
  maintainAspectRatio: true,
  plugins: {
    tooltip: { enabled: false },
    legend: { display: false },
  },
}



const emptyForm = () => ({ name: '', host: '', connection_mode: 'direct', proxy_server_id: '', ssh_port: 22, ssh_user: 'root', ssh_auth_type: 'password', ssh_password: '', ssh_key: '', hostname: '' })
const form = ref(emptyForm())
const connectionModeOptions = [{ value: 'direct', label: '直接访问' }, { value: 'proxy', label: '代理访问' }]
const proxyServerOptions = computed(() => servers.value.filter(server => !server.proxy_server_id && server.id !== editingId.value).map(server => ({ value: String(server.id), label: `${server.name} (${server.host})` })))
const connectionSnapshot = (value) => JSON.stringify([value.host, value.connection_mode, value.proxy_server_id, value.ssh_port, value.ssh_user, value.ssh_auth_type, value.ssh_password, value.ssh_key])
function serverName(id) { return servers.value.find(server => server.id === id)?.name || `#${id}` }
function connectionPayload(value) { const { connection_mode, ...details } = value; return { ...details, proxy_server_id: connection_mode === 'proxy' ? Number(value.proxy_server_id) || null : null } }
let hostnameConnectionSnapshot = connectionSnapshot(form.value)
watch(form, () => {
  probeResult.value = null
  const current = connectionSnapshot(form.value)
  if (current !== hostnameConnectionSnapshot) {
    hostnameConnectionSnapshot = current
    form.value.hostname = ''
    hostnameError.value = ''
  }
}, { deep: true })

onMounted(() => {
  fetchServers()
  document.addEventListener('visibilitychange', syncResourcePolling)
})
onUnmounted(() => {
  resourcePolling.stop()
  document.removeEventListener('visibilitychange', syncResourcePolling)
})

watch(activeSection, (section) => {
  syncResourcePolling()
})

async function fetchServers() {
  await serversResource.refresh()
}

function resourceFor(serverID) { return resourceStatsByServerID.value.get(Number(serverID)) || null }
function metricPercent(value) { return Math.min(100, Math.max(0, Number(value) || 0)) }
function formatPercent(value) { return value == null ? '-' : `${metricPercent(value).toFixed(1)}%` }
function resourceLevelClass(value) { const percent = metricPercent(value); return percent >= 90 ? 'is-danger' : percent >= 70 ? 'is-warning' : 'is-ok' }
function resourceStatusClass(stats) { return stats?.status === 'ready' ? 'badge-online' : stats?.status === 'unreachable' ? 'badge-danger' : 'badge-deploying' }
function resourceStatusLabel(stats) { return stats?.status === 'ready' ? '已采集' : stats?.status === 'unreachable' ? '不可达' : '等待采集' }
function formatLoad(stats) { if (!stats?.load_1m && stats?.load_1m !== 0) return '-'; const cores = Number(stats.cpu_cores) || 0; return cores ? `${Number(stats.load_1m).toFixed(2)} / ${cores} 核` : Number(stats.load_1m).toFixed(2) }
function formatUptime(value) {
  if (!value) return '-'
  const labels = { week: '周', day: '天', hour: '小时', minute: '分钟' }
  const parts = []
  for (const match of String(value).matchAll(/(\d+)\s*(weeks?|days?|hours?|minutes?)/gi)) {
    const unit = match[2].replace(/s$/i, '')
    if (labels[unit]) parts.push(`${match[1]}${labels[unit]}`)
  }
  return parts.length ? parts.slice(0, 2).join('') : value
}
async function refreshResourceStats() {
  const result = await resourceStatsResource.refresh()
  if (result !== undefined) {
    resourceStatsUpdatedAt.value = new Date().toISOString()
  }
}

function stopResourcePolling() {
  resourcePolling.stop()
}

function syncResourcePolling() {
  stopResourcePolling()
  if (activeSection.value !== 'monitoring' || document.hidden) return
  resourcePolling.start({ immediate: true })
}

async function addServer() {
  if (savingServer.value) return
  formError.value = ''
  if (form.value.connection_mode === 'proxy' && !form.value.proxy_server_id) {
    formError.value = '请选择代理节点'
    return
  }
  if (!form.value.hostname.trim()) {
    hostnameError.value = '请先点击“自动获取”获取机器名'
    return
  }
  if (servers.value.some(server => server.host === form.value.host.trim() && server.id !== editingId.value)) {
    formError.value = '该主机已添加，请编辑已有服务器'
    return
  }
  savingServer.value = true
  try {
    if (editingId.value) {
      const { ssh_password, ssh_key, ...details } = connectionPayload(form.value)
      await updateServer(editingId.value, {
        ...details,
        ...(ssh_password ? { ssh_password } : {}),
        ...(ssh_key ? { ssh_key } : {}),
      })
    } else {
      await createServer(connectionPayload(form.value))
    }
    closeForm()
    await fetchServers()
  } catch (e) { formError.value = e.message || '保存服务器失败' }
  finally { savingServer.value = false }
}

function startEdit(srv) {
  editingId.value = srv.id
  probeResult.value = null
  const nextForm = {
    name: srv.name === srv.host ? '' : srv.name,
    host: srv.host,
    connection_mode: srv.proxy_server_id ? 'proxy' : 'direct',
    proxy_server_id: srv.proxy_server_id ? String(srv.proxy_server_id) : '',
    ssh_port: srv.ssh_port || 22,
    ssh_user: srv.ssh_user || 'root',
    ssh_auth_type: srv.ssh_auth_type || 'password',
    ssh_password: '',
    ssh_key: '',
    hostname: srv.hostname || '',
  }
  hostnameConnectionSnapshot = connectionSnapshot(nextForm)
  form.value = nextForm
  hostnameError.value = ''
  showAdd.value = true
}

function closeForm() {
  hostnameRequestToken++
  hostnameFetching.value = false
  showAdd.value = false
  editingId.value = null
  probeResult.value = null
  formError.value = ''
  hostnameError.value = ''
  resetForm()
}

async function fetchHostname() {
  if (hostnameFetching.value || !form.value.host.trim() || !form.value.ssh_port) return
  if (form.value.connection_mode === 'proxy' && !form.value.proxy_server_id) { hostnameError.value = '请先选择代理节点'; return }
  const id = editingId.value
  const details = connectionPayload(form.value)
  const snapshot = connectionSnapshot(form.value)
  const token = ++hostnameRequestToken
  hostnameFetching.value = true
  hostnameError.value = ''
  formError.value = ''
  try {
    const result = await resolveServerHostname(id, details)
    if (token === hostnameRequestToken && showAdd.value && editingId.value === id && connectionSnapshot(form.value) === snapshot) form.value.hostname = result.hostname
  } catch (error) {
    if (token === hostnameRequestToken && showAdd.value && editingId.value === id && connectionSnapshot(form.value) === snapshot) hostnameError.value = error.message || '获取机器名失败'
  } finally {
    if (token === hostnameRequestToken) hostnameFetching.value = false
  }
}

async function probeServer() {
  if (!editingId.value || probing.value) return
  if (form.value.connection_mode === 'proxy' && !form.value.proxy_server_id) { probeResult.value = { reachable: false, error: '请先选择代理节点' }; return }
  const id = editingId.value
  const details = connectionPayload(form.value)
  const snapshot = connectionSnapshot(form.value)
  probeResult.value = null
  probing.value = true
  try {
    const result = await probeServerRequest(id, details)
    if (showAdd.value && editingId.value === id && connectionSnapshot(form.value) === snapshot) probeResult.value = result
  } catch (e) {
    if (showAdd.value && editingId.value === id && connectionSnapshot(form.value) === snapshot) probeResult.value = { reachable: false, error: e.message }
  } finally { probing.value = false }
}

function confirmDelete(srv) { deleteTarget.value = srv; deleteError.value = '' }
async function deleteServer() {
  if (!deleteTarget.value || deletingServer.value) return
  deleteError.value = ''
  deletingServer.value = true
  try {
    await deleteServerRequest(deleteTarget.value.id)
    deleteTarget.value = null
    await fetchServers()
  } catch (e) { deleteError.value = e.message || '删除服务器失败' }
  finally { deletingServer.value = false }
}
async function unbindServer(srv) {
  if (!window.confirm(`解除 ${srv.name} 与集群节点 ${srv.k8s_node_name || '-'} 的绑定？此操作不会删除节点或影响 Pod。`)) return
  unbindingId.value = srv.id
  unbindError.value = ''
  try {
    await unbindServerRequest(srv.id)
    await fetchServers()
  } catch (e) { unbindError.value = e.message || '解除绑定失败' } finally { unbindingId.value = null }
}
async function openStats(id) {
  const srv = servers.value.find(s => s.id === id)
  if (!srv) return
  statsServer.value = srv
  await Promise.all([serverStatsResource.refresh(id), loadChartRuntime()])
}

let chartRuntimePromise = null
async function loadChartRuntime() {
  if (DoughnutComponent.value) return
  if (!chartRuntimePromise) {
    chartRuntimePromise = Promise.all([import('vue-chartjs'), import('chart.js')])
      .then(([vueChart, chart]) => {
        chart.Chart.register(chart.ArcElement, chart.Tooltip)
        DoughnutComponent.value = vueChart.Doughnut
      })
      .catch(error => {
        chartLoadError.value = error?.message || '图表组件加载失败'
      })
  }
  return chartRuntimePromise
}

function openTerminal(id) {
  const srv = servers.value.find(s => s.id === id)
  if (srv) terminalServer.value = srv
}

function memPercent(d) {
  if (!d?.memory_total_mb || d.memory_total_mb <= 0) return 0
  return ((d.memory_used_mb || 0) / d.memory_total_mb) * 100
}
function diskPercent(d) {
  if (!d?.disk_total_gb || d.disk_total_gb <= 0) return 0
  return ((d.disk_used_gb || 0) / d.disk_total_gb) * 100
}
function formatMB(mb) {
  if (mb == null) return '-'
  if (mb >= 1024) return (mb / 1024).toFixed(1) + ' GB'
  return mb + ' MB'
}

function resetForm() {
  const nextForm = emptyForm()
  hostnameConnectionSnapshot = connectionSnapshot(nextForm)
  form.value = nextForm
}
</script>

<style scoped>
.back-link{display:inline-flex;align-items:center;gap:6px;margin-bottom:var(--space-16);color:var(--text-secondary);font-size:13px;text-decoration:none}
.back-link:hover{color:var(--action-primary)}
.modal-wide { max-width: 640px; }
.section-copy {
  margin: 0;
  color: var(--text-secondary);
  font-size: 13px;
  line-height: 1.6;
}
.stats-grid {
  display: flex;
  gap: 16px;
  justify-content: center;
}
.stats-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  width: 130px;
}
.stats-card canvas {
  width: 110px !important;
  height: 110px !important;
}
.ring-title {
  margin-top: 4px;
  font-size: 12px;
  color: var(--text-secondary);
  font-weight: 600;
  text-transform: uppercase;
}
.ring-detail {
  margin-top: 2px;
  font-size: 11px;
  color: var(--text-muted);
}
.stats-info {
  text-align: center;
  font-size: 12px;
  color: var(--text-secondary);
}
.server-content { margin-top: var(--tabbed-page-content-gap); }
.server-list-toolbar { display: flex; min-height: 62px; align-items: center; justify-content: flex-end; gap: var(--space-8); padding: 12px var(--space-16); }
.server-table-wrap { padding: 0 var(--space-16); }
.hostname-form-row { display: flex; align-items: center; gap: var(--space-8); }
.hostname-form-row .form-input { flex: 1; min-width: 0; }
.hostname-form-row .btn { flex: none; }
@media (max-width: 480px) { .hostname-form-row { flex-wrap: wrap; } .hostname-form-row .btn { width: 100%; justify-content: center; } }
.server-probe { display: flex; align-items: center; gap: var(--space-12); flex-wrap: wrap; margin-top: var(--space-12); }
.server-probe-result { font-size: 12px; overflow-wrap: anywhere; }
.resource-monitoring-toolbar { display: flex; min-height: 62px; align-items: center; justify-content: flex-end; gap: var(--space-8); padding: 12px var(--space-16); }
.resource-monitoring-meta { color: var(--text-muted); font-size: 11px; }
.resource-monitoring-card > .k8s-banner { margin: 0 var(--space-16) var(--space-12); }
.resource-table-wrap { padding: 0 var(--space-16); }
.resource-table td { padding-top: 10px; padding-bottom: 10px; }
.resource-server-name, .resource-server-host { white-space: nowrap; }
.resource-server-host { color: var(--text-secondary); font-size: 11px; }
.resource-metric { display: grid; min-width: 130px; gap: 4px; }
.resource-metric strong { color: var(--text-secondary); font-size: 11px; font-weight: 600; white-space: nowrap; }
.resource-meter { display: block; width: 100%; height: 4px; overflow: hidden; border-radius: 2px; background: var(--surface-subtle); }
.resource-meter i { display: block; height: 100%; border-radius: inherit; background: var(--success); }
.resource-meter i.is-warning { background: var(--warning); }
.resource-meter i.is-danger { background: var(--danger); }
.resource-error { display: block; max-width: 170px; margin-top: 3px; overflow: hidden; color: var(--danger); font-size: 10px; text-overflow: ellipsis; white-space: nowrap; }
@media (max-width: 640px) {
  .resource-monitoring-toolbar { align-items: flex-end; flex-direction: column; }
  .resource-metric { min-width: 116px; }
}

</style>
