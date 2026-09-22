<template>
  <div class="operation-page">
    <SectionTabsHeader title="操作历史" :tabs="pageTabs" active-tab="history" test-id-prefix="operation-page">
      <template #actions><span class="operation-summary">共 {{ total }} 条</span></template>
    </SectionTabsHeader>
    <FeedbackBanner v-if="error" tone="warning" :message="error" />
    <WorkspaceHeader title="执行记录" description="查看异步操作流程的执行步骤与结果。" />
    <SurfaceCard class="operation-card" padding="none">
      <div class="operation-table-toolbar">
        <div class="operation-filters">
          <label class="operation-search-field">
            <Search :size="15" aria-hidden="true" />
            <span class="sr-only">关键字</span>
            <input v-model.trim="keyword" class="form-input operation-search" placeholder="搜索步骤或详情" @keydown.enter.prevent="load(true)" />
          </label>
        <SelectMenu v-model="resourceType" class="form-select" aria-label="资源类型">
          <option value="">全部资源</option><option value="application">应用</option><option value="server">服务器</option><option value="platform">平台</option>
        </SelectMenu>
        <SelectMenu v-model="status" class="form-select" aria-label="执行状态">
          <option value="">全部状态</option><option value="running">执行中</option><option value="success">成功</option><option value="failed">失败</option>
        </SelectMenu>
        <button class="btn btn-primary" @click="load(true)">筛选</button>
        <button class="icon-button operation-reset" type="button" title="重置筛选" aria-label="重置筛选" @click="resetFilters"><RotateCcw :size="15" /></button>
        </div>
      </div>
      <EmptyState v-if="loading" variant="loading" message="加载中..." />
      <EmptyState v-else-if="operations.length === 0" message="暂无操作历史" />
      <div v-else class="table-wrap">
        <table class="data-table"><thead><tr><th>时间</th><th>流程 / 资源</th><th>步骤</th><th>状态</th><th>详情</th></tr></thead>
          <tbody><tr v-for="operation in operations" :key="operation.id"><td>{{ formatTime(operation.created_at) }}</td><td>{{ resourceLabel(operation.resource_type) }} #{{ operation.resource_id }}</td><td>{{ operation.step }}</td><td><span class="badge" :class="statusClass(operation.status)">{{ statusLabel(operation.status) }}</span></td><td class="cell-secondary">{{ operation.detail || '-' }}</td></tr></tbody>
        </table>
      </div>
      <div v-if="total > pageSize" class="pagination"><button class="btn btn-sm" :disabled="offset === 0" @click="previous">上一页</button><span>{{ currentPage }} / {{ totalPages }}</span><button class="btn btn-sm" :disabled="offset + pageSize >= total" @click="next">下一页</button></div>
    </SurfaceCard>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { RotateCcw, Search } from 'lucide-vue-next'
import { getOperations } from '../api/operations.js'
import { useAsyncResource } from '../composables/useAsyncResource.js'
import EmptyState from '../components/EmptyState.vue'
import FeedbackBanner from '../components/FeedbackBanner.vue'
import SectionTabsHeader from '../components/SectionTabsHeader.vue'
import SurfaceCard from '../components/SurfaceCard.vue'
import WorkspaceHeader from '../components/WorkspaceHeader.vue'
import { formatShortDateTime as formatTime } from '../utils/formatters.js'

const pageSize = 20
const operations = ref([])
const total = ref(0)
const offset = ref(0)
const resourceType = ref('')
const status = ref('')
const keyword = ref('')
const error = ref('')
const operationResource = useAsyncResource(({ signal }, params) => getOperations(params, { signal }), { operations: [], total: 0 })
const loading = operationResource.loading
const currentPage = computed(() => Math.floor(offset.value / pageSize) + 1)
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))
const pageTabs = [{ id: 'history', label: '执行记录' }]

onMounted(() => { void load(true) })

async function load(reset = false) {
  if (reset) offset.value = 0
  error.value = ''
  const result = await operationResource.refresh({ limit: pageSize, offset: offset.value, resourceType: resourceType.value, status: status.value, keyword: keyword.value })
  if (!result) {
    error.value = operationResource.error.value?.message || '加载操作历史失败'
    return
  }
  operations.value = result.operations || []
  total.value = result.total || 0
}
async function resetFilters() {
  resourceType.value = ''
  status.value = ''
  keyword.value = ''
  await load(true)
}
async function previous() { offset.value = Math.max(0, offset.value - pageSize); await load() }
async function next() { offset.value += pageSize; await load() }
function resourceLabel(value) { return ({ application: '应用', server: '服务器', platform: '平台' })[value] || value }
function statusLabel(value) { return ({ running: '执行中', success: '成功', failed: '失败' })[value] || value }
function statusClass(value) { return ({ running: 'badge-deploying', success: 'badge-online', failed: 'badge-danger' })[value] || 'badge-offline' }
</script>

<style scoped>
.operation-page { display: grid; gap: var(--space-4); }
.operation-summary { color: var(--text-secondary); font-size: 13px; }
.operation-card { display: block; }
.operation-table-toolbar { padding: 14px var(--space-20); }
.operation-filters { display: grid; grid-template-columns: minmax(220px, 1fr) 150px 130px auto 34px; gap: 8px; align-items: center; }
.operation-search-field { display: flex; min-width: 0; align-items: center; gap: 8px; min-height: 36px; color: var(--text-muted); }
.operation-search-field:focus-within { color: var(--action-primary); }
.operation-search { flex: 1; min-width: 0; min-height: 36px; padding: 7px 0; border: 0; border-radius: 0; background: transparent; box-shadow: none; outline: 0; }
.operation-search:focus, .operation-search:focus-visible { box-shadow: none; outline: 0; }
.operation-filters :deep(.select-menu-trigger) { min-height: 36px; }
.operation-reset { width: 34px; height: 34px; }
.operation-card > .empty-state { padding: var(--space-24); }
.operation-card > .table-wrap { padding: 0 var(--space-20); }
.operation-card > .pagination { padding: 0 var(--space-20) var(--space-16); }
.sr-only { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0, 0, 0, 0); white-space: nowrap; border: 0; }
@media (max-width: 760px) { .operation-table-toolbar { padding: 12px 14px; }.operation-filters { grid-template-columns: 1fr; }.operation-card > .table-wrap, .operation-card > .pagination { padding-right: 14px; padding-left: 14px; } }
</style>
