<template>
  <div class="dashboard-page">
    <header class="page-header dashboard-header sticky-page-header">
      <h1 class="page-title">平台健康度</h1>
      <div class="dashboard-header-actions">
        <button class="icon-button" type="button" title="刷新首页" aria-label="刷新首页" :disabled="clustersLoading" @click="refreshAll"><RefreshCw :size="15" :class="{ 'is-spinning': clustersLoading }" /></button>
        <router-link class="btn btn-primary" to="/applications">部署服务</router-link>
      </div>
    </header>

    <SurfaceCard class="dashboard-resources">
      <template #header><h2 class="card-title">平台资源</h2></template>
      <template #actions>
        <span v-if="activeClusters.length" class="dashboard-coverage" :class="{ 'is-partial': resourceClusters.length < activeClusters.length || hasPartialMetrics }">已汇总 {{ resourceClusters.length }}/{{ activeClusters.length }} 个已部署集群<template v-if="hasPartialMetrics"> · 部分指标不可用</template></span>
        <router-link class="panel-link" to="/cluster?tab=list">管理集群 <ArrowUpRight :size="14" /></router-link>
      </template>
      <EmptyState v-if="clustersLoading && !clusters.length" variant="loading" message="正在读取集群状态..." />
      <EmptyState v-else-if="!clusters.length" message="暂无集群" />
      <EmptyState v-else-if="!activeClusters.length" message="暂无已部署集群" />
      <div v-else class="dashboard-resource-grid" aria-label="多集群资源汇总">
        <div class="dashboard-resource-metric"><strong>{{ aggregateMetric('nodes_total') }}</strong><span>节点</span></div>
        <div class="dashboard-resource-metric"><strong :class="readyTone('pods')">{{ aggregateReadyMetric('pods') }}</strong><span>Pods 就绪</span></div>
        <div class="dashboard-resource-metric"><strong :class="readyTone('deployments')">{{ aggregateReadyMetric('deployments') }}</strong><span>Deployments 就绪</span></div>
        <div class="dashboard-resource-metric"><strong>{{ aggregateMetric('services_total') }}</strong><span>服务</span></div>
        <div class="dashboard-resource-metric"><strong>{{ aggregateMetric('namespaces') }}</strong><span>命名空间</span></div>
        <div class="dashboard-resource-metric dashboard-capacity"><strong>{{ aggregateMetric('cpu_cores_total') }} vCPU · {{ aggregateMemory() }}</strong><span>计算容量</span></div>
      </div>
    </SurfaceCard>

    <div class="dashboard-lower-grid">
      <section class="dashboard-trends-panel">
        <MetricTrendChart v-if="trendsAvailable" class="dashboard-trend-chart" title="资源趋势" :unit="selectedTrendMetric.unit" :loading="trends.loading.value" :series="selectedTrendSeries">
          <template #title><div class="dashboard-trend-title"><SelectMenu v-model="selectedClusterID" class="form-select dashboard-cluster-select" aria-label="选择趋势集群">
            <option value="">选择集群</option>
            <option v-for="cluster in clusters" :key="cluster.id" :value="String(cluster.id)">{{ cluster.name }}</option>
          </SelectMenu><h2 class="card-title">资源趋势</h2></div></template>
          <template #actions>
            <router-link v-if="selectedClusterID" class="panel-link" :to="`/cluster/${selectedClusterID}/monitoring`">查看监控 <ArrowUpRight :size="14" /></router-link>
          </template>
          <template #toolbar>
            <div class="dashboard-trend-toolbar">
              <div class="dashboard-segmented" role="group" aria-label="资源类型"><button v-for="metric in trendMetrics" :key="metric.id" type="button" :class="{ active: selectedTrendMetricId === metric.id }" @click="selectTrendMetric(metric.id)">{{ metric.label }}</button></div>
              <div class="dashboard-segmented" role="group" aria-label="展示方式"><button type="button" :class="{ active: trendDisplayMode === 'average' }" @click="trendDisplayMode = 'average'">集群平均</button><button type="button" :class="{ active: trendDisplayMode === 'nodes' }" @click="trendDisplayMode = 'nodes'">按节点</button></div>
              <SelectMenu v-if="trendDisplayMode === 'nodes' && trendNodeOptions.length" v-model="selectedTrendNode" class="form-select dashboard-node-select" aria-label="选择节点"><option value="">全部节点</option><option v-for="node in trendNodeOptions" :key="node" :value="node">{{ node }}</option></SelectMenu>
            </div>
          </template>
        </MetricTrendChart>
        <SurfaceCard v-else class="dashboard-trend-empty">
          <template #header><div class="dashboard-trend-title">
            <SelectMenu v-model="selectedClusterID" class="form-select dashboard-cluster-select" aria-label="选择趋势集群">
              <option value="">选择集群</option>
              <option v-for="cluster in clusters" :key="cluster.id" :value="String(cluster.id)">{{ cluster.name }}</option>
            </SelectMenu>
            <h2 class="card-title">资源趋势</h2>
          </div></template>
          <template #actions>
            <router-link v-if="selectedClusterID" class="panel-link" :to="`/cluster/${selectedClusterID}/monitoring`">查看监控 <ArrowUpRight :size="14" /></router-link>
          </template>
          <EmptyState :variant="monitoringStatus.loading.value ? 'loading' : 'empty'" :message="trendEmptyMessage">
            <template v-if="selectedClusterID && monitoringStatus.data.value?.state === 'not_installed'" #action><router-link class="btn btn-sm" :to="`/cluster/${selectedClusterID}/monitoring`">安装监控</router-link></template>
          </EmptyState>
        </SurfaceCard>
      </section>

      <SurfaceCard class="dashboard-application-panel">
        <template #header><h2 class="card-title">全平台应用</h2></template>
        <template #actions><router-link class="panel-link" to="/applications">查看应用</router-link></template>
        <div v-if="dashboardSectionError('applications')" class="dashboard-application-unavailable">应用状态暂不可用</div>
        <template v-else>
          <div class="application-summary-count"><strong>{{ applicationMetric('total_applications') }}</strong><span>个应用</span></div>
          <div class="application-status-grid">
            <div><strong class="metric-success">{{ applicationMetric('successful_applications') }}</strong><span>运行正常</span></div>
            <div><strong>{{ applicationMetric('releasing_applications') }}</strong><span>发布中</span></div>
            <div><strong class="metric-warn">{{ applicationMetric('failed_applications') }}</strong><span>发布失败</span></div>
            <div><strong>{{ applicationMetric('unreleased_applications') }}</strong><span>未发布</span></div>
          </div>
          <div class="application-latest-release"><span>最近发布</span><strong>{{ applicationSummary?.latest_release?.application_name || '暂无发布记录' }}</strong></div>
        </template>
      </SurfaceCard>

      <SurfaceCard class="dashboard-activity-panel">
        <template #header><h2 class="card-title">最近操作</h2></template>
        <template #actions><router-link class="panel-link" to="/audit">查看全部</router-link></template>
        <EmptyState v-if="dashboard.loading.value" variant="loading" message="正在读取..." />
        <EmptyState v-else-if="dashboardSectionError('recent_logs')" message="操作记录暂不可用" />
        <EmptyState v-else-if="!recentLogs.length" message="暂无操作记录" />
        <div v-else class="activity-list"><div v-for="log in recentLogs.slice(0, 4)" :key="log.id" class="activity-row"><span class="badge activity-action" :class="actionBadge(log.action)" :title="actionLabel(log.action)">{{ actionLabel(log.action) }}</span><span class="activity-resource" :title="`${resourceLabel(log.resource_type)} #${log.resource_id}`">{{ resourceLabel(log.resource_type) }} #{{ log.resource_id }}</span></div></div>
      </SurfaceCard>
    </div>

    <ErrorNoticeModal :open="Boolean(pageError)" :message="pageError" teleport @close="dismissedError = true" />
  </div>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { ArrowUpRight, RefreshCw } from 'lucide-vue-next'
import { getDashboardClusters, getDashboardOverview } from '../api/dashboard.js'
import { getMonitoringDashboard, getMonitoringStatus } from '../api/monitoring.js'
import EmptyState from '../components/EmptyState.vue'
import ErrorNoticeModal from '../components/ErrorNoticeModal.vue'
import SelectMenu from '../components/SelectMenu.vue'
import SurfaceCard from '../components/SurfaceCard.vue'
import { useAsyncResource } from '../composables/useAsyncResource.js'
import MetricTrendChart from './monitoring/MetricTrendChart.vue'

const dashboard = useAsyncResource(getDashboardOverview)
const clusterResource = useAsyncResource(getDashboardClusters, [])
const monitoringStatus = useAsyncResource(({ signal }, id) => getMonitoringStatus({ signal, clusterID: id }))
const trends = useAsyncResource(({ signal }, id) => getMonitoringDashboard('6h', { signal, clusterID: id }))
const clusters = computed(() => clusterResource.data.value || [])
const clustersLoading = clusterResource.loading
const activeClusters = computed(() => clusters.value.filter(cluster => cluster.health !== 'pending'))
const resourceClusters = computed(() => activeClusters.value.filter(cluster => cluster.metrics && Object.keys(cluster.metrics).some(key => key.endsWith('_total') || key === 'namespaces')))
const resourceKeys = ['nodes_total', 'pods_ready', 'pods_total', 'deployments_ready', 'deployments_total', 'services_total', 'namespaces', 'cpu_cores_total', 'memory_mb_total']
const hasPartialMetrics = computed(() => resourceClusters.value.some(cluster => resourceKeys.some(key => !Number.isFinite(Number(cluster.metrics?.[key])) || cluster.metrics[key] == null)))
const selectedClusterID = ref('')
const selectedCluster = computed(() => clusters.value.find(item => String(item.id) === selectedClusterID.value))
const applicationSummary = computed(() => dashboard.data.value?.application_summary || null)
const recentLogs = computed(() => dashboard.data.value?.recent_logs || [])
const dismissedError = ref(false)
const pageError = computed(() => dismissedError.value ? '' : clusterResource.error.value?.message || dashboard.error.value?.message || '')
const trendMetrics = [{ id: 'cpu', label: 'CPU', unit: '%' }, { id: 'memory', label: '内存', unit: '%' }, { id: 'disk', label: '磁盘', unit: '%' }]
const selectedTrendMetricId = ref('cpu')
const selectedTrendMetric = computed(() => trendMetrics.find(item => item.id === selectedTrendMetricId.value) || trendMetrics[0])
const selectedTrendNode = ref('')
const trendDisplayMode = ref('nodes')
const rawTrendSeries = computed(() => matrixToSeries(trends.data.value?.trends?.[selectedTrendMetricId.value]))
const trendNodeOptions = computed(() => [...new Set(rawTrendSeries.value.map(item => item.label).filter(Boolean))])
const selectedTrendSeries = computed(() => trendDisplayMode.value === 'average'
  ? aggregateSeries(rawTrendSeries.value, `${selectedCluster.value?.name || '集群'} ${selectedTrendMetric.value.label} 平均`)
  : selectedTrendNode.value ? rawTrendSeries.value.filter(item => item.label === selectedTrendNode.value) : rawTrendSeries.value)
const trendsAvailable = computed(() => Boolean(selectedCluster.value && !trends.error.value && (monitoringStatus.data.value?.state === 'ready' || Number(monitoringStatus.data.value?.ready_replicas) > 0)))
const trendEmptyMessage = computed(() => {
  if (!selectedClusterID.value) return '选择集群查看资源趋势'
  if (selectedCluster.value?.health === 'unavailable') return '集群不可达，无法读取资源趋势'
  if (selectedCluster.value?.health === 'pending') return '集群尚未部署控制平面'
  if (monitoringStatus.loading.value) return '正在检测监控状态...'
  if (monitoringStatus.data.value?.state === 'not_installed') return '当前集群未安装 VictoriaMetrics'
  if (monitoringStatus.error.value) return '监控状态暂不可用'
  if (trends.error.value) return '资源趋势暂不可用'
  return '监控尚未就绪，暂无资源趋势'
})

onMounted(() => { void refreshAll() })
watch(selectedClusterID, id => { void refreshTrend(id) })

async function refreshAll() {
  dismissedError.value = false
  void dashboard.refresh()
  const before = selectedClusterID.value
  const result = await clusterResource.refresh()
  if (!result) return
  if (!result.some(item => String(item.id) === before)) {
    selectedClusterID.value = String(result.find(item => item.health === 'healthy' || item.health === 'degraded')?.id || '')
  } else {
    await refreshTrend(before)
  }
}

async function refreshTrend(id) {
  monitoringStatus.cancel()
  trends.cancel()
  monitoringStatus.data.value = null
  trends.data.value = null
  selectedTrendNode.value = ''
  const cluster = clusters.value.find(item => String(item.id) === id)
  if (!cluster || cluster.health === 'pending' || cluster.health === 'unavailable') return
  const status = await monitoringStatus.refresh(id)
  if (selectedClusterID.value !== id) return
  if (status?.state === 'ready' || Number(status?.ready_replicas) > 0) await trends.refresh(id)
}

function metricTotal(key) {
  const values = resourceClusters.value.map(cluster => cluster.metrics?.[key]).filter(value => value != null && Number.isFinite(Number(value)))
  return values.length === resourceClusters.value.length && values.length > 0 ? values.reduce((sum, value) => sum + Number(value), 0) : null
}
function aggregateMetric(key) { return metricTotal(key) ?? '—' }
function aggregateReadyMetric(prefix) {
  const ready = metricTotal(`${prefix}_ready`)
  const total = metricTotal(`${prefix}_total`)
  return ready == null || total == null ? '—' : `${ready}/${total}`
}
function readyTone(prefix) {
  const ready = metricTotal(`${prefix}_ready`)
  const total = metricTotal(`${prefix}_total`)
  return ready == null || total == null ? '' : ready === total ? 'metric-success' : 'metric-warn'
}
function aggregateMemory() {
  const total = metricTotal('memory_mb_total')
  return total == null ? '—' : total >= 1024 ? `${(total / 1024).toFixed(1)} GiB` : `${total} MiB`
}
function applicationMetric(key) { return applicationSummary.value?.[key] ?? '—' }
function dashboardSectionError(key) { return Boolean(dashboard.data.value?.errors?.[key]) }
function selectTrendMetric(id) { selectedTrendMetricId.value = id; selectedTrendNode.value = '' }
function matrixToSeries(result) {
  return (result?.result || []).map((item, index) => ({ label: item.metric?.node || item.metric?.instance || `序列 ${index + 1}`, values: (item.values || []).map(([timestamp, value]) => ({ timestamp: Number(timestamp), value: Number(value) })) }))
}
function aggregateSeries(series, label) {
  const points = new Map()
  for (const item of series) for (const point of item.values || []) {
    const current = points.get(point.timestamp) || { sum: 0, count: 0 }
    current.sum += Number(point.value)
    current.count++
    points.set(point.timestamp, current)
  }
  return [{ label, values: [...points.entries()].sort(([left], [right]) => left - right).map(([timestamp, point]) => ({ timestamp, value: point.sum / point.count })) }]
}
function actionBadge(action) { return ({ create: 'badge-online', issue: 'badge-online', renew: 'badge-online', delete: 'badge-danger', revoke: 'badge-danger', deploy: 'badge-deploying' })[action] || '' }
function actionLabel(action) { return ({ create: '创建', update: '更新', delete: '删除', deploy: '部署', issue: '签发', renew: '续期', revoke: '吊销' })[action] || action }
function resourceLabel(type) { return ({ server: '服务器', site: '站点', cert: '证书', nginx: 'NGINX' })[type] || type }
</script>

<style scoped>
.dashboard-page { display: grid; gap: 18px; min-width: 0; }
.dashboard-header { align-items: center; }
.dashboard-header-actions { display: flex; align-items: center; gap: 8px; }
.dashboard-resources { min-width: 0; }
.dashboard-coverage { color: var(--text-muted); font-size: 11px; white-space: nowrap; }
.dashboard-coverage.is-partial { color: var(--warning); }
.dashboard-resource-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 22px 16px; padding: 8px 0 4px; }
.dashboard-resource-metric { display: grid; min-width: 0; gap: 5px; }
.dashboard-resource-metric strong { overflow: hidden; color: var(--text-primary); font: 700 21px/1.2 var(--font-mono); text-overflow: ellipsis; white-space: nowrap; }
.dashboard-resource-metric strong.metric-success { color: var(--success); }
.dashboard-resource-metric strong.metric-warn { color: var(--warning); }
.dashboard-resource-metric span { color: var(--text-muted); font-size: 11px; }
.dashboard-resource-metric.dashboard-capacity strong { overflow: visible; font-size: 17px; text-overflow: clip; white-space: normal; }
.panel-link { display: inline-flex; align-items: center; gap: 3px; color: var(--action-primary); font-size: 11px; text-decoration: none; white-space: nowrap; }
.panel-link:hover { text-decoration: underline; }
.dashboard-lower-grid { display: grid; grid-template-columns: minmax(0, 1.6fr) minmax(220px, .8fr) minmax(220px, .8fr); gap: 16px; align-items: stretch; }
.dashboard-trends-panel { display: flex; min-width: 0; min-height: 290px; flex-direction: column; }
.dashboard-trend-title { display: flex; min-width: 0; align-items: center; gap: 12px; }
.dashboard-trend-title h2 { margin: 0; white-space: nowrap; }
.dashboard-cluster-select { width: clamp(130px, 13vw, 190px); min-height: 36px; }
.dashboard-trend-empty { display: flex; min-height: 290px; flex: 1; flex-direction: column; }
.dashboard-trend-empty :deep(.empty-state) { min-height: 180px; flex: 1; }
:deep(.dashboard-trend-chart) { display: flex; flex: 1; min-height: 290px; flex-direction: column; }
:deep(.dashboard-trend-chart .metric-trend-canvas), :deep(.dashboard-trend-chart .metric-trend-empty) { height: auto; min-height: 160px; flex: 1; }
.dashboard-trend-toolbar { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
.dashboard-segmented { display: inline-flex; padding: 2px; border: 1px solid var(--border-muted); border-radius: var(--radius-control); background: var(--surface-subtle); }
.dashboard-segmented button { min-height: 26px; padding: 3px 9px; border: 0; border-radius: 5px; background: transparent; color: var(--text-secondary); cursor: pointer; font-size: 11px; }
.dashboard-segmented button.active { background: var(--surface-raised); box-shadow: var(--shadow-soft); color: var(--action-primary); font-weight: 700; }
.dashboard-node-select { width: auto; min-width: 120px; min-height: 30px; font-size: 11px; }
.dashboard-application-panel, .dashboard-activity-panel { min-width: 0; }
.application-summary-count { display: flex; align-items: baseline; gap: 6px; }
.application-summary-count strong { color: var(--text-primary); font: 700 28px/1 var(--font-mono); }
.application-summary-count span, .application-status-grid span, .application-latest-release > span { color: var(--text-muted); font-size: 11px; }
.application-status-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 15px 8px; margin: 20px 0; }
.application-status-grid div { display: grid; gap: 4px; }
.application-status-grid strong { color: var(--text-primary); font: 700 19px/1 var(--font-mono); }
.application-status-grid .metric-success { color: var(--success); }
.application-status-grid .metric-warn { color: var(--warning); }
.application-latest-release { display: grid; gap: 4px; padding-top: 12px; border-top: 1px solid var(--border-muted); }
.application-latest-release strong { overflow: hidden; color: var(--text-primary); font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
.dashboard-application-unavailable { color: var(--text-muted); font-size: 12px; }
.activity-list { display: grid; }
.activity-row { display: flex; min-width: 0; align-items: center; gap: 8px; min-height: 42px; border-bottom: 1px solid var(--border-muted); color: var(--text-secondary); font-size: 11px; }
.activity-row:last-child { border-bottom: 0; }
.activity-action, .activity-resource { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.activity-action { max-width: 58%; flex: 0 1 auto; }
.activity-resource { flex: 1; }
@media (max-width: 1180px) { .dashboard-lower-grid { grid-template-columns: minmax(0, 1.5fr) minmax(220px, 1fr); } .dashboard-activity-panel { grid-column: 1 / -1; } }
@media (max-width: 760px) { .dashboard-lower-grid { grid-template-columns: 1fr; } .dashboard-activity-panel { grid-column: auto; } .dashboard-resource-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } .dashboard-cluster-select { width: 150px; } }
@media (max-width: 480px) { .dashboard-header { align-items: flex-start; flex-wrap: wrap; gap: 12px; } .dashboard-header-actions { margin-left: auto; } .dashboard-coverage { white-space: normal; text-align: right; } .dashboard-resource-metric.dashboard-capacity strong { font-size: 15px; } .dashboard-trend-title { gap: 8px; } .dashboard-cluster-select { width: 120px; } }
</style>
