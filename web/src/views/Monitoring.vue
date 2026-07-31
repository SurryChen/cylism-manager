<template>
  <div>
    <div class="page-header">
      <div>
        <h1 class="page-title">集群监控</h1>
        <p class="page-subtitle">VictoriaMetrics 单节点指标存储</p>
      </div>
      <div v-if="status?.state !== 'not_installed'" class="btn-group">
        <button class="btn" @click="refresh">刷新</button>
        <button class="btn btn-danger" @click="confirmUninstall = true">卸载</button>
      </div>
    </div>

    <div v-if="error" class="k8s-banner k8s-banner-warn section-gap">{{ error }}</div>

    <section v-if="loaded && status?.state === 'not_installed'" class="card install-card">
      <div class="card-header"><div><h2 class="card-title">安装 VictoriaMetrics</h2><p class="status-copy">数据固定存放在所选节点的本地目录</p></div><span class="badge badge-offline">未安装</span></div>
      <form class="install-form" @submit.prevent="install">
        <div class="form-group"><label class="form-label">数据节点</label><select v-model="form.node_name" class="form-select" required><option value="" disabled>选择就绪节点</option><option v-for="node in readyNodes" :key="node.name" :value="node.name">{{ displayNode(node) }}</option></select></div>
        <div class="form-row"><div class="form-group"><label class="form-label">本地数据目录</label><input v-model.trim="form.data_path" class="form-input" required placeholder="/data/victoria-metrics" /></div><div class="form-group"><label class="form-label">指标保留天数</label><input v-model.number="form.retention_days" class="form-input" type="number" min="1" max="365" required /></div></div>
        <p class="form-hint">目录必须位于 <code>/data/</code>。安装后不能直接变更节点或目录，卸载也不会删除已有指标数据。</p>
        <div class="modal-actions"><button class="btn btn-primary" :disabled="installing || !form.node_name">{{ installing ? '正在提交...' : '安装监控' }}</button></div>
      </form>
    </section>

    <template v-else-if="loaded && status">
      <section class="monitoring-summary section-gap">
        <article class="metric"><span>运行状态</span><strong><span class="badge" :class="statusClass">{{ statusLabel }}</span></strong><small>{{ status.message }}</small></article>
        <article class="metric"><span>数据节点</span><strong class="metric-code">{{ nodeDisplayName(status.node_name) || '-' }}</strong><small>{{ status.node_name || '-' }}</small></article>
        <article class="metric"><span>数据目录</span><strong class="metric-code">{{ status.data_path || '-' }}</strong><small>保留 {{ status.retention_days || '-' }} 天</small></article>
        <article class="metric"><span>采集节点</span><strong>{{ status.node_exporter_ready || 0 }} / {{ status.node_exporter_desired || 0 }}</strong><small>node-exporter 已就绪</small></article>
      </section>

      <section v-if="status.state === 'ready'" class="monitoring-grid">
        <article class="card targets-card"><div class="card-header"><div><h2 class="card-title">采集目标</h2><p class="status-copy">节点与容器指标</p></div><span class="badge" :class="targetSummary.failed ? 'badge-danger' : 'badge-online'">{{ targetSummary.active }} 个在线</span></div><div v-if="targetsLoading" class="empty-inline">正在读取采集状态...</div><div v-else-if="targetRows.length" class="target-list"><div v-for="target in targetRows" :key="target.key" class="target-row"><span><strong>{{ target.job }}</strong><small>{{ target.instance }}</small></span><span class="badge" :class="target.health === 'up' ? 'badge-online' : 'badge-danger'">{{ target.health === 'up' ? '正常' : '异常' }}</span></div></div><div v-else class="empty-inline">等待首次指标采集</div></article>
        <article class="card quick-card"><div class="card-header"><div><h2 class="card-title">采集概况</h2><p class="status-copy">最近一次 PromQL 结果</p></div></div><div class="quick-metrics"><div><span>节点目标</span><strong>{{ quickMetrics.nodes }}</strong></div><div><span>容器目标</span><strong>{{ quickMetrics.cadvisor }}</strong></div><div><span>在线比例</span><strong>{{ quickMetrics.health }}</strong></div></div></article>
      </section>

      <section v-if="status.state === 'ready'" class="card query-card section-gap"><div class="card-header"><div><h2 class="card-title">指标查询</h2><p class="status-copy">PromQL</p></div><div class="query-presets"><button v-for="preset in presets" :key="preset.query" class="btn btn-sm" @click="runPreset(preset)">{{ preset.label }}</button></div></div><form class="query-form" @submit.prevent="runQuery"><input v-model.trim="query" class="form-input" placeholder="up" maxlength="2048" /><button class="btn btn-primary" :disabled="querying || !query">{{ querying ? '查询中...' : '查询' }}</button></form><pre v-if="queryResult" class="query-result">{{ queryResult }}</pre><div v-else class="empty-inline">选择预设指标或输入 PromQL 查询</div></section>

      <section v-if="status.state !== 'ready'" class="card wait-card"><div class="empty-state"><span class="empty-icon">◌</span><span class="empty-text">等待 VictoriaMetrics 工作负载就绪</span></div></section>
    </template>

    <div v-if="confirmUninstall" class="overlay" @click.self="confirmUninstall = false"><div class="modal"><h2 class="modal-title">卸载 VictoriaMetrics</h2><p class="confirm-copy">将删除监控工作负载和采集配置，但不会删除 {{ status?.node_name }} 上的 {{ status?.data_path }} 数据目录。</p><div class="modal-actions"><button class="btn" @click="confirmUninstall = false">取消</button><button class="btn btn-danger" :disabled="uninstalling" @click="uninstall">{{ uninstalling ? '卸载中...' : '确认卸载' }}</button></div></div></div>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { api } from '../api/index.js'

const status = ref(null)
const nodes = ref([])
const loaded = ref(false)
const error = ref('')
const installing = ref(false)
const uninstalling = ref(false)
const confirmUninstall = ref(false)
const targets = ref(null)
const targetsLoading = ref(false)
const querying = ref(false)
const query = ref('')
const queryResult = ref('')
const quickMetrics = ref({ nodes: '-', cadvisor: '-', health: '-' })
const form = ref({ node_name: '', data_path: '/data/victoria-metrics', retention_days: 14 })
let refreshTimer

const presets = [
  { label: '全部目标', query: 'up{job=~"kubernetes-(nodes|cadvisor)"}' },
  { label: '节点 CPU', query: '100 - (avg by (instance) (rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100)' },
  { label: '节点内存', query: '100 * (1 - node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes)' },
  { label: '节点磁盘', query: '100 * (1 - node_filesystem_avail_bytes{fstype!~"tmpfs|overlay"} / node_filesystem_size_bytes{fstype!~"tmpfs|overlay"})' },
  { label: '节点采集', query: 'up{job="kubernetes-nodes"}' },
  { label: '容器采集', query: 'up{job="kubernetes-cadvisor"}' },
]
const readyNodes = computed(() => nodes.value.filter(node => node.ready))
const statusLabel = computed(() => ({ ready: '已就绪', installing: '安装中', degraded: '异常', unavailable: '不可用' }[status.value?.state] || '状态未知'))
const statusClass = computed(() => ({ ready: 'badge-online', installing: 'badge-deploying', degraded: 'badge-danger' }[status.value?.state] || 'badge-offline'))
const targetRows = computed(() => {
  const active = targets.value?.activeTargets || []
  const dropped = targets.value?.droppedTargets || []
  return [...active, ...dropped].slice(0, 12).map((target, index) => ({ key: `${target.scrapePool || 'target'}-${target.discoveredLabels?.__address__ || index}`, job: target.labels?.job || target.scrapePool || 'unknown', instance: target.labels?.instance || target.discoveredLabels?.__address__ || '-', health: target.health || 'down' }))
})
const targetSummary = computed(() => {
  const active = targets.value?.activeTargets || []
  return { active: active.filter(target => target.health === 'up').length, failed: active.some(target => target.health !== 'up') }
})

onMounted(async () => {
  await refresh()
  refreshTimer = window.setInterval(refresh, 15000)
})
onBeforeUnmount(() => window.clearInterval(refreshTimer))

async function refresh() {
  error.value = ''
  try {
    const [nextStatus, nodeList] = await Promise.all([api.get('/monitoring/status'), api.get('/nodes')])
    status.value = nextStatus
    nodes.value = nodeList || []
    if (!form.value.node_name) form.value.node_name = readyNodes.value[0]?.name || ''
    if (status.value?.state === 'ready') await Promise.all([loadTargets(), loadQuickMetrics()])
  } catch (e) { error.value = e.message || '加载监控状态失败' } finally { loaded.value = true }
}

function displayNode(node) { return `${nodeDisplayName(node.name)} (${node.name})` }
function nodeDisplayName(name) { return nodes.value.find(node => node.name === name)?.name || name }

async function install() {
  installing.value = true
  error.value = ''
  try { status.value = await api.post('/monitoring/install', form.value); await refresh() } catch (e) { error.value = e.message || '安装 VictoriaMetrics 失败' } finally { installing.value = false }
}

async function uninstall() {
  uninstalling.value = true
  error.value = ''
  try { await api.delete('/monitoring'); confirmUninstall.value = false; targets.value = null; queryResult.value = ''; await refresh() } catch (e) { error.value = e.message || '卸载 VictoriaMetrics 失败' } finally { uninstalling.value = false }
}

async function loadTargets() {
  targetsLoading.value = true
  try { targets.value = await api.get('/monitoring/targets') } catch (e) { error.value = e.message || '读取采集目标失败' } finally { targetsLoading.value = false }
}

async function loadQuickMetrics() {
  const [nodesResult, cadvisorResult, healthResult] = await Promise.all([
    api.get(`/monitoring/query?query=${encodeURIComponent('sum(up{job="kubernetes-nodes"})')}`),
    api.get(`/monitoring/query?query=${encodeURIComponent('sum(up{job="kubernetes-cadvisor"})')}`),
    api.get(`/monitoring/query?query=${encodeURIComponent('sum(up{job=~"kubernetes-(nodes|cadvisor)"}) / count(up{job=~"kubernetes-(nodes|cadvisor)"}) * 100')}`),
  ])
  quickMetrics.value = { nodes: scalarValue(nodesResult), cadvisor: scalarValue(cadvisorResult), health: `${scalarValue(healthResult)}%` }
}

function scalarValue(result) {
  const value = result?.result?.[0]?.value?.[1]
  const number = Number(value)
  return Number.isFinite(number) ? Number.isInteger(number) ? String(number) : number.toFixed(1) : '-'
}

function runPreset(preset) { query.value = preset.query; runQuery() }
async function runQuery() {
  querying.value = true
  error.value = ''
  try { queryResult.value = JSON.stringify(await api.get(`/monitoring/query?query=${encodeURIComponent(query.value)}`), null, 2) } catch (e) { error.value = e.message || '查询指标失败' } finally { querying.value = false }
}
</script>

<style scoped>
.page-header,.card-header{display:flex;align-items:flex-start;justify-content:space-between;gap:var(--space-16)}.status-copy,.metric small,.form-hint,.confirm-copy{margin:5px 0 0;color:var(--text-secondary);font-size:12px}.install-card{max-width:720px}.install-form{margin-top:var(--space-20)}.monitoring-summary{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:var(--space-12)}.metric{display:grid;min-width:0;gap:5px;padding:14px;border:1px solid var(--border-muted);border-radius:var(--radius-control);background:var(--surface-subtle)}.metric>span,.quick-metrics span{color:var(--text-secondary);font-size:11px}.metric strong{min-width:0;font-size:18px}.metric small{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.metric-code{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-family:var(--font-mono);font-size:13px!important}.monitoring-grid{display:grid;grid-template-columns:minmax(0,3fr) minmax(260px,2fr);gap:var(--space-16);margin-bottom:var(--space-20)}.targets-card,.quick-card,.query-card{margin:0}.target-list{display:grid;gap:7px;margin-top:var(--space-16)}.target-row{display:flex;align-items:center;justify-content:space-between;gap:12px;padding:8px 0;border-bottom:1px solid var(--border-muted)}.target-row:last-child{border-bottom:0}.target-row span:first-child{display:grid;min-width:0;gap:2px}.target-row strong,.target-row small{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.target-row small{color:var(--text-secondary);font:11px/1.3 var(--font-mono)}.quick-metrics{display:grid;gap:12px;margin-top:var(--space-16)}.quick-metrics div{display:flex;align-items:center;justify-content:space-between;padding:9px 0;border-bottom:1px solid var(--border-muted)}.quick-metrics div:last-child{border-bottom:0}.quick-metrics strong{font-size:17px}.query-presets{display:flex;flex-wrap:wrap;justify-content:flex-end;gap:6px}.query-form{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:8px;margin-top:var(--space-16)}.query-result{max-height:360px;overflow:auto;margin:var(--space-16) 0 0;padding:12px;border:1px solid var(--border-muted);border-radius:var(--radius-control);background:var(--surface-subtle);color:var(--text-primary);font:11px/1.5 var(--font-mono)}.empty-inline{padding:16px 0;color:var(--text-muted);font-size:12px}.wait-card .empty-state{min-height:140px}@media(max-width:760px){.monitoring-summary{grid-template-columns:repeat(2,minmax(0,1fr))}.monitoring-grid{grid-template-columns:1fr}.query-form{grid-template-columns:1fr}.query-form .btn{width:100%}.page-header{flex-direction:column}.page-header .btn-group{width:100%}.page-header .btn{flex:1}}@media(max-width:440px){.monitoring-summary{grid-template-columns:1fr}.query-presets{justify-content:flex-start}}
</style>
