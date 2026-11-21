<template>
  <section class="alert-history">
    <SectionTabsHeader title="告警记录" :tabs="tabs" :active-tab="view" :sticky="false" @select="selectView">
      <template #title><h2 class="history-title">告警记录</h2></template>
      <template #actions><span class="history-summary">共 {{ view === 'rules' ? (ruleManager?.total || 0) : total }} {{ response?.view === 'frequency' && view !== 'rules' ? '组' : view === 'rules' ? '条' : '次' }}</span><button class="icon-button" title="刷新告警记录" aria-label="刷新告警记录" :disabled="refreshing" @click="refresh"><RefreshCw :size="15" :class="{ 'is-spinning': refreshing }" /></button></template>
    </SectionTabsHeader>
    <ErrorNoticeModal :open="!!error" title="读取告警记录失败" :message="error?.message || '读取告警记录失败'" @close="error = null" />
    <AlertSuppressionRules v-if="view === 'rules'" ref="ruleManager" :cluster-id="clusterId" />
    <SurfaceCard v-else class="history-card" padding="none">
      <form class="history-toolbar" @submit.prevent="load">
        <input v-model.trim="node" class="form-input" aria-label="历史告警节点" placeholder="节点名称（精确匹配）" />
        <SelectMenu v-model="alertName" class="history-select" aria-label="历史告警类型"><option value="">全部类型</option><option v-for="rule in ruleOptions" :key="rule.id" :value="rule.id">{{ rule.label }}</option></SelectMenu>
        <SelectMenu v-model="days" class="history-select" aria-label="历史告警时间范围"><option value="1">最近 24 小时</option><option value="7">最近 7 天</option><option value="30">最近 30 天</option></SelectMenu>
        <SelectMenu v-model="status" class="history-select" aria-label="历史告警状态"><option value="">全部状态</option><option value="firing">待恢复</option><option value="resolved">已恢复</option></SelectMenu>
        <button class="btn btn-primary" :disabled="loading">筛选</button>
        <button class="icon-button" type="button" title="重置告警历史筛选" aria-label="重置告警历史筛选" @click="resetFilters"><RotateCcw :size="15" /></button>
      </form>
      <EmptyState v-if="loading && !items.length" variant="loading" message="正在读取告警记录..." />
      <EmptyState v-else-if="!items.length" message="暂无告警记录" description="从启用历史存储后收到的告警回调开始记录，早期未保存的记录无法补回。" />
      <div v-else class="table-wrap" :aria-busy="loading">
        <table v-if="response?.view === 'frequency'" class="data-table">
          <thead><tr><th>节点</th><th>告警类型</th><th>触发次数</th><th>最近触发</th><th>操作</th></tr></thead>
          <tbody><tr v-for="item in items" :key="`${item.node_name}:${item.alert_name}`"><td>{{ item.node_name || '—' }}</td><td><span class="cell-primary">{{ item.rule_name || ruleLabel(item.alert_name) }}</span><span class="cell-secondary history-detail">{{ item.alert_name }}</span></td><td>{{ item.count }}</td><td>{{ formatTime(item.last_seen) }}</td><td><button class="btn btn-sm" @click="createRule(item)">屏蔽此类</button></td></tr></tbody>
        </table>
        <table v-else class="data-table">
          <thead><tr><th>开始时间</th><th>节点 / 对象</th><th>告警</th><th>严重程度</th><th>状态</th><th>恢复时间</th><th>通知</th><th>操作</th></tr></thead>
          <tbody><tr v-for="item in items" :key="item.id"><td>{{ formatTime(item.starts_at) }}</td><td>{{ item.node_name || '—' }}</td><td><span class="cell-primary">{{ item.rule_name || ruleLabel(item.alert_name) }}</span><span class="cell-secondary history-detail">{{ item.summary || item.alert_name }}</span></td><td><span class="badge" :class="item.severity === 'critical' ? 'badge-danger' : 'badge-deploying'">{{ item.severity === 'critical' ? '严重' : item.severity === 'warning' ? '警告' : item.severity || '—' }}</span></td><td><span class="badge" :class="item.status === 'resolved' ? 'badge-online' : 'badge-offline'">{{ item.status === 'resolved' ? '已恢复' : '待恢复' }}</span></td><td>{{ item.ends_at ? formatTime(item.ends_at) : '—' }}</td><td>{{ item.notification_suppressed ? '最近回调已屏蔽' : '—' }}</td><td><button class="btn btn-sm" @click="createRule(item)">屏蔽此类</button></td></tr></tbody>
        </table>
      </div>
      <PaginationControls :page="page" :page-size="pageSize" :total="total" :loading="loading" label="告警记录分页" @page-change="changePage" @page-size-change="changePageSize" />
      <p class="history-note">按告警开始时间筛选；同一次告警的重复通知仅计一次，频次按触发次数降序排列。平台屏蔽通知的告警仍计入；Alertmanager 静默期间未发送的回调不计入。待恢复表示尚未收到恢复回调，当前状态请以正在告警列表为准。</p>
    </SurfaceCard>
  </section>
</template>

<script setup>
import { computed, nextTick, ref, watch } from 'vue'
import AlertSuppressionRules from './AlertSuppressionRules.vue'
import { alertRuleOptions as ruleOptions, alertRuleLabel as ruleLabel } from './alertRules.js'
import { RefreshCw, RotateCcw } from 'lucide-vue-next'
import { getAlertingHistory } from '../../api/alerting.js'
import { usePaginatedResource } from '../../composables/usePaginatedResource.js'
import SectionTabsHeader from '../../components/SectionTabsHeader.vue'
import SurfaceCard from '../../components/SurfaceCard.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import EmptyState from '../../components/EmptyState.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import PaginationControls from '../../components/PaginationControls.vue'
import { formatDateTime as formatTime } from '../../utils/formatters.js'

const props = defineProps({ clusterId: { type: [String, Number], default: '' } })
const view = ref('history'), node = ref(''), alertName = ref(''), days = ref('7'), status = ref('')
const tabs = [{ id: 'history', label: '历史告警' }, { id: 'frequency', label: '频次统计' }, { id: 'rules', label: '屏蔽规则' }]
const pagination = usePaginatedResource(async (filters, options) => ({ ...await getAlertingHistory(filters, options), view: filters.view }), 'items')
const { items, total, page, pageSize, loading, error, response, changePage, changePageSize } = pagination
const ruleManager = ref(null)
const refreshing = computed(() => view.value === 'rules' ? !!ruleManager.value?.loading : loading.value)
function refresh() { return view.value === 'rules' ? ruleManager.value?.refresh() : pagination.refresh() }
async function createRule(item) { view.value = 'rules'; await nextTick(); ruleManager.value?.open({ node_name: item.node_name, alert_names: [item.alert_name], name: `${item.node_name || '全部节点'} · ${ruleLabel(item.alert_name)}`.slice(0, 100) }) }
function load() { return pagination.load({ clusterID: props.clusterId, view: view.value, node: node.value, alertName: alertName.value, days: days.value, status: status.value }) }
function selectView(value) { view.value = value; if (value !== 'rules') void load() }
function resetFilters() { node.value = ''; alertName.value = ''; days.value = '7'; status.value = ''; void load() }
watch(() => props.clusterId, () => { pagination.reset(); node.value = ''; alertName.value = ''; days.value = '7'; status.value = ''; void load() }, { immediate: true })
defineExpose({ refresh })
</script>

<style scoped>
.alert-history{margin-top:var(--space-24);min-width:0}.alert-history :deep(.section-tabs-header){position:relative}.history-title{margin:0 0 23px;font-size:18px}.alert-history :deep(.section-tabs-actions){align-items:center;gap:8px}.history-summary{color:var(--text-secondary);font-size:12px;white-space:nowrap}.history-card{display:block}.history-toolbar{display:flex;align-items:center;gap:8px;padding:14px var(--space-20)}.history-toolbar .form-input{min-width:180px;min-height:var(--button-height);flex:1;font-size:12px}.history-toolbar .history-select{width:150px}.history-toolbar .btn,.history-toolbar .icon-button{flex-shrink:0}.history-card>.table-wrap{padding:0 var(--space-20)}.history-card .data-table{min-width:700px}.history-detail{display:block;margin-top:4px}.history-note{margin:0;padding:12px var(--space-20);border-top:1px solid var(--border-muted);color:var(--text-secondary);font-size:12px;line-height:1.6}@media(max-width:900px){.history-toolbar{flex-wrap:wrap}}@media(max-width:640px){.history-title{margin-bottom:var(--space-12)}.history-toolbar{flex-direction:column;align-items:stretch;padding:12px 14px}.history-toolbar .form-input,.history-toolbar .history-select{width:100%}.history-card>.table-wrap{padding:0 14px}.history-note{padding:12px 14px}}
</style>
