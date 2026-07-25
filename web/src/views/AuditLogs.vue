<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">审计日志</h1>
      <div class="btn-group filter-bar">
        <select v-model="filterType" class="form-select"><option value="">全部资源</option><option value="server">服务器</option><option value="site">站点</option><option value="cert">证书</option><option value="nginx">NGINX</option></select>
        <select v-model="filterAction" class="form-select"><option value="">全部操作</option><option value="create">创建</option><option value="update">更新</option><option value="delete">删除</option><option value="deploy">部署</option><option value="issue">签发</option><option value="renew">续期</option><option value="revoke">吊销</option></select>
        <button class="btn btn-sm" @click="fetchLogs">筛选</button>
      </div>
    </div>

    <div class="card">
      <div v-if="logs.length===0" class="empty-state"><span class="empty-icon">☰</span><span class="empty-text">暂无审计日志</span></div>
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead><tr><th>时间</th><th>操作</th><th>资源</th><th>ID</th><th>详情</th></tr></thead>
          <tbody>
            <tr v-for="log in logs" :key="log.id">
              <td>{{ formatTime(log.created_at) }}</td>
              <td><span class="badge" :class="actionBadge(log.action)">{{ actionLabel(log.action) }}</span></td>
              <td>{{ resourceLabel(log.resource_type) }}</td><td>#{{ log.resource_id }}</td>
              <td class="cell-secondary">{{ truncateDetail(log.detail) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <div v-if="total>20" class="pagination">
        <button class="btn btn-sm" :disabled="offset===0" @click="prevPage">上一页</button>
        <span class="pagination-status">{{ Math.floor(offset/20)+1 }} / {{ Math.ceil(total/20) }}</span>
        <button class="btn btn-sm" :disabled="offset+20>=total" @click="nextPage">下一页</button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../api/index.js'

const logs = ref([])
const total = ref(0)
const offset = ref(0)
const filterType = ref('')
const filterAction = ref('')

onMounted(fetchLogs)

async function fetchLogs() {
  offset.value = 0
  const params = new URLSearchParams({ limit:'20', offset:'0' })
  if(filterType.value) params.set('resource_type',filterType.value)
  if(filterAction.value) params.set('action',filterAction.value)
  try { const d = await api.get(`/audit-logs?${params}`); logs.value = (d && d.data) || []; total.value = (d && d.total) || 0 } catch(e){console.error(e)}
}
async function prevPage() { offset.value=Math.max(0,offset.value-20); await loadPage() }
async function nextPage() { offset.value=offset.value+20; await loadPage() }
async function loadPage() {
  const params = new URLSearchParams({ limit:'20', offset:String(offset.value) })
  if(filterType.value) params.set('resource_type',filterType.value)
  if(filterAction.value) params.set('action',filterAction.value)
  const d = await api.get(`/audit-logs?${params}`); logs.value = (d && d.data) || []
}
function formatTime(d) { if(!d)return'-'; return new Date(d).toLocaleString('zh-CN',{month:'short',day:'numeric',hour:'2-digit',minute:'2-digit'}) }
function actionLabel(a) { const m={create:'创建',update:'更新',delete:'删除',deploy:'部署',issue:'签发',renew:'续期',revoke:'吊销',reload:'重载',generate:'生成'}; return m[a]||a }
function resourceLabel(r) { const m={server:'服务器',site:'站点',cert:'证书',nginx:'NGINX'}; return m[r]||r }
function actionBadge(a) { const m={create:'badge-online',update:'badge-deploying',delete:'badge-danger',deploy:'badge-deploying',issue:'badge-online',renew:'badge-online',revoke:'badge-danger',reload:'badge-deploying',generate:'badge-online'}; return m[a]||'' }
function truncateDetail(d) { if(!d)return'-'; try{ const o=JSON.parse(d); return o.request?.domain||o.domain||d.substring(0,80) } catch{ return d.substring(0,80) } }
</script>
