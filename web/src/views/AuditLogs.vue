<template>
  <div class="audit-page">
    <PageHeader title="审计日志" description="筛查用户、系统和自动化动作，支持资源、动作和关键字检索。" class="audit-page-header">
      <template #actions>
        <div class="audit-toolbar">
          <span class="audit-summary">共 {{ total }} 条</span>
          <span class="audit-summary">第 {{ currentPage }} / {{ totalPages }} 页</span>
        </div>
      </template>
    </PageHeader>
    <FeedbackBanner v-if="error" tone="warning" :message="error" class="page-error" />

    <div class="card audit-card">
      <div class="audit-filter-panel">
        <div class="audit-filter-bar">
          <label class="audit-filter-group">
            <span class="audit-filter-label">资源</span>
            <select v-model="filterType" class="form-select audit-control">
              <option value="">全部资源</option>
              <option value="server">服务器</option>
              <option value="site">站点</option>
              <option value="cert">证书</option>
              <option value="nginx">NGINX</option>
              <option value="application">应用</option>
              <option value="agent_runtime">助手实例</option>
              <option value="platform">平台</option>
            </select>
          </label>
          <label class="audit-filter-group">
            <span class="audit-filter-label">操作</span>
            <select v-model="filterAction" class="form-select audit-control">
              <option value="">全部操作</option>
              <option value="create">创建</option>
              <option value="update">更新</option>
              <option value="delete">删除</option>
              <option value="deploy">部署</option>
              <option value="issue">签发</option>
              <option value="renew">续期</option>
              <option value="revoke">吊销</option>
            </select>
          </label>
          <label class="audit-filter-group audit-filter-search">
            <span class="audit-filter-label">关键字</span>
            <input v-model.trim="keyword" class="form-input audit-search audit-control" placeholder="搜索详情、备注或资源名" @keydown.enter.prevent="applyFilters" />
          </label>
        </div>
        <div class="audit-filter-actions">
          <button data-testid="audit-apply-filters" class="btn btn-sm btn-primary" @click="applyFilters">筛选</button>
          <button data-testid="audit-reset-filters" class="btn btn-sm" @click="resetFilters">重置</button>
        </div>
      </div>

      <EmptyState v-if="loading" variant="loading" message="加载中..." />
      <EmptyState v-else-if="logs.length===0" icon="☰" message="暂无审计日志" />
      <div v-else class="table-wrap">
        <table class="data-table audit-table">
          <thead>
            <tr><th>时间</th><th>操作</th><th>资源</th><th>ID</th><th>详情</th></tr>
          </thead>
          <tbody>
            <tr v-for="log in logs" :key="log.id" class="audit-row" @click="openDetail(log)">
              <td>{{ formatTime(log.created_at) }}</td>
              <td><span class="badge" :class="actionBadge(log.action)">{{ actionLabel(log.action) }}</span></td>
              <td>{{ resourceLabel(log.resource_type) }}</td>
              <td>#{{ log.resource_id }}</td>
              <td class="cell-secondary">{{ truncateDetail(log.detail) }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div v-if="total > pageSize" class="pagination">
        <button class="btn btn-sm" :disabled="offset===0" @click="prevPage">上一页</button>
        <span class="pagination-status">{{ currentPage }} / {{ totalPages }}</span>
        <button class="btn btn-sm" :disabled="offset + pageSize >= total" @click="nextPage">下一页</button>
      </div>
    </div>

    <BaseModal
      :open="!!selectedLog"
      title="审计详情"
      size="large"
      dialog-class="audit-detail-modal"
      overlay-class="audit-detail-overlay"
      @close="closeDetail"
    >
          <p class="audit-copy">点击表格行后查看完整内容和结构化字段。</p>

          <div class="audit-detail-meta">
            <span class="badge" :class="actionBadge(selectedLog.action)">{{ actionLabel(selectedLog.action) }}</span>
            <span class="badge badge-offline">{{ resourceLabel(selectedLog.resource_type) }}</span>
            <span>#{{ selectedLog.resource_id }}</span>
            <span>{{ formatTime(selectedLog.created_at) }}</span>
          </div>

          <div class="audit-detail-grid">
            <span class="detail-label">用户</span><span>{{ selectedLog.user_id || '-' }}</span>
            <span class="detail-label">资源类型</span><span>{{ resourceLabel(selectedLog.resource_type) }}</span>
            <span class="detail-label">动作</span><span>{{ actionLabel(selectedLog.action) }}</span>
            <span class="detail-label">资源 ID</span><span>#{{ selectedLog.resource_id }}</span>
          </div>

          <div v-if="parsedDetail" class="audit-detail-block">
            <h3 class="audit-detail-subtitle">结构化内容</h3>
            <div class="audit-detail-grid audit-detail-grid-compact">
              <template v-for="(value, key) in parsedDetail" :key="key">
                <span class="detail-label">{{ key }}</span><span class="audit-detail-value">{{ formatDetailValue(value) }}</span>
              </template>
            </div>
          </div>

          <div class="audit-detail-block">
            <h3 class="audit-detail-subtitle">原始详情</h3>
            <pre class="audit-detail-pre">{{ selectedLog.detail || '-' }}</pre>
          </div>

      <template #actions>
        <button class="btn btn-primary" @click="closeDetail">关闭</button>
      </template>
    </BaseModal>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { getAuditLogs } from '../api/audit.js'
import { useAsyncResource } from '../composables/useAsyncResource.js'
import BaseModal from '../components/BaseModal.vue'
import EmptyState from '../components/EmptyState.vue'
import FeedbackBanner from '../components/FeedbackBanner.vue'
import PageHeader from '../components/PageHeader.vue'
import { formatShortDateTime as formatTime } from '../utils/formatters.js'

const pageSize = 20
const logs = ref([])
const total = ref(0)
const offset = ref(0)
const filterType = ref('')
const filterAction = ref('')
const keyword = ref('')
const auditResource = useAsyncResource(({ signal }, params) => getAuditLogs(params, { signal }), { data: [], total: 0 })
const loading = auditResource.loading
const error = ref('')
const selectedLog = ref(null)

const currentPage = computed(() => Math.floor(offset.value / pageSize) + 1)
const totalPages = computed(() => Math.max(1, Math.ceil((total.value || 0) / pageSize)))
const parsedDetail = computed(() => parseDetail(selectedLog.value?.detail))

onMounted(() => { void loadLogs(true) })

async function applyFilters() {
  await loadLogs(true)
}

async function resetFilters() {
  filterType.value = ''
  filterAction.value = ''
  keyword.value = ''
  await loadLogs(true)
}

async function loadLogs(resetOffset = false) {
  if (resetOffset) offset.value = 0
  error.value = ''
  const result = await auditResource.refresh({
    limit: pageSize,
    offset: offset.value,
    resourceType: filterType.value,
    action: filterAction.value,
    keyword: keyword.value,
  })
  if (!result) {
    if (auditResource.error.value) error.value = auditResource.error.value.message || '加载审计日志失败'
    return
  }
  logs.value = result.data || []
  total.value = result.total || 0
}

async function prevPage() {
  offset.value = Math.max(0, offset.value - pageSize)
  await loadLogs(false)
}

async function nextPage() {
  offset.value += pageSize
  await loadLogs(false)
}

function openDetail(log) {
  selectedLog.value = log
}

function closeDetail() {
  selectedLog.value = null
}

function actionLabel(a) {
  const m = { create: '创建', update: '更新', delete: '删除', deploy: '部署', issue: '签发', renew: '续期', revoke: '吊销', reload: '重载', generate: '生成' }
  return m[a] || a
}

function resourceLabel(r) {
  const m = { server: '服务器', site: '站点', cert: '证书', nginx: 'NGINX', application: '应用', agent_runtime: '助手实例', platform: '平台' }
  return m[r] || r
}

function actionBadge(a) {
  const m = { create: 'badge-online', update: 'badge-deploying', delete: 'badge-danger', deploy: 'badge-deploying', issue: 'badge-online', renew: 'badge-online', revoke: 'badge-danger', reload: 'badge-deploying', generate: 'badge-online' }
  return m[a] || ''
}

function truncateDetail(d) {
  if (!d) return '-'
  const parsed = parseDetail(d)
  if (parsed) {
    return parsed.request?.domain || parsed.domain || parsed.message || JSON.stringify(parsed).slice(0, 96)
  }
  return String(d).slice(0, 96)
}

function parseDetail(detail) {
  if (!detail) return null
  try {
    const parsed = JSON.parse(detail)
    return parsed && typeof parsed === 'object' ? parsed : null
  } catch {
    return null
  }
}

function formatDetailValue(value) {
  if (value === null || value === undefined || value === '') return '-'
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  return JSON.stringify(value, null, 2)
}
</script>

<style scoped>
.audit-page {
  display: grid;
  gap: var(--space-16);
}

.audit-page-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.audit-copy {
  margin: 8px 0 0;
  color: var(--text-secondary);
  font-size: 13px;
  line-height: 1.6;
}

.audit-toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.audit-summary {
  padding: 6px 10px;
  border-radius: var(--radius-control);
  background: var(--surface-subtle);
  color: var(--text-secondary);
  font-size: 12px;
}

.audit-card {
  display: grid;
  gap: var(--space-16);
}

.audit-filter-panel {
  display: grid;
  gap: 10px;
  padding: 14px;
  border: 1px solid var(--border-muted);
  border-radius: var(--radius-control);
  background: var(--surface-subtle);
}

.audit-filter-bar {
  display: grid;
  grid-template-columns: repeat(2, minmax(180px, 220px)) minmax(0, 1fr);
  gap: 10px 12px;
  align-items: end;
}

.audit-search {
  min-width: 0;
}

.audit-filter-group {
  display: grid;
  gap: 6px;
}

.audit-filter-label {
  color: var(--text-secondary);
  font-size: 11px;
  font-weight: 600;
}

.audit-control {
  min-height: 36px;
  padding-top: 7px;
  padding-bottom: 7px;
  border-radius: 10px;
}

.audit-filter-search {
  min-width: 0;
}

.audit-filter-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.audit-table .audit-row {
  cursor: pointer;
}

.audit-table .audit-row:hover {
  background: var(--surface-hover);
}

.audit-detail-overlay {
  z-index: 1400;
}

.audit-detail-modal {
  width: min(760px, calc(100vw - 32px));
  max-height: calc(100dvh - 32px);
  overflow: auto;
}

.audit-detail-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.audit-detail-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 14px;
  color: var(--text-secondary);
  font-size: 12px;
}

.audit-detail-grid {
  display: grid;
  grid-template-columns: 140px minmax(0, 1fr);
  gap: 10px 12px;
  padding: 14px 0;
}

.audit-detail-grid-compact {
  padding-top: 0;
}

.audit-detail-block {
  margin-top: 14px;
  padding-top: 14px;
  border-top: 1px solid var(--border-muted);
}

.audit-detail-subtitle {
  margin: 0 0 12px;
  color: var(--text-primary);
  font-size: 14px;
}

.audit-detail-value {
  min-width: 0;
  overflow-wrap: anywhere;
  color: var(--text-primary);
  font-size: 12px;
}

.audit-detail-pre {
  margin: 0;
  padding: 12px;
  border: 1px solid var(--border-muted);
  border-radius: var(--radius-control);
  background: var(--surface-subtle);
  color: var(--text-primary);
  font: 12px/1.6 var(--font-mono);
  white-space: pre-wrap;
  word-break: break-word;
}

@media (max-width: 640px) {
  .audit-page-header,
  .audit-detail-header {
    align-items: stretch;
    flex-direction: column;
  }

  .audit-filter-bar {
    grid-template-columns: 1fr;
  }

  .audit-filter-actions {
    justify-content: stretch;
  }

  .audit-filter-actions .btn {
    flex: 1;
  }

  .audit-detail-grid {
    grid-template-columns: 1fr;
  }
}
</style>
