<template>
  <div>
    <SectionTabsHeader title="集群监控" :tabs="tabs" :active-tab="activeTab" @select="activeTab = $event">
      <template #actions>
        <div v-if="status?.state !== 'not_installed'" class="btn-group">
          <button class="btn" @click="refresh">刷新</button>
          <button class="btn btn-danger" @click="confirmUninstall = true">卸载</button>
        </div>
      </template>
    </SectionTabsHeader>

    <main class="monitoring-content">
      <div v-if="error" class="k8s-banner k8s-banner-warn section-gap">{{ error }}</div>

      <section v-if="loaded && status?.state === 'not_installed'" class="card monitoring-install-card">
        <div class="card-header"><div><h2 class="card-title">VictoriaMetrics 未安装</h2><p class="status-copy">选择数据节点并配置本地目录后，平台将部署指标存储和节点采集组件。</p></div><span class="badge badge-offline">未安装</span></div>
        <form class="install-form" @submit.prevent="install">
          <div class="form-group"><label class="form-label">数据节点</label><select v-model="form.node_name" class="form-select" required><option value="" disabled>选择就绪节点</option><option v-for="node in readyNodes" :key="node.name" :value="node.name">{{ displayNode(node) }}</option></select><p class="form-hint">将使用 kubernetes.io/hostname 标签约束 VictoriaMetrics 到所选节点。</p></div>
          <div class="form-row"><div class="form-group"><label class="form-label">本地数据目录</label><input v-model.trim="form.data_path" class="form-input" required placeholder="/data/victoria-metrics" /></div><div class="form-group"><label class="form-label">指标保留天数</label><input v-model.number="form.retention_days" class="form-input" type="number" min="1" max="365" required /></div></div>
          <p class="form-hint">目录必须位于 <code>/data/</code>。安装后不能直接变更节点或目录，卸载也不会删除已有指标数据。</p>
          <div class="modal-actions status-actions"><button class="btn btn-primary" :disabled="installing || !form.node_name">{{ installing ? '正在提交...' : '安装监控' }}</button><button type="button" class="btn" :disabled="installing" @click="refresh">重新检测</button></div>
        </form>
      </section>

      <template v-else-if="loaded && status">
        <section v-if="status.state !== 'ready'" class="card wait-card"><div class="empty-state"><span class="empty-icon">◌</span><span class="empty-text">等待 VictoriaMetrics 工作负载就绪</span></div></section>

        <template v-else-if="activeTab === 'overview'">
          <section class="metric-grid monitoring-summary section-gap">
            <article class="metric"><span>监控状态</span><strong><span class="badge" :class="statusClass">{{ statusLabel }}</span></strong><small>{{ status.message }}</small></article>
            <article class="metric"><span>可用节点</span><strong>{{ readyNodes.length }} / {{ nodes.length }}</strong><small>{{ status.node_exporter_ready || 0 }} / {{ status.node_exporter_desired || 0 }} node-exporter 就绪</small></article>
            <article class="metric"><span>最高 CPU</span><strong>{{ formatPercent(highestCPU?.cpu) }}</strong><small>{{ highestCPU?.name || '等待指标采集' }}</small></article>
            <article class="metric"><span>最高内存</span><strong>{{ formatPercent(highestMemory?.memory) }}</strong><small>{{ highestMemory?.name || '等待指标采集' }}</small></article>
            <article class="metric"><span>最高磁盘</span><strong>{{ formatPercent(highestDisk?.disk) }}</strong><small>{{ highestDisk?.name || '等待指标采集' }}</small></article>
          </section>

          <section class="monitoring-section-heading section-gap"><div><h2>资源趋势</h2><p>按节点对比持续资源压力</p></div><RangePicker :range="trendRange" @select="trendRange = $event" /></section>
          <section class="monitoring-trend-grid">
            <MetricTrendChart title="CPU 使用率" subtitle="5 分钟平均" unit="%" :threshold="85" :loading="trendsLoading" :series="nodeTrends.cpu" />
            <MetricTrendChart title="内存使用率" subtitle="可用内存占比" unit="%" :threshold="90" :loading="trendsLoading" :series="nodeTrends.memory" />
            <MetricTrendChart title="根磁盘使用率" subtitle="仅统计 / 挂载点" unit="%" :threshold="85" :loading="trendsLoading" :series="nodeTrends.disk" />
            <MetricTrendChart title="网络入站速率" subtitle="不含 lo 与 veth" unit=" MB/s" :loading="trendsLoading" :series="nodeTrends.network" />
          </section>
        </template>

        <template v-else-if="activeTab === 'nodes'">
          <section class="monitoring-section-heading section-gap"><div><h2>节点资源</h2><p>选择节点查看当前资源与历史趋势</p></div><RangePicker :range="trendRange" @select="trendRange = $event" /></section>
          <section class="card section-gap"><div class="table-wrap"><table class="data-table node-table"><thead><tr><th>节点</th><th>状态</th><th>CPU</th><th>内存</th><th>根磁盘</th><th>网络入站</th></tr></thead><tbody><tr v-for="node in nodeRows" :key="node.name" :class="{ 'is-selected': selectedNode === node.name }" @click="selectedNode = node.name"><td class="cell-primary">{{ displayNode(node) }}<small class="cell-secondary">{{ node.name }}</small></td><td><span class="badge" :class="node.ready ? 'badge-online' : 'badge-danger'">{{ node.ready ? '就绪' : '不可用' }}</span></td><td>{{ formatPercent(node.cpu) }}</td><td>{{ formatPercent(node.memory) }}</td><td>{{ formatPercent(node.disk) }}</td><td>{{ formatRate(node.network) }}</td></tr></tbody></table></div></section>
          <section v-if="selectedNode" class="monitoring-section-heading section-gap"><div><h2>{{ displayNode({ name: selectedNode }) }} 趋势</h2><p>仅展示当前选择的节点</p></div></section>
          <section v-if="selectedNode" class="monitoring-trend-grid"><MetricTrendChart title="CPU 使用率" unit="%" :threshold="85" :loading="trendsLoading" :series="selectedSeries(nodeTrends.cpu)" /><MetricTrendChart title="内存使用率" unit="%" :threshold="90" :loading="trendsLoading" :series="selectedSeries(nodeTrends.memory)" /><MetricTrendChart title="根磁盘使用率" unit="%" :threshold="85" :loading="trendsLoading" :series="selectedSeries(nodeTrends.disk)" /><MetricTrendChart title="网络入站速率" unit=" MB/s" :loading="trendsLoading" :series="selectedSeries(nodeTrends.network)" /></section>
        </template>

        <template v-else-if="activeTab === 'workloads'">
          <section class="monitoring-section-heading section-gap"><div><h2>工作负载资源</h2><p>按当前资源使用排序，定位最需要排查的 Pod</p></div><button class="icon-button" title="刷新工作负载指标" aria-label="刷新工作负载指标" :disabled="workloadsLoading" @click="loadWorkloads"><RefreshCw :size="16" :class="{ 'is-spinning': workloadsLoading }" /></button></section>
          <section class="monitoring-workload-grid"><article class="card"><div class="card-header"><div><h2 class="card-title">CPU 使用最高</h2><p class="status-copy">最近 5 分钟平均</p></div></div><WorkloadTable :rows="workloads.cpu" unit="m" :loading="workloadsLoading" /></article><article class="card"><div class="card-header"><div><h2 class="card-title">内存使用最高</h2><p class="status-copy">工作集内存</p></div></div><WorkloadTable :rows="workloads.memory" unit="MiB" :loading="workloadsLoading" /></article></section>
        </template>

        <template v-else>
          <section class="monitoring-summary metric-grid section-gap"><article class="metric"><span>数据节点</span><strong class="metric-code">{{ nodeDisplayName(status.node_name) || '-' }}</strong><small>{{ status.node_name || '-' }}</small></article><article class="metric"><span>数据目录</span><strong class="metric-code">{{ status.data_path || '-' }}</strong><small>保留 {{ status.retention_days || '-' }} 天</small></article><article class="metric"><span>采集节点</span><strong>{{ status.node_exporter_ready || 0 }} / {{ status.node_exporter_desired || 0 }}</strong><small>node-exporter 已就绪</small></article></section>
          <section class="monitoring-grid section-gap"><article class="card targets-card"><div class="card-header"><div><h2 class="card-title">采集目标</h2><p class="status-copy">节点、容器与 node-exporter</p></div><span class="badge" :class="targetSummary.failed ? 'badge-danger' : 'badge-online'">{{ targetSummary.active }} 个在线</span></div><div v-if="targetsLoading" class="empty-inline">正在读取采集状态...</div><div v-else-if="targetRows.length" class="target-list"><div v-for="target in targetRows" :key="target.key" class="target-row"><span><strong>{{ target.job }}</strong><small>{{ target.instance }}</small></span><span class="badge" :class="target.health === 'up' ? 'badge-online' : 'badge-danger'">{{ target.health === 'up' ? '正常' : '异常' }}</span></div></div><div v-else class="empty-inline">等待首次指标采集</div></article><article class="card quick-card"><div class="card-header"><div><h2 class="card-title">采集配置</h2><p class="status-copy">趋势图需要按节点标签采集</p></div></div><p class="diagnostic-copy">同步后约 1 分钟开始出现按节点拆分的趋势数据。</p><button class="btn" :disabled="syncing" @click="syncConfiguration">{{ syncing ? '同步中...' : '同步采集配置' }}</button></article></section>
          <section class="card query-card"><div class="card-header"><div><h2 class="card-title">高级查询</h2><p class="status-copy">PromQL</p></div><div class="query-presets"><button v-for="preset in presets" :key="preset.query" class="btn btn-sm" @click="runQuery(preset.query)">{{ preset.label }}</button></div></div><form class="query-form" @submit.prevent="runQuery(query)"><input v-model.trim="query" class="form-input" placeholder="up" maxlength="2048" /><button class="btn btn-primary" :disabled="querying || !query">{{ querying ? '查询中...' : '查询' }}</button></form><pre v-if="queryResult" class="query-result">{{ queryResult }}</pre><div v-else class="empty-inline">输入 PromQL 查询监控原始指标</div></section>
        </template>
      </template>
    </main>

    <div v-if="confirmUninstall" class="overlay" @click.self="confirmUninstall = false"><div class="modal"><h2 class="modal-title">卸载 VictoriaMetrics</h2><p class="confirm-copy">将删除监控工作负载和采集配置，但不会删除 {{ status?.node_name }} 上的 {{ status?.data_path }} 数据目录。</p><div class="modal-actions"><button class="btn" @click="confirmUninstall = false">取消</button><button class="btn btn-danger" :disabled="uninstalling" @click="uninstall">{{ uninstalling ? '卸载中...' : '确认卸载' }}</button></div></div></div>
  </div>
</template>

<script setup>
import { computed, defineComponent, h, onMounted, ref, watch } from 'vue'
import { RefreshCw } from 'lucide-vue-next'
import { api } from '../api/index.js'
import MetricTrendChart from '../components/MetricTrendChart.vue'
import SectionTabsHeader from '../components/SectionTabsHeader.vue'

const tabs = [
  { id: 'overview', label: '概览' },
  { id: 'nodes', label: '节点' },
  { id: 'workloads', label: '工作负载' },
  { id: 'diagnostics', label: '诊断' },
]
const trendRanges = ['1h', '6h', '24h', '7d']
const status = ref(null)
const nodes = ref([])
const activeTab = ref('overview')
const loaded = ref(false)
const error = ref('')
const installing = ref(false)
const uninstalling = ref(false)
const syncing = ref(false)
const confirmUninstall = ref(false)
const targets = ref(null)
const targetsLoading = ref(false)
const trendsLoading = ref(false)
const workloadsLoading = ref(false)
const querying = ref(false)
const query = ref('')
const queryResult = ref('')
const trendRange = ref('6h')
const selectedNode = ref('')
const nodeTrends = ref({ cpu: [], memory: [], disk: [], network: [] })
const workloads = ref({ cpu: [], memory: [] })
const form = ref({ node_name: '', data_path: '/data/victoria-metrics', retention_days: 14 })

const presets = [
  { label: '全部目标', query: 'up{job=~"kubernetes-(nodes|cadvisor)"}' },
  { label: '节点 CPU', query: '100 - (avg by (node) (rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100)' },
  { label: '节点内存', query: '100 * (1 - node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes)' },
  { label: '节点磁盘', query: 'max by (node) (100 * (1 - node_filesystem_avail_bytes{mountpoint="/",fstype!~"tmpfs|overlay"} / node_filesystem_size_bytes{mountpoint="/",fstype!~"tmpfs|overlay"}))' },
]
const readyNodes = computed(() => nodes.value.filter(node => node.ready))
const statusLabel = computed(() => ({ ready: '已就绪', installing: '安装中', degraded: '异常', unavailable: '不可用' }[status.value?.state] || '状态未知'))
const statusClass = computed(() => ({ ready: 'badge-online', installing: 'badge-deploying', degraded: 'badge-danger' }[status.value?.state] || 'badge-offline'))
const targetRows = computed(() => {
  const active = targets.value?.activeTargets || []
  const dropped = targets.value?.droppedTargets || []
  return [...active, ...dropped].slice(0, 12).map((target, index) => ({ key: `${target.scrapePool || 'target'}-${target.discoveredLabels?.__address__ || index}`, job: target.labels?.job || target.scrapePool || 'unknown', instance: target.labels?.node || target.labels?.instance || target.discoveredLabels?.__address__ || '-', health: target.health || 'down' }))
})
const targetSummary = computed(() => {
  const active = targets.value?.activeTargets || []
  return { active: active.filter(target => target.health === 'up').length, failed: active.some(target => target.health !== 'up') }
})
const nodeRows = computed(() => nodes.value.map(node => ({
  ...node,
  cpu: latestSeriesValue(nodeTrends.value.cpu, node.name),
  memory: latestSeriesValue(nodeTrends.value.memory, node.name),
  disk: latestSeriesValue(nodeTrends.value.disk, node.name),
  network: latestSeriesValue(nodeTrends.value.network, node.name),
})))
const highestCPU = computed(() => highestNode('cpu'))
const highestMemory = computed(() => highestNode('memory'))
const highestDisk = computed(() => highestNode('disk'))

const RangePicker = defineComponent({
  props: { range: { type: String, required: true } },
  emits: ['select'],
  setup(props, { emit }) {
    return () => h('label', { class: 'range-select' }, [
      h('span', '时间范围'),
      h('select', { class: 'form-select trend-range-select', value: props.range, onChange: event => emit('select', event.target.value) }, trendRanges.map(range => h('option', { value: range }, rangeLabel(range)))),
    ])
  },
})
const WorkloadTable = defineComponent({
  props: { rows: { type: Array, required: true }, unit: { type: String, required: true }, loading: Boolean },
  setup(props) {
    return () => props.loading ? h('div', { class: 'empty-inline' }, '正在读取工作负载指标...') : props.rows.length ? h('div', { class: 'table-wrap' }, [h('table', { class: 'data-table' }, [h('thead', [h('tr', [h('th', '命名空间'), h('th', 'Pod'), h('th', '当前值')])]), h('tbody', props.rows.map(row => h('tr', { key: `${row.namespace}/${row.pod}` }, [h('td', row.namespace), h('td', { class: 'cell-primary' }, row.pod), h('td', `${row.value.toFixed(props.unit === 'm' ? 0 : 1)} ${props.unit}`)])))])]) : h('div', { class: 'empty-inline' }, '暂未采集到容器资源指标')
  },
})

onMounted(async () => {
  await refresh()
})
watch([activeTab, trendRange], async () => {
  if (status.value?.state === 'ready') await loadActiveData()
})

async function refresh() {
  error.value = ''
  try {
    const [nextStatus, nodeList] = await Promise.all([api.get('/monitoring/status'), api.get('/nodes')])
    status.value = nextStatus
    nodes.value = nodeList || []
    if (!form.value.node_name) form.value.node_name = readyNodes.value[0]?.name || ''
    if (status.value?.state === 'ready') await loadActiveData()
  } catch (e) { error.value = e.message || '加载监控状态失败' } finally { loaded.value = true }
}

function displayNode(node) { return nodeDisplayName(node.name) }
function nodeDisplayName(name) { return nodes.value.find(node => node.name === name)?.display_name || nodes.value.find(node => node.name === name)?.name || name }

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

async function syncConfiguration() {
  syncing.value = true
  error.value = ''
  try {
    await api.post('/monitoring/install', { node_name: status.value.node_name, data_path: status.value.data_path, retention_days: status.value.retention_days })
    await refresh()
  } catch (e) { error.value = e.message || '同步采集配置失败' } finally { syncing.value = false }
}

async function loadTargets() {
  targetsLoading.value = true
  try { targets.value = await api.get('/monitoring/targets') } catch (e) { error.value = e.message || '读取采集状态失败' } finally { targetsLoading.value = false }
}

async function loadActiveData() {
  if (activeTab.value === 'overview' || activeTab.value === 'nodes') await loadNodeTrends()
  if (activeTab.value === 'workloads') await loadWorkloads()
  if (activeTab.value === 'diagnostics') await loadTargets()
}

async function loadNodeTrends() {
  trendsLoading.value = true
  try {
    const dashboard = await api.get(`/monitoring/dashboard?range=${trendRange.value}`)
    nodeTrends.value = Object.fromEntries(Object.entries(dashboard?.trends || {}).map(([key, result]) => [key, matrixToSeries(result)]))
    if (!selectedNode.value || !nodeRows.value.some(node => node.name === selectedNode.value)) selectedNode.value = nodeRows.value[0]?.name || ''
  } catch (e) { error.value = e.message || '读取节点趋势失败' } finally { trendsLoading.value = false }
}

async function loadWorkloads() {
  workloadsLoading.value = true
  try {
    const [cpu, memory] = await Promise.all([
      api.get(`/monitoring/query?query=${encodeURIComponent('topk(12, sum by (namespace, pod) (rate(container_cpu_usage_seconds_total{container!="",image!=""}[5m])) * 1000)')}`),
      api.get(`/monitoring/query?query=${encodeURIComponent('topk(12, sum by (namespace, pod) (container_memory_working_set_bytes{container!="",image!=""}) / 1024 / 1024)')}`),
    ])
    workloads.value = { cpu: vectorToWorkloads(cpu), memory: vectorToWorkloads(memory) }
  } catch (e) { error.value = e.message || '读取工作负载指标失败' } finally { workloadsLoading.value = false }
}

function matrixToSeries(result) {
  return (result?.result || []).map((item, index) => ({ label: metricNodeName(item.metric, index), values: (item.values || []).map(([timestamp, value]) => ({ timestamp: Number(timestamp), value: Number(value) })) }))
}
function metricNodeName(metric, index) {
  if (metric?.node) return metric.node
  const host = (metric?.instance || '').replace(/:\d+$/, '')
  return nodes.value.find(node => node.internal_ip === host)?.name || metric?.instance || `序列 ${index + 1}`
}
function vectorToWorkloads(result) {
  return (result?.result || []).map(item => ({ namespace: item.metric?.namespace || '-', pod: item.metric?.pod || '-', value: Number(item.value?.[1]) || 0 }))
}
function latestSeriesValue(series, name) {
  const values = series.find(item => item.label === name)?.values || []
  return values.at(-1)?.value ?? null
}
function highestNode(metric) {
  return nodeRows.value.filter(node => Number.isFinite(node[metric])).sort((left, right) => right[metric] - left[metric])[0] || null
}
function selectedSeries(series) { return series.filter(item => item.label === selectedNode.value) }
function formatPercent(value) { return Number.isFinite(value) ? `${value.toFixed(1)}%` : '-' }
function formatRate(value) { return Number.isFinite(value) ? `${value.toFixed(2)} MB/s` : '-' }
function rangeLabel(range) { return ({ '1h': '最近 1 小时', '6h': '最近 6 小时', '24h': '最近 24 小时', '7d': '最近 7 天' }[range] || range) }

async function runQuery(queryText) {
  querying.value = true
  error.value = ''
  try { query.value = queryText; queryResult.value = JSON.stringify(await api.get(`/monitoring/query?query=${encodeURIComponent(queryText)}`), null, 2) } catch (e) { error.value = e.message || '查询指标失败' } finally { querying.value = false }
}
</script>

<style scoped>
.monitoring-content { margin-top: var(--space-20); }.status-copy,.metric small,.form-hint,.confirm-copy,.monitoring-section-heading p{margin:5px 0 0;color:var(--text-secondary);font-size:12px}.install-form{margin-top:var(--space-20)}.status-actions{justify-content:flex-start;margin-top:var(--space-16)}.monitoring-summary{grid-template-columns:repeat(5,minmax(0,1fr))}.metric{display:grid;min-width:0;gap:5px;padding:14px;border:1px solid var(--border-muted);border-radius:var(--radius-control);background:var(--surface-subtle)}.metric>span{color:var(--text-secondary);font-size:11px}.metric strong{min-width:0;font-size:18px}.metric small{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.metric-code{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-family:var(--font-mono);font-size:13px!important}.monitoring-section-heading{display:flex;align-items:center;justify-content:space-between;gap:var(--space-16)}.monitoring-section-heading h2{margin:0;color:var(--text-primary);font-size:16px}.range-select{display:flex;align-items:center;gap:8px;color:var(--text-secondary);font-size:11px;font-weight:700;white-space:nowrap}.trend-range-select{width:148px;min-height:34px;padding:6px 28px 6px 9px;font-size:11px}.monitoring-trend-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:var(--space-16)}.monitoring-workload-grid,.monitoring-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:var(--space-16)}.node-table tbody tr{cursor:pointer}.node-table tbody tr.is-selected{background:var(--success-surface)}.targets-card,.quick-card,.query-card{margin:0}.target-list{display:grid;gap:7px;margin-top:var(--space-16)}.target-row{display:flex;align-items:center;justify-content:space-between;gap:12px;padding:8px 0;border-bottom:1px solid var(--border-muted)}.target-row:last-child{border-bottom:0}.target-row span:first-child{display:grid;min-width:0;gap:2px}.target-row strong,.target-row small{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.target-row small{color:var(--text-secondary);font:11px/1.3 var(--font-mono)}.diagnostic-copy{margin:0 0 var(--space-16);color:var(--text-secondary);font-size:12px;line-height:1.6}.query-presets{display:flex;flex-wrap:wrap;justify-content:flex-end;gap:6px}.query-form{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:8px;margin-top:var(--space-16)}.query-result{max-height:360px;overflow:auto;margin:var(--space-16) 0 0;padding:12px;border:1px solid var(--border-muted);border-radius:var(--radius-control);background:var(--surface-subtle);color:var(--text-primary);font:11px/1.5 var(--font-mono)}.empty-inline{padding:16px 0;color:var(--text-muted);font-size:12px}.wait-card .empty-state{min-height:140px}@media(max-width:960px){.monitoring-summary{grid-template-columns:repeat(3,minmax(0,1fr))}}@media(max-width:760px){.monitoring-trend-grid,.monitoring-workload-grid,.monitoring-grid{grid-template-columns:1fr}.monitoring-summary{grid-template-columns:repeat(2,minmax(0,1fr))}.query-form{grid-template-columns:1fr}.query-form .btn{width:100%}}@media(max-width:440px){.monitoring-summary{grid-template-columns:1fr}.monitoring-section-heading{align-items:flex-start;flex-direction:column}.range-select{align-items:flex-start;flex-direction:column}.trend-range-select{width:100%}}
</style>
