<template>
  <div class="product-detail-content">
    <SurfaceCard v-if="kind === 'tasks' && product.next_action" class="product-table-card" padding="none">
      <div class="product-toolbar"><span class="product-muted">已有下一步：{{ product.next_action }}{{ product.next_due_date ? ` · ${dateOnly(product.next_due_date)}` : '' }}</span><button class="btn btn-sm product-primary" :disabled="busy" @click="importAction">转为事项</button></div>
    </SurfaceCard>
    <ProductTaskBoard v-if="kind === 'tasks' && taskGroups !== undefined" :active="boardView" :product-id="product.id" :groups="taskGroups" :revision="boardRevision" :busy="busy" @add="openTaskInGroup" @edit="openEditor" @toggle="toggleTask" @changed="updateTaskGroups" @error="error = $event">
      <div class="task-view-toolbar" role="group" aria-label="待办布局">
          <button type="button" class="btn btn-sm" :aria-pressed="boardView" @click="taskView = 'board'"><Columns3 :size="14" aria-hidden="true" />看板</button>
          <button type="button" class="btn btn-sm" :aria-pressed="!boardView" @click="taskView = 'list'"><ListTodo :size="14" aria-hidden="true" />清单</button>
      </div>
    </ProductTaskBoard>
    <SurfaceCard v-if="!boardView" class="product-table-card" :class="{ 'task-list-card': kind === 'tasks', 'version-list-card': kind === 'versions', 'distribution-list-card': kind === 'distributions' }" padding="none">
      <div class="product-toolbar">
        <input v-model.trim="keyword" class="form-input product-search" :aria-label="`搜索${noun}`" :placeholder="`搜索${noun}`" />
        <SelectMenu v-if="kind === 'tasks'" v-model="status" class="product-stage-filter" aria-label="待办状态筛选">
          <option value="">全部状态</option>
          <option v-for="item in taskStatuses" :key="item.id" :value="item.id">{{ item.id === 'todo' ? '待处理' : item.label }}</option>
        </SelectMenu>
        <SelectMenu v-if="kind === 'distributions'" v-model="status" class="product-stage-filter" aria-label="记录状态筛选"><option value="">全部状态</option><option v-for="item in distributionStatuses" :key="item.id" :value="item.id">{{ item.label }}</option></SelectMenu>
        <SelectMenu v-if="kind === 'versions' && releaseUnits.length" v-model="unitID" class="product-stage-filter" aria-label="按发布端筛选"><option value="">全部发布端</option><option :value="0">项目默认</option><option v-for="unit in releaseUnits" :key="unit.id" :value="unit.id">{{ unit.name }}</option></SelectMenu>
        <SelectMenu v-if="kind === 'tasks'" v-model="versionID" class="product-stage-filter" aria-label="记录版本筛选"><option value="">全部版本</option><option v-for="version in filteredVersionOptions" :key="version.id" :value="version.id">{{ versionLabel(releaseUnits, version) }}</option></SelectMenu>
        <button v-if="kind === 'distributions'" type="button" class="btn distribution-filter-toggle" :aria-expanded="distributionFiltersOpen" :aria-controls="distributionFiltersID" @click="distributionFiltersOpen = !distributionFiltersOpen"><Filter :size="15" aria-hidden="true" />筛选<span v-if="distributionFilterCount">{{ distributionFilterCount }}</span></button>
        <button v-if="kind === 'tasks' || hasActiveFilters" class="icon-button" title="重置筛选" aria-label="重置筛选" @click="resetFilters"><RotateCcw :size="15" /></button>
        <div v-if="kind === 'versions'" class="version-toolbar-actions">
          <details ref="versionSettings" class="version-settings" @focusout="closeSettingsOnBlur" @keydown.esc.stop="closeVersionSettings(true)">
            <summary class="icon-button" title="版本设置" aria-label="版本设置"><Settings :size="16" aria-hidden="true" /></summary>
            <div class="version-settings-actions">
              <button type="button" class="btn btn-sm" @click="closeVersionSettings(); unitsOpen = true">管理发布端</button>
              <button type="button" class="btn btn-sm" @click="closeVersionSettings(); repositoriesOpen = true">GitHub 仓库</button>
            </div>
          </details>
          <button type="button" class="btn btn-primary" :disabled="busy || optionsLoading || Boolean(optionsError)" @click="openEditor()">新增版本</button>
        </div>
        <div v-else-if="kind === 'distributions'" class="distribution-toolbar-actions">
          <button v-if="hasWeChatChannels" type="button" class="btn" @click="wechatOpen = true">微信接入</button>
          <button v-if="product.channels?.length" type="button" class="icon-button" title="发行渠道" aria-label="发行渠道" @click="$emit('show-channels')"><Settings :size="16" aria-hidden="true" /></button>
          <button type="button" class="btn btn-primary" :disabled="busy || Boolean(product.channels?.length && (optionsLoading || optionsError))" @click="product.channels?.length ? openEditor() : $emit('show-channels')">{{ product.channels?.length ? '新增发行记录' : '添加发行渠道' }}</button>
        </div>
        <div v-else class="task-list-actions">
          <span v-if="!resource.loading.value && !resource.error.value" class="product-muted">{{ resource.total.value }} 项</span>
          <button class="btn" type="button" :disabled="busy" @click="openEditor()">新增事项</button>
        </div>
      </div>
      <div v-if="kind === 'distributions' && distributionFiltersOpen" :id="distributionFiltersID" class="distribution-filters" role="group" aria-label="更多发行筛选">
        <SelectMenu v-if="releaseUnits.length" v-model="unitID" aria-label="按发布端筛选"><option value="">全部发布端</option><option :value="0">项目默认</option><option v-for="unit in releaseUnits" :key="unit.id" :value="unit.id">{{ unit.name }}</option></SelectMenu>
        <SelectMenu v-model="versionID" aria-label="记录版本筛选"><option value="">全部版本</option><option v-for="version in filteredVersionOptions" :key="version.id" :value="version.id">{{ versionLabel(releaseUnits, version) }}</option></SelectMenu>
        <SelectMenu v-model="audience" aria-label="发行分类筛选"><option value="">全部发行</option><option v-for="item in distributionAudiences" :key="item.id" :value="item.id">{{ item.label }}</option></SelectMenu>
      </div>
      <form v-if="kind === 'tasks'" class="task-quick-add" @submit.prevent="quickAddTask">
        <Plus :size="16" aria-hidden="true" />
        <input v-model.trim="quickTaskTitle" class="form-input" maxlength="512" aria-label="快速添加待办" placeholder="输入待办，按回车添加" :disabled="busy" />
        <button class="btn btn-primary btn-sm" type="submit" :disabled="busy || !quickTaskTitle">添加</button>
      </form>
      <EmptyState v-if="optionsError" message="版本选项读取失败"><template #action><button class="btn btn-sm" @click="loadOptions">重试</button></template></EmptyState>
      <EmptyState v-else-if="resource.loading.value && !resource.items.value.length" variant="loading" message="正在读取记录..." />
      <EmptyState v-else-if="resource.error.value" message="记录读取失败"><template #action><button class="btn btn-sm" @click="load">重试</button></template></EmptyState>
      <EmptyState v-else-if="!resource.total.value" :class="{ 'task-empty': kind === 'tasks' }" :message="emptyMessage" :description="kind === 'distributions' && !hasActiveFilters && !product.channels?.length ? '先添加发行渠道，再记录版本的发行进度。' : ''" />
      <ul v-else-if="kind === 'tasks'" class="task-list">
        <ProductTaskItem v-for="row in resource.items.value" :key="row.id" :task="row" :version-names="taskVersionIDs(row).map(versionName)" :group-name="taskGroupName(row.group_id)" :busy="busy" :completion-disabled="resource.loading.value" :edit-disabled="optionsLoading || Boolean(optionsError)" @edit="openEditor" @remove="confirmTaskRemoval" @toggle="toggleTask" />
      </ul>
      <div v-else class="table-wrap">
        <DataTable v-if="kind === 'versions'">
          <thead><tr><th>版本</th><th v-if="releaseUnits.length">发布端</th><th>说明</th><th>发版日期</th><th>关联部署</th><TableActionCell header /></tr></thead>
          <tbody>
            <tr v-for="row in resource.items.value" :key="row.id">
              <td class="lifecycle-version-cell"><button type="button" class="version-detail-link" aria-haspopup="dialog" @click="detailVersion = row">{{ row.name }}</button></td>
              <td v-if="releaseUnits.length">{{ releaseUnitName(releaseUnits, row.release_unit_id) }}</td>
              <td class="lifecycle-version-description">
                <OverflowTooltip v-if="row.notes || row.goal" :text="row.notes || row.goal" multiline class="cell-secondary lifecycle-version-preview" />
                <span v-else class="cell-secondary">—</span>
              </td>
              <td>{{ row.released_on || '—' }}</td>
              <td class="cell-secondary">{{ deploymentCount(row) ? `${deploymentCount(row)} 条记录` : '未关联' }}</td>
              <TableActionCell><button type="button" class="icon-button" title="编辑版本" aria-label="编辑版本" :disabled="busy || optionsLoading || Boolean(optionsError)" @click="openEditor(row)"><Pencil :size="15" /></button></TableActionCell>
            </tr>
          </tbody>
        </DataTable>
        <DataTable v-else>
          <thead><tr><th>发行渠道</th><th>产品版本</th><th>发行分类</th><th>部署环境 / 组件</th><th>状态</th><th>更新时间 / 备注</th><TableActionCell header /></tr></thead>
          <tbody>
            <tr v-for="row in resource.items.value" :key="row.id">
              <td class="distribution-channel-cell">
                <span class="cell-primary distribution-channel-name">{{ channelName(row.channel_id) }}</span>
                <span v-if="channelRecord(row.channel_id)?.name && channelRecord(row.channel_id).name !== channelName(row.channel_id)" class="cell-secondary product-line">{{ channelRecord(row.channel_id).name }}</span>
                <span v-if="releaseUnits.length && channelRecord(row.channel_id)?.release_unit_id != null" class="cell-secondary product-line">{{ releaseUnitName(releaseUnits, channelRecord(row.channel_id).release_unit_id) }}</span>
                <span v-if="channelRecord(row.channel_id)?.app_identifier" class="cell-secondary product-line distribution-channel-identifier">应用标识 {{ channelRecord(row.channel_id).app_identifier }}</span>
                <a v-if="channelRecord(row.channel_id)?.public_url" class="distribution-channel-link" :href="channelRecord(row.channel_id).public_url" :title="channelRecord(row.channel_id).public_url" target="_blank" rel="noopener noreferrer"><ExternalLink :size="12" aria-hidden="true" /><span>{{ channelRecord(row.channel_id).public_url }}</span></a>
                <a v-if="channelRecord(row.channel_id)?.admin_url" class="distribution-channel-link" :href="channelRecord(row.channel_id).admin_url" :title="channelRecord(row.channel_id).admin_url" target="_blank" rel="noopener noreferrer"><ExternalLink :size="12" aria-hidden="true" /><span>管理后台</span></a>
              </td>
              <td class="distribution-version-cell">
                {{ row.version_id ? versionName(row.version_id) : row.platform_version }}
                <span v-if="row.version_id && row.platform_version && row.platform_version !== versionRecord(row.version_id)?.name" class="cell-secondary product-line">渠道版本 {{ row.platform_version }}</span>
                <span v-if="!row.version_id" class="cell-secondary product-line">未关联正式版本</span>
                <span v-if="row.build_identifier" class="cell-secondary product-line">构建 {{ row.build_identifier }}</span>
              </td>
              <td class="distribution-audience-cell">{{ audienceLabel(row.audience) }}</td>
              <td class="distribution-environment-cell">
                <span>{{ row.environment_name || '未关联环境' }}</span>
                <span class="cell-secondary product-line">{{ row.environment_id ? row.deployment_ids?.length ? `${row.deployment_ids.length} 个部署组件` : '尚未关联部署组件' : '环境待定' }}</span>
              </td>
              <td><span class="badge" :class="lifecycleClass(row.status)">{{ labelFor(distributionStatuses, row.status) }}</span></td>
              <td>{{ formatTime(row.updated_at) }}<span v-if="row.wechat_published_at" class="cell-secondary product-line">微信上线 {{ formatTime(row.wechat_published_at) }}</span><span class="cell-secondary product-line lifecycle-description">{{ row.notes }}</span></td>
              <TableActionCell><div class="btn-group"><button class="icon-button" title="发行历史" aria-label="发行历史" @click="showHistory(row)"><History :size="15" /></button><button class="icon-button" title="编辑发行记录" aria-label="编辑发行记录" :disabled="busy || optionsLoading || Boolean(optionsError)" @click="openEditor(row)"><Pencil :size="15" /></button></div></TableActionCell>
            </tr>
          </tbody>
        </DataTable>
      </div>
      <div v-if="recordCountOnly && resource.total.value" class="lifecycle-list-count cell-secondary" :class="{ 'version-list-count': kind === 'versions' }" aria-live="polite">共 {{ resource.total.value }} 条</div>
      <ResourcePagination :resource="resource" :label="`${noun}分页`" :hide-single-page="kind === 'tasks' || recordCountOnly" />
    </SurfaceCard>
    <LifecycleEditor :open="editorOpen" :busy="busy" :kind="kind" :record="editing" :product-id="product.id" :versions="versions" :channels="product.channels || []" :release-units="releaseUnits" :environments="environments" :groups="taskGroups || []" :expand-deployments="editDeployments" @close="editorOpen = false" @save="save" @remove="confirmTaskRemoval" @error="error = $event" />
    <ReleaseUnitManager v-if="kind === 'versions'" :open="unitsOpen" :product="product" @close="unitsOpen = false" @updated="updateUnits" @error="error = $event" />
    <ProductGitHubRepositories v-if="kind === 'versions' && repositoriesOpen" :open="repositoriesOpen" :product-id="product.id" :units="releaseUnits" @close="repositoriesOpen = false" />
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
        <div v-for="source in detailVersion.github_sources" :key="source.binding_id">
          <dt>{{ source.component }}</dt>
          <dd><a :href="`https://github.com/${source.repository}/commit/${source.sha}`" target="_blank" rel="noopener noreferrer">{{ source.repository }}</a><div class="cell-secondary"><code>{{ source.tag ? `${source.tag} · ` : '' }}{{ source.sha }}</code></div></dd>
        </div>
        <div v-if="detailVersion.build_url"><dt>构建记录</dt><dd><a :href="detailVersion.build_url" target="_blank" rel="noopener noreferrer">{{ detailVersion.build_url }}</a></dd></div>
        <div v-if="detailVersion.artifact_url"><dt>构建产物</dt><dd><a :href="detailVersion.artifact_url" target="_blank" rel="noopener noreferrer">{{ detailVersion.artifact_url }}</a></dd></div>
        <div><dt>关联部署</dt><dd>
          <button v-if="deploymentCount(detailVersion)" type="button" class="version-detail-link" @click="showDeployments(detailVersion)">{{ deploymentCount(detailVersion) }} 条记录</button>
          <button v-else type="button" class="version-detail-link" :disabled="busy || optionsLoading || Boolean(optionsError)" @click="openEditor(detailVersion, true)">关联部署</button>
        </dd></div>
      </dl>
      <template #actions>
        <div class="version-detail-actions">
          <button type="button" class="btn" @click="$emit('select-version', detailVersion.id)">查看待办</button>
          <button v-if="product.channels?.length" type="button" class="btn" @click="$emit('select-distributions', detailVersion.id)">查看发行记录</button>
          <button type="button" class="btn btn-primary" :disabled="busy || optionsLoading || Boolean(optionsError)" @click="openEditor(detailVersion)">编辑版本</button>
        </div>
      </template>
    </BaseModal>
    <BaseModal teleport :open="Boolean(historyRecord)" title="发行状态历史" size="large" @close="historyRecord = null">
      <EmptyState v-if="history.loading.value" variant="loading" message="正在读取历史..." />
      <EmptyState v-else-if="history.error.value" message="历史读取失败"><template #action><button class="btn btn-sm" @click="history.refresh()">重试</button></template></EmptyState>
      <div v-else class="table-wrap"><DataTable><thead><tr><th>记录时间</th><th>状态</th><th>环境 / 部署</th><th>平台版本 / Build</th><th>备注</th></tr></thead><tbody><tr v-for="event in history.items.value" :key="event.id"><td>{{ formatTime(event.created_at) }}</td><td>{{ labelFor(distributionStatuses, event.status) }}<span class="cell-secondary product-line">{{ audienceLabel(event.audience) }}</span></td><td>{{ event.environment_name || '—' }}<span v-if="event.deployment_ids?.length" class="cell-secondary product-line">{{ event.deployment_ids.length }} 个部署组件</span></td><td>{{ event.platform_version }} / {{ event.build_identifier || '—' }}</td><td class="lifecycle-description">{{ event.notes || '—' }}</td></tr></tbody></DataTable></div>
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
    <WeChatChannelSettings v-if="kind === 'distributions' && hasWeChatChannels" :open="wechatOpen" :product="product" :authorization-return="wechatReturn" @close="wechatOpen = false; emit('close-wechat')" @changed="refreshWeChatRecords" />
    <ErrorNoticeModal :open="Boolean(error)" :message="error" teleport @close="error = ''" />
  </div>
</template>
<script setup>
import DataTable from '../../components/DataTable.vue'
import TableActionCell from '../../components/TableActionCell.vue'
import { distributionAudiences, audienceLabel, environmentKindLabel } from './environment-utils.js'
import { computed, onBeforeUnmount, ref, useId, watch } from 'vue'
import { Columns3, ExternalLink, Filter, History, ListTodo, Pencil, Plus, RotateCcw, Settings } from 'lucide-vue-next'
import SurfaceCard from '../../components/SurfaceCard.vue'
import EmptyState from '../../components/EmptyState.vue'
import OverflowTooltip from '../../components/OverflowTooltip.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import BaseModal from '../../components/BaseModal.vue'
import ConfirmDialog from '../../components/ConfirmDialog.vue'
import ResourcePagination from '../../components/ResourcePagination.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import LifecycleEditor from './LifecycleEditor.vue'
import ProductTaskBoard from './ProductTaskBoard.vue'
import ProductTaskItem from './ProductTaskItem.vue'
import ReleaseUnitManager from './ReleaseUnitManager.vue'
import ProductGitHubRepositories from './ProductGitHubRepositories.vue'
import WeChatChannelSettings from './WeChatChannelSettings.vue'
import { weChatChannels } from '../../api/wechat.js'
import { getLifecyclePage, getLifecycleOptions, getDistributionEvents, saveLifecycleRecord, deleteProductTask, importProductNextAction } from '../../api/product-lifecycle.js'
import { usePaginatedResource } from '../../composables/usePaginatedResource.js'
import { PAGE_SIZE_OPTIONS } from '../../composables/usePagination.js'
import { labelFor, platforms, dateOnly } from './product-utils.js'
import { taskStatuses, versionStatuses, distributionStatuses, lifecycleClass, deploymentCount, deploymentPath, releaseUnitName, versionLabel } from './lifecycle-utils.js'
import { formatShortDateTime as formatTime } from '../../utils/formatters.js'
const props = defineProps({ product: { type: Object, required: true }, kind: { type: String, required: true }, initialVersionId: { type: [Number, String], default: '' }, wechatReturn: Object })
const emit = defineEmits(['changed', 'select-version', 'select-distributions', 'show-channels', 'update-release-units', 'close-wechat'])
const audience = ref('')
const unitID = ref('')
const unitsOpen = ref(false)
const repositoriesOpen = ref(false)
const wechatOpen = ref(false)
const hasWeChatChannels = computed(() => weChatChannels(props.product).length > 0)
watch(() => props.wechatReturn, result => { if (result && props.kind === 'distributions') wechatOpen.value = true }, { immediate: true })
const versionSettings = ref(null)
const distributionFiltersID = `distribution-filters-${useId()}`
const distributionFiltersOpen = ref(Boolean(props.initialVersionId))
const releaseUnits = computed(() => props.product.release_units || [])
const environments = computed(() => (props.product.projects || []).flatMap(project => (project.environments || []).map(environment => ({ ...environment, project_name: project.name }))))
const keyword = ref(''), status = ref(''), versionID = ref(props.initialVersionId), versions = ref([]), optionsLoading = ref(false), optionsError = ref(''), busy = ref(false), error = ref(''), editorOpen = ref(false), editing = ref(null), historyRecord = ref(null), deploymentVersion = ref(null)
const detailVersion = ref(null)
const editDeployments = ref(false)
const taskToDelete = ref(null)
const quickTaskTitle = ref('')
const taskGroups = ref(undefined)
const taskView = ref(props.initialVersionId ? 'list' : 'board')
const boardRevision = ref(0)
const boardView = computed(() => props.kind === 'tasks' && taskView.value === 'board' && taskGroups.value !== undefined)
const distributionFilterCount = computed(() => Number(Boolean(versionID.value)) + Number(Boolean(audience.value)) + Number(unitID.value !== ''))
const hasActiveFilters = computed(() => Boolean(keyword.value || status.value || distributionFilterCount.value))
const noun = computed(() => ({ tasks: '事项', versions: '版本', distributions: '发行记录' })[props.kind])
const emptyMessage = computed(() => {
  if (keyword.value || versionID.value || audience.value || unitID.value !== '') return '没有符合条件的记录'
  if (props.kind === 'tasks') return status.value ? '当前状态下没有待办' : '还没有待办，试试在上方添加一条'
  return status.value ? '没有符合条件的记录' : props.kind === 'distributions' ? '暂无发行记录' : `暂无${noun.value}，可先新增一条`
})
const resource = usePaginatedResource((query, options) => getLifecyclePage(props.product.id, props.kind, query, options), 'items')
const recordCountOnly = computed(() => props.kind !== 'tasks' && resource.pageSize.value === PAGE_SIZE_OPTIONS[0] && resource.totalPages.value === 1)
const history = usePaginatedResource((query, options) => getDistributionEvents(props.product.id, historyRecord.value.id, query, options), 'items')
const deploymentRows = usePaginatedResource((query, options) => getLifecyclePage(props.product.id, 'deployments', { ...query, version_id: deploymentVersion.value.id }, options), 'items')
const versionRecord = id => versions.value.find(version => version.id === id)
const versionName = id => versionRecord(id) ? versionLabel(releaseUnits.value, versionRecord(id)) : `版本 #${id}`
const taskVersionIDs = row => row.version_ids || (row.version_id ? [row.version_id] : [])
const taskGroupName = id => taskGroups.value?.find(group => group.id === id)?.name || ''
const filteredVersionOptions = computed(() => versions.value.filter(version => unitID.value === '' || (version.release_unit_id ?? 0) === unitID.value))
const channelRecord = id => props.product.channels?.find(item => item.id === id)
const channelName = id => { const channel = channelRecord(id); return channel ? labelFor(platforms, channel.platform) || channel.name : `渠道 #${id}` }
let optionsRequest = 0
async function loadOptions() {
  const request = ++optionsRequest; optionsLoading.value = true; optionsError.value = ''
  try {
    const data = await getLifecycleOptions(props.product.id)
    if (request !== optionsRequest) return
    versions.value = data.versions
    taskGroups.value = Array.isArray(data.task_groups) ? data.task_groups : null
  }
  catch (cause) { if (request === optionsRequest) { optionsError.value = cause.message; error.value = cause.message } }
  finally { if (request === optionsRequest) optionsLoading.value = false }
}
function filters() { return { keyword: keyword.value, status: status.value, ...(props.kind === 'distributions' ? { audience: audience.value } : {}), ...(versionID.value && props.kind !== 'versions' ? { version_id: versionID.value } : {}), ...(unitID.value !== '' ? { release_unit_id: unitID.value } : {}) } }
function load() { return resource.load(filters()) }
function resetFilters() {
  keyword.value = ''
  status.value = ''
  versionID.value = ''
  audience.value = ''
  unitID.value = ''
}
function closeVersionSettings(restoreFocus = false) {
  const settings = versionSettings.value
  if (!settings?.open) return
  settings.open = false
  if (restoreFocus) settings.querySelector('summary')?.focus()
}
function closeSettingsOnBlur(event) {
  if (!event.currentTarget.contains(event.relatedTarget)) closeVersionSettings()
}
function refreshTasks() {
  boardRevision.value++
  const applied = resource.filters.value
  return keyword.value === applied.keyword && status.value === applied.status && (versionID.value || '') === (applied.version_id || '') ? resource.refresh() : load()
}
function openTaskInGroup(id) { openEditor(taskGroups.value === null ? null : { group_id: id }) }
function confirmTaskRemoval(task) {
  editorOpen.value = false
  taskToDelete.value = task
}
async function updateTaskGroups() {
  await loadOptions()
  await refreshTasks()
}
function openEditor(row = null, expandDeployments = false) {
  if (props.kind === 'distributions' && !props.product.channels?.length) { error.value = '请先配置发行渠道'; return }
  detailVersion.value = null
  editing.value = row
  editDeployments.value = expandDeployments
  editorOpen.value = true
}
function updateUnits(units) {
  if (unitID.value && !units.some(unit => unit.id === unitID.value)) unitID.value = ''
  emit('update-release-units', units)
}
async function refreshWeChatRecords() {
  await Promise.all([resource.refresh(), loadOptions()])
  emit('changed')
}
async function save(record) {
  busy.value = true
  try { await saveLifecycleRecord(props.product.id, props.kind, record); editorOpen.value = false; await Promise.all([props.kind === 'tasks' ? refreshTasks() : resource.refresh(), loadOptions()]); emit('changed') }
  catch (cause) { error.value = cause.message } finally { busy.value = false }
}
async function quickAddTask() {
  const title = quickTaskTitle.value.trim()
  if (!title || busy.value) return
  busy.value = true
  try {
    await saveLifecycleRecord(props.product.id, 'tasks', { title, kind: 'action', priority: 'normal', status: 'todo' })
    quickTaskTitle.value = ''
    const isFiltered = keyword.value || versionID.value || (status.value && status.value !== 'todo')
    if (isFiltered) resetFilters()
    else await load()
    emit('changed')
  } catch (cause) {
    error.value = cause.message
  } finally {
    busy.value = false
  }
}
async function toggleTask(row, event) {
  const checked = row.status === 'done'
  event.target.checked = checked
  if (busy.value || row.status === 'cancelled') return
  busy.value = true
  try {
    await saveLifecycleRecord(props.product.id, 'tasks', { ...row, status: checked ? 'todo' : 'done' })
    await refreshTasks()
    emit('changed')
  } catch (cause) {
    error.value = cause.message
  } finally {
    busy.value = false
  }
}
async function removeTask() {
  if (busy.value || !taskToDelete.value) return
  busy.value = true
  try {
    await deleteProductTask(props.product.id, taskToDelete.value.id)
    taskToDelete.value = null
    await refreshTasks()
    emit('changed')
  } catch (cause) {
    error.value = cause.message
  } finally {
    busy.value = false
  }
}
async function importAction() { busy.value = true; try { await importProductNextAction(props.product.id); await refreshTasks(); emit('changed') } catch (cause) { error.value = cause.message } finally { busy.value = false } }
function showHistory(row) { history.reset(); historyRecord.value = row; void history.load() }
function showDeployments(row) {
  detailVersion.value = null
  deploymentRows.reset()
  deploymentVersion.value = row
  void deploymentRows.load()
}
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
watch(() => props.initialVersionId, value => { versionID.value = value; if (value) { taskView.value = 'list'; distributionFiltersOpen.value = true } })
watch([() => props.product.id, () => props.kind], () => { detailVersion.value = null; taskToDelete.value = null; repositoriesOpen.value = false; resource.reset(); void load(); void loadOptions() }, { immediate: true })
let missingChannelRefresh = ''
watch(() => {
  if (props.kind !== 'distributions') return ''
  const missing = [...new Set(resource.items.value.map(row => row.channel_id).filter(id => !channelRecord(id)))].sort((a, b) => a - b)
  return missing.length ? [props.product.id, ...missing].join(':') : ''
}, key => {
  if (!key) { missingChannelRefresh = ''; return }
  if (key === missingChannelRefresh) return
  missingChannelRefresh = key
  emit('changed')
})
onBeforeUnmount(() => { optionsRequest++; resource.cancel(); history.cancel(); deploymentRows.cancel() })
</script>
<style scoped>
.product-detail-content { display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--space-16); min-width: 0; }
.lifecycle-description { max-width: 320px; white-space: pre-wrap; overflow-wrap: anywhere; }
.task-view-toolbar { display: inline-grid; grid-template-columns: repeat(2, 1fr); padding: 2px; border: 1px solid var(--border-muted); border-radius: var(--radius-control); background: var(--surface-subtle); }
.task-view-toolbar .btn { padding-inline: var(--space-12); border-color: transparent; background: transparent; color: var(--text-secondary); font-size: var(--button-font-size); transform: none; }
.task-view-toolbar .btn:hover { color: var(--text-primary); }
.task-view-toolbar .btn[aria-pressed="true"] { border-color: var(--border-muted); background: var(--surface-raised); color: var(--text-primary); }
.task-quick-add { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; gap: var(--space-8); margin: 0 var(--space-20) var(--space-12); padding: var(--space-4) var(--space-8) var(--space-4) var(--space-12); border: 1px solid var(--border-muted); border-radius: var(--radius-control); background: var(--surface-input); color: var(--action-primary); }
.task-quick-add .form-input { width: 100%; padding-inline: var(--space-4); border: 0; background: transparent; }
.task-quick-add:focus-within { border-color: var(--focus); }
.task-quick-add .form-input:focus-visible { outline: none; }
.task-list-card .product-toolbar { flex-direction: row; align-items: center; flex-wrap: wrap; padding-top: var(--space-12); padding-bottom: var(--space-12); }
.task-list-card .product-search { flex: 1 1 130px; width: auto; min-width: 0; max-width: 320px; }
.task-list-card .product-stage-filter { width: 140px; }
.task-list-actions { display: flex; flex: none; align-items: center; gap: var(--space-12); margin-inline-start: auto; white-space: nowrap; }
.task-list-card > .task-empty { min-height: 100px; margin-inline: var(--space-20); padding-block: var(--space-16); border-top: 1px solid var(--border-muted); }
.task-list { margin: 0; padding: 0 var(--space-20) var(--space-4); list-style: none; }
.lifecycle-version-cell { max-width: 240px; overflow-wrap: anywhere; }
.lifecycle-version-description { width: 55%; min-width: 160px; }
.lifecycle-version-preview { contain: inline-size; }
.lifecycle-version-notes { white-space: pre-wrap; overflow-wrap: anywhere; }
.version-toolbar-actions { display: flex; flex: none; align-items: center; justify-content: flex-end; gap: var(--space-8); margin-inline-start: auto; }
.distribution-list-card .product-toolbar { flex-direction: row; flex-wrap: wrap; }
.distribution-list-card .data-table { min-width: 840px; }
.distribution-channel-cell { min-width: 200px; max-width: 300px; }
.distribution-version-cell, .distribution-audience-cell { white-space: nowrap; }
.distribution-environment-cell { min-width: 132px; }
.distribution-channel-name, .distribution-channel-identifier { overflow-wrap: anywhere; }
.distribution-channel-link { display: flex; align-items: center; gap: var(--space-4); width: fit-content; max-width: 100%; margin-top: var(--space-4); color: var(--action-primary-hover); font-size: 11px; text-decoration: none; }
.distribution-channel-link svg { flex-shrink: 0; }
.distribution-channel-link span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.distribution-channel-link:hover { text-decoration: underline; }
.distribution-channel-link:focus-visible { outline: 2px solid var(--focus); outline-offset: 2px; }
.distribution-list-card .product-search { flex: 1 1 160px; min-width: 0; width: auto; max-width: 320px; }
.distribution-toolbar-actions { display: flex; align-items: center; gap: var(--space-8); margin-inline-start: auto; }
.distribution-filter-toggle[aria-expanded="true"] { border-color: var(--action-primary); background: var(--surface-hover); }
.distribution-filters { display: flex; flex-wrap: wrap; gap: var(--space-8); padding: 0 var(--space-20) var(--space-12); }
.distribution-filters > .select-menu { flex: 1 1 140px; min-width: 0; max-width: 220px; }
.distribution-list-card > .empty-state { min-height: 140px; }
.version-list-card:has(.version-settings[open]) { overflow: visible; }
.version-settings { position: relative; }
.version-settings summary { list-style: none; }
.version-settings summary::-webkit-details-marker { display: none; }
.version-settings-actions { position: absolute; z-index: 2; top: calc(100% + var(--space-4)); right: 0; display: grid; gap: var(--space-4); min-width: 128px; padding: var(--space-4); border: 1px solid var(--border-muted); border-radius: var(--radius-control); background: var(--surface-raised); box-shadow: var(--shadow); }
.version-settings-actions .btn { justify-content: flex-start; border-color: transparent; background: transparent; transform: none; }
.version-settings-actions .btn:hover { background: var(--surface-hover); }
.version-detail-link { padding: 0; border: 0; background: transparent; color: var(--action-primary-hover); font: inherit; font-weight: 650; text-align: start; cursor: pointer; }
.version-detail-link:hover { text-decoration: underline; }
.version-detail-link:focus-visible { outline: 2px solid var(--focus); outline-offset: 2px; }
.version-detail-link:disabled { opacity: .5; cursor: default; }
.version-detail-actions { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: var(--space-8); }
.lifecycle-list-count { padding: var(--space-12) var(--space-20); }
@media (max-width: 760px) {
  .distribution-list-card .product-search { flex-basis: 100%; max-width: none; }
  .distribution-filters { padding-inline: 14px; }
  .distribution-filters > .select-menu { flex-basis: 100%; max-width: none; }
  .task-list-card .product-toolbar { display: grid; grid-template-columns: minmax(0, 1fr) auto; }
  .task-list-card .product-search, .task-list-card .product-stage-filter { grid-column: 1 / -1; width: 100%; max-width: none; }
  .task-list-card .product-toolbar, .task-list { padding-inline: 14px; }
  .task-quick-add, .task-list-card > .task-empty { margin-inline: 14px; }
}
@media (max-width: 640px) {
  .lifecycle-list-count { padding-inline: 14px; }
}
</style>
