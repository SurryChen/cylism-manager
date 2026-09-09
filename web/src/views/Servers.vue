<template>
  <div>
    <SectionTabsHeader title="服务器" :tabs="sections" :active-tab="activeSection" @select="activeSection = $event">
      <template #actions><button class="btn btn-primary" @click="showAdd = true">+ 添加服务器</button></template>
    </SectionTabsHeader>

    <main class="server-content">
    <template v-if="activeSection === 'configuration'">
    <div class="card section-gap">
      <p class="section-copy">
        这里维护服务器台账、SSH 凭据与连通性。集群节点已经拆分到“集群节点”页面统一查看与操作。
      </p>
    </div>

    <div class="card">
      <div v-if="servers.length === 0" class="empty-state">
        <span class="empty-icon">⬡</span><span class="empty-text">暂无服务器</span>
      </div>
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <th>名称</th><th>主机</th><th>SSH 用户</th><th>认证方式</th><th>SSH 连通</th><th>集群状态</th><th>节点名</th><th></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="srv in servers" :key="srv.id">
              <td class="cell-primary">{{ srv.name }}</td>
              <td>{{ srv.host }}</td>
              <td>{{ srv.ssh_user || 'root' }}</td>
              <td>{{ srv.ssh_auth_type === 'key' ? '密钥' : '密码' }}</td>
              <td>
                <button class="btn btn-sm" @click="probeServer(srv.id)" :disabled="probingId === srv.id">
                  {{ probingId === srv.id ? '...' : '🔍' }}
                </button>
              </td>

              <td>
                <span class="badge" :class="srv.cluster_role ? 'badge-online' : 'badge-offline'">
                  {{ srv.cluster_role ? '已在集群' : '未加入' }}
                </span>
              </td>
              <td>{{ srv.k8s_node_name || '-' }}</td>
              <td>
                <div class="btn-group action-cell">
                  <button v-if="!srv.cluster_role" class="btn btn-sm" @click="startImport(srv.id)">导入集群</button>
                  <button v-if="!srv.cluster_role" class="btn btn-sm" @click="startEdit(srv)">编辑</button>
                  <button v-if="!srv.cluster_role" class="btn btn-sm btn-danger" @click="confirmDelete(srv)">删除</button>
                  <button v-if="srv.cluster_role || srv.k8s_node_name" class="btn btn-sm btn-danger" :disabled="unbindingId === srv.id" @click="unbindServer(srv)">{{ unbindingId === srv.id ? '解绑中...' : '解除绑定' }}</button>
                  <button class="btn btn-sm" @click="openTerminal(srv.id)">💻</button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
    </template>

    <template v-else-if="activeSection === 'monitoring'">
      <section class="card resource-overview section-gap">
        <div><h2 class="resource-overview-title">资源概览</h2><p class="resource-overview-meta">{{ resourceSamplingLabel }}</p></div>
        <div class="btn-group"><button class="icon-button" title="刷新资源数据" aria-label="刷新资源数据" :disabled="resourceStatsLoading" @click="refreshResourceStats"><RefreshCw :size="16" :class="{ 'is-spinning': resourceStatsLoading }" /></button></div>
      </section>
      <div v-if="servers.length === 0" class="empty-state"><span class="empty-icon">⬡</span><span class="empty-text">暂无服务器</span></div>
      <div v-else class="card">
        <div v-if="resourceStatsLoading && !resourceStats.length" class="empty-state"><span class="empty-text">正在采集服务器资源...</span></div>
        <div v-else class="table-wrap"><table class="data-table resource-table"><thead><tr><th>服务器</th><th>采集状态</th><th>CPU</th><th>内存</th><th>磁盘 /</th><th>负载</th><th>运行时间</th><th>采样时间</th></tr></thead><tbody><tr v-for="srv in servers" :key="srv.id" class="resource-row" @click="openStats(srv.id)"><td class="cell-primary">{{ srv.name }}<small class="cell-secondary">{{ srv.host }}</small></td><td><span class="badge" :class="resourceStatusClass(resourceFor(srv.id))">{{ resourceStatusLabel(resourceFor(srv.id)) }}</span><small v-if="resourceFor(srv.id)?.error" class="resource-error">{{ resourceFor(srv.id).error }}</small></td><td><div class="resource-metric"><strong>{{ formatPercent(resourceFor(srv.id)?.cpu_percent) }}</strong><span class="resource-meter"><i :class="resourceLevelClass(resourceFor(srv.id)?.cpu_percent)" :style="{ width: `${metricPercent(resourceFor(srv.id)?.cpu_percent)}%` }" /></span></div></td><td><div class="resource-metric"><strong>{{ formatMB(resourceFor(srv.id)?.memory_used_mb) }} / {{ formatMB(resourceFor(srv.id)?.memory_total_mb) }}</strong><span class="resource-meter"><i :class="resourceLevelClass(memPercent(resourceFor(srv.id)))" :style="{ width: `${metricPercent(memPercent(resourceFor(srv.id)))}%` }" /></span></div></td><td><div class="resource-metric"><strong>{{ resourceFor(srv.id)?.disk_used_gb ?? '-' }} / {{ resourceFor(srv.id)?.disk_total_gb ?? '-' }} GB</strong><span class="resource-meter"><i :class="resourceLevelClass(diskPercent(resourceFor(srv.id)))" :style="{ width: `${metricPercent(diskPercent(resourceFor(srv.id)))}%` }" /></span></div></td><td>{{ formatLoad(resourceFor(srv.id)) }}</td><td>{{ resourceFor(srv.id)?.uptime || '-' }}</td><td>{{ formatSampleTime(resourceFor(srv.id)?.sampled_at) }}</td></tr></tbody></table></div>
      </div>
    </template>

    <template v-else>
      <section class="card network-overview section-gap">
        <h2 class="network-overview-title">网络诊断</h2>
        <button class="icon-button" title="刷新网络诊断" aria-label="刷新网络诊断" :disabled="networkDiagnosticsLoading" @click="refreshNetworkDiagnostics"><RefreshCw :size="16" :class="{ 'is-spinning': networkDiagnosticsLoading }" /></button>
      </section>
      <div v-if="networkDiagnosticsLoading && !networkDiagnostics.servers.length" class="empty-state"><span class="empty-text">正在采集网络状态...</span></div>
      <div v-else-if="networkDiagnosticsError" class="empty-state"><span class="empty-text">{{ networkDiagnosticsError }}</span></div>
      <div v-else-if="!networkDiagnostics.servers.length" class="empty-state"><span class="empty-icon">⬡</span><span class="empty-text">暂无诊断结果</span></div>
      <section v-else class="card section-gap network-table-card">
        <div class="table-wrap"><table class="data-table network-table"><thead><tr><th>服务器</th><th>K3s 网络</th><th>Tailscale</th><th>Tailnet IP</th><th>UDP</th><th>IPv4</th><th>最近 DERP</th></tr></thead><tbody><tr v-for="diagnostic in networkDiagnostics.servers" :key="diagnostic.server_id"><td class="cell-primary">{{ diagnostic.name }}<small class="cell-secondary">{{ diagnostic.k8s_unit || '-' }}</small></td><td><span class="badge" :class="networkModeClass(diagnostic)">{{ networkModeLabel(diagnostic.network_mode) }}</span></td><td><span class="badge" :class="tailscaleStatusClass(diagnostic)">{{ tailscaleStatusLabel(diagnostic) }}</span></td><td>{{ diagnostic.tailscale?.tailnet_ip || '-' }}</td><td>{{ booleanLabel(diagnostic.tailscale?.udp) }}</td><td>{{ booleanLabel(diagnostic.tailscale?.ipv4) }}</td><td>{{ diagnostic.tailscale?.nearest_derp || '-' }}</td></tr></tbody></table></div>
      </section>
      <section v-if="networkDiagnostics.links.length" class="card network-table-card">
        <div class="table-wrap"><table class="data-table network-table"><thead><tr><th>源服务器</th><th>目标服务器</th><th>链路</th><th>延迟</th><th>DERP</th><th>状态</th></tr></thead><tbody><tr v-for="link in networkDiagnostics.links" :key="`${link.source_server_id}-${link.target_server_id}`"><td class="cell-primary">{{ diagnosticServerName(link.source_server_id) }}</td><td class="cell-primary">{{ diagnosticServerName(link.target_server_id) }}</td><td><span class="badge" :class="linkPathClass(link.path)">{{ linkPathLabel(link.path) }}</span></td><td>{{ link.latency_ms ? `${link.latency_ms} ms` : '-' }}</td><td>{{ link.derp_region || '-' }}</td><td>{{ linkErrorLabel(link.error_code) }}</td></tr></tbody></table></div>
      </section>
    </template>
    </main>

    <!-- Add Server modal -->
    <div v-if="showAdd" class="overlay" @click.self="showAdd = false">
      <div class="modal">
        <h2 class="modal-title">{{ editingId ? '编辑服务器' : '添加服务器' }}</h2>
        <form @submit.prevent="addServer">
          <div class="form-row">
            <div class="form-group"><label class="form-label">名称</label><input v-model="form.name" class="form-input" placeholder="我的服务器" required /></div>
            <div class="form-group"><label class="form-label">主机</label><input v-model="form.host" class="form-input" placeholder="192.168.1.100" required /></div>
          </div>
          <div class="form-row">
            <div class="form-group"><label class="form-label">SSH 端口</label><input v-model.number="form.ssh_port" class="form-input" type="number" placeholder="22" /></div>
            <div class="form-group"><label class="form-label">SSH 用户</label><input v-model="form.ssh_user" class="form-input" placeholder="root" /></div>
          </div>
          <div class="form-group"><label class="form-label">认证方式</label><select v-model="form.ssh_auth_type" class="form-select"><option value="password">密码</option><option value="key">密钥</option></select></div>
          <div class="form-group" v-if="form.ssh_auth_type === 'password'"><label class="form-label">SSH 密码</label><input v-model="form.ssh_password" class="form-input" type="password" placeholder="输入密码" /></div>
          <div class="form-group" v-if="form.ssh_auth_type === 'key'"><label class="form-label">SSH 密钥</label><textarea v-model="form.ssh_key" class="form-input textarea-input" placeholder="粘贴私钥内容" /></div>
          <div class="modal-actions"><button type="button" class="btn" @click="closeForm">取消</button><button type="submit" class="btn btn-primary">{{ editingId ? '保存修改' : '确认添加' }}</button></div>
        </form>
      </div>
    </div>

    <!-- SSH probe result modal -->
    <div v-if="probeResult" class="overlay" @click.self="probeResult = null">
      <div class="modal">
        <h2 class="modal-title">SSH 连通性检测</h2>
        <div class="detail-grid">
          <span class="detail-label">主机：</span><span>{{ probeResult.host }}</span>
          <span class="detail-label">结果：</span>
          <span>
            <span v-if="probeResult.reachable" class="badge badge-online">✅ 在线 ({{ probeResult.latency_ms }}ms)</span>
            <span v-else class="badge badge-danger">✗ 不可达</span>
          </span>
          <span v-if="!probeResult.reachable" class="detail-label">错误：</span>
          <span v-if="!probeResult.reachable">{{ probeResult.error }}</span>
        </div>
        <div class="modal-actions"><button class="btn" @click="probeResult = null">关闭</button></div>
      </div>
    </div>

    <!-- Import modal -->
    <div v-if="importState" class="overlay">
      <div class="modal modal-wide">
        <h2 class="modal-title">导入集群 - {{ importServer?.name }}</h2>
        <!-- Step 1: 检测中 -->
        <div v-if="importState.phase === 'detecting'">
          <p style="margin-bottom:16px;color:var(--text-secondary)">正在通过 SSH 连接服务器并匹配集群节点...</p>
          <div class="modal-actions">
            <button class="btn" @click="importState = null">取消</button>
          </div>
        </div>
        <!-- Step 2: 确认 -->
        <div v-if="importState.phase === 'confirm'">
          <table style="width:100%;margin-bottom:16px;font-size:13px">
            <tbody>
              <tr><td style="color:var(--text-secondary);padding:6px 0">服务器</td><td>{{ importServer?.name }}</td></tr>
              <tr><td style="color:var(--text-secondary);padding:6px 0">主机名</td><td>{{ importState.info?.hostname }}</td></tr>
              <tr><td style="color:var(--text-secondary);padding:6px 0">集群节点</td><td>{{ importState.info?.node_name }}</td></tr>
              <tr><td style="color:var(--text-secondary);padding:6px 0">角色</td><td><span class="badge" :class="importState.info?.role === 'control-plane' ? 'badge-online' : 'badge-offline'">{{ importState.info?.role }}</span></td></tr>
              <tr><td style="color:var(--text-secondary);padding:6px 0">版本</td><td>{{ importState.info?.version }}</td></tr>
              <tr><td style="color:var(--text-secondary);padding:6px 0">内网 IP</td><td>{{ importState.info?.internal_ip }}</td></tr>
              <tr><td style="color:var(--text-secondary);padding:6px 0">操作系统</td><td>{{ importState.info?.os }}</td></tr>
            </tbody>
          </table>
          <div class="modal-actions">
            <button class="btn" @click="importState = null">取消</button>
            <button class="btn btn-primary" @click="doConfirmImport">确认导入</button>
          </div>
        </div>
        <!-- Error -->
        <div v-if="importState.phase === 'error'">
          <p style="color:var(--color-danger);margin-bottom:16px">{{ importState.error }}</p>
          <div class="modal-actions">
            <button class="btn" @click="importState = null">关闭</button>
            <button class="btn btn-primary" @click="startImport(importServer.id)">重试</button>
          </div>
        </div>
      </div>
    </div>

    
    <!-- Stats modal -->
    <div v-if="statsServer" class="overlay" @click.self="statsServer = null">
      <div class="modal modal-wide">
        <h2 class="modal-title">资源监控 — {{ statsServer?.name }}</h2>
        <div v-if="statsLoading" style="color:var(--text-secondary);text-align:center;padding:20px">加载中...</div>
        <div v-else class="stats-grid">
          <div class="stats-card">
            <Doughnut :data="cpuChartData" :options="chartOptions" />
            <div class="ring-title">CPU</div>
          </div>
          <div class="stats-card">
            <Doughnut :data="memChartData" :options="chartOptions" />
            <div class="ring-title">内存</div>
            <div class="ring-detail">{{ formatMB(statsData.memory_used_mb) }} / {{ formatMB(statsData.memory_total_mb) }}</div>
          </div>
          <div class="stats-card">
            <Doughnut :data="diskChartData" :options="chartOptions" />
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

    <!-- Terminal modal -->
    <Teleport to="body">
    <div v-if="terminalServer" class="overlay terminal-overlay">
      <div class="terminal-modal">
        <div class="terminal-modal-header">
          <span>💻 SSH 终端 — {{ terminalServer?.name }} ({{ terminalServer?.host }})</span>
          <button class="btn btn-sm btn-icon" @click="closeTerminal" title="关闭">✕</button>
        </div>
        <div class="terminal-body">
          <div v-if="termStatus === 'connecting'" class="terminal-placeholder">⏳ 正在连接...</div>
          <div v-else-if="termStatus === 'error'" class="terminal-placeholder terminal-error">
            ❌ {{ termError }}
            <button class="btn btn-sm btn-primary" style="margin-left:8px" @click="openTerminal(terminalServer.id)">重试</button>
          </div>
          <div v-else-if="termStatus === 'closed'" class="terminal-placeholder">
            🔌 连接已断开
            <button class="btn btn-sm btn-primary" style="margin-left:8px" @click="openTerminal(terminalServer.id)">重连</button>
          </div>
          <div ref="terminalEl" class="terminal-container" v-show="termStatus === 'connected'"></div>
        </div>
        <div class="terminal-modal-footer">
          <span v-if="termStatus === 'connected'" class="terminal-status-ok">🟢 已连接</span>
          <span v-else-if="termStatus === 'connecting'" class="terminal-status-connecting">🟡 连接中</span>
          <span v-else-if="termStatus === 'error'" class="terminal-status-error">🔴 连接失败</span>
          <span v-else class="terminal-status-closed">⚫ 已断开</span>
        </div>
      </div>
    </div>
    </Teleport>

    <div v-if="deleteTarget" class="overlay" @click.self="deleteTarget = null">
      <div class="modal"><h2 class="modal-title">删除服务器</h2><p class="modal-copy">确定删除 <strong>{{ deleteTarget.name }}</strong> 吗？</p><div class="modal-actions"><button class="btn" @click="deleteTarget = null">取消</button><button class="btn btn-danger" @click="deleteServer">确认删除</button></div></div>
    </div>
  </div>
</template>

<script setup>
import { computed, ref, onMounted, onUnmounted, Teleport, watch } from 'vue'
import { api } from '../api/index.js'
import { getServerNetworkDiagnostics, getServerResourceStats, getServers, getServerStats } from '../api/servers.js'
import { useAsyncResource } from '../composables/useAsyncResource.js'
import { RefreshCw } from 'lucide-vue-next'
import SectionTabsHeader from '../components/SectionTabsHeader.vue'
import { Doughnut } from 'vue-chartjs'
import { Chart as ChartJS, ArcElement, Tooltip } from 'chart.js'

ChartJS.register(ArcElement, Tooltip)
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import { installTerminalClipboard } from '../utils/terminalClipboard.js'

const serversResource = useAsyncResource(({ signal }) => getServers({ signal }), [])
const servers = serversResource.data
const activeSection = ref('configuration')
const sections = [
  { id: 'configuration', label: '基本配置' },
  { id: 'monitoring', label: '资源监控' },
  { id: 'network-diagnostics', label: '网络诊断' },
]
const resourceStatsResource = useAsyncResource(({ signal }) => getServerResourceStats({ signal }), [])
const resourceStats = resourceStatsResource.data
const resourceStatsLoading = resourceStatsResource.loading
const resourceStatsUpdatedAt = ref('')
const networkDiagnosticsResource = useAsyncResource(async ({ signal }) => {
  const result = await getServerNetworkDiagnostics({ signal })
  return {
    servers: Array.isArray(result?.servers) ? result.servers : [],
    links: Array.isArray(result?.links) ? result.links : [],
  }
}, { servers: [], links: [] })
const networkDiagnostics = networkDiagnosticsResource.data
const networkDiagnosticsLoading = networkDiagnosticsResource.loading
const networkDiagnosticsError = computed(() => networkDiagnosticsResource.error.value ? '网络诊断请求失败' : '')
const showAdd = ref(false)
const editingId = ref(null)
const deleteTarget = ref(null)
const probingId = ref(null)
const unbindingId = ref(null)
const probeResult = ref(null)
const importState = ref(null)
const importServer = ref(null)
const statsServer = ref(null)
const serverStatsResource = useAsyncResource(({ signal }, serverID) => getServerStats(serverID, { signal }), {})
const statsData = serverStatsResource.data
const statsLoading = serverStatsResource.loading
const terminalServer = ref(null)
const terminalEl = ref(null)
const termStatus = ref(null)
const termError = ref('')
let resourcePollTimer

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
const networkDiagnosticsByServerID = computed(() => new Map(networkDiagnostics.value.servers.map(diagnostic => [Number(diagnostic.server_id), diagnostic])))
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



let termInstance = null
let termWs = null
const form = ref({ name: '', host: '', ssh_port: 22, ssh_user: 'root', ssh_auth_type: 'password', ssh_password: '', ssh_key: '' })

onMounted(() => {
  fetchServers()
  document.addEventListener('visibilitychange', syncResourcePolling)
})
onUnmounted(() => {
  stopResourcePolling()
  document.removeEventListener('visibilitychange', syncResourcePolling)
  closeTerminal()
})

watch(activeSection, (section) => {
  syncResourcePolling()
  if (section === 'network-diagnostics') refreshNetworkDiagnostics()
})

async function fetchServers() { await serversResource.refresh() }

function resourceFor(serverID) { return resourceStatsByServerID.value.get(Number(serverID)) || null }
function metricPercent(value) { return Math.min(100, Math.max(0, Number(value) || 0)) }
function formatPercent(value) { return value == null ? '-' : `${metricPercent(value).toFixed(1)}%` }
function resourceLevelClass(value) { const percent = metricPercent(value); return percent >= 90 ? 'is-danger' : percent >= 70 ? 'is-warning' : 'is-ok' }
function resourceStatusClass(stats) { return stats?.status === 'ready' ? 'badge-online' : stats?.status === 'unreachable' ? 'badge-danger' : 'badge-deploying' }
function resourceStatusLabel(stats) { return stats?.status === 'ready' ? '已采集' : stats?.status === 'unreachable' ? '不可达' : '等待采集' }
function formatLoad(stats) { if (!stats?.load_1m && stats?.load_1m !== 0) return '-'; const cores = Number(stats.cpu_cores) || 0; return cores ? `${Number(stats.load_1m).toFixed(2)} / ${cores} 核` : Number(stats.load_1m).toFixed(2) }
function formatSampleTime(value) { if (!value) return '-'; const date = new Date(value); return Number.isNaN(date.getTime()) ? '-' : date.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', second: '2-digit' }) }

async function refreshResourceStats() {
  const result = await resourceStatsResource.refresh()
  if (result !== undefined) {
    resourceStatsUpdatedAt.value = new Date().toISOString()
  }
}

async function refreshNetworkDiagnostics() {
  await networkDiagnosticsResource.refresh()
}

function networkModeLabel(mode) {
  return ({ k3s_embedded_tailscale: 'K3s 内建 Tailscale', external_tailscale: '外部 Tailscale', standard_network: '标准网络', unknown: '未知' })[mode] || '未知'
}
function networkModeClass(diagnostic) { return diagnostic.error_code ? 'badge-danger' : diagnostic.network_mode === 'k3s_embedded_tailscale' ? 'badge-online' : diagnostic.network_mode === 'external_tailscale' ? 'badge-deploying' : 'badge-offline' }
function tailscaleStatusLabel(diagnostic) { if (!diagnostic.tailscale?.installed) return '未安装'; return diagnostic.tailscale.online ? '在线' : '未连接' }
function tailscaleStatusClass(diagnostic) { return diagnostic.tailscale?.online ? 'badge-online' : diagnostic.tailscale?.installed ? 'badge-deploying' : 'badge-offline' }
function booleanLabel(value) { return value === true ? '可用' : value === false ? '不可用' : '-' }
function diagnosticServerName(serverID) { return networkDiagnosticsByServerID.value.get(Number(serverID))?.name || '-' }
function linkPathLabel(path) { return ({ direct: 'UDP 直连', derp: 'DERP 中继', unreachable: '不可达', unknown: '未知' })[path] || '未知' }
function linkPathClass(path) { return path === 'direct' ? 'badge-online' : path === 'derp' ? 'badge-deploying' : path === 'unreachable' ? 'badge-danger' : 'badge-offline' }
function linkErrorLabel(code) { return ({ ping_timeout: '超时', ping_failed: '探测失败', ping_unclassified: '未识别', invalid_target: '目标无效' })[code] || (code ? '异常' : '正常') }

function stopResourcePolling() {
  if (resourcePollTimer) window.clearInterval(resourcePollTimer)
  resourcePollTimer = undefined
}

function syncResourcePolling() {
  stopResourcePolling()
  if (activeSection.value !== 'monitoring' || document.hidden) return
  refreshResourceStats()
  resourcePollTimer = window.setInterval(refreshResourceStats, 10000)
}

async function addServer() {
  try {
    if (editingId.value) {
      await api.put('/servers/' + editingId.value, form.value)
    } else {
      await api.post('/servers', form.value)
    }
    closeForm()
    fetchServers()
  } catch (e) { console.error(e) }
}

function startEdit(srv) {
  editingId.value = srv.id
  form.value = {
    name: srv.name,
    host: srv.host,
    ssh_port: srv.ssh_port || 22,
    ssh_user: srv.ssh_user || 'root',
    ssh_auth_type: srv.ssh_auth_type || 'password',
    ssh_password: '',
    ssh_key: '',
  }
  showAdd.value = true
}

function closeForm() {
  showAdd.value = false
  editingId.value = null
  resetForm()
}

async function probeServer(id) {
  probingId.value = id
  try {
    const srv = servers.value.find(s => s.id === id)
    const result = await api.post(`/servers/${id}/probe`)
    probeResult.value = { host: srv?.host || '', ...result }
  } catch (e) { probeResult.value = { host: '', reachable: false, error: e.message } }
  probingId.value = null
}

async function startImport(id) {
  const srv = servers.value.find(s => s.id === id)
  if (!srv) return
  importServer.value = srv
  importState.value = { phase: 'detecting' }

  try {
    const result = await api.post(`/nodes/${id}/preimport`)
    importState.value = { phase: 'confirm', info: result }
  } catch (e) {
    importState.value = { phase: 'error', error: e.message || '预检失败' }
  }
}

async function doConfirmImport() {
  const info = importState.value.info
  if (!info) return
  importState.value.phase = 'detecting'
  try {
    await api.post(`/nodes/${importServer.value.id}/import`, {
      hostname: info.node_name,
      role: info.role,
    })
    importState.value = null
    importServer.value = null
    fetchServers()
  } catch (e) {
    importState.value = { phase: 'error', error: e.message || '导入失败' }
  }
}

function confirmDelete(srv) { deleteTarget.value = srv }
async function deleteServer() { try { await api.delete(`/servers/${deleteTarget.value.id}`); deleteTarget.value = null; fetchServers() } catch (e) { console.error(e) } }
async function unbindServer(srv) {
  if (!window.confirm(`解除 ${srv.name} 与集群节点 ${srv.k8s_node_name || '-'} 的绑定？此操作不会删除节点或影响 Pod。`)) return
  unbindingId.value = srv.id
  try {
    await api.post(`/servers/${srv.id}/unbind`)
    fetchServers()
  } catch (e) { console.error(e) } finally { unbindingId.value = null }
}
async function openStats(id) {
  const srv = servers.value.find(s => s.id === id)
  if (!srv) return
  statsServer.value = srv
  await serverStatsResource.refresh(id)
}

function openTerminal(id) {
  const srv = servers.value.find(s => s.id === id)
  if (!srv) return
  terminalServer.value = srv
  termStatus.value = 'connecting'
  termError.value = ''
  document.body.style.overflow = 'hidden'

  setTimeout(() => {
    const el = terminalEl.value
    if (!el) { termStatus.value = 'error'; termError.value = '终端容器未就绪'; return }

    const rootStyle = getComputedStyle(document.documentElement)
    const term = new Terminal({
      cursorBlink: true,
      cursorStyle: 'bar',
      fontSize: 14,
      fontFamily: '"JetBrains Mono", "Cascadia Code", "Fira Code", monospace',
      letterSpacing: 0,
      lineHeight: 1.2,
      theme: {
        background: rootStyle.getPropertyValue('--terminal-background').trim(),
        foreground: rootStyle.getPropertyValue('--terminal-foreground').trim(),
        cursor: rootStyle.getPropertyValue('--terminal-cursor').trim(),
        selectionBackground: rootStyle.getPropertyValue('--terminal-selection').trim(),
      },
    })
    const fitAddon = new FitAddon()
    term.loadAddon(fitAddon)
    term.open(el)
    installTerminalClipboard(term)

    // 给 xterm 内部容器加圆角样式
    const xtermScreen = el.querySelector('.xterm-screen')
    if (xtermScreen) xtermScreen.style.borderRadius = '8px'

    // 手动计算行列数（不用 fitAddon，避免放大字体替代调行列数）
    fitAddon.fit()

    const wsUrl = `${location.protocol === 'https:' ? 'wss:' : 'ws:'}//${location.host}/api/servers/${id}/terminal`
    const token = localStorage.getItem('access_token')
    const ws = new WebSocket(wsUrl + '?token=' + encodeURIComponent(token || ''))
    ws.binaryType = 'arraybuffer'

    ws.onopen = () => {
      termStatus.value = 'connected'
      term.onData(data => {
        if (ws.readyState === WebSocket.OPEN) ws.send(data)
      })
    }
    ws.onmessage = (e) => {
      if (e.data instanceof ArrayBuffer) {
        term.write(new Uint8Array(e.data))
      }
    }
    ws.onclose = () => {
      termStatus.value = 'closed'
    }
    ws.onerror = () => {
      termStatus.value = 'error'
      termError.value = 'WebSocket 连接失败'
    }

    termInstance = term
    termWs = ws

    // resize 自适应：fit + PTY resize 通知后端
    const sendResize = () => {
      try {
        // 手动计算行列数（不用 fitAddon，避免放大字体替代调行列数）
    fitAddon.fit()
        if (ws.readyState === WebSocket.OPEN) {
          ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }))
        }
      } catch (_) {}
    }
    const observer = new ResizeObserver(sendResize)
    observer.observe(el)
    term._resizeObserver = observer
  }, 100)
}

function closeTerminal() {
  if (termInstance && termInstance._resizeObserver) {
    termInstance._resizeObserver.disconnect()
  }
  if (termWs) termWs.close()
  if (termInstance) termInstance.dispose()
  termInstance = null
  termWs = null
  termStatus.value = null
  termError.value = ''
  terminalServer.value = null
  document.body.style.overflow = ''
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

function resetForm() { form.value = { name: '', host: '', ssh_port: 22, ssh_user: 'root', ssh_auth_type: 'password', ssh_password: '', ssh_key: '' } }
</script>

<style scoped>
.modal-wide { max-width: 640px; }
.section-copy {
  margin: 0;
  color: var(--text-secondary);
  font-size: 13px;
  line-height: 1.6;
}
.terminal-overlay {
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 9999;
  backdrop-filter: blur(6px);
}
.terminal-modal {
  width: 85vw;
  max-width: 1100px;
  height: 82vh;
  display: flex;
  flex-direction: column;
  border: 1px solid var(--border);
  border-radius: 12px;
  background: var(--surface-raised);
  box-shadow: var(--shadow);
  overflow: hidden;
}
.terminal-modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 16px;
  border-bottom: 1px solid var(--border-muted);
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
  flex-shrink: 0;
}
.btn-icon {
  width: 28px;
  height: 28px;
  padding: 0;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--text-secondary);
  font-size: 16px;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}
.btn-icon:hover {
  background: var(--surface-hover);
  color: var(--text-primary);
}
.terminal-body {
  flex: 1;
  padding: 12px;
  overflow: hidden;
  position: relative;
  min-height: 0;
}
.terminal-placeholder {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  color: var(--text-secondary);
  font-size: 14px;
  gap: 8px;
}
.terminal-error {
  color: var(--danger);
}
.terminal-container {
  width: 100%;
  height: 100%;
  overflow: hidden;
  background: var(--terminal-background);
}
.terminal-container :deep(.xterm) {
  height: 100%;
  border-radius: 8px;
}
.terminal-container :deep(.xterm-viewport) {
  scrollbar-width: thin;
  scrollbar-color: var(--text-muted) transparent;
}
.terminal-container :deep(.xterm-viewport::-webkit-scrollbar) {
  width: 6px;
}
.terminal-container :deep(.xterm-viewport::-webkit-scrollbar-thumb) {
  background: var(--text-muted);
  border-radius: 3px;
}
.terminal-container :deep(.xterm-viewport::-webkit-scrollbar-track) {
  background: transparent;
}
.terminal-container :deep(.xterm-screen:focus-within) {
  outline: none;
}
.terminal-modal-footer {
  display: flex;
  align-items: center;
  padding: 6px 16px;
  border-top: 1px solid var(--border-muted);
  font-size: 11px;
  flex-shrink: 0;
}
.terminal-status-ok { color: var(--success); }
.terminal-status-connecting { color: var(--warning); }
.terminal-status-error { color: var(--danger); }
.terminal-status-closed { color: var(--text-muted); }



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
.server-content { margin-top: var(--space-20); }
.resource-overview { display: flex; align-items: center; justify-content: space-between; gap: var(--space-16); }
.resource-overview-title { margin: 0; color: var(--text-primary); font-size: 16px; }
.resource-overview-meta { margin: 4px 0 0; color: var(--text-muted); font-size: 11px; }
.network-overview { display: flex; align-items: center; justify-content: space-between; gap: var(--space-16); }
.network-overview-title { margin: 0; color: var(--text-primary); font-size: 16px; }
.network-table-card { margin-top: var(--space-16); }
.network-table td { white-space: nowrap; }
.resource-row { cursor: pointer; }
.resource-row:hover { background: var(--surface-hover); }
.resource-metric { display: grid; min-width: 130px; gap: 6px; }
.resource-metric strong { color: var(--text-secondary); font-size: 11px; font-weight: 600; white-space: nowrap; }
.resource-meter { display: block; width: 100%; height: 4px; overflow: hidden; border-radius: 2px; background: var(--surface-subtle); }
.resource-meter i { display: block; height: 100%; border-radius: inherit; background: var(--success); }
.resource-meter i.is-warning { background: var(--warning); }
.resource-meter i.is-danger { background: var(--danger); }
.resource-error { display: block; max-width: 170px; margin-top: 3px; overflow: hidden; color: var(--danger); font-size: 10px; text-overflow: ellipsis; white-space: nowrap; }
@media (max-width: 640px) {
  .resource-overview { align-items: flex-start; }
  .network-overview { align-items: flex-start; }
  .resource-metric { min-width: 116px; }
}

</style>
