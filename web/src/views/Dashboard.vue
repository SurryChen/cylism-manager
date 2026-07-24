<template>
  <div class="dashboard-page">
    <div class="page-header">
      <div><span class="page-eyebrow">Operations / Overview</span><h1 class="page-title">服务健康度</h1><p class="page-subtitle">集中查看受管服务、证书风险与最近变更。</p></div>
    </div>

    <section v-if="k8sStats" class="cluster-strip section-gap">
      <div class="cluster-strip-heading"><span class="page-eyebrow">Kubernetes</span><strong>集群运行概况</strong></div>
      <div class="cluster-strip-stats">
        <div><strong class="metric-accent">{{ k8sStats.nodes_total || 0 }}</strong><span>节点</span></div>
        <div><strong>{{ k8sStats.namespaces || 0 }}</strong><span>命名空间</span></div>
        <div><strong class="metric-success">{{ k8sStats.pods_ready || 0 }}/{{ k8sStats.pods_total || 0 }}</strong><span>Pods 就绪</span></div>
        <div><strong class="metric-version">{{ k8sStats.version || '-' }}</strong><span>K3s 版本</span></div>
      </div>
    </section>
    <div v-else-if="k8sError" class="k8s-banner k8s-banner-warn section-gap">
      ⚠ K8s 集群未连接，集群信息不可用
    </div>

    <div class="metric-grid dashboard-metrics">
      <div class="metric">
        <div class="metric-value">{{ stats.total_servers || 0 }}</div>
        <div class="metric-label">服务器</div>
      </div>
      <div class="metric">
        <div class="metric-value metric-accent">{{ stats.online_servers || 0 }}</div>
        <div class="metric-label">在线</div>
      </div>
      <div class="metric">
        <div class="metric-value">{{ stats.total_sites || 0 }}</div>
        <div class="metric-label">站点</div>
      </div>
      <div class="metric">
        <div class="metric-value metric-warn">{{ stats.expiring_certs || 0 }}</div>
        <div class="metric-label">即将到期</div>
      </div>
    </div>

    <div class="dashboard-workspace">
      <section class="card dashboard-primary-panel">
        <div class="card-header">
          <div><span class="page-eyebrow">Risk queue</span><h2 class="card-title">即将到期的证书</h2></div>
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
        <div class="card-header"><div><span class="page-eyebrow">Change log</span><h2 class="card-title">最近操作</h2></div></div>
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
import { ref, onMounted } from 'vue'
import { api } from '../api/index.js'

const stats = ref({})
const expiringCerts = ref([])
const recentLogs = ref([])
const k8sStats = ref(null)
const k8sError = ref('')

onMounted(async () => {
  try {
    const res = await api.get('/dashboard')
    const data = await res.json()
    stats.value = data.stats || {}
    expiringCerts.value = data.expiring_certs || []
    recentLogs.value = data.recent_logs || []
  } catch (e) { console.error(e) }

  // 获取 K8s 集群状态
  try {
    const r = await api.get('/k8s/dashboard')
    const d = await r.json()
    if (d.error) { k8sError.value = d.error }
    else { k8sStats.value = d }
  } catch(e) { /* silent */ }
})

function formatDate(d) { if (!d) return '-'; return new Date(d).toLocaleDateString('zh-CN', { month:'short', day:'numeric', year:'numeric' }) }
function formatTime(d) { if (!d) return '-'; return new Date(d).toLocaleString('zh-CN', { month:'short', day:'numeric', hour:'2-digit', minute:'2-digit' }) }
function actionBadge(a) { const m = { create:'badge-online',issue:'badge-online',renew:'badge-online',delete:'badge-danger',revoke:'badge-danger',deploy:'badge-deploying' }; return m[a]||'' }
function actionLabel(a) { const m = { create:'创建',update:'更新',delete:'删除',deploy:'部署',issue:'签发',renew:'续期',revoke:'吊销',reload:'重载',generate:'生成' }; return m[a]||a }
function resourceLabel(r) { const m = { server:'服务器',site:'站点',cert:'证书',nginx:'NGINX' }; return m[r]||r }
</script>

<style scoped>
.dashboard-page { max-width: 1320px; }
.cluster-strip { display: flex; align-items: center; gap: 28px; min-height: 94px; padding: 18px 22px; border: 1px solid var(--border); border-radius: var(--radius-panel); background: var(--surface-glass); box-shadow: var(--shadow-soft); backdrop-filter: blur(22px) saturate(125%); }
.cluster-strip-heading { display: flex; min-width: 142px; flex-direction: column; gap: 5px; }.cluster-strip-heading .page-eyebrow { margin: 0; }.cluster-strip-heading strong { color: var(--text-primary); font-size: 13px; }
.cluster-strip-stats { display: grid; width: 100%; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 14px; }.cluster-strip-stats > div { display: flex; min-width: 0; flex-direction: column; gap: 4px; padding-left: 16px; border-left: 1px solid var(--border-muted); }.cluster-strip-stats strong { overflow: hidden; color: var(--text-primary); font-size: 19px; font-variant-numeric: tabular-nums; text-overflow: ellipsis; white-space: nowrap; }.cluster-strip-stats span { color: var(--text-muted); font-size: 10px; }
.dashboard-metrics { grid-template-columns: repeat(4, minmax(0, 1fr)); }.dashboard-metrics .metric { min-height: 96px; padding: 16px 18px; }.dashboard-metrics .metric-value { font-size: 26px; }.dashboard-workspace { display: grid; grid-template-columns: minmax(0, 7fr) minmax(310px, 4fr); gap: var(--space-20); }.dashboard-primary-panel, .dashboard-activity-panel { min-width: 0; }.dashboard-workspace .card-header { align-items: flex-start; margin-bottom: 16px; }.dashboard-workspace .page-eyebrow { margin-bottom: 5px; }.panel-caption { padding: 5px 7px; border-radius: 5px; background: var(--warning-surface); color: var(--warning); font: 10px/1 var(--font-mono); }.dashboard-empty-state { min-height: 230px; }.metric-version { font-size: 15px !important; }
@media (max-width: 960px) { .dashboard-workspace { grid-template-columns: 1fr; }.dashboard-activity-panel { min-height: 0; } }
@media (max-width: 700px) { .cluster-strip { align-items: flex-start; flex-direction: column; gap: 16px; }.cluster-strip-stats { grid-template-columns: repeat(2, minmax(0, 1fr)); }.cluster-strip-stats > div:nth-child(odd) { padding-left: 0; border-left: 0; }.dashboard-metrics { grid-template-columns: repeat(2, minmax(0, 1fr)); }.dashboard-metrics .metric { min-height: 88px; }.dashboard-workspace { gap: var(--space-16); } }
</style>
