<template>
  <div class="dashboard-page">
    <div class="page-header dashboard-header">
      <div class="dashboard-title-block">
        <h1 class="page-title">平台健康度</h1>
      </div>
      <router-link class="btn btn-primary dashboard-primary-action" to="/applications">部署服务</router-link>
    </div>

    <section class="dashboard-main-grid">
      <article class="card dashboard-health-panel" :class="{ 'is-loading': k8sLoading }" :aria-busy="k8sLoading">
        <div class="card-header"><h2 class="card-title">集群概况</h2><div class="dashboard-health-header"><span :class="`cluster-status cluster-status-${k8sStatusTone}`">{{ k8sStatusLabel }}</span><router-link class="panel-link" to="/servers">查看集群</router-link></div></div>
        <section class="dashboard-overview-section" aria-label="集群资源">
          <div class="dashboard-overview-grid">
            <div class="dashboard-overview-metric"><strong>{{ k8sMetric('nodes_total') }}</strong><span>节点</span></div>
            <div class="dashboard-overview-metric"><strong class="metric-success">{{ k8sReadyMetric('pods_ready', 'pods_total') }}</strong><span>Pods 就绪</span></div>
            <div class="dashboard-overview-metric"><strong>{{ k8sReadyMetric('deployments_ready', 'deployments_total') }}</strong><span>Deployments 就绪</span></div>
            <div class="dashboard-overview-metric"><strong>{{ k8sMetric('services_total') }}</strong><span>服务</span></div>
            <div class="dashboard-overview-metric"><strong>{{ k8sMetric('namespaces') }}</strong><span>命名空间</span></div>
            <div class="dashboard-overview-metric dashboard-version"><strong :title="k8sMetric('version')">{{ k8sMetric('version') }}</strong><span>Kubernetes 版本</span></div>
          </div>
          <div class="dashboard-cluster-health">
            <div class="dashboard-cluster-health-heading"><span>节点健康</span><strong>{{ k8sReadyMetric('nodes_ready', 'nodes_total') }}</strong></div>
            <div class="dashboard-cluster-health-status" role="list" aria-label="节点健康状态">
              <div v-for="segment in nodeHealthSegments" :key="segment.key" class="dashboard-node-health-item" :class="`dashboard-node-health-${segment.tone}`" role="listitem">
                <span class="dashboard-node-health-dot" aria-hidden="true"></span>
                <span>{{ segment.label }}</span>
                <strong>{{ segment.count }}</strong>
              </div>
            </div>
            <div class="dashboard-cluster-health-meta"><span>{{ k8sMetric('control_plane_nodes') }} 控制面</span><span>{{ k8sMetric('worker_nodes') }} 工作节点</span><span>{{ k8sMetric('cpu_cores_total') }} vCPU · {{ k8sMemoryMetric() }}</span></div>
          </div>
        </section>
      </article>

      <article class="card dashboard-application-panel" :class="{ 'is-loading': dashboardLoading && !applicationSummary }" :aria-busy="dashboardLoading">
        <div class="card-header"><h2 class="card-title">应用情况</h2><router-link class="panel-link" to="/applications">查看应用</router-link></div>
        <div class="dashboard-application-content dashboard-card-scroll-region" tabindex="0">
          <div v-if="dashboardSectionError('applications')" class="dashboard-application-unavailable"><span class="empty-icon">!</span><span>应用状态暂不可用</span></div>
          <template v-else>
            <div class="application-summary-count"><strong>{{ applicationMetric('total_applications') }}</strong><span>个应用</span></div>
            <div class="application-status-grid">
              <div><strong class="metric-success">{{ applicationMetric('successful_applications') }}</strong><span>运行正常</span></div>
              <div><strong class="metric-accent">{{ applicationMetric('releasing_applications') }}</strong><span>发布中</span></div>
              <div><strong :class="{ 'metric-warn': Number(applicationSummary?.failed_applications || 0) > 0 }">{{ applicationMetric('failed_applications') }}</strong><span>发布失败</span></div>
              <div><strong>{{ applicationMetric('unreleased_applications') }}</strong><span>未发布</span></div>
            </div>
            <div class="application-latest-release">
              <span>最近发布</span>
              <template v-if="applicationSummary?.latest_release"><strong>{{ applicationSummary.latest_release.application_name }}</strong><span>{{ releaseStatusLabel(applicationSummary.latest_release.status) }}<template v-if="applicationSummary.latest_release.version"> · {{ applicationSummary.latest_release.version }}</template></span></template>
              <span v-else>暂无发布记录</span>
            </div>
          </template>
        </div>
      </article>

      <aside class="card dashboard-side-panel">
        <div class="card-header"><h2 class="card-title">快捷操作</h2><button class="icon-button dashboard-action-settings" type="button" title="编辑快捷操作" aria-label="编辑快捷操作" :aria-expanded="quickActionsEditing" @click="quickActionsEditing = true"><Settings2 :size="15" /></button></div>
        <nav class="dashboard-action-list dashboard-card-scroll-region" aria-label="快捷操作" tabindex="0">
          <router-link v-for="action in visibleQuickActions" :key="action.id" class="dashboard-action-card" :class="{ 'dashboard-action-card-primary': action.id === 'deploy' }" :to="action.to"><span class="dashboard-action-icon">{{ action.icon }}</span><strong>{{ action.label }}</strong><span class="action-arrow">→</span></router-link>
        </nav>
      </aside>

      <BaseModal :open="quickActionsEditing" title="配置快捷操作" size="small" @close="quickActionsEditing = false">
        <div class="quick-actions-editor">
          <div class="quick-actions-editor-header"><strong>显示入口</strong></div>
          <label v-for="action in quickActions" :key="action.id" class="quick-action-option"><input type="checkbox" :checked="quickActionIds.includes(action.id)" :disabled="quickActionIds.length === 1 && quickActionIds.includes(action.id)" @change="toggleQuickAction(action.id)" /><span>{{ action.label }}</span></label>
        </div>
        <template #actions><button class="btn btn-primary" type="button" @click="quickActionsEditing = false">完成</button></template>
      </BaseModal>
    </section>

    <section class="dashboard-insights-grid">
      <section class="dashboard-trends-panel">
        <div v-if="trendError" class="card dashboard-trends-empty"><div class="card-header"><h2 class="card-title">资源趋势</h2><router-link class="panel-link" to="/monitoring">查看监控</router-link></div><div><span class="empty-icon">!</span><span class="empty-text">监控趋势暂不可用</span></div></div>
        <MetricTrendChart v-else class="dashboard-trend-chart" title="资源趋势" :unit="selectedTrendMetric.unit" :loading="trendLoading" :series="selectedTrendSeries">
          <template #actions><router-link class="panel-link" to="/monitoring">查看监控</router-link></template>
          <template #toolbar>
            <div class="dashboard-trend-toolbar">
              <div class="dashboard-segmented" role="group" aria-label="资源类型"><button v-for="metric in trendMetrics" :key="metric.id" type="button" :class="{ active: selectedTrendMetricId === metric.id }" @click="selectTrendMetric(metric.id)">{{ metric.label }}</button></div>
              <div class="dashboard-segmented" role="group" aria-label="展示方式"><button type="button" :class="{ active: trendDisplayMode === 'average' }" @click="trendDisplayMode = 'average'">集群平均</button><button type="button" :class="{ active: trendDisplayMode === 'nodes' }" @click="trendDisplayMode = 'nodes'">按节点</button></div>
              <select v-if="trendDisplayMode === 'nodes' && trendNodeOptions.length" v-model="selectedTrendNode" class="form-select dashboard-node-select" aria-label="选择节点"><option value="">全部节点</option><option v-for="node in trendNodeOptions" :key="node" :value="node">{{ node }}</option></select>
            </div>
          </template>
        </MetricTrendChart>
      </section>

      <aside class="card dashboard-activity-panel">
        <div class="card-header"><h2 class="card-title">运行检查</h2><router-link class="panel-link" to="/monitoring?tab=alerts">查看告警</router-link></div>
        <div class="attention-list"><router-link v-for="item in runtimeChecks" :key="item.key" class="attention-row" :to="item.to"><span class="attention-dot" :class="item.level"></span><strong class="attention-title">{{ item.title }}</strong><span class="attention-value">{{ item.value }}</span></router-link></div>
        <div class="activity-heading"><strong>最近操作</strong><router-link class="panel-link" to="/audit">查看全部</router-link></div>
        <div v-if="dashboardError || dashboardSectionError('recent_logs')" class="empty-state dashboard-empty-state"><span class="empty-icon">!</span><span class="empty-text">操作记录暂不可用</span></div><div v-else-if="dashboardLoading" class="empty-state dashboard-empty-state"><span class="empty-text">正在读取...</span></div><div v-else-if="recentLogs.length === 0" class="empty-state dashboard-empty-state"><span class="empty-icon">⊙</span><span class="empty-text">暂无操作记录</span></div><div v-else class="activity-list"><div v-for="log in recentLogs.slice(0, 2)" :key="log.id" class="activity-row"><span class="badge" :class="actionBadge(log.action)">{{ actionLabel(log.action) }}</span><span>{{ resourceLabel(log.resource_type) }} #{{ log.resource_id }}</span></div></div>
      </aside>
    </section>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { Settings2 } from 'lucide-vue-next'
import { getAlertOverview, getDashboardOverview, getKubernetesDashboard } from '../api/dashboard.js'
import { getMonitoringDashboard } from '../api/monitoring.js'
import BaseModal from '../components/BaseModal.vue'
import { useAsyncResource } from '../composables/useAsyncResource.js'
import MetricTrendChart from './monitoring/MetricTrendChart.vue'

const dashboard = useAsyncResource(getDashboardOverview)
const kubernetesDashboard = useAsyncResource(getKubernetesDashboard)
const alerting = useAsyncResource(getAlertOverview)
const monitoringDashboard = useAsyncResource(({ signal }) => getMonitoringDashboard('6h', { signal }))

const quickActions = [
  { id: 'deploy', label: '部署 / 更新服务', to: '/applications', icon: '⧉' },
  { id: 'workloads', label: '查看工作负载', to: '/resources?tab=workloads', icon: '⌁' },
  { id: 'monitoring', label: '查看监控', to: '/monitoring', icon: '◔' },
  { id: 'audit', label: '查看日志', to: '/audit', icon: '▦' },
]
const defaultQuickActionIds = quickActions.map(action => action.id)
const quickActionIds = ref([...defaultQuickActionIds])
const quickActionsEditing = ref(false)

const stats = computed(() => dashboard.data.value?.stats || {})
const applicationSummary = computed(() => dashboard.data.value?.application_summary || null)
const recentLogs = computed(() => dashboard.data.value?.recent_logs || [])
const dashboardLoading = dashboard.loading
const dashboardErrors = computed(() => dashboard.data.value?.errors || {})
const dashboardError = computed(() => dashboard.error.value?.message || '')
const k8sStats = kubernetesDashboard.data
const k8sLoading = kubernetesDashboard.loading
const k8sError = computed(() => kubernetesDashboard.error.value?.message || '')
const k8sErrors = computed(() => k8sStats.value?.errors || {})
const k8sStatusLabel = computed(() => {
  if (k8sLoading.value || !k8sStats.value) return '读取中'
  if (k8sError.value) return '不可用'
  if (k8sStats.value?.partial) return '部分可用'
  if (k8sStats.value && k8sStats.value.version_consistent === false) return '多版本'
  return '运行正常'
})
const k8sStatusTone = computed(() => {
  if (k8sLoading.value || !k8sStats.value) return 'loading'
  return k8sError.value || k8sStats.value.partial || k8sStats.value.version_consistent === false ? 'warn' : 'ok'
})
const nodeHealthSegments = computed(() => {
  if (k8sLoading.value || !k8sStats.value || k8sError.value) {
    return [{ key: 'ready', label: 'Ready', count: '—', tone: 'muted' }]
  }
  const ready = Math.max(0, Number(k8sStats.value.nodes_ready || 0))
  const total = Math.max(0, Number(k8sStats.value.nodes_total || 0))
  const notReady = Math.max(0, total - ready)
  const segments = [{ key: 'ready', label: 'Ready', count: ready, tone: 'success' }]
  if (notReady > 0) segments.push({ key: 'not-ready', label: '异常', count: notReady, tone: 'warning' })
  return segments
})
const alertOverview = alerting.data
const alertingLoading = alerting.loading
const alertingError = computed(() => alerting.error.value?.message || '')

const firingAlerts = computed(() => Number(alertOverview.value?.firing || 0))
const alertMetricValue = computed(() => alertOverview.value ? firingAlerts.value : '-')
const notReadyDeployments = computed(() => {
  if (k8sLoading.value || !k8sStats.value || k8sSectionError('deployments')) return null
  return Math.max(0, Number(k8sStats.value.deployments_total || 0) - Number(k8sStats.value.deployments_ready || 0))
})
const notReadyPods = computed(() => {
  if (k8sLoading.value || !k8sStats.value || k8sSectionError('pods')) return null
  return Math.max(0, Number(k8sStats.value.pods_total || 0) - Number(k8sStats.value.pods_ready || 0))
})
const trendLoading = monitoringDashboard.loading
const trendError = computed(() => monitoringDashboard.error.value?.message || '')
const trendMetrics = [
  { id: 'cpu', label: 'CPU', unit: '%' },
  { id: 'memory', label: '内存', unit: '%' },
  { id: 'disk', label: '磁盘', unit: '%' },
]
const selectedTrendMetricId = ref('cpu')
const trendDisplayMode = ref('nodes')
const selectedTrendNode = ref('')
const rawTrendSeries = computed(() => {
  const trends = monitoringDashboard.data.value?.trends || {}
  return Object.fromEntries(trendMetrics.map(metric => [metric.id, matrixToSeries(trends[metric.id])]))
})
const selectedTrendMetric = computed(() => trendMetrics.find(metric => metric.id === selectedTrendMetricId.value) || trendMetrics[0])
const trendNodeOptions = computed(() => [...new Set((rawTrendSeries.value[selectedTrendMetricId.value] || []).map(series => series.label).filter(Boolean))])
const selectedTrendSeries = computed(() => {
  const series = rawTrendSeries.value[selectedTrendMetricId.value] || []
  if (trendDisplayMode.value === 'nodes') return selectedTrendNode.value ? series.filter(item => item.label === selectedTrendNode.value) : series
  return aggregateSeries(series, `集群 ${selectedTrendMetric.value.label} 平均`)
})
const runtimeChecks = computed(() => [
  {
    key: 'deployments',
    level: readinessCheckLevel('deployments', notReadyDeployments.value),
    title: 'Deployment 就绪',
    value: k8sReadyMetric('deployments_ready', 'deployments_total'),
    to: '/resources?tab=workloads',
  },
  {
    key: 'pods',
    level: readinessCheckLevel('pods', notReadyPods.value),
    title: 'Pod 就绪',
    value: k8sReadyMetric('pods_ready', 'pods_total'),
    to: '/resources?tab=workloads',
  },
  {
    key: 'alerts',
    level: alertingLoading.value || alertingError.value ? 'is-muted' : firingAlerts.value > 0 ? 'is-danger' : 'is-success',
    title: '告警',
    value: alertingLoading.value || alertingError.value ? '—' : firingAlerts.value > 0 ? firingAlerts.value : '正常',
    to: '/monitoring?tab=alerts',
  },
  {
    key: 'certificates',
    level: certificateCheckLevel(),
    title: '证书',
    value: certificateCheckValue(),
    to: '/network?tab=certificates',
  },
])

onMounted(() => {
  loadQuickActions()
  void dashboard.refresh()
  void kubernetesDashboard.refresh()
  void alerting.refresh()
  void monitoringDashboard.refresh()
})

function k8sMetric(key) {
  if (k8sLoading.value || k8sStats.value?.[key] === undefined || k8sStats.value?.[key] === null) return '—'
  return k8sStats.value[key]
}

function k8sReadyMetric(readyKey, totalKey) {
  if (k8sLoading.value || k8sStats.value?.[readyKey] === undefined || k8sStats.value?.[totalKey] === undefined) return '—'
  return `${k8sStats.value[readyKey]}/${k8sStats.value[totalKey]}`
}
function k8sMemoryMetric() {
  if (k8sLoading.value || k8sStats.value?.memory_mb_total === undefined || k8sStats.value?.memory_mb_total === null) return '—'
  const memoryMB = Number(k8sStats.value.memory_mb_total)
  if (!Number.isFinite(memoryMB)) return '—'
  return memoryMB >= 1024 ? `${(memoryMB / 1024).toFixed(1)} GiB` : `${memoryMB} MiB`
}
function readinessCheckLevel(section, notReady) {
  if (k8sLoading.value || k8sSectionError(section) || notReady === null) return 'is-muted'
  return notReady > 0 ? 'is-warning' : 'is-success'
}
function certificateCheckLevel() {
  if (dashboardLoading.value || dashboardError.value || dashboardSectionError('expiring_certs')) return 'is-muted'
  return Number(stats.value.expiring_certs || 0) > 0 ? 'is-warning' : 'is-success'
}
function certificateCheckValue() {
  if (dashboardLoading.value || dashboardError.value || dashboardSectionError('expiring_certs')) return '—'
  const expiring = Number(stats.value.expiring_certs || 0)
  return expiring > 0 ? `${expiring} 风险` : '正常'
}
function applicationMetric(key) {
  if (dashboardLoading.value || !applicationSummary.value || applicationSummary.value[key] === undefined) return '—'
  return applicationSummary.value[key]
}
function releaseStatusLabel(status) {
  const labels = {
    draft: '草稿', validating: '校验中', applying: '发布中', waiting_ready: '等待就绪', verifying: '验证中',
    succeeded: '已完成', failed: '失败', rolling_back: '回滚中', rolled_back: '已回滚',
  }
  return labels[status] || status
}
const visibleQuickActions = computed(() => quickActionIds.value.map(id => quickActions.find(action => action.id === id)).filter(Boolean))
function loadQuickActions() {
  try {
    const saved = JSON.parse(window.localStorage.getItem('cylism.dashboard.quick-actions') || 'null')
    const valid = Array.isArray(saved) ? saved.filter(id => defaultQuickActionIds.includes(id)) : []
    if (valid.length) quickActionIds.value = [...new Set(valid)]
  } catch { /* Ignore unavailable or malformed browser storage. */ }
}
function toggleQuickAction(id) {
  const next = quickActionIds.value.includes(id) ? quickActionIds.value.filter(item => item !== id) : [...quickActionIds.value, id]
  if (!next.length) return
  quickActionIds.value = next
  try { window.localStorage.setItem('cylism.dashboard.quick-actions', JSON.stringify(next)) } catch { /* Ignore unavailable browser storage. */ }
}
function selectTrendMetric(id) {
  selectedTrendMetricId.value = id
  selectedTrendNode.value = ''
}
function dashboardMetric(key) { return dashboard.data.value !== null && stats.value[key] !== undefined ? stats.value[key] : '—' }
function dashboardSectionError(key) { return Boolean(dashboardErrors.value[key]) }
function k8sSectionError(key) { return Boolean(k8sErrors.value[key]) || Boolean(k8sError.value) }

function matrixToSeries(result) {
  return (result?.result || []).map((item, index) => ({ label: metricNodeName(item.metric, index), values: (item.values || []).map(([timestamp, value]) => ({ timestamp: Number(timestamp), value: Number(value) })) }))
}

function metricNodeName(metric, index) {
  if (metric?.node) return metric.node
  return metric?.instance || `序列 ${index + 1}`
}

function aggregateSeries(series, label) {
  const points = new Map()
  for (const item of series) {
    for (const point of item.values || []) {
      const current = points.get(point.timestamp) || { sum: 0, count: 0 }
      current.sum += Number(point.value)
      current.count += 1
      points.set(point.timestamp, current)
    }
  }
  return [{ label, values: [...points.entries()].sort(([left], [right]) => left - right).map(([timestamp, point]) => ({ timestamp, value: point.sum / point.count })) }]
}

function actionBadge(a) { const m = { create:'badge-online',issue:'badge-online',renew:'badge-online',delete:'badge-danger',revoke:'badge-danger',deploy:'badge-deploying' }; return m[a]||'' }
function actionLabel(a) { const m = { create:'创建',update:'更新',delete:'删除',deploy:'部署',issue:'签发',renew:'续期',revoke:'吊销',reload:'重载',generate:'生成' }; return m[a]||a }
function resourceLabel(r) { const m = { server:'服务器',site:'站点',cert:'证书',nginx:'NGINX' }; return m[r]||r }
</script>

<style scoped>
.dashboard-page { width: 100%; max-width: none; display: grid; gap: 18px; }
.dashboard-header { align-items: center; margin-bottom: 0; }
.dashboard-title-block { display: grid; gap: 5px; min-width: 0; }
.dashboard-primary-action { flex: 0 0 auto; min-height: 36px; padding: 0 14px; text-decoration: none; }
.dashboard-primary-action:hover, .dashboard-primary-action:focus-visible { text-decoration: none; }

.dashboard-main-grid { display: grid; grid-template-columns: minmax(0, 1.4fr) minmax(280px, 1fr) minmax(240px, .85fr); gap: 16px; align-items: stretch; }
.dashboard-health-panel, .dashboard-application-panel, .dashboard-side-panel { min-width: 0; min-height: 0; overflow: hidden; }
.dashboard-health-panel { display: flex; flex-direction: column; }
.dashboard-health-panel.is-loading { opacity: .86; }
.cluster-status { font-size: 10px; font-weight: 600; }
.cluster-status-ok { color: var(--success); }
.cluster-status-warn { color: var(--warning); }
.cluster-status-loading { color: var(--text-muted); }
.dashboard-health-header { display: flex; align-items: center; gap: 12px; }
.dashboard-card-scroll-region { min-height: 0; overflow: auto; overscroll-behavior: contain; scrollbar-color: var(--border-strong) transparent; scrollbar-width: thin; }
.dashboard-card-scroll-region::-webkit-scrollbar { width: 6px; height: 6px; }
.dashboard-card-scroll-region::-webkit-scrollbar-thumb { border-radius: 999px; background: var(--border-strong); }
.dashboard-overview-section { display: grid; flex: 1; align-content: center; gap: 8px; }
.dashboard-platform-section { margin-top: 22px; }
.dashboard-overview-grid, .dashboard-platform-grid { display: grid; gap: 11px 24px; }
.dashboard-overview-grid { grid-template-columns: repeat(3, minmax(0, 1fr)); }
.dashboard-platform-grid { grid-template-columns: repeat(4, minmax(0, 1fr)); }
.dashboard-overview-metric { display: grid; min-width: 0; gap: 5px; padding: 0; }
.dashboard-overview-metric strong { overflow: hidden; color: var(--text-primary); font: 700 18px/1 var(--font-mono); font-variant-numeric: tabular-nums; text-overflow: ellipsis; white-space: nowrap; }
.dashboard-overview-metric span { overflow: hidden; color: var(--text-muted); font-size: 10px; text-overflow: ellipsis; white-space: nowrap; }
.dashboard-version strong { font-size: 14px; }
.dashboard-cluster-health { display: flex; align-items: center; flex-wrap: wrap; gap: 7px 13px; margin-top: 8px; padding-top: 10px; border-top: 1px solid var(--border-muted); }
.dashboard-cluster-health-heading, .dashboard-cluster-health-meta { display: flex; align-items: center; gap: 6px; color: var(--text-muted); font-size: 10px; }
.dashboard-cluster-health-heading strong { color: var(--text-primary); font: 700 12px/1 var(--font-mono); }
.dashboard-cluster-health-status { display: flex; flex-wrap: wrap; gap: 7px; }
.dashboard-node-health-item { display: inline-flex; align-items: center; gap: 5px; color: var(--text-secondary); font-size: 10px; }
.dashboard-node-health-item strong { color: var(--text-primary); font: 700 11px/1 var(--font-mono); }
.dashboard-node-health-dot { width: 7px; height: 7px; flex: 0 0 auto; border-radius: 50%; background: var(--text-muted); }
.dashboard-node-health-success .dashboard-node-health-dot { background: var(--success); }
.dashboard-node-health-warning .dashboard-node-health-dot { background: var(--warning); }
.dashboard-cluster-health-meta { flex-wrap: wrap; gap: 0; }
.dashboard-cluster-health-meta span + span::before { content: '·'; margin: 0 6px; color: var(--text-muted); }
.dashboard-application-panel { display: flex; min-width: 0; flex-direction: column; padding: 16px; }
.dashboard-application-panel .card-header { margin-bottom: 12px; }
.dashboard-application-content { display: flex; min-height: 0; flex: 1; flex-direction: column; padding-right: 4px; }
.application-summary-count { display: flex; align-items: baseline; gap: 6px; }
.application-summary-count strong { color: var(--text-primary); font: 700 30px/1 var(--font-mono); }
.application-summary-count span, .application-status-grid span, .application-latest-release > span:first-child { color: var(--text-muted); font-size: 10px; }
.application-status-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 10px; margin-top: 20px; }
.application-status-grid div { display: grid; min-width: 0; gap: 5px; }
.application-status-grid strong { color: var(--text-primary); font: 700 18px/1 var(--font-mono); }
.application-latest-release { display: grid; gap: 5px; margin-top: auto; padding-top: 15px; }
.application-latest-release strong, .application-latest-release span:not(:first-child) { overflow: hidden; color: var(--text-primary); font-size: 11px; text-overflow: ellipsis; white-space: nowrap; }
.application-latest-release span:not(:first-child) { color: var(--text-secondary); font-size: 10px; }
.dashboard-application-unavailable { display: flex; min-height: 100px; align-items: center; gap: 8px; color: var(--text-muted); font-size: 11px; }

.dashboard-side-panel { display: flex; flex-direction: column; }
.dashboard-action-list { display: grid; min-height: 0; flex: 1; gap: 2px; padding-right: 4px; }
.dashboard-action-card { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; gap: 10px; align-items: center; min-height: 44px; padding: 7px 4px; border-bottom: 1px solid var(--border-muted); color: var(--text-primary); text-decoration: none; }
.dashboard-action-card:last-child { border-bottom: 0; }
.dashboard-action-card:hover { color: var(--action-primary); }
.dashboard-action-card-primary { color: var(--action-primary); }
.dashboard-action-settings { color: var(--text-muted); }
.dashboard-action-icon { display: grid; place-items: center; width: 28px; height: 28px; border-radius: 7px; background: var(--surface-hover); font-size: 14px; }
.dashboard-action-card strong { font-size: 12px; }
.action-arrow { color: var(--text-muted); font-size: 14px; }
.quick-actions-editor { display: grid; gap: 7px; }
.quick-actions-editor-header { display: flex; align-items: center; justify-content: space-between; padding-bottom: 7px; border-bottom: 1px solid var(--border-muted); color: var(--text-secondary); font-size: 11px; }
.quick-action-option { display: flex; min-height: 34px; align-items: center; gap: 8px; padding: 0 2px; color: var(--text-secondary); font-size: 12px; }
.quick-action-option input { accent-color: var(--action-primary); }

.dashboard-insights-grid { display: grid; grid-template-columns: minmax(0, 1.7fr) minmax(250px, .8fr); gap: 16px; align-items: stretch; }
.dashboard-insights-grid { min-height: 0; }
.dashboard-trends-panel { display: flex; min-width: 0; min-height: 0; }
.dashboard-trends-empty { display: flex; flex: 1; min-height: 220px; align-items: center; justify-content: center; gap: 8px; }
:deep(.dashboard-trend-chart) { display: flex; flex: 1; min-height: 0; padding: 14px; flex-direction: column; }
:deep(.dashboard-trend-chart .card-header) { margin-bottom: 8px; }
:deep(.dashboard-trend-chart .metric-trend-subtitle), :deep(.dashboard-trend-chart .metric-trend-threshold) { font-size: 9px; }
:deep(.dashboard-trend-chart .metric-trend-canvas), :deep(.dashboard-trend-chart .metric-trend-empty) { height: auto; min-height: 0; flex: 1; }
:deep(.dashboard-trend-chart .metric-trend-toolbar) { margin-top: 0; }
.dashboard-trend-toolbar { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
.dashboard-segmented { display: inline-flex; padding: 2px; border: 1px solid var(--border-muted); border-radius: var(--radius-control); background: var(--surface-subtle); }
.dashboard-segmented button { min-height: 26px; padding: 3px 9px; border: 0; border-radius: 5px; background: transparent; color: var(--text-secondary); cursor: pointer; font-size: 11px; }
.dashboard-segmented button.active { background: var(--surface-raised); box-shadow: var(--shadow-soft); color: var(--action-primary); font-weight: 700; }
.dashboard-node-select { min-height: 30px; width: auto; min-width: 120px; padding-top: 4px; padding-bottom: 4px; font-size: 11px; }

.dashboard-activity-panel { display: flex; min-width: 0; flex-direction: column; padding: 16px; }
.dashboard-activity-panel .card-header { margin-bottom: 10px; }
.panel-link { color: var(--action-primary); font-size: 11px; text-decoration: none; }
.panel-link:hover { text-decoration: underline; }
.attention-list { display: grid; grid-template-rows: repeat(4, minmax(32px, auto)); gap: 4px; }
.attention-row { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; gap: 8px; align-items: center; padding: 8px 0; color: var(--text-primary); text-decoration: none; }
.attention-row:hover { color: var(--action-primary); }
.attention-dot { width: 8px; height: 8px; border-radius: 50%; background: var(--text-muted); }
.attention-dot.is-danger { background: var(--danger); }.attention-dot.is-warning { background: var(--warning); }.attention-dot.is-success { background: var(--success); }.attention-dot.is-muted { background: var(--text-muted); }
.attention-title { min-width: 0; overflow: hidden; color: var(--text-primary); font-size: 11px; text-overflow: ellipsis; white-space: nowrap; }
.attention-value { color: var(--text-primary); font: 700 11px/1 var(--font-mono); }
.activity-heading { display: flex; align-items: center; justify-content: space-between; margin-top: 15px; padding-top: 12px; border-top: 1px solid var(--border-muted); color: var(--text-primary); font-size: 11px; }
.activity-list { display: grid; gap: 0; }
.activity-row { display: grid; grid-template-columns: auto minmax(0, 1fr); gap: 8px; align-items: center; padding: 9px 0; color: var(--text-secondary); font-size: 10px; }
.activity-row:last-child { border-bottom: 0; }
.dashboard-empty-state { min-height: 72px; }

/* Keep the desktop overview within the content viewport while preserving a natural
   scrolling layout for short windows and mobile screens. */
@media (min-width: 961px) and (min-height: 700px) {
  .dashboard-page { height: calc(100dvh - var(--topbar-height) - var(--shell-padding) - 36px); min-height: 0; grid-template-rows: auto minmax(220px, 250px) minmax(260px, 1fr); }
  .dashboard-main-grid, .dashboard-insights-grid { min-height: 0; }
  .dashboard-health-panel, .dashboard-application-panel, .dashboard-side-panel, .dashboard-activity-panel { height: 100%; }
  .dashboard-side-panel { overflow: hidden; }
  .dashboard-activity-panel { overflow: auto; }
}

@media (max-width: 960px) {
  .dashboard-main-grid, .dashboard-insights-grid { grid-template-columns: 1fr; }
}
@media (min-width: 961px) and (max-width: 1200px) {
  .dashboard-main-grid { grid-template-columns: minmax(0, 1.5fr) minmax(240px, .8fr); }
  .dashboard-health-panel { grid-column: 1 / -1; }
}
@media (max-width: 640px) {
  .dashboard-page { gap: 14px; }
  .dashboard-header { align-items: flex-start; }
  .dashboard-primary-action { align-self: flex-end; }
  .dashboard-overview-metric strong { font-size: 16px; }
  .dashboard-version strong { font-size: 12px; }
  .dashboard-platform-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  :deep(.dashboard-trend-chart .metric-trend-canvas), :deep(.dashboard-trend-chart .metric-trend-empty) { height: 170px; }
}
</style>
