<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">概览</h1>
    </div>

    <div class="metric-grid">
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

    <section class="card" style="margin-bottom: var(--space-24);">
      <div class="card-header">
        <h2 class="card-title">即将到期的证书</h2>
      </div>
      <div v-if="expiringCerts.length === 0" class="empty-state">
        <span class="empty-icon">✓</span>
        <span class="empty-text">30 天内没有即将到期的证书</span>
      </div>
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead>
            <tr><th>域名</th><th>到期时间</th><th>状态</th></tr>
          </thead>
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

    <section class="card">
      <div class="card-header"><h2 class="card-title">最近操作</h2></div>
      <div v-if="recentLogs.length === 0" class="empty-state">
        <span class="empty-icon">⊙</span><span class="empty-text">暂无操作记录</span>
      </div>
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead>
            <tr><th>操作</th><th>资源</th><th>时间</th></tr>
          </thead>
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
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../api/index.js'

const stats = ref({})
const expiringCerts = ref([])
const recentLogs = ref([])

onMounted(async () => {
  try {
    const res = await api.get('/dashboard')
    const data = await res.json()
    stats.value = data.stats || {}
    expiringCerts.value = data.expiring_certs || []
    recentLogs.value = data.recent_logs || []
  } catch (e) { console.error(e) }
})

function formatDate(d) { if (!d) return '-'; return new Date(d).toLocaleDateString('zh-CN', { month:'short', day:'numeric', year:'numeric' }) }
function formatTime(d) { if (!d) return '-'; return new Date(d).toLocaleString('zh-CN', { month:'short', day:'numeric', hour:'2-digit', minute:'2-digit' }) }
function actionBadge(a) { const m = { create:'badge-online',issue:'badge-online',renew:'badge-online',delete:'badge-danger',revoke:'badge-danger',deploy:'badge-deploying' }; return m[a]||'' }
function actionLabel(a) { const m = { create:'创建',update:'更新',delete:'删除',deploy:'部署',issue:'签发',renew:'续期',revoke:'吊销',reload:'重载',generate:'生成' }; return m[a]||a }
function resourceLabel(r) { const m = { server:'服务器',site:'站点',cert:'证书',nginx:'NGINX' }; return m[r]||r }
</script>
