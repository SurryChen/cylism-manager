<template>
  <div class="audit-page">
    <SectionTabsHeader title="审计日志" :tabs="pageTabs" active-tab="logs" test-id-prefix="audit-page">
      <template #actions>
        <div class="audit-toolbar">
          <span class="audit-summary">共 {{ total }} 条</span>
          <span class="audit-summary">第 {{ currentPage }} / {{ totalPages }} 页</span>
        </div>
      </template>
    </SectionTabsHeader>
    <FeedbackBanner v-if="error" tone="warning" :message="error" class="page-error" />

    <SurfaceCard class="audit-card">
      <div class="audit-filter-panel">
        <div class="audit-primary-filters">
          <label class="audit-search-field">
            <Search :size="15" aria-hidden="true" />
            <span class="sr-only">关键字</span>
            <input v-model.trim="keyword" class="form-input audit-search audit-control" placeholder="搜索目标、摘要或操作者" @keydown.enter.prevent="applyFilters" />
          </label>
          <label class="audit-filter-group">
            <span class="sr-only">结果</span>
            <SelectMenu v-model="filterOutcome" :options="outcomeOptions" placeholder="全部结果" aria-label="结果" />
          </label>
          <label class="audit-filter-group">
            <span class="sr-only">资源</span>
            <SelectMenu v-model="filterType" :options="resourceOptions" placeholder="全部资源" aria-label="资源" />
          </label>
          <button class="btn btn-sm audit-filter-trigger" :class="{ 'is-active': advancedOpen || activeAdvancedCount }" type="button" data-testid="audit-open-filters" :aria-expanded="advancedOpen" @click="openAdvancedFilters"><Filter :size="15" />筛选<span v-if="activeAdvancedCount" class="filter-count">{{ activeAdvancedCount }}</span></button>
          <button data-testid="audit-apply-filters" class="btn btn-sm btn-primary" type="button" @click="applyFilters">应用</button>
          <button data-testid="audit-reset-filters" class="icon-button audit-reset" type="button" title="重置筛选" aria-label="重置筛选" @click="resetFilters"><RotateCcw :size="15" /></button>
        </div>
        <div v-if="activeFilters.length" class="active-filters audit-active-filters">
          <span class="active-filters-label">已启用</span>
          <button v-for="item in activeFilters" :key="item.key" class="filter-chip" type="button" @click="clearFilter(item.key)">{{ item.label }}<X :size="13" aria-hidden="true" /></button>
        </div>
      </div>

      <EmptyState v-if="loading" variant="loading" message="加载中..." />
      <EmptyState v-else-if="logs.length===0" icon="☰" message="暂无审计日志" />
      <div v-else class="table-wrap">
        <table class="data-table audit-table">
          <thead>
            <tr><th>时间</th><th>结果</th><th>动作</th><th>目标</th><th>操作者</th><th>摘要</th></tr>
          </thead>
          <tbody>
            <tr v-for="log in logs" :key="log.id" class="audit-row" @click="openDetail(log)">
              <td>{{ formatTime(log.created_at) }}</td>
              <td><span class="badge" :class="outcomeBadge(log.outcome)">{{ outcomeLabel(log.outcome) }}</span></td>
              <td>{{ actionLabel(log.action) }}</td>
              <td>{{ log.target_name || `${resourceLabel(log.resource_type)} #${log.resource_id}` }}</td>
              <td>{{ log.actor_name || (log.actor_type === 'system' ? '系统' : log.user_id ? `用户 #${log.user_id}` : '-') }}</td>
              <td class="cell-secondary">{{ log.summary || legacySummary(log) }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div v-if="total > pageSize" class="pagination">
        <button class="btn btn-sm" :disabled="offset===0" @click="prevPage">上一页</button>
        <span class="pagination-status">{{ currentPage }} / {{ totalPages }}</span>
        <button class="btn btn-sm" :disabled="offset + pageSize >= total" @click="nextPage">下一页</button>
      </div>
    </SurfaceCard>

    <BaseModal :open="advancedOpen" title="筛选审计日志" size="small" dialog-class="audit-filter-modal" @close="closeAdvancedFilters">
      <div class="audit-advanced-grid">
        <label class="form-group"><span class="form-label">来源</span><SelectMenu v-model="draftSource" :options="sourceOptions" placeholder="全部来源" aria-label="来源" /></label>
        <label class="form-group"><span class="form-label">操作者类型</span><SelectMenu v-model="draftActorType" :options="actorOptions" placeholder="全部操作者" aria-label="操作者类型" /></label>
        <label class="form-group audit-field-wide"><span class="form-label">动作</span><SelectMenu v-model="draftAction" :options="actionOptions" placeholder="全部动作" aria-label="动作" /></label>
        <label class="form-group audit-field-wide"><span class="form-label">目标名称</span><input v-model.trim="draftTargetName" class="form-input" placeholder="按目标名称匹配" /></label>
        <label class="form-group"><span class="form-label">开始日期</span><input v-model="draftCreatedFrom" class="form-input" type="date" /></label>
        <label class="form-group"><span class="form-label">结束日期</span><input v-model="draftCreatedTo" class="form-input" type="date" /><span v-if="dateRangeInvalid" class="form-error">结束日期不能早于开始日期</span></label>
      </div>
      <template #actions>
        <button class="btn" type="button" @click="closeAdvancedFilters">取消</button>
        <button class="btn btn-primary" type="button" :disabled="dateRangeInvalid" @click="applyAdvancedFilters">应用筛选</button>
      </template>
    </BaseModal>

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
            <span class="badge" :class="outcomeBadge(selectedLog.outcome)">{{ outcomeLabel(selectedLog.outcome) }}</span>
            <span class="badge badge-offline">{{ resourceLabel(selectedLog.resource_type) }}</span>
            <span>#{{ selectedLog.resource_id }}</span>
            <span>{{ formatTime(selectedLog.created_at) }}</span>
          </div>

          <div class="audit-detail-grid">
            <span class="detail-label">操作者</span><span>{{ selectedLog.actor_name || selectedLog.user_id || '-' }}</span>
            <span class="detail-label">来源</span><span>{{ selectedLog.source || '历史记录' }}</span>
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
import { Filter, RotateCcw, Search, X } from 'lucide-vue-next'
import { getAuditLogs } from '../api/audit.js'
import { useAsyncResource } from '../composables/useAsyncResource.js'
import BaseModal from '../components/BaseModal.vue'
import EmptyState from '../components/EmptyState.vue'
import FeedbackBanner from '../components/FeedbackBanner.vue'
import SectionTabsHeader from '../components/SectionTabsHeader.vue'
import SelectMenu from '../components/SelectMenu.vue'
import SurfaceCard from '../components/SurfaceCard.vue'
import { formatShortDateTime as formatTime } from '../utils/formatters.js'

const pageSize = 20
const logs = ref([])
const total = ref(0)
const offset = ref(0)
const filterType = ref('')
const filterAction = ref('')
const filterOutcome = ref('')
const filterSource = ref('')
const filterActorType = ref('')
const keyword = ref('')
const filterTargetName = ref('')
const filterCreatedFrom = ref('')
const filterCreatedTo = ref('')
const advancedOpen = ref(false)
const draftSource = ref('')
const draftAction = ref('')
const draftActorType = ref('')
const draftTargetName = ref('')
const draftCreatedFrom = ref('')
const draftCreatedTo = ref('')
const auditResource = useAsyncResource(({ signal }, params) => getAuditLogs(params, { signal }), { data: [], total: 0 })
const loading = auditResource.loading
const error = ref('')
const selectedLog = ref(null)
const pageTabs = [{ id: 'logs', label: '审计记录' }]

const currentPage = computed(() => Math.floor(offset.value / pageSize) + 1)
const totalPages = computed(() => Math.max(1, Math.ceil((total.value || 0) / pageSize)))
const parsedDetail = computed(() => parseDetail(selectedLog.value?.detail))
const resourceOptions = [
  { value: 'application', label: '应用' }, { value: 'workload', label: '工作负载' }, { value: 'server', label: '服务器' },
  { value: 'cluster_node', label: '集群节点' }, { value: 'site', label: '站点' }, { value: 'domain', label: '域名' },
  { value: 'certificate', label: '证书' }, { value: 'ingress', label: 'Ingress' }, { value: 'platform', label: '平台' },
  { value: 'runtime', label: 'Agent 运行时' }, { value: 'node_registry_mirror', label: '节点镜像源' }, { value: 'image_registry', label: '镜像仓库' },
  { value: 'managed_registry', label: '托管仓库' }, { value: 'registry_proxy', label: 'Registry Proxy' }, { value: 'chart_repository', label: 'Chart 仓库' },
  { value: 'secret', label: 'Secret' }, { value: 'configmap', label: 'ConfigMap' }, { value: 'service', label: 'Service' },
  { value: 'namespace', label: '命名空间' }, { value: 'storage', label: '存储' }, { value: 'monitoring', label: '监控' },
  { value: 'logging', label: '日志' }, { value: 'alerting', label: '告警' }, { value: 'identity', label: '身份认证' },
  { value: 'delegation', label: '委托' }, { value: 'temporary_token', label: '临时令牌' }, { value: 'cluster_dns', label: '集群 DNS' },
  { value: 'system_component', label: '系统组件' }, { value: 'tailnet', label: 'Tailnet' }, { value: 'admin_record', label: '管理记录' }, { value: 'ingress_route', label: 'Ingress 路由' },
]
const outcomeOptions = [{ value: 'succeeded', label: '成功' }, { value: 'failed', label: '失败' }, { value: 'denied', label: '已拒绝' }, { value: 'accepted', label: '已受理' }]
const sourceOptions = [{ value: 'api', label: '平台操作' }, { value: 'agent', label: 'Agent' }, { value: 'delegation', label: '委托' }, { value: 'system', label: '系统' }, { value: 'legacy', label: '历史记录' }]
const actorOptions = [{ value: 'user', label: '用户' }, { value: 'agent', label: 'Agent' }, { value: 'system', label: '系统' }]
const actionOptions = [
  { value: 'create', label: '创建（历史）' }, { value: 'update', label: '更新（历史）' }, { value: 'delete', label: '删除（历史）' }, { value: 'deploy', label: '部署（历史）' },
  { value: 'application.create', label: '创建应用' }, { value: 'application.update', label: '更新应用' }, { value: 'application.release', label: '发布应用' },
  { value: 'application.delegation.create', label: '创建应用委托' }, { value: 'workload.scale', label: '扩缩工作负载' }, { value: 'workload.restart', label: '重启工作负载' },
  { value: 'workload.rollback', label: '回滚工作负载' }, { value: 'registry.mirror.verify', label: '验证镜像源' }, { value: 'registry.mirror.apply', label: '应用镜像源' },
  { value: 'secret.read', label: '读取 Secret' }, { value: 'server.terminal.start', label: '启动服务器终端' }, { value: 'workload.pod.terminal.start', label: '启动 Pod 终端' },
  { value: 'auth.login', label: '登录' }, { value: 'auth.logout', label: '退出登录' }, { value: 'auth.refresh', label: '刷新令牌' },
]
const activeAdvancedCount = computed(() => [filterSource.value, filterAction.value, filterActorType.value, filterTargetName.value, filterCreatedFrom.value, filterCreatedTo.value].filter(Boolean).length)
const dateRangeInvalid = computed(() => Boolean(draftCreatedFrom.value && draftCreatedTo.value && draftCreatedFrom.value > draftCreatedTo.value))
const activeFilters = computed(() => {
  const items = []
  if (keyword.value) items.push({ key: 'keyword', label: `关键词：${keyword.value}` })
  if (filterOutcome.value) items.push({ key: 'outcome', label: `结果：${outcomeLabel(filterOutcome.value)}` })
  if (filterType.value) items.push({ key: 'resourceType', label: `资源：${resourceLabel(filterType.value)}` })
  if (filterSource.value) items.push({ key: 'source', label: `来源：${sourceLabel(filterSource.value)}` })
  if (filterActorType.value) items.push({ key: 'actorType', label: `操作者：${actorTypeLabel(filterActorType.value)}` })
  if (filterAction.value) items.push({ key: 'action', label: `动作：${actionLabel(filterAction.value)}` })
  if (filterTargetName.value) items.push({ key: 'targetName', label: `目标：${filterTargetName.value}` })
  if (filterCreatedFrom.value) items.push({ key: 'createdFrom', label: `起始：${filterCreatedFrom.value}` })
  if (filterCreatedTo.value) items.push({ key: 'createdTo', label: `截止：${filterCreatedTo.value}` })
  return items
})

onMounted(() => { void loadLogs(true) })

async function applyFilters() {
  await loadLogs(true)
}

async function resetFilters() {
  filterType.value = ''
  filterAction.value = ''
  filterOutcome.value = ''
  filterSource.value = ''
  filterActorType.value = ''
  keyword.value = ''
  filterTargetName.value = ''
  filterCreatedFrom.value = ''
  filterCreatedTo.value = ''
  advancedOpen.value = false
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
    outcome: filterOutcome.value,
    source: filterSource.value,
    actorType: filterActorType.value,
    keyword: keyword.value,
    targetName: filterTargetName.value,
    createdFrom: filterCreatedFrom.value,
    createdTo: filterCreatedTo.value,
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

function openAdvancedFilters() {
  draftSource.value = filterSource.value
  draftAction.value = filterAction.value
  draftActorType.value = filterActorType.value
  draftTargetName.value = filterTargetName.value
  draftCreatedFrom.value = filterCreatedFrom.value
  draftCreatedTo.value = filterCreatedTo.value
  advancedOpen.value = true
}

function closeAdvancedFilters() { advancedOpen.value = false }

async function applyAdvancedFilters() {
  filterSource.value = draftSource.value
  filterAction.value = draftAction.value
  filterActorType.value = draftActorType.value
  filterTargetName.value = draftTargetName.value
  filterCreatedFrom.value = draftCreatedFrom.value
  filterCreatedTo.value = draftCreatedTo.value
  advancedOpen.value = false
  await loadLogs(true)
}

function clearFilter(key) {
  const values = { keyword: keyword, outcome: filterOutcome, resourceType: filterType, source: filterSource, actorType: filterActorType, action: filterAction, targetName: filterTargetName, createdFrom: filterCreatedFrom, createdTo: filterCreatedTo }
  if (values[key]) values[key].value = ''
  void loadLogs(true)
}

function actionLabel(a) {
  const m = Object.fromEntries(actionOptions.map(item => [item.value, item.label.replace('（历史）', '')]))
  Object.assign(m, { issue: '签发', renew: '续期', revoke: '吊销', reload: '重载', generate: '生成' })
  return m[a] || a
}

function resourceLabel(r) {
  const m = Object.fromEntries(resourceOptions.map(item => [item.value, item.label]))
  return m[r] || r
}

function sourceLabel(value) { return ({ api: '平台操作', agent: 'Agent', delegation: '委托', system: '系统', legacy: '历史记录' })[value] || value }
function actorTypeLabel(value) { return ({ user: '用户', agent: 'Agent', system: '系统' })[value] || value }

function actionBadge(a) {
  const m = { create: 'badge-online', update: 'badge-deploying', delete: 'badge-danger', deploy: 'badge-deploying', issue: 'badge-online', renew: 'badge-online', revoke: 'badge-danger', reload: 'badge-deploying', generate: 'badge-online' }
  return m[a] || ''
}

function outcomeLabel(value) { return ({ succeeded: '成功', failed: '失败', denied: '已拒绝', accepted: '已受理' })[value] || '成功' }
function outcomeBadge(value) { return ({ succeeded: 'badge-online', failed: 'badge-danger', denied: 'badge-danger', accepted: 'badge-deploying' })[value] || 'badge-online' }
function legacySummary(log) { return `执行 ${actionLabel(log.action)}：${resourceLabel(log.resource_type)} #${log.resource_id}` }

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
  gap: var(--space-4);
}

/* Keep labels available to assistive technology without adding visual layout noise. */
.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
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

.audit-primary-filters { display: grid; grid-template-columns: minmax(220px, 1fr) 130px 150px auto auto 34px; gap: 8px; align-items: center; }
.audit-search-field { display: flex; min-width: 0; align-items: center; gap: 8px; min-height: 36px; padding: 0 10px; border-radius: var(--radius-control); background: transparent; color: var(--text-muted); }
.audit-search-field:focus-within { color: var(--action-primary); }
.audit-search-field .audit-search { flex: 1; min-width: 0; min-height: 36px; padding: 7px 0; border: 0; border-radius: 0; background: transparent; box-shadow: none; outline: 0; }
.audit-search-field .audit-search:focus, .audit-search-field .audit-search:focus-visible { box-shadow: none; outline: 0; }

.audit-filter-group {
  display: grid;
  gap: 6px;
}

.audit-control {
  min-height: 36px;
  padding-top: 7px;
  padding-bottom: 7px;
  border-radius: 10px;
}

.audit-filter-group :deep(.select-menu-trigger) { min-height: 36px; }

.audit-filter-trigger { display: inline-flex; align-items: center; justify-content: center; gap: 6px; white-space: nowrap; }
.audit-filter-trigger.is-active { border-color: var(--action-primary); color: var(--action-primary); }
.filter-count { display: inline-grid; min-width: 17px; height: 17px; place-items: center; padding: 0 4px; border-radius: 99px; background: var(--action-primary); color: var(--surface); font-size: 10px; }
.audit-reset { width: 34px; height: 34px; }
.audit-active-filters { margin: 10px 0 0; }
.active-filters-label { color: var(--text-muted); font-size: 11px; }
.audit-advanced-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px 12px; }
.audit-field-wide { grid-column: 1 / -1; }
.audit-filter-modal { width: min(560px, calc(100vw - 32px)); }

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

  .audit-primary-filters { grid-template-columns: minmax(0, 1fr) 34px; }
  .audit-search-field { grid-column: 1 / -1; }
  .audit-primary-filters .audit-filter-group { min-width: 0; }
  .audit-primary-filters .audit-filter-trigger { grid-column: 1; }
  .audit-primary-filters .btn-primary { grid-column: 2; grid-row: 2; padding: 0 8px; }
  .audit-reset { grid-column: 2; grid-row: 3; }

  .audit-detail-grid {
    grid-template-columns: 1fr;
  }

  .audit-advanced-grid { grid-template-columns: 1fr; }
  .audit-field-wide { grid-column: auto; }
}
</style>
