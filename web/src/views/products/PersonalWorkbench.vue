<template>
  <div class="product-page">
    <SectionTabsHeader title="个人工作台" :tabs="[{ id: 'overview', label: '产品与行动' }]" active-tab="overview"><template #actions><div class="product-actions"><router-link class="btn btn-sm" to="/products">产品中心</router-link><button class="icon-button" title="刷新工作台" aria-label="刷新工作台" :disabled="resource.loading.value" @click="load"><RefreshCw :size="15" /></button></div></template></SectionTabsHeader>
    <EmptyState v-if="resource.loading.value" variant="loading" message="正在读取工作台..." />
    <EmptyState v-else-if="resource.error.value" message="工作台读取失败"><template #action><button class="btn btn-sm" @click="load">重试</button></template></EmptyState>
    <div v-else class="product-workbench-content">
      <div class="product-stat-grid">
        <SurfaceCard class="product-stat"><strong>{{ summary.active.length }}</strong><span>活跃产品</span></SurfaceCard>
        <SurfaceCard class="product-stat"><strong>{{ summary.actions.length }}</strong><span>下一步行动</span></SurfaceCard>
        <SurfaceCard class="product-stat"><strong :class="{ 'product-warning': summary.due.length }">{{ summary.due.length }}</strong><span>逾期或 7 天内到期</span></SurfaceCard>
        <SurfaceCard class="product-stat"><strong :class="{ 'product-warning': summary.attention.length }">{{ summary.attention.length }}</strong><span>发行关注项</span></SurfaceCard>
      </div>
      <SurfaceCard class="product-table-card" padding="none">
        <div class="product-toolbar"><h2 class="product-workbench-heading">下一步行动</h2><SelectMenu v-model="actionFilter" aria-label="行动范围" class="product-stage-filter"><option value="all">全部行动</option><option value="due">临近与逾期</option></SelectMenu></div>
        <EmptyState v-if="!actions.length" :message="actionFilter === 'due' ? '暂无临近或逾期行动' : '暂无下一步行动，在产品详情中设置'" />
        <div v-else class="table-wrap"><table class="data-table"><thead><tr><th>产品</th><th>下一步行动</th><th>截止日期</th><th>提醒</th></tr></thead><tbody><tr v-for="product in actions" :key="product.id"><td><router-link :to="`/products/${product.id}`">{{ product.name }}</router-link></td><td>{{ product.next_action }}</td><td>{{ dateOnly(product.next_due_date) || '—' }}</td><td><span class="badge" :class="dueClass(dueState(product.next_due_date, now))">{{ dueLabel(dueState(product.next_due_date, now)) }}</span></td></tr></tbody></table></div>
      </SurfaceCard>
      <SurfaceCard class="product-table-card" padding="none">
        <div class="product-toolbar"><h2 class="product-workbench-heading">需要关注的发行</h2><span class="product-muted">手工维护 · 按记录状态汇总</span></div>
        <EmptyState v-if="!summary.attention.length" message="暂无待发布、审核中或受阻的发行渠道" />
        <div v-else class="table-wrap"><table class="data-table"><thead><tr><th>产品 / 渠道</th><th>状态</th><th>线上 → 待发布</th><th>记录更新时间</th></tr></thead><tbody><tr v-for="{ product, channel } in summary.attention" :key="channel.id"><td><router-link :to="`/products/${product.id}`">{{ product.name }}</router-link><span class="cell-secondary product-line">{{ channel.name || labelFor(platforms, channel.platform) }}</span></td><td><span class="badge" :class="channelClass(channel.publish_status)">{{ labelFor(channelStatuses, channel.publish_status) }}</span></td><td>{{ channel.online_version || '—' }} → {{ channel.pending_version || '—' }}</td><td>{{ formatTime(channel.updated_at) }}</td></tr></tbody></table></div>
      </SurfaceCard>
      <SurfaceCard class="product-table-card" padding="none">
        <div class="product-toolbar"><h2 class="product-workbench-heading">活跃产品</h2><span class="product-muted">想法、开发中与运营中</span></div>
        <EmptyState v-if="!summary.active.length" message="还没有活跃产品"><template #action><router-link class="btn btn-primary" to="/products">创建产品</router-link></template></EmptyState>
        <div v-else class="table-wrap"><table class="data-table"><thead><tr><th>产品</th><th>简介</th><th>阶段</th></tr></thead><tbody><tr v-for="product in summary.active" :key="product.id"><td><router-link :to="`/products/${product.id}`">{{ product.name }}</router-link></td><td class="cell-secondary">{{ product.summary || '—' }}</td><td><span class="badge" :class="stageClass(product.stage)">{{ labelFor(stages, product.stage) }}</span></td></tr></tbody></table></div>
      </SurfaceCard>
    </div>
    <ErrorNoticeModal :open="Boolean(error)" :message="error" teleport @close="error = ''" />
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { RefreshCw } from 'lucide-vue-next'
import { getProducts } from '../../api/products.js'
import { useAsyncResource } from '../../composables/useAsyncResource.js'
import SectionTabsHeader from '../../components/SectionTabsHeader.vue'
import SurfaceCard from '../../components/SurfaceCard.vue'
import EmptyState from '../../components/EmptyState.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import { formatShortDateTime as formatTime } from '../../utils/formatters.js'
import { workbenchSummary, dueState, dueLabel, dueClass, dateOnly, stages, stageClass, platforms, channelStatuses, channelClass, labelFor } from './product-utils.js'
import '../../styles/products.css'

const resource = useAsyncResource(getProducts, [])
const error = ref(''), actionFilter = ref('all'), now = ref(new Date())
const summary = computed(() => workbenchSummary(resource.data.value || [], now.value))
const actions = computed(() => actionFilter.value === 'due' ? summary.value.due : summary.value.actions)
let timer
async function load() { now.value = new Date(); await resource.refresh(); if (resource.error.value) error.value = resource.error.value.message }
onMounted(() => { void load(); timer = setInterval(() => { now.value = new Date() }, 60000) })
onBeforeUnmount(() => clearInterval(timer))
</script>

<style scoped>
.product-workbench-content { display: grid; gap: var(--space-16); min-width: 0; }
.product-workbench-heading { margin: 0; font-size: 14px; }
</style>
