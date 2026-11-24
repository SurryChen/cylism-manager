<template>
  <div class="product-detail-content">
    <SurfaceCard v-if="kind === 'tasks' && product.next_action" class="product-table-card" padding="none">
      <div class="product-toolbar"><span class="product-muted">已有下一步：{{ product.next_action }}{{ product.next_due_date ? ` · ${dateOnly(product.next_due_date)}` : '' }}</span><button class="btn btn-sm product-primary" :disabled="busy" @click="importAction">转为事项</button></div>
    </SurfaceCard>
    <SurfaceCard class="product-table-card" padding="none">
      <div class="product-toolbar">
        <input v-model.trim="keyword" class="form-input product-search" :aria-label="`搜索${noun}`" :placeholder="`搜索${noun}`" />
        <SelectMenu v-if="kind !== 'versions'" v-model="status" class="product-stage-filter" aria-label="记录状态筛选"><option value="">全部状态</option><option v-for="item in statuses" :key="item.id" :value="item.id">{{ item.label }}</option></SelectMenu>
        <SelectMenu v-if="kind !== 'tasks' && releaseUnits.length" v-model="unitID" class="product-stage-filter" aria-label="按发布端筛选"><option value="">全部发布端</option><option :value="0">项目默认</option><option v-for="unit in releaseUnits" :key="unit.id" :value="unit.id">{{ unit.name }}</option></SelectMenu>
        <SelectMenu v-if="kind !== 'versions'" v-model="versionID" class="product-stage-filter" aria-label="记录版本筛选"><option value="">全部版本</option><option v-for="version in filteredVersionOptions" :key="version.id" :value="version.id">{{ versionLabel(releaseUnits, version) }}</option></SelectMenu>
        <SelectMenu v-if="kind === 'distributions'" v-model="audience" class="product-stage-filter" aria-label="发行分类筛选"><option value="">全部发行</option><option v-for="item in distributionAudiences" :key="item.id" :value="item.id">{{ item.label }}</option></SelectMenu>
        <button class="icon-button" title="重置筛选" aria-label="重置筛选" @click="keyword = ''; status = ''; versionID = ''; audience = ''; unitID = ''"><RotateCcw :size="15" /></button>
        <button v-if="kind === 'versions'" class="btn" type="button" @click="unitsOpen = true">管理发布端</button>
        <button v-if="kind === 'distributions'" class="btn" @click="$emit('show-channels')">发行渠道</button>
        <button class="btn btn-primary product-primary" :disabled="busy || (kind !== 'tasks' && (optionsLoading || Boolean(optionsError)))" @click="openEditor()">新增{{ noun }}</button>
      </div>
      <EmptyState v-if="optionsError" message="版本选项读取失败"><template #action><button class="btn btn-sm" @click="loadOptions">重试</button></template></EmptyState>
      <EmptyState v-else-if="resource.loading.value && !resource.items.value.length" variant="loading" message="正在读取记录..." />
      <EmptyState v-else-if="resource.error.value" message="记录读取失败"><template #action><button class="btn btn-sm" @click="load">重试</button></template></EmptyState>
      <EmptyState v-else-if="!resource.total.value" :message="emptyMessage" />
      <div v-else class="table-wrap">
        <DataTable v-if="kind === 'tasks'">
          <thead><tr><th>事项 / 说明</th><th>类型 / 优先级</th><th>关联发版</th><th>状态</th><th>截止日期</th><TableActionCell header /></tr></thead>
          <tbody>
            <tr v-for="row in resource.items.value" :key="row.id">
              <td class="lifecycle-task-cell">
                {{ row.title }}
                <OverflowTooltip v-if="row.details" :text="row.details" class="cell-secondary product-line lifecycle-task-description" />
              </td>
              <td>{{ labelFor(taskKinds, row.kind) }} · {{ labelFor(priorities, row.priority) }}</td>
              <td>
                <span v-for="id in taskVersionIDs(row)" :key="id" class="task-release-label">{{ versionName(id) }}</span>
                <span v-if="!taskVersionIDs(row).length" class="cell-secondary">—</span>
              </td>
              <td><span class="badge" :class="lifecycleClass(row.status)">{{ labelFor(taskStatuses, row.status) }}</span></td>
              <td>{{ row.due_date || '—' }}<span v-if="row.due_date && !['done', 'cancelled'].includes(row.status)" class="badge product-line" :class="dueClass(dueState(row.due_date))">{{ dueLabel(dueState(row.due_date)) }}</span></td>
              <TableActionCell>
                <button type="button" class="icon-button" title="编辑事项" aria-label="编辑事项" :disabled="busy || optionsLoading || Boolean(optionsError)" @click="openEditor(row)"><Pencil :size="15" /></button>
                <button type="button" class="icon-button" title="删除事项" aria-label="删除事项" :disabled="busy" @click="taskToDelete = row"><Trash2 :size="15" /></button>
              </TableActionCell>
            </tr>
          </tbody>
        </DataTable>
        <DataTable v-else-if="kind === 'versions'">
          <thead><tr><th>版本</th><th v-if="releaseUnits.length">发布端</th><th>说明</th><th>发版日期</th><th>关联部署</th><TableActionCell header /></tr></thead>
          <tbody>
            <tr v-for="row in resource.items.value" :key="row.id">
              <td class="lifecycle-version-cell">{{ row.name }}</td>
              <td v-if="releaseUnits.length">{{ releaseUnitName(releaseUnits, row.release_unit_id) }}</td>
              <td class="lifecycle-version-description">
                <OverflowTooltip v-if="row.notes || row.goal" :text="row.notes || row.goal" multiline class="cell-secondary lifecycle-version-preview" />
                <span v-else class="cell-secondary">—</span>
              </td>
              <td>{{ row.released_on || '—' }}</td>
              <td>
                <button v-if="deploymentCount(row)" type="button" class="btn btn-sm" @click="showDeployments(row)">{{ deploymentCount(row) }} 条记录</button>
                <button v-else type="button" class="btn btn-sm" :disabled="busy" @click="openEditor(row, true)">关联部署</button>
              </td>
              <TableActionCell>
                <button class="icon-button" title="查看版本详情" aria-label="查看版本详情" @click="detailVersion = row"><Eye :size="15" /></button>
                <button class="icon-button" title="查看版本事项" aria-label="查看版本事项" @click="$emit('select-version', row.id)"><ListTodo :size="15" /></button>
                <button v-if="product.channels?.length" class="icon-button" title="查看版本发行记录" aria-label="查看版本发行记录" @click="$emit('select-distributions', row.id)"><Package :size="15" /></button>
                <button class="icon-button" title="编辑版本" aria-label="编辑版本" :disabled="busy || optionsLoading || Boolean(optionsError)" @click="openEditor(row)"><Pencil :size="15" /></button>
              </TableActionCell>
            </tr>
          </tbody>
        </DataTable>
        <DataTable v-else>
          <thead><tr><th>产品版本</th><th>发行渠道 / 分类</th><th>状态</th><th>更新时间 / 备注</th><TableActionCell header /></tr></thead>
          <tbody>
            <tr v-for="row in resource.items.value" :key="row.id">
              <td>
                {{ row.version_id ? versionName(row.version_id) : row.platform_version }}
                <span v-if="row.version_id && row.platform_version && row.platform_version !== versionRecord(row.version_id)?.name" class="cell-secondary product-line">渠道版本 {{ row.platform_version }}</span>
                <span v-if="!row.version_id" class="cell-secondary product-line">未关联正式版本</span>
                <span v-if="row.build_identifier" class="cell-secondary product-line">构建 {{ row.build_identifier }}</span>
              </td>
              <td>{{ channelName(row.channel_id) }}<span class="cell-secondary product-line">{{ audienceLabel(row.audience) }}</span></td>
              <td><span class="badge" :class="lifecycleClass(row.status)">{{ labelFor(distributionStatuses, row.status) }}</span></td>
              <td>{{ formatTime(row.updated_at) }}<span class="cell-secondary product-line lifecycle-description">{{ row.notes }}</span></td>
              <TableActionCell><div class="btn-group"><button class="icon-button" title="发行历史" aria-label="发行历史" @click="showHistory(row)"><History :size="15" /></button><button class="icon-button" title="编辑发行记录" aria-label="编辑发行记录" :disabled="busy || optionsLoading || Boolean(optionsError)" @click="openEditor(row)"><Pencil :size="15" /></button></div></TableActionCell>
            </tr>
          </tbody>
        </DataTable>
      </div>
      <ResourcePagination :resource="resource" :label="`${noun}分页`" />
    </SurfaceCard>
    <LifecycleEditor :open="editorOpen" :busy="busy" :kind="kind" :record="editing" :product-id="product.id" :versions="versions" :channels="product.channels || []" :release-units="releaseUnits" :expand-deployments="editDeployments" @close="editorOpen = false" @save="save" @error="error = $event" />
    <ReleaseUnitManager v-if="kind === 'versions'" :open="unitsOpen" :product="product" @close="unitsOpen = false" @updated="updateUnits" @error="error = $event" />
    <ConfirmDialog teleport :open="Boolean(taskToDelete)" title="删除事项" :message="`确认删除事项“${taskToDelete?.title || ''}”？`" :busy="busy" confirm-text="删除" @cancel="taskToDelete = null" @confirm="removeTask" />
    <BaseModal teleport :open="Boolean(detailVersion)" :title="`版本详情 · ${detailVersion?.name || ''}`" size="large" @close="detailVersion = null">
      <dl v-if="detailVersion" class="product-detail-list">
        <div v-if="releaseUnits.length"><dt>发布端</dt><dd>{{ releaseUnitName(releaseUnits, detailVersion.release_unit_id) }}</dd></div>
        <div v-if="detailVersion.released_on"><dt>正式发版日期</dt><dd>{{ detailVersion.released_on }}</dd></div>
        <div v-else-if="detailVersion.status"><dt>历史状态</dt><dd>{{ labelFor(versionStatuses, detailVersion.status) }}</dd></div>
        <div v-if="detailVersion.target_date"><dt>目标日期</dt><dd>{{ detailVersion.target_date }}</dd></div>
        <div v-if="detailVersion.goal"><dt>本次目标</dt><dd>{{ detailVersion.goal }}</dd></div>
        <div v-if="detailVersion.acceptance"><dt>验收项与验证结果</dt><dd>{{ detailVersion.acceptance }}</dd></div>
        <div v-if="detailVersion.notes"><dt>版本说明</dt><dd class="lifecycle-version-notes">{{ detailVersion.notes }}</dd></div>
        <div v-if="detailVersion.repository_url"><dt>代码仓库</dt><dd><a :href="detailVersion.repository_url" target="_blank" rel="noopener noreferrer">{{ detailVersion.repository_url }}</a></dd></div>
        <div v-if="detailVersion.commit_sha"><dt>提交 SHA</dt><dd>{{ detailVersion.commit_sha }}</dd></div>
        <div v-if="detailVersion.build_url"><dt>构建记录</dt><dd><a :href="detailVersion.build_url" target="_blank" rel="noopener noreferrer">{{ detailVersion.build_url }}</a></dd></div>
        <div v-if="detailVersion.artifact_url"><dt>构建产物</dt><dd><a :href="detailVersion.artifact_url" target="_blank" rel="noopener noreferrer">{{ detailVersion.artifact_url }}</a></dd></div>
      </dl>
    </BaseModal>
    <BaseModal teleport :open="Boolean(historyRecord)" title="发行状态历史" size="large" @close="historyRecord = null">
      <EmptyState v-if="history.loading.value" variant="loading" message="正在读取历史..." />
      <EmptyState v-else-if="history.error.value" message="历史读取失败"><template #action><button class="btn btn-sm" @click="history.refresh()">重试</button></template></EmptyState>
      <div v-else class="table-wrap"><DataTable><thead><tr><th>记录时间</th><th>状态</th><th>平台版本 / Build</th><th>备注</th></tr></thead><tbody><tr v-for="event in history.items.value" :key="event.id"><td>{{ formatTime(event.created_at) }}</td><td>{{ labelFor(distributionStatuses, event.status) }}<span class="cell-secondary product-line">{{ audienceLabel(event.audience) }}</span></td><td>{{ event.platform_version }} / {{ event.build_identifier || '—' }}</td><td class="lifecycle-description">{{ event.notes || '—' }}</td></tr></tbody></DataTable></div>
      <ResourcePagination :resource="history" label="发行历史分页" />
    </BaseModal>
    <BaseModal teleport :open="Boolean(deploymentVersion)" title="关联部署记录" size="large" @close="closeDeployments">
      <EmptyState v-if="deploymentRows.loading.value" variant="loading" message="正在读取部署..." />
      <EmptyState v-else-if="deploymentRows.error.value" message="部署读取失败"><template #action><button class="btn btn-sm" @click="deploymentRows.refresh()">重试</button></template></EmptyState>
      <div v-else class="table-wrap"><DataTable><thead><tr><th>组件 / 环境</th><th>版本</th><th>状态</th><th>入口</th></tr></thead><tbody><tr v-for="row in deploymentRows.items.value" :key="(row.source || 'application') + '-' + row.id"><td>{{ row.application_name }} · {{ row.environment_name || '—' }}（{{ environmentKindLabel(row.environment_kind) }}）</td><td>{{ row.version || `#${row.id}` }}</td><td>{{ row.status }}</td><td><router-link :to="deploymentPath(row)">{{ row.source === 'platform' ? '平台发布与更新' : '查看部署记录' }}</router-link></td></tr></tbody></DataTable></div>
      <ResourcePagination :resource="deploymentRows" label="关联部署分页" />
      <template #actions>
        <button type="button" class="btn" :disabled="busy" @click="editLinkedDeployments">编辑关联</button>
      </template>
    </BaseModal>
    <ErrorNoticeModal :open="Boolean(error)" :message="error" teleport @close="error = ''" />
  </div>
</template>
<script setup>
import DataTable from '../../components/DataTable.vue'
import TableActionCell from '../../components/TableActionCell.vue'
import { distributionAudiences, audienceLabel, environmentKindLabel } from './environment-utils.js'
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { Eye, History, ListTodo, Package, Pencil, RotateCcw, Trash2 } from 'lucide-vue-next'
import SurfaceCard from '../../components/SurfaceCard.vue'
import EmptyState from '../../components/EmptyState.vue'
import OverflowTooltip from '../../components/OverflowTooltip.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import BaseModal from '../../components/BaseModal.vue'
import ConfirmDialog from '../../components/ConfirmDialog.vue'
import ResourcePagination from '../../components/ResourcePagination.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import LifecycleEditor from './LifecycleEditor.vue'
import ReleaseUnitManager from './ReleaseUnitManager.vue'
import { getLifecyclePage, getLifecycleOptions, getDistributionEvents, saveLifecycleRecord, deleteProductTask, importProductNextAction } from '../../api/product-lifecycle.js'
import { usePaginatedResource } from '../../composables/usePaginatedResource.js'
import { labelFor, platforms, dateOnly, dueState, dueClass, dueLabel } from './product-utils.js'
import { taskKinds, taskStatuses, priorities, versionStatuses, distributionStatuses, lifecycleClass, deploymentCount, deploymentPath, releaseUnitName, versionLabel } from './lifecycle-utils.js'
import { formatShortDateTime as formatTime } from '../../utils/formatters.js'
const props = defineProps({ product: { type: Object, required: true }, kind: { type: String, required: true }, initialVersionId: { type: [Number, String], default: '' } })
const emit = defineEmits(['changed', 'select-version', 'select-distributions', 'show-channels', 'update-release-units'])
const audience = ref('')
const unitID = ref('')
const unitsOpen = ref(false)
const releaseUnits = computed(() => props.product.release_units || [])
const keyword = ref(''), status = ref(''), versionID = ref(props.initialVersionId), versions = ref([]), optionsLoading = ref(false), optionsError = ref(''), busy = ref(false), error = ref(''), editorOpen = ref(false), editing = ref(null), historyRecord = ref(null), deploymentVersion = ref(null)
const detailVersion = ref(null)
const editDeployments = ref(false)
const taskToDelete = ref(null)
const noun = computed(() => ({ tasks: '事项', versions: '版本', distributions: '发行记录' })[props.kind])
const statuses = computed(() => ({ tasks: taskStatuses, versions: versionStatuses, distributions: distributionStatuses })[props.kind])
const emptyMessage = computed(() => keyword.value || status.value || versionID.value || audience.value || unitID.value !== '' ? '没有符合条件的记录' : props.kind === 'distributions' ? '暂无发行记录' : `暂无${noun.value}，可先新增一条`)
const resource = usePaginatedResource((query, options) => getLifecyclePage(props.product.id, props.kind, query, options), 'items')
const history = usePaginatedResource((query, options) => getDistributionEvents(props.product.id, historyRecord.value.id, query, options), 'items')
const deploymentRows = usePaginatedResource((query, options) => getLifecyclePage(props.product.id, 'deployments', { ...query, version_id: deploymentVersion.value.id }, options), 'items')
const versionRecord = id => versions.value.find(version => version.id === id)
const versionName = id => versionRecord(id) ? versionLabel(releaseUnits.value, versionRecord(id)) : `版本 #${id}`
const taskVersionIDs = row => row.version_ids || (row.version_id ? [row.version_id] : [])
const filteredVersionOptions = computed(() => versions.value.filter(version => unitID.value === '' || (version.release_unit_id ?? 0) === unitID.value))
const channelName = id => { const channel = props.product.channels?.find(item => item.id === id); return channel ? channel.name || labelFor(platforms, channel.platform) : `渠道 #${id}` }
let optionsRequest = 0
async function loadOptions() {
  const request = ++optionsRequest; optionsLoading.value = true; optionsError.value = ''
  try { const data = await getLifecycleOptions(props.product.id); if (request === optionsRequest) versions.value = data.versions }
  catch (cause) { if (request === optionsRequest) { optionsError.value = cause.message; error.value = cause.message } }
  finally { if (request === optionsRequest) optionsLoading.value = false }
}
function load() { return resource.load({ keyword: keyword.value, status: status.value, ...(props.kind === 'distributions' ? { audience: audience.value } : {}), ...(versionID.value && props.kind !== 'versions' ? { version_id: versionID.value } : {}), ...(unitID.value !== '' ? { release_unit_id: unitID.value } : {}) }) }
function openEditor(row = null, expandDeployments = false) {
  if (props.kind === 'distributions' && !props.product.channels?.length) { error.value = '请先配置发行渠道'; return }
  editing.value = row
  editDeployments.value = expandDeployments
  editorOpen.value = true
}
function updateUnits(units) {
  if (unitID.value && !units.some(unit => unit.id === unitID.value)) unitID.value = ''
  emit('update-release-units', units)
}
async function save(record) {
  busy.value = true
  try { await saveLifecycleRecord(props.product.id, props.kind, record); editorOpen.value = false; await Promise.all([resource.refresh(), loadOptions()]); emit('changed') }
  catch (cause) { error.value = cause.message } finally { busy.value = false }
}
async function removeTask() {
  if (busy.value || !taskToDelete.value) return
  busy.value = true
  try {
    await deleteProductTask(props.product.id, taskToDelete.value.id)
    taskToDelete.value = null
    await resource.refresh()
    emit('changed')
  } catch (cause) {
    error.value = cause.message
  } finally {
    busy.value = false
  }
}
async function importAction() { busy.value = true; try { await importProductNextAction(props.product.id); await resource.refresh(); emit('changed') } catch (cause) { error.value = cause.message } finally { busy.value = false } }
function showHistory(row) { history.reset(); historyRecord.value = row; void history.load() }
function showDeployments(row) { deploymentRows.reset(); deploymentVersion.value = row; void deploymentRows.load() }
function closeDeployments() {
  deploymentRows.reset()
  deploymentVersion.value = null
}
function editLinkedDeployments() {
  const row = deploymentVersion.value
  closeDeployments()
  openEditor(row, true)
}
watch(unitID, () => { if (versionID.value && !filteredVersionOptions.value.some(version => version.id === versionID.value)) versionID.value = '' })
watch([keyword, status, versionID, audience, unitID], load)
watch(() => props.initialVersionId, value => { versionID.value = value })
watch(() => [props.product.id, props.kind], () => { detailVersion.value = null; taskToDelete.value = null; resource.reset(); void load(); void loadOptions() }, { immediate: true })
onBeforeUnmount(() => { optionsRequest++; resource.cancel(); history.cancel(); deploymentRows.cancel() })
</script>
<style scoped>
.product-detail-content { display: grid; gap: var(--space-16); min-width: 0; }
.lifecycle-description { max-width: 320px; white-space: pre-wrap; overflow-wrap: anywhere; }
.lifecycle-task-cell { width: 50%; }
.lifecycle-task-description { contain: inline-size; }
.task-release-label { display: block; }
.lifecycle-version-cell { max-width: 240px; overflow-wrap: anywhere; }
.lifecycle-version-description { width: 55%; min-width: 160px; }
.lifecycle-version-preview { contain: inline-size; }
.lifecycle-version-notes { white-space: pre-wrap; overflow-wrap: anywhere; }
</style>
