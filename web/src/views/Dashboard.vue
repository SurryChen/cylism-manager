<template>
  <div class="dashboard-page">
    <div class="page-header">
      <div><h1 class="page-title">服务健康度</h1><p class="page-subtitle">集中查看受管服务、证书风险与最近变更。</p></div>
    </div>

    <section class="cluster-strip section-gap" :class="{ 'is-loading': k8sLoading }" :aria-busy="k8sLoading">
      <div class="cluster-strip-heading"><strong>集群运行概况</strong></div>
      <div class="cluster-strip-stats">
        <div><strong>{{ k8sReadyMetric('deployments_ready', 'deployments_total') }}</strong><span>Deployments 就绪</span></div>
        <div><strong>{{ k8sMetric('services_total') }}</strong><span>Services</span></div>
        <div><strong class="metric-accent">{{ k8sMetric('nodes_total') }}</strong><span>节点</span></div>
        <div><strong>{{ k8sMetric('namespaces') }}</strong><span>命名空间</span></div>
        <div><strong class="metric-success">{{ k8sReadyMetric('pods_ready', 'pods_total') }}</strong><span>Pods 就绪</span></div>
        <div><strong class="metric-version">{{ k8sMetric('version') }}</strong><span>K3s 版本</span></div>
      </div>
    </section>

    <div class="metric-grid dashboard-metrics">
      <div class="metric">
        <div class="metric-value">{{ stats.total_servers || 0 }}</div>
        <div class="metric-label">服务器</div>
      </div>
      <div class="metric">
        <div class="metric-value">{{ stats.total_sites || 0 }}</div>
        <div class="metric-label">站点</div>
      </div>
      <div class="metric">
        <div class="metric-value metric-warn">{{ stats.expiring_certs || 0 }}</div>
        <div class="metric-label">即将到期</div>
      </div>
      <div class="metric">
        <div class="metric-value" :class="{ 'metric-warn': firingAlerts > 0 }">{{ alertMetricValue }}</div>
        <div class="metric-label">触发告警</div>
      </div>
      <div class="metric">
        <div class="metric-value" :class="{ 'metric-warn': notReadyDeployments > 0 }">{{ notReadyDeployments }}</div>
        <div class="metric-label">未就绪应用</div>
      </div>
      <div class="metric">
        <div class="metric-value">{{ recentLogs.length }}</div>
        <div class="metric-label">最近操作</div>
      </div>
    </div>

    <section class="card dashboard-actions-panel">
      <div class="card-header">
        <h2 class="card-title">常用入口</h2>
        <span class="panel-caption">部署、更新、日志、监控</span>
      </div>
      <div class="dashboard-action-grid">
        <router-link class="dashboard-action-card" to="/applications">
          <span class="dashboard-action-icon">⧉</span>
          <div>
            <strong>部署 / 更新服务</strong>
            <small>进入应用工作台，创建发布或调整现有服务。</small>
          </div>
        </router-link>
        <router-link class="dashboard-action-card" to="/monitoring">
          <span class="dashboard-action-icon">◔</span>
          <div>
            <strong>查看监控</strong>
            <small>观察节点、工作负载和资源趋势。</small>
          </div>
        </router-link>
        <router-link class="dashboard-action-card" to="/audit">
          <span class="dashboard-action-icon">▦</span>
          <div>
            <strong>查看日志</strong>
            <small>检索审计记录、筛选操作和查看详情。</small>
          </div>
        </router-link>
        <router-link class="dashboard-action-card" to="/servers">
          <span class="dashboard-action-icon">⌁</span>
          <div>
            <strong>服务器与集群</strong>
            <small>查看节点、网络和基础设施状态。</small>
          </div>
        </router-link>
      </div>
    </section>

    <section class="dashboard-attention-grid">
      <article class="card dashboard-attention-panel">
        <div class="card-header">
          <h2 class="card-title">待关注事项</h2>
          <span class="panel-caption">按风险排序</span>
        </div>
        <div class="attention-list">
          <router-link v-for="item in attentionItems" :key="item.key" class="attention-row" :to="item.to">
            <span class="attention-dot" :class="item.level"></span>
            <span class="attention-copy">
              <strong>{{ item.title }}</strong>
              <small>{{ item.description }}</small>
            </span>
            <span class="attention-value">{{ item.value }}</span>
          </router-link>
        </div>
      </article>

      <article class="card dashboard-alert-panel">
        <div class="card-header">
          <h2 class="card-title">告警概览</h2>
          <router-link class="panel-link" to="/monitoring?tab=alerts">进入告警</router-link>
        </div>
        <div v-if="alertOverview" class="alert-summary-grid">
          <div><strong :class="{ 'metric-warn': firingAlerts > 0 }">{{ firingAlerts }}</strong><span>触发中</span></div>
          <div><strong>{{ activeAlerts.length }}</strong><span>活跃告警</span></div>
          <div><strong>{{ silencedAlerts }}</strong><span>已静默</span></div>
        </div>
        <div v-if="activeAlerts.length" class="alert-preview-list">
          <router-link v-for="alert in activeAlerts.slice(0, 3)" :key="alert.fingerprint || alertTitle(alert)" class="alert-preview-row" to="/monitoring?tab=alerts">
            <span class="badge" :class="alertSeverityBadge(alert)">{{ alertSeverityLabel(alert) }}</span>
            <span><strong>{{ alertTitle(alert) }}</strong><small>{{ alertMeta(alert) }}</small></span>
          </router-link>
        </div>
        <div v-else class="empty-state dashboard-alert-empty">
          <span class="empty-icon">{{ alertOverview ? '✓' : '◌' }}</span>
          <span class="empty-text">{{ alertOverview ? '当前没有触发中的告警' : (alertingError || '告警组件暂不可用') }}</span>
        </div>
      </article>
    </section>

    <div class="dashboard-workspace">
      <section class="card dashboard-primary-panel">
        <div class="card-header">
          <h2 class="card-title">即将到期的证书</h2>
          <span class="panel-caption">未来 30 天</span>
        </div>
        <div v-if="expiringCerts.length === 0" class="empty-state dashboard-empty-state">
          <span class="empty-icon">✓</span>
          <span class="empty-text">30 天内没有即将到期的证书</span>
        </div>
        <div v-else class="table-wrap">
          <table class="data-table">
            <thead><tr><th>域名</th><th>到期时间</th><th>状态</th></tr></thead>
            <tbody>
              <tr v-for="cert in expiringCerts" :key="cert.id">
                <td>{{ cert.domains }}</td>
                <td>{{ formatDate(cert.valid_to) }}</td>
                <td><span class="badge badge-warn">即将到期</span></td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <section class="card dashboard-activity-panel">
        <div class="card-header"><h2 class="card-title">最近操作</h2></div>
        <div v-if="recentLogs.length === 0" class="empty-state dashboard-empty-state">
          <span class="empty-icon">⊙</span><span class="empty-text">暂无操作记录</span>
        </div>
        <div v-else class="table-wrap">
          <table class="data-table">
            <thead><tr><th>操作</th><th>资源</th><th>时间</th></tr></thead>
            <tbody>
              <tr v-for="log in recentLogs" :key="log.id">
                <td><span class="badge" :class="actionBadge(log.action)">{{ actionLabel(log.action) }}</span></td>
                <td>{{ resourceLabel(log.resource_type) }} #{{ log.resource_id }}</td>
                <td>{{ formatTime(log.created_at) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted } from 'vue'
import { getAlertOverview, getDashboardOverview, getKubernetesDashboard } from '../api/dashboard.js'
import { useAsyncResource } from '../composables/useAsyncResource.js'

const dashboard = useAsyncResource(getDashboardOverview, {})
const kubernetesDashboard = useAsyncResource(getKubernetesDashboard)
const alerting = useAsyncResource(getAlertOverview)

const stats = computed(() => dashboard.data.value?.stats || {})
const expiringCerts = computed(() => dashboard.data.value?.expiring_certs || [])
const recentLogs = computed(() => dashboard.data.value?.recent_logs || [])
const k8sStats = kubernetesDashboard.data
const k8sLoading = kubernetesDashboard.loading
const alertOverview = alerting.data
const alertingError = computed(() => alerting.error.value?.message || '')

const activeAlerts = computed(() => alertOverview.value?.active || [])
const firingAlerts = computed(() => Number(alertOverview.value?.firing || 0))
const silencedAlerts = computed(() => Number(alertOverview.value?.silenced || 0))
const alertMetricValue = computed(() => alertOverview.value ? firingAlerts.value : '-')
const notReadyDeployments = computed(() => Math.max(0, Number(k8sStats.value?.deployments_total || 0) - Number(k8sStats.value?.deployments_ready || 0)))
const notReadyPods = computed(() => Math.max(0, Number(k8sStats.value?.pods_total || 0) - Number(k8sStats.value?.pods_ready || 0)))
const attentionItems = computed(() => {
  const items = []
  if (firingAlerts.value > 0) {
    items.push({ key: 'alerts', level: 'is-danger', title: '存在触发中的告警', description: '优先进入告警页确认影响范围与处理建议。', value: firingAlerts.value, to: '/monitoring?tab=alerts' })
  } else if (alertingError.value) {
    items.push({ key: 'alerting-unavailable', level: 'is-muted', title: '告警组件暂不可用', description: '无法读取 Alertmanager 状态，可进入监控页重新检测。', value: '—', to: '/monitoring?tab=alerts' })
  }
  if (Number(stats.value.expiring_certs || 0) > 0) {
    items.push({ key: 'certs', level: 'is-warning', title: '证书即将到期', description: '检查证书续期状态，避免入口访问中断。', value: stats.value.expiring_certs, to: '/network?tab=certificates' })
  }
  if (notReadyDeployments.value > 0) {
    items.push({ key: 'deployments', level: 'is-warning', title: '有应用未完全就绪', description: 'Deployment Ready 数低于期望值，建议查看工作负载。', value: notReadyDeployments.value, to: '/resources?tab=workloads' })
  }
  if (notReadyPods.value > 0) {
    items.push({ key: 'pods', level: 'is-warning', title: '有 Pod 未就绪', description: '可能存在拉取镜像、探针或调度问题。', value: notReadyPods.value, to: '/resources?tab=workloads' })
  }
  if (items.length === 0) {
    items.push({ key: 'healthy', level: 'is-success', title: '当前暂无高优先级事项', description: '服务、证书和集群概况没有明显风险。', value: 'OK', to: '/applications' })
  }
  return items.slice(0, 4)
})

onMounted(() => {
  void dashboard.refresh()
  void kubernetesDashboard.refresh()
  void alerting.refresh()
})

function k8sMetric(key) {
  if (k8sLoading.value || k8sStats.value?.[key] === undefined || k8sStats.value?.[key] === null) return '—'
  return k8sStats.value[key]
}

function k8sReadyMetric(readyKey, totalKey) {
  if (k8sLoading.value || k8sStats.value?.[readyKey] === undefined || k8sStats.value?.[totalKey] === undefined) return '—'
  return `${k8sStats.value[readyKey]}/${k8sStats.value[totalKey]}`
}

function formatDate(d) { if (!d) return '-'; return new Date(d).toLocaleDateString('zh-CN', { month:'short', day:'numeric', year:'numeric' }) }
function formatTime(d) { if (!d) return '-'; return new Date(d).toLocaleString('zh-CN', { month:'short', day:'numeric', hour:'2-digit', minute:'2-digit' }) }
function actionBadge(a) { const m = { create:'badge-online',issue:'badge-online',renew:'badge-online',delete:'badge-danger',revoke:'badge-danger',deploy:'badge-deploying' }; return m[a]||'' }
function actionLabel(a) { const m = { create:'创建',update:'更新',delete:'删除',deploy:'部署',issue:'签发',renew:'续期',revoke:'吊销',reload:'重载',generate:'生成' }; return m[a]||a }
function resourceLabel(r) { const m = { server:'服务器',site:'站点',cert:'证书',nginx:'NGINX' }; return m[r]||r }
function alertTitle(alert) { return alert?.annotations?.summary || alert?.labels?.alertname || '未命名告警' }
function alertMeta(alert) {
  const labels = alert?.labels || {}
  return [labels.node, labels.namespace, labels.pod, labels.mountpoint].filter(Boolean).join(' · ') || '等待告警标签'
}
function alertSeverityLabel(alert) { return alert?.labels?.severity === 'critical' ? '严重' : '告警' }
function alertSeverityBadge(alert) { return alert?.labels?.severity === 'critical' ? 'badge-danger' : 'badge-deploying' }
</script>

<style scoped>
.dashboard-page { max-width: 1320px; margin: 0 auto; }
.cluster-strip { display: flex; align-items: center; gap: 28px; min-height: 94px; padding: 18px 22px; border: 1px solid var(--border); border-radius: var(--radius-panel); background: var(--surface-glass); box-shadow: var(--shadow-soft); backdrop-filter: blur(22px) saturate(125%); }.cluster-strip.is-loading { opacity: .86; }
.cluster-strip-heading { display: flex; min-width: 142px; flex-direction: column; gap: 5px; }.cluster-strip-heading strong { color: var(--text-primary); font-size: 13px; }
.cluster-strip-stats { display: grid; width: 100%; grid-template-columns: repeat(6, minmax(0, 1fr)); gap: 14px; }.cluster-strip-stats > div { display: flex; min-width: 0; flex-direction: column; gap: 4px; padding-left: 16px; border-left: 1px solid var(--border-muted); }.cluster-strip-stats strong { overflow: hidden; color: var(--text-primary); font-size: 19px; font-variant-numeric: tabular-nums; text-overflow: ellipsis; white-space: nowrap; }.cluster-strip-stats span { color: var(--text-muted); font-size: 10px; }
.dashboard-metrics { grid-template-columns: repeat(6, minmax(0, 1fr)); }.dashboard-metrics .metric { min-height: 96px; padding: 16px 18px; }.dashboard-metrics .metric-value { font-size: 26px; }.dashboard-workspace, .dashboard-attention-grid { display: grid; grid-template-columns: minmax(0, 7fr) minmax(310px, 4fr); gap: var(--space-20); }.dashboard-attention-grid { margin-top: var(--space-20); }.dashboard-workspace { margin-top: var(--space-20); }.dashboard-primary-panel, .dashboard-activity-panel, .dashboard-attention-panel, .dashboard-alert-panel { min-width: 0; }.dashboard-workspace .card-header, .dashboard-attention-grid .card-header { align-items: flex-start; margin-bottom: 16px; }.panel-caption { padding: 5px 7px; border-radius: 5px; background: var(--warning-surface); color: var(--warning); font: 10px/1 var(--font-mono); }.panel-link { color: var(--action-primary); font-size: 12px; text-decoration: none; }.panel-link:hover { text-decoration: underline; }.dashboard-empty-state { min-height: 230px; }.metric-version { font-size: 15px !important; }
.dashboard-actions-panel { display: grid; gap: 14px; padding: 18px 20px; }
.dashboard-action-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; }
.dashboard-action-card { display: grid; grid-template-columns: auto minmax(0, 1fr); gap: 12px; align-items: center; padding: 14px 15px; border: 1px solid var(--border-muted); border-radius: var(--radius-control); background: linear-gradient(180deg, var(--surface-raised), var(--surface-subtle)); color: var(--text-primary); text-decoration: none; transition: transform .15s ease, border-color .15s ease, box-shadow .15s ease; }
.dashboard-action-card:hover { border-color: var(--border); box-shadow: var(--shadow-soft); transform: translateY(-1px); }
.dashboard-action-icon { display: grid; place-items: center; width: 38px; height: 38px; border-radius: 12px; background: var(--surface-hover); color: var(--action-primary); font-size: 17px; }
.dashboard-action-card strong { display: block; color: var(--text-primary); font-size: 13px; }
.dashboard-action-card small { display: block; margin-top: 4px; color: var(--text-secondary); font-size: 12px; line-height: 1.45; }
.attention-list { display: grid; gap: 8px; }
.attention-row { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; gap: 12px; align-items: center; padding: 12px 13px; border: 1px solid var(--border-muted); border-radius: var(--radius-control); background: var(--surface-subtle); color: var(--text-primary); text-decoration: none; }
.attention-row:hover { border-color: var(--border); background: var(--surface-hover); }
.attention-dot { width: 9px; height: 9px; border-radius: 50%; background: var(--text-muted); box-shadow: 0 0 0 4px var(--surface-raised); }
.attention-dot.is-danger { background: var(--danger); }.attention-dot.is-warning { background: var(--warning); }.attention-dot.is-success { background: var(--success); }.attention-dot.is-muted { background: var(--text-muted); }
.attention-copy { display: grid; min-width: 0; gap: 3px; }.attention-copy strong, .attention-copy small { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }.attention-copy strong { font-size: 13px; }.attention-copy small { color: var(--text-secondary); font-size: 12px; }
.attention-value { color: var(--text-primary); font: 700 13px/1 var(--font-mono); }
.alert-summary-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 8px; }
.alert-summary-grid div { display: grid; gap: 4px; padding: 12px; border: 1px solid var(--border-muted); border-radius: var(--radius-control); background: var(--surface-subtle); }
.alert-summary-grid strong { color: var(--text-primary); font-size: 22px; font-variant-numeric: tabular-nums; }.alert-summary-grid span { color: var(--text-secondary); font-size: 11px; }
.alert-preview-list { display: grid; gap: 8px; margin-top: 12px; }
.alert-preview-row { display: grid; grid-template-columns: auto minmax(0, 1fr); gap: 10px; align-items: center; padding: 9px 0; color: var(--text-primary); text-decoration: none; border-top: 1px solid var(--border-muted); }
.alert-preview-row span:last-child { display: grid; min-width: 0; gap: 3px; }.alert-preview-row strong, .alert-preview-row small { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }.alert-preview-row strong { font-size: 12px; }.alert-preview-row small { color: var(--text-secondary); font-size: 11px; }
.dashboard-alert-empty { min-height: 130px; }
@media (max-width: 960px) { .dashboard-workspace, .dashboard-attention-grid { grid-template-columns: 1fr; }.dashboard-activity-panel { min-height: 0; } }
@media (max-width: 960px) { .dashboard-action-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 700px) { .cluster-strip { align-items: flex-start; flex-direction: column; gap: 16px; }.cluster-strip-stats { grid-template-columns: repeat(3, minmax(0, 1fr)); }.dashboard-metrics { grid-template-columns: repeat(2, minmax(0, 1fr)); }.dashboard-metrics .metric { min-height: 88px; }.dashboard-workspace, .dashboard-attention-grid { gap: var(--space-16); }.dashboard-action-grid { grid-template-columns: 1fr; } }
</style>
