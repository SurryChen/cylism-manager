<template>
  <div class="product-detail-content">
    <SurfaceCard v-if="kind === 'tasks' && product.next_action" class="product-table-card" padding="none">
      <div class="product-toolbar"><span class="product-muted">已有下一步：{{ product.next_action }}{{ product.next_due_date ? ` · ${dateOnly(product.next_due_date)}` : '' }}</span><button class="btn btn-sm product-primary" :disabled="busy" @click="importAction">转为事项</button></div>
    </SurfaceCard>
    <SurfaceCard class="product-table-card" padding="none">
      <div class="product-toolbar">
        <input v-model.trim="keyword" class="form-input product-search" :aria-label="`搜索${noun}`" :placeholder="`搜索${noun}`" />
        <SelectMenu v-model="status" class="product-stage-filter" aria-label="记录状态筛选"><option value="">全部状态</option><option v-for="item in statuses" :key="item.id" :value="item.id">{{ item.label }}</option></SelectMenu>
        <SelectMenu v-if="kind !== 'versions'" v-model="versionID" class="product-stage-filter" aria-label="记录版本筛选"><option value="">全部版本</option><option v-for="version in versions" :key="version.id" :value="version.id">{{ version.name }}</option></SelectMenu>
        <button class="icon-button" title="重置筛选" aria-label="重置筛选" @click="keyword = ''; status = ''; versionID = ''"><RotateCcw :size="15" /></button>
        <button v-if="kind === 'distributions'" class="btn" @click="$emit('show-channels')">发行渠道</button>
        <button class="btn btn-primary product-primary" :disabled="busy || optionsLoading || Boolean(optionsError)" @click="openEditor()">新增{{ noun }}</button>
      </div>
      <EmptyState v-if="optionsError" message="版本选项读取失败"><template #action><button class="btn btn-sm" @click="loadOptions">重试</button></template></EmptyState>
      <EmptyState v-else-if="resource.loading.value && !resource.items.value.length" variant="loading" message="正在读取记录..." />
      <EmptyState v-else-if="resource.error.value" message="记录读取失败"><template #action><button class="btn btn-sm" @click="load">重试</button></template></EmptyState>
      <EmptyState v-else-if="!resource.total.value" :message="emptyMessage" />
      <div v-else class="table-wrap">
        <table v-if="kind === 'tasks'" class="data-table">
          <thead><tr><th>事项 / 说明</th><th>类型 / 优先级</th><th>产品版本</th><th>状态</th><th>截止日期</th><th>操作</th></tr></thead>
          <tbody><tr v-for="row in resource.items.value" :key="row.id"><td>{{ row.title }}<span class="cell-secondary product-line lifecycle-description">{{ row.details }}</span></td><td>{{ labelFor(taskKinds, row.kind) }} · {{ labelFor(priorities, row.priority) }}</td><td>{{ versionName(row.version_id) }}</td><td><span class="badge" :class="lifecycleClass(row.status)">{{ labelFor(taskStatuses, row.status) }}</span></td><td>{{ row.due_date || '—' }}<span v-if="row.due_date && !['done', 'cancelled'].includes(row.status)" class="badge product-line" :class="dueClass(dueState(row.due_date))">{{ dueLabel(dueState(row.due_date)) }}</span></td><td><button class="icon-button" title="编辑事项" aria-label="编辑事项" :disabled="busy || optionsLoading || Boolean(optionsError)" @click="openEditor(row)"><Pencil :size="15" /></button></td></tr></tbody>
        </table>
        <table v-else-if="kind === 'versions'" class="data-table">
          <thead><tr><th>版本 / 目标</th><th>状态</th><th>关联部署</th><th>操作</th></tr></thead>
          <tbody>
            <tr v-for="row in resource.items.value" :key="row.id">
              <td class="lifecycle-version-cell">{{ row.name }}<span v-if="row.goal" class="cell-secondary product-line lifecycle-version-goal">{{ row.goal }}</span></td>
              <td><span class="badge" :class="lifecycleClass(row.status)">{{ labelFor(versionStatuses, row.status) }}</span><span v-if="row.target_date" class="cell-secondary product-line">目标 {{ row.target_date }}</span></td>
              <td><button v-if="deploymentCount(row)" class="btn btn-sm" @click="showDeployments(row)">{{ deploymentCount(row) }} 条记录</button><span v-else class="cell-secondary">—</span></td>
              <td><div class="product-actions"><button class="icon-button" title="查看版本详情" aria-label="查看版本详情" @click="detailVersion = row"><Eye :size="15" /></button><button class="icon-button" title="查看版本事项" aria-label="查看版本事项" @click="$emit('select-version', row.id)"><ListTodo :size="15" /></button><button class="icon-button" title="编辑版本" aria-label="编辑版本" :disabled="busy || optionsLoading || Boolean(optionsError)" @click="openEditor(row)"><Pencil :size="15" /></button></div></td>
            </tr>
          </tbody>
        </table>
        <table v-else class="data-table">
          <thead><tr><th>产品版本</th><th>发行渠道</th><th>平台版本 / Build</th><th>状态</th><th>更新时间 / 备注</th><th>操作</th></tr></thead>
          <tbody><tr v-for="row in resource.items.value" :key="row.id"><td>{{ versionName(row.version_id) }}</td><td>{{ channelName(row.channel_id) }}</td><td>{{ row.platform_version }}<span class="cell-secondary product-line">{{ row.build_identifier || '未填 Build 标识' }}</span></td><td><span class="badge" :class="lifecycleClass(row.status)">{{ labelFor(distributionStatuses, row.status) }}</span></td><td>{{ formatTime(row.updated_at) }}<span class="cell-secondary product-line lifecycle-description">{{ row.notes }}</span></td><td><div class="product-actions"><button class="icon-button" title="发行历史" aria-label="发行历史" @click="showHistory(row)"><History :size="15" /></button><button class="icon-button" title="编辑发行记录" aria-label="编辑发行记录" :disabled="busy || optionsLoading || Boolean(optionsError)" @click="openEditor(row)"><Pencil :size="15" /></button></div></td></tr></tbody>
        </table>
      </div>
      <ResourcePagination :resource="resource" :label="`${noun}分页`" />
    </SurfaceCard>
    <LifecycleEditor :open="editorOpen" :busy="busy" :kind="kind" :record="editing" :product-id="product.id" :versions="versions" :channels="product.channels || []" @close="editorOpen = false" @save="save" @error="error = $event" />
    <BaseModal teleport :open="Boolean(detailVersion)" :title="`版本详情 · ${detailVersion?.name || ''}`" size="large" @close="detailVersion = null">
      <dl v-if="detailVersion" class="product-detail-list">
        <div><dt>状态</dt><dd><span class="badge" :class="lifecycleClass(detailVersion.status)">{{ labelFor(versionStatuses, detailVersion.status) }}</span></dd></div>
        <div v-if="detailVersion.target_date"><dt>目标日期</dt><dd>{{ detailVersion.target_date }}</dd></div>
        <div v-if="detailVersion.goal"><dt>本次目标</dt><dd>{{ detailVersion.goal }}</dd></div>
        <div v-if="detailVersion.acceptance"><dt>验收项与验证结果</dt><dd>{{ detailVersion.acceptance }}</dd></div>
        <div v-if="detailVersion.notes"><dt>备注 / 发行说明</dt><dd>{{ detailVersion.notes }}</dd></div>
        <div v-if="detailVersion.repository_url"><dt>代码仓库</dt><dd><a :href="detailVersion.repository_url" target="_blank" rel="noopener noreferrer">{{ detailVersion.repository_url }}</a></dd></div>
        <div v-if="detailVersion.commit_sha"><dt>提交 SHA</dt><dd>{{ detailVersion.commit_sha }}</dd></div>
        <div v-if="detailVersion.build_url"><dt>构建记录</dt><dd><a :href="detailVersion.build_url" target="_blank" rel="noopener noreferrer">{{ detailVersion.build_url }}</a></dd></div>
        <div v-if="detailVersion.artifact_url"><dt>构建产物</dt><dd><a :href="detailVersion.artifact_url" target="_blank" rel="noopener noreferrer">{{ detailVersion.artifact_url }}</a></dd></div>
      </dl>
    </BaseModal>
    <BaseModal teleport :open="Boolean(historyRecord)" title="发行状态历史" size="large" @close="historyRecord = null">
      <EmptyState v-if="history.loading.value" variant="loading" message="正在读取历史..." />
      <EmptyState v-else-if="history.error.value" message="历史读取失败"><template #action><button class="btn btn-sm" @click="history.refresh()">重试</button></template></EmptyState>
      <div v-else class="table-wrap"><table class="data-table"><thead><tr><th>记录时间</th><th>状态</th><th>平台版本 / Build</th><th>备注</th></tr></thead><tbody><tr v-for="event in history.items.value" :key="event.id"><td>{{ formatTime(event.created_at) }}</td><td>{{ labelFor(distributionStatuses, event.status) }}</td><td>{{ event.platform_version }} / {{ event.build_identifier || '—' }}</td><td class="lifecycle-description">{{ event.notes || '—' }}</td></tr></tbody></table></div>
      <ResourcePagination :resource="history" label="发行历史分页" />
    </BaseModal>
    <BaseModal teleport :open="Boolean(deploymentVersion)" title="关联部署记录" size="large" @close="deploymentVersion = null">
      <EmptyState v-if="deploymentRows.loading.value" variant="loading" message="正在读取部署..." />
      <EmptyState v-else-if="deploymentRows.error.value" message="部署读取失败"><template #action><button class="btn btn-sm" @click="deploymentRows.refresh()">重试</button></template></EmptyState>
      <div v-else class="table-wrap"><table class="data-table"><thead><tr><th>组件 / 环境</th><th>版本</th><th>状态</th><th>入口</th></tr></thead><tbody><tr v-for="row in deploymentRows.items.value" :key="(row.source || 'application') + '-' + row.id"><td>{{ row.application_name }} · {{ row.environment_name || '—' }}</td><td>{{ row.version || `#${row.id}` }}</td><td>{{ row.status }}</td><td><router-link :to="deploymentPath(row)">{{ row.source === 'platform' ? '平台发布与更新' : '查看部署记录' }}</router-link></td></tr></tbody></table></div>
      <ResourcePagination :resource="deploymentRows" label="关联部署分页" />
    </BaseModal>
    <ErrorNoticeModal :open="Boolean(error)" :message="error" teleport @close="error = ''" />
  </div>
</template>
<script setup>
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { Eye, History, ListTodo, Pencil, RotateCcw } from 'lucide-vue-next'
import SurfaceCard from '../../components/SurfaceCard.vue'
import EmptyState from '../../components/EmptyState.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import BaseModal from '../../components/BaseModal.vue'
import ResourcePagination from '../../components/ResourcePagination.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import LifecycleEditor from './LifecycleEditor.vue'
import { getLifecyclePage, getLifecycleOptions, getDistributionEvents, saveLifecycleRecord, importProductNextAction } from '../../api/product-lifecycle.js'
import { usePaginatedResource } from '../../composables/usePaginatedResource.js'
import { labelFor, platforms, dateOnly, dueState, dueClass, dueLabel } from './product-utils.js'
import { taskKinds, taskStatuses, priorities, versionStatuses, distributionStatuses, lifecycleClass, deploymentCount, deploymentPath } from './lifecycle-utils.js'
import { formatShortDateTime as formatTime } from '../../utils/formatters.js'
const props = defineProps({ product: { type: Object, required: true }, kind: { type: String, required: true }, initialVersionId: { type: [Number, String], default: '' } })
const emit = defineEmits(['changed', 'select-version', 'show-channels'])
const keyword = ref(''), status = ref(''), versionID = ref(props.initialVersionId), versions = ref([]), optionsLoading = ref(false), optionsError = ref(''), busy = ref(false), error = ref(''), editorOpen = ref(false), editing = ref(null), historyRecord = ref(null), deploymentVersion = ref(null)
const detailVersion = ref(null)
const noun = computed(() => ({ tasks: '事项', versions: '版本', distributions: '发行记录' })[props.kind])
const statuses = computed(() => ({ tasks: taskStatuses, versions: versionStatuses, distributions: distributionStatuses })[props.kind])
const emptyMessage = computed(() => keyword.value || status.value || versionID.value ? '没有符合条件的记录' : props.kind === 'distributions' ? '暂无发行记录，请先创建产品版本并配置发行渠道' : `暂无${noun.value}，可先新增一条`)
const resource = usePaginatedResource((query, options) => getLifecyclePage(props.product.id, props.kind, query, options), 'items')
const history = usePaginatedResource((query, options) => getDistributionEvents(props.product.id, historyRecord.value.id, query, options), 'items')
const deploymentRows = usePaginatedResource((query, options) => getLifecyclePage(props.product.id, 'deployments', { ...query, version_id: deploymentVersion.value.id }, options), 'items')
const versionName = id => versions.value.find(version => version.id === id)?.name || (id ? `版本 #${id}` : '未安排版本')
const channelName = id => { const channel = props.product.channels?.find(item => item.id === id); return channel ? channel.name || labelFor(platforms, channel.platform) : `渠道 #${id}` }
let optionsRequest = 0
async function loadOptions() {
  const request = ++optionsRequest; optionsLoading.value = true; optionsError.value = ''
  try { const data = await getLifecycleOptions(props.product.id); if (request === optionsRequest) versions.value = data.versions }
  catch (cause) { if (request === optionsRequest) { optionsError.value = cause.message; error.value = cause.message } }
  finally { if (request === optionsRequest) optionsLoading.value = false }
}
function load() { return resource.load({ keyword: keyword.value, status: status.value, ...(versionID.value && props.kind !== 'versions' ? { version_id: versionID.value } : {}) }) }
function openEditor(row = null) {
  if (props.kind === 'distributions' && (!props.product.channels?.length || !versions.value.length)) { error.value = '请先在版本页新增产品版本，并在发行渠道页配置渠道'; return }
  editing.value = row; editorOpen.value = true
}
async function save(record) {
  busy.value = true
  try { await saveLifecycleRecord(props.product.id, props.kind, record); editorOpen.value = false; await Promise.all([resource.refresh(), loadOptions()]); emit('changed') }
  catch (cause) { error.value = cause.message } finally { busy.value = false }
}
async function importAction() { busy.value = true; try { await importProductNextAction(props.product.id); await resource.refresh(); emit('changed') } catch (cause) { error.value = cause.message } finally { busy.value = false } }
function showHistory(row) { history.reset(); historyRecord.value = row; void history.load() }
function showDeployments(row) { deploymentRows.reset(); deploymentVersion.value = row; void deploymentRows.load() }
watch([keyword, status, versionID], load)
watch(() => props.initialVersionId, value => { versionID.value = value })
watch(() => [props.product.id, props.kind], () => { detailVersion.value = null; resource.reset(); void load(); void loadOptions() }, { immediate: true })
onBeforeUnmount(() => { optionsRequest++; resource.cancel(); history.cancel(); deploymentRows.cancel() })
</script>
<style scoped>
.product-detail-content { display: grid; gap: var(--space-16); min-width: 0; }
.lifecycle-description { max-width: 320px; white-space: pre-wrap; overflow-wrap: anywhere; }
.lifecycle-version-cell { width: 50%; max-width: 480px; overflow-wrap: anywhere; }
.lifecycle-version-goal { display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; overflow: hidden; white-space: pre-wrap; overflow-wrap: anywhere; }
</style>
