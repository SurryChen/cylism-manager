<template>
  <div class="operation-page">
    <SectionTabsHeader title="操作历史" :tabs="pageTabs" active-tab="history" test-id-prefix="operation-page">
      <template #actions><span class="operation-summary">共 {{ total }} 条</span></template>
    </SectionTabsHeader>
    <ErrorNoticeModal :open="!!error" :message="error?.message || '加载操作历史失败'" title="读取操作历史失败" @close="error = null" />
    <SurfaceCard class="operation-card" padding="none">
      <div class="operation-table-toolbar">
        <div class="operation-filters">
          <label class="operation-search-field">
            <Search :size="15" aria-hidden="true" />
            <span class="sr-only">关键字</span>
            <input v-model.trim="keyword" class="form-input operation-search" placeholder="搜索步骤或详情" @keydown.enter.prevent="load()" />
          </label>
        <SelectMenu v-model="resourceType" class="form-select" aria-label="资源类型">
          <option value="">全部资源</option><option value="application">应用</option><option value="server">服务器</option><option value="platform">平台</option>
        </SelectMenu>
        <SelectMenu v-model="status" class="form-select" aria-label="执行状态">
          <option value="">全部状态</option><option value="running">执行中</option><option value="success">成功</option><option value="failed">失败</option>
        </SelectMenu>
        <button class="btn btn-primary" @click="load()">筛选</button>
        <button class="icon-button operation-reset" type="button" title="重置筛选" aria-label="重置筛选" @click="resetFilters"><RotateCcw :size="15" /></button>
        </div>
      </div>
      <EmptyState v-if="loading && !operations.length" variant="loading" message="加载中..." />
      <EmptyState v-else-if="operations.length === 0" message="暂无操作历史" />
      <div v-else class="table-wrap" :aria-busy="loading">
        <DataTable><thead><tr><th>时间</th><th>流程 / 资源</th><th>步骤</th><th>状态</th><th>详情</th></tr></thead>
          <tbody><tr v-for="operation in operations" :key="operation.id"><td>{{ formatTime(operation.created_at) }}</td><td>{{ resourceLabel(operation.resource_type) }} #{{ operation.resource_id }}</td><td>{{ operation.step }}</td><td><span class="badge" :class="statusClass(operation.status)">{{ statusLabel(operation.status) }}</span></td><td class="cell-secondary">{{ operation.detail || '-' }}</td></tr></tbody>
        </DataTable>
      </div>
      <PaginationControls :page="currentPage" :page-size="pageSize" :total="total" :loading="loading" label="操作历史分页" @page-change="changePage" @page-size-change="changePageSize" />
    </SurfaceCard>
  </div>
</template>

<script setup>
import DataTable from '../components/DataTable.vue'
import { onMounted, ref } from 'vue'
import { RotateCcw, Search } from 'lucide-vue-next'
import { getOperations } from '../api/operations.js'
import { usePaginatedResource } from '../composables/usePaginatedResource.js'
import EmptyState from '../components/EmptyState.vue'
import ErrorNoticeModal from '../components/ErrorNoticeModal.vue'
import PaginationControls from '../components/PaginationControls.vue'
import SelectMenu from '../components/SelectMenu.vue'
import SectionTabsHeader from '../components/SectionTabsHeader.vue'
import SurfaceCard from '../components/SurfaceCard.vue'
import { formatShortDateTime as formatTime } from '../utils/formatters.js'

const resourceType = ref('')
const status = ref('')
const keyword = ref('')
const pagination = usePaginatedResource(getOperations, 'operations')
const { items: operations, total, page: currentPage, pageSize, loading, error, changePage, changePageSize } = pagination
const pageTabs = [{ id: 'history', label: '执行记录' }]

onMounted(() => { void load() })

async function load() {
  await pagination.load({ resourceType: resourceType.value, status: status.value, keyword: keyword.value })
}
async function resetFilters() {
  resourceType.value = ''
  status.value = ''
  keyword.value = ''
  await load()
}
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

.sr-only { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0, 0, 0, 0); white-space: nowrap; border: 0; }
@media (max-width: 760px) { .operation-table-toolbar { padding: 12px 14px; }.operation-filters { grid-template-columns: 1fr; } }
</style>
