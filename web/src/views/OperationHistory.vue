<template>
  <div class="operation-page">
    <SectionTabsHeader title="操作历史" :tabs="pageTabs" active-tab="history" test-id-prefix="operation-page">
      <template #actions><span class="operation-summary">共 {{ total }} 条</span></template>
    </SectionTabsHeader>
    <FeedbackBanner v-if="error" tone="warning" :message="error" />
    <section class="card operation-card">
      <div class="operation-filters">
        <SelectMenu v-model="resourceType" class="form-select" aria-label="资源类型">
          <option value="">全部资源</option><option value="application">应用</option><option value="server">服务器</option><option value="platform">平台</option>
        </SelectMenu>
        <SelectMenu v-model="status" class="form-select" aria-label="执行状态">
          <option value="">全部状态</option><option value="running">执行中</option><option value="success">成功</option><option value="failed">失败</option>
        </SelectMenu>
        <input v-model.trim="keyword" class="form-input" placeholder="搜索步骤或详情" @keydown.enter.prevent="load(true)" />
        <button class="btn btn-primary" @click="load(true)">筛选</button>
      </div>
      <EmptyState v-if="loading" variant="loading" message="加载中..." />
      <EmptyState v-else-if="operations.length === 0" message="暂无操作历史" />
      <div v-else class="table-wrap">
        <table class="data-table"><thead><tr><th>时间</th><th>流程 / 资源</th><th>步骤</th><th>状态</th><th>详情</th></tr></thead>
          <tbody><tr v-for="operation in operations" :key="operation.id"><td>{{ formatTime(operation.created_at) }}</td><td>{{ resourceLabel(operation.resource_type) }} #{{ operation.resource_id }}</td><td>{{ operation.step }}</td><td><span class="badge" :class="statusClass(operation.status)">{{ statusLabel(operation.status) }}</span></td><td class="cell-secondary">{{ operation.detail || '-' }}</td></tr></tbody>
        </table>
      </div>
      <div v-if="total > pageSize" class="pagination"><button class="btn btn-sm" :disabled="offset === 0" @click="previous">上一页</button><span>{{ currentPage }} / {{ totalPages }}</span><button class="btn btn-sm" :disabled="offset + pageSize >= total" @click="next">下一页</button></div>
    </section>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { getOperations } from '../api/operations.js'
import { useAsyncResource } from '../composables/useAsyncResource.js'
import EmptyState from '../components/EmptyState.vue'
import FeedbackBanner from '../components/FeedbackBanner.vue'
import SectionTabsHeader from '../components/SectionTabsHeader.vue'
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
async function previous() { offset.value = Math.max(0, offset.value - pageSize); await load() }
async function next() { offset.value += pageSize; await load() }
function resourceLabel(value) { return ({ application: '应用', server: '服务器', platform: '平台' })[value] || value }
function statusLabel(value) { return ({ running: '执行中', success: '成功', failed: '失败' })[value] || value }
function statusClass(value) { return ({ running: 'badge-deploying', success: 'badge-online', failed: 'badge-danger' })[value] || 'badge-offline' }
</script>

<style scoped>
.operation-page { display: grid; gap: var(--space-4); }
.operation-summary { color: var(--text-secondary); font-size: 13px; }
.operation-card { display: grid; gap: var(--space-16); }
.operation-filters { display: grid; grid-template-columns: 180px 140px minmax(180px, 1fr) auto; gap: 10px; }
@media (max-width: 760px) { .operation-filters { grid-template-columns: 1fr; } }
</style>
