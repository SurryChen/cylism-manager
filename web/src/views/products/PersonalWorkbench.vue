<template>
  <div class="product-page">
    <SectionTabsHeader title="研发工作台" :tabs="[{ id: 'overview', label: '总览' }]" active-tab="overview"><template #actions><div class="product-actions"><router-link class="btn btn-sm" to="/projects">项目中心</router-link><button class="icon-button" title="刷新工作台" aria-label="刷新工作台" :disabled="resource.loading.value" @click="load"><RefreshCw :size="15" /></button></div></template></SectionTabsHeader>
    <EmptyState v-if="resource.loading.value" variant="loading" message="正在读取工作台..." />
    <EmptyState v-else-if="resource.error.value" message="工作台读取失败"><template #action><button class="btn btn-sm" @click="load">重试</button></template></EmptyState>
    <div v-else class="product-workbench-content">
      <div class="product-stat-grid">
        <SurfaceCard class="product-stat"><strong>{{ summary.active.length }}</strong><span>活跃项目</span></SurfaceCard>
        <SurfaceCard class="product-stat"><strong>{{ summary.actions.length }}</strong><span>待办与进行中事项</span></SurfaceCard>
        <SurfaceCard class="product-stat"><strong :class="{ 'product-warning': summary.due.length }">{{ summary.due.length }}</strong><span>逾期或 7 天内到期</span></SurfaceCard>
        <SurfaceCard class="product-stat"><strong :class="{ 'product-warning': summary.needsAction.length }">{{ summary.needsAction.length }}</strong><span>发行待处理 · {{ summary.waiting.length }} 项等待审核</span></SurfaceCard>
      </div>
      <SurfaceCard class="product-table-card" padding="none">
        <div class="product-toolbar"><h2 class="product-workbench-heading">下一步行动</h2><SelectMenu v-model="actionFilter" aria-label="行动范围" class="product-stage-filter"><option value="all">全部行动</option><option value="due">临近与逾期</option><option value="blocked">受阻事项</option></SelectMenu></div>
        <EmptyState v-if="!actions.length" :message="actionFilter === 'due' ? '暂无临近或逾期行动' : '暂无下一步行动，在项目详情中设置'" />
        <div v-else class="table-wrap"><DataTable><thead><tr><th>项目</th><th>事项 / 下一步行动</th><th>状态</th><th>截止日期</th></tr></thead><tbody><tr v-for="task in actionPage.pagedItems.value" :key="task.id"><td><router-link :to="{ path: projectPath(task.product_id), query: { tab: 'tasks' } }">{{ productName(task.product_id) }}</router-link></td><td>{{ task.title }}<span v-if="task.legacy" class="cell-secondary product-line">原下一步行动 · 可在事项页转为事项</span></td><td><span class="badge" :class="lifecycleClass(task.status)">{{ labelFor(taskStatuses, task.status) }}</span></td><td>{{ dateOnly(task.due_date) || '未设截止日期' }}<span v-if="task.due_date" class="badge product-line" :class="dueClass(dueState(task.due_date, now))">{{ dueLabel(dueState(task.due_date, now)) }}</span></td></tr></tbody></DataTable></div>
        <PaginationControls :page="actionPage.page.value" :page-size="actionPage.pageSize.value" :total="actionPage.total.value" @page-change="actionPage.changePage" @page-size-change="actionPage.changePageSize" />
      </SurfaceCard>
      <SurfaceCard class="product-table-card" padding="none">
        <div class="product-toolbar"><h2 class="product-workbench-heading">发行跟进</h2><span class="product-muted">手工记录 · 待处理 {{ summary.needsAction.length }} 项 / 等待审核 {{ summary.waiting.length }} 项</span></div>
        <EmptyState v-if="!summary.followups.length" message="暂无需要跟进的发行" />
        <div v-else class="table-wrap"><DataTable><thead><tr><th>项目 / 渠道</th><th>状态</th><th>渠道版本</th><th>建议下一步</th></tr></thead><tbody><tr v-for="record in distributionPage.pagedItems.value" :key="record.id"><td><router-link :to="{ path: projectPath(record.product_id), query: { tab: record.legacy ? 'channels' : 'distributions' } }">{{ productName(record.product_id) }}</router-link><span class="cell-secondary product-line">{{ channelName(record) }}</span></td><td><span class="badge" :class="lifecycleClass(record.status)">{{ labelFor(distributionStatuses, record.status) }}</span></td><td>{{ record.platform_version || '—' }}</td><td>{{ nextStep(record.status) }}</td></tr></tbody></DataTable></div>
        <PaginationControls :page="distributionPage.page.value" :page-size="distributionPage.pageSize.value" :total="distributionPage.total.value" @page-change="distributionPage.changePage" @page-size-change="distributionPage.changePageSize" />
      </SurfaceCard>
      <SurfaceCard class="product-table-card" padding="none">
        <div class="product-toolbar"><h2 class="product-workbench-heading">规划提醒</h2><span class="product-muted">{{ summary.unplanned.length }} 个活跃项目尚无待办行动</span><router-link class="btn btn-sm product-primary" to="/projects">全部项目</router-link></div>
        <EmptyState v-if="!summary.unplanned.length" :message="summary.active.length ? '活跃项目均有待办行动' : '还没有活跃项目'" />
        <div v-else class="table-wrap"><DataTable><thead><tr><th>项目</th><th>阶段</th><th>下一步</th></tr></thead><tbody><tr v-for="product in planningPage.pagedItems.value" :key="product.id"><td><router-link :to="{ path: projectPath(product.id), query: { tab: 'tasks' } }">{{ product.name }}</router-link></td><td>{{ labelFor(stages, product.stage) }}</td><td><router-link :to="{ path: projectPath(product.id), query: { tab: 'tasks' } }">规划事项</router-link></td></tr></tbody></DataTable></div>
        <PaginationControls :page="planningPage.page.value" :page-size="planningPage.pageSize.value" :total="planningPage.total.value" @page-change="planningPage.changePage" @page-size-change="planningPage.changePageSize" />
      </SurfaceCard>
    </div>
    <ErrorNoticeModal :open="Boolean(error)" :message="error" teleport @close="error = ''" />
  </div>
</template>

<script setup>
import DataTable from '../../components/DataTable.vue'
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { RefreshCw } from 'lucide-vue-next'
import { getProductWorkbench } from '../../api/product-lifecycle.js'
import { developmentSummary, taskStatuses, distributionStatuses, lifecycleClass } from './lifecycle-utils.js'
import { useAsyncResource } from '../../composables/useAsyncResource.js'
import { usePagination } from '../../composables/usePagination.js'
import PaginationControls from '../../components/PaginationControls.vue'
import SectionTabsHeader from '../../components/SectionTabsHeader.vue'
import SurfaceCard from '../../components/SurfaceCard.vue'
import EmptyState from '../../components/EmptyState.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import { dueState, dueLabel, dueClass, dateOnly, stages, platforms, labelFor } from './product-utils.js'
import '../../styles/products.css'

const resource = useAsyncResource(getProductWorkbench, {})
const error = ref(''), actionFilter = ref('all'), now = ref(new Date())
const summary = computed(() => developmentSummary(resource.data.value || {}, now.value))
const actions = computed(() => actionFilter.value === 'due' ? summary.value.due : actionFilter.value === 'blocked' ? summary.value.actions.filter(task => task.status === 'blocked') : summary.value.actions)
const projectPath = id => { const product = (resource.data.value?.products || []).find(item => item.id === id); return product?.project_id ? '/projects/' + product.project_id : '/products/' + id }
const productName = id => summary.value.active.find(product => product.id === id)?.name || `产品 #${id}`
function channelName(record) { const channel = summary.value.active.find(product => product.id === record.product_id)?.channels?.find(item => item.id === record.channel_id); return channel ? channel.name || labelFor(platforms, channel.platform) : `渠道 #${record.channel_id}` }
const nextStep = status => ({ rejected: '查看拒绝原因，修正后新增提交记录', blocked: '检查并处理备注中的阻塞原因', approved: '确认发布条件并安排上线', ready: '完成检查后提交或发布', in_review: '等待平台结果，有变化时更新记录' })[status] || '查看发行记录'
const actionPage = usePagination(actions)
const distributionPage = usePagination(computed(() => summary.value.followups))
const planningPage = usePagination(computed(() => summary.value.unplanned))
watch(actionFilter, () => actionPage.changePage(1))
let timer
async function load() { now.value = new Date(); await resource.refresh(); if (resource.error.value) error.value = resource.error.value.message }
onMounted(() => { void load(); timer = setInterval(() => { now.value = new Date() }, 60000) })
onBeforeUnmount(() => clearInterval(timer))
</script>

<style scoped>
.product-workbench-content { display: grid; gap: var(--space-16); min-width: 0; }
.product-workbench-heading { margin: 0; font-size: 14px; }
</style>
