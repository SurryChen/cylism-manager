<template>
  <BaseModal teleport :open="open" :title="`${form.id ? '编辑' : '新增'}${noun}`" :size="kind === 'versions' ? 'medium' : 'large'" :show-close="!busy" :close-on-overlay="!busy" :close-on-escape="!busy" @close="$emit('close')">
    <form :id="formID" @submit.prevent="submit">
      <fieldset :disabled="busy" class="lifecycle-fields product-form-grid">
        <template v-if="kind === 'tasks'">
          <label class="form-group product-full-width"><span class="form-label">事项标题 *</span><input v-model.trim="form.title" class="form-input" aria-label="事项标题" maxlength="512" required /></label>
          <label class="form-group"><span class="form-label">类型</span><SelectMenu v-model="form.kind" aria-label="事项类型"><option v-for="item in taskKinds" :key="item.id" :value="item.id">{{ item.label }}</option></SelectMenu></label>
          <label class="form-group"><span class="form-label">优先级</span><SelectMenu v-model="form.priority" aria-label="事项优先级"><option v-for="item in priorities" :key="item.id" :value="item.id">{{ item.label }}</option></SelectMenu></label>
        </template>
        <template v-else-if="kind === 'versions'">
          <label class="form-group"><span class="form-label">版本号 *</span><input v-model.trim="form.name" class="form-input" maxlength="128" placeholder="例如 1.1.0" required /></label>
          <label class="form-group"><span class="form-label">正式发版日期{{ form.id && !record?.released_on ? '（历史记录可留空）' : ' *' }}</span><input v-model="form.released_on" class="form-input" type="date" aria-label="正式发版日期" :required="!form.id || Boolean(record?.released_on)" /></label>
          <label v-if="releaseUnits.length" class="form-group product-full-width"><span class="form-label">发布端 *</span><SelectMenu v-model="form.release_unit_id" aria-label="版本发布端"><option value="" disabled>请选择发布端</option><option :value="0">项目默认</option><option v-for="unit in releaseUnits" :key="unit.id" :value="unit.id">{{ unit.name }}</option></SelectMenu></label>
        </template>
        <div v-if="kind === 'tasks' && form.id" class="form-group">
          <label class="form-label" :for="taskVersionsID">关联发版（可选）</label>
          <SelectMenu :id="taskVersionsID" v-model="form.version_ids" v-model:search.trim="versionKeyword" multiple searchable :disabled="busy" :options="versionOptions" aria-label="关联发版" search-label="搜索发布端或版本" :placeholder="form.version_ids.length ? `已选 ${form.version_ids.length} 个版本` : '选择已交付的版本'">
            <template #feedback><span v-if="!versionOptions.length" class="product-muted">没有匹配的版本</span></template>
          </SelectMenu>
        </div>
        <template v-if="kind === 'distributions'">
          <label class="form-group"><span class="form-label">发行渠道 *</span><SelectMenu v-model="form.channel_id" aria-label="发行渠道" :disabled="Boolean(form.id)" @change="form.version_id = ''"><option :value="0">请选择渠道</option><option v-for="channel in channels" :key="channel.id" :value="channel.id">{{ channelOptionLabel(channel) }}</option></SelectMenu></label>
          <label class="form-group">
            <span class="form-label">关联正式版本</span>
            <SelectMenu v-model="form.version_id" aria-label="关联产品版本" :disabled="Boolean(record?.version_id) || !form.channel_id">
              <option value="">暂不关联</option>
              <option v-for="version in channelVersions" :key="version.id" :value="version.id">{{ versionLabel(releaseUnits, version) }}</option>
            </SelectMenu>
          </label>
          <label class="form-group"><span class="form-label">发行分类</span><SelectMenu v-model="form.audience" aria-label="发行分类"><option v-for="item in distributionAudiences" :key="item.id" :value="item.id">{{ item.label }}</option></SelectMenu></label>
          <label class="form-group"><span class="form-label">部署环境（可选）</span><SelectMenu :model-value="form.environment_id || 0" aria-label="发行部署环境" @update:model-value="setEnvironment"><option :value="0">暂不关联环境</option><option v-for="environment in environments" :key="environment.id" :value="environment.id">{{ environment.project_name }} · {{ environment.name }}（{{ environmentKindLabel(environment.kind) }}）</option></SelectMenu></label>
          <div v-if="form.environment_id" class="product-full-width version-association">
            <button type="button" class="version-association-toggle" aria-label="关联环境部署" :aria-expanded="deploymentsOpen" :aria-controls="deploymentPanelID" @click="deploymentsOpen = !deploymentsOpen">
              <span>关联该环境的部署组件</span>
              <span v-if="form.deployment_ids?.length" class="cell-secondary">已选择 {{ form.deployment_ids.length }} 个组件</span>
              <ChevronDown :size="15" aria-hidden="true" />
            </button>
            <div v-if="deploymentsOpen" :id="deploymentPanelID" class="version-association-body">
              <div class="version-deployment-tools">
                <input ref="deploymentSearch" v-model.trim="deploymentKeyword" class="form-input" aria-label="搜索环境部署" placeholder="搜索组件或版本" />
                <button v-if="form.deployment_ids?.length" type="button" class="btn" @click="form.deployment_ids = []">清空选择</button>
              </div>
              <EmptyState v-if="deployments.loading.value" variant="loading" message="正在读取该环境的部署记录..." />
              <EmptyState v-else-if="deployments.error.value" message="部署记录读取失败"><template #action><button type="button" class="btn btn-sm" @click="loadDeployments">重试</button></template></EmptyState>
              <div v-else class="product-checklist"><label v-for="deployment in deployments.items.value" :key="deployment.id"><input type="checkbox" :value="deployment.id" :checked="deploymentSelected(deployment)" @change="setDeploymentSelected(deployment, $event.target.checked)" />{{ deployment.application_name }} · {{ deployment.version || `#${deployment.id}` }} · {{ deployment.status }}</label><span v-if="!deployments.total.value" class="product-muted">该环境暂无可关联的部署记录，可先保存发行计划，部署后再补关联。</span></div>
              <ResourcePagination v-if="deployments.total.value" :resource="deployments" label="环境部署分页" :show-error="false" hide-single-page />
            </div>
          </div>
        </template>
        <label v-if="kind !== 'versions'" class="form-group"><span class="form-label">状态</span><SelectMenu v-model="form.status" aria-label="记录状态"><option v-for="item in statuses" :key="item.id" :value="item.id">{{ item.label }}</option></SelectMenu></label>
        <label v-if="kind === 'tasks' && taskGroups.length" class="form-group">
          <span class="form-label">待办分组</span>
          <SelectMenu v-model="form.group_id" aria-label="待办分组">
            <option :value="0">未分组</option>
            <option v-for="group in taskGroups" :key="group.id" :value="group.id">{{ group.name }}</option>
          </SelectMenu>
        </label>
        <label v-if="kind === 'tasks'" class="form-group"><span class="form-label">截止日期（可选）</span><input v-model="form.due_date" type="date" class="form-input" aria-label="计划日期" /></label>
        <label class="form-group product-full-width">
          <span class="form-label">{{ descriptionLabel }}</span>
          <textarea v-model="form[kind === 'tasks' ? 'details' : 'notes']" class="form-input" :aria-label="kind === 'versions' ? '版本说明' : '记录说明'" maxlength="8000" />
        </label>
        <template v-if="kind === 'distributions'">
          <div class="product-full-width">
            <button v-if="form.version_id" type="button" class="btn btn-sm" :aria-expanded="channelFieldsOpen" :aria-controls="channelFieldsID" @click="channelFieldsOpen = !channelFieldsOpen">渠道版本与构建标识</button>
            <div v-show="channelFieldsOpen || !form.version_id" :id="channelFieldsID" class="product-form-grid" :class="{ 'lifecycle-extra-panel': form.version_id }">
              <label class="form-group">
                <span class="form-label">渠道版本号{{ form.version_id ? '（可选）' : ' *' }}</span>
                <input v-model.trim="form.platform_version" class="form-input" maxlength="128" :required="!form.version_id" :placeholder="selectedVersionName || '例如 2.8.1'" />
              </label>
              <label class="form-group">
                <span class="form-label">构建标识（可选）</span>
                <input v-model.trim="form.build_identifier" class="form-input" maxlength="128" placeholder="例如 versionCode" />
              </label>
            </div>
          </div>
        </template>
        <div v-if="kind === 'versions'" class="product-full-width version-associations">
          <VersionTaskPicker
            v-if="open"
            v-model="form.task_ids"
            :product-id="productId"
            :version-id="form.id"
            :disabled="busy"
          />
          <div class="version-association">
            <button type="button" class="version-association-toggle" aria-label="关联 GitHub 版本" :aria-expanded="githubOpen" :aria-controls="githubPanelID" @click="githubOpen = !githubOpen">
              <span>关联 GitHub 版本</span>
              <span v-if="form.github_sources?.length" class="cell-secondary">已关联 {{ form.github_sources.length }} 个组件</span>
              <ChevronDown :size="15" aria-hidden="true" />
            </button>
            <div v-show="githubOpen" :id="githubPanelID" class="version-association-body">
              <VersionGitHubPicker v-if="githubOpen && open" v-model="form.github_sources" :product-id="productId" :release-unit-id="form.release_unit_id" :disabled="busy" @error="$emit('error', $event)" />
            </div>
          </div>
          <div class="version-association">
            <button type="button" class="version-association-toggle" aria-label="关联部署记录" :aria-expanded="deploymentsOpen" :aria-controls="deploymentPanelID" @click="deploymentsOpen = !deploymentsOpen">
              <span>关联部署记录</span>
              <span v-if="deploymentCount(form)" class="cell-secondary">已选择 {{ deploymentCount(form) }} 条</span>
              <ChevronDown :size="15" aria-hidden="true" />
            </button>
            <div v-show="deploymentsOpen" :id="deploymentPanelID" class="version-association-body">
              <template v-if="deploymentsOpen">
                <div class="version-deployment-tools">
                  <input ref="deploymentSearch" v-model.trim="deploymentKeyword" class="form-input" aria-label="搜索部署记录" placeholder="搜索组件或版本" />
                  <button v-if="deploymentCount(form)" type="button" class="btn" @click="form.deployment_ids = []; form.platform_release_ids = []">清空选择</button>
                </div>
                <EmptyState v-if="deployments.loading.value" variant="loading" message="正在读取部署记录..." />
                <EmptyState v-else-if="deployments.error.value" message="部署记录读取失败"><template #action><button type="button" class="btn btn-sm" @click="loadDeployments">重试</button></template></EmptyState>
                <div v-else class="product-checklist"><label v-for="deployment in deployments.items.value" :key="`${deployment.source || 'application'}-${deployment.id}`"><input type="checkbox" :value="deployment.id" :checked="deploymentSelected(deployment)" @change="setDeploymentSelected(deployment, $event.target.checked)" />{{ deployment.application_name }} · {{ deployment.environment_name || '未命名环境' }}（{{ environmentKindLabel(deployment.environment_kind) }}） · {{ deployment.version || `#${deployment.id}` }} · {{ deployment.status }}</label><span v-if="!deployments.total.value" class="product-muted">暂无可关联的部署记录</span></div>
                <ResourcePagination v-if="deployments.total.value" :resource="deployments" label="可关联部署分页" :show-error="false" hide-single-page />
              </template>
            </div>
          </div>
        </div>
      </fieldset>
    </form>
    <template #actions>
      <button v-if="kind === 'tasks' && form.id" type="button" class="btn btn-danger task-editor-remove" :disabled="busy" @click="emit('remove', record)">删除事项</button>
      <button class="btn" :disabled="busy" @click="$emit('close')">取消</button>
      <button class="btn btn-primary" :disabled="busy" type="submit" :form="formID">{{ busy ? '保存中...' : `保存${noun}` }}</button>
    </template>
  </BaseModal>
</template>
<script setup>
import { distributionAudiences, environmentKindLabel } from './environment-utils.js'
import { computed, nextTick, ref, useId, watch } from 'vue'
import { ChevronDown } from 'lucide-vue-next'
import BaseModal from '../../components/BaseModal.vue'
import EmptyState from '../../components/EmptyState.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import ResourcePagination from '../../components/ResourcePagination.vue'
import VersionTaskPicker from './VersionTaskPicker.vue'
import VersionGitHubPicker from './VersionGitHubPicker.vue'
import { usePaginatedResource } from '../../composables/usePaginatedResource.js'
import { getLifecyclePage } from '../../api/product-lifecycle.js'
import { labelFor, localDate, platforms } from './product-utils.js'
import { taskKinds, taskStatuses, priorities, distributionStatuses, deploymentCount, versionLabel } from './lifecycle-utils.js'
const props = defineProps({ open: Boolean, busy: Boolean, expandDeployments: Boolean, kind: { type: String, required: true }, productId: { type: Number, required: true }, record: { type: Object, default: null }, versions: { type: Array, default: () => [] }, channels: { type: Array, default: () => [] }, releaseUnits: { type: Array, default: () => [] }, environments: { type: Array, default: () => [] }, groups: { type: Array, default: () => [] } })
const taskGroups = computed(() => props.groups || [])
const emit = defineEmits(['close', 'save', 'remove', 'error'])
const formID = `lifecycle-${useId()}`
const deploymentPanelID = `${formID}-deployments`
const githubPanelID = `${formID}-github`
const channelFieldsID = `${formID}-channel-version`
const taskVersionsID = `${formID}-task-versions`
const form = ref({})
const versionKeyword = ref('')
const channelFieldsOpen = ref(false)
const selectedVersionName = computed(() => props.versions.find(version => version.id === Number(form.value.version_id))?.name)
const versionOptions = computed(() => props.versions
  .map(version => ({ value: version.id, label: versionLabel(props.releaseUnits, version) }))
  .filter(option => option.label.toLowerCase().includes(versionKeyword.value.toLowerCase())))
const channelVersions = computed(() => {
  const unitID = props.channels.find(channel => channel.id === Number(form.value.channel_id))?.release_unit_id
  return props.versions.filter(version => unitID == null || (version.release_unit_id ?? 0) === unitID || version.id === props.record?.version_id)
})
const deploymentsOpen = ref(false)
const githubOpen = ref(false)
const deploymentSearch = ref(null)
const deploymentKeyword = ref('')
function channelOptionLabel(channel) {
  const platform = labelFor(platforms, channel.platform)
  return channel.name && channel.name !== platform ? platform + ' · ' + channel.name : platform
}
const noun = computed(() => ({ tasks: '事项', versions: '版本', distributions: '发行记录' })[props.kind])
const statuses = computed(() => props.kind === 'tasks' ? taskStatuses : distributionStatuses)
const descriptionLabel = computed(() => ({ tasks: '说明与处理结果', versions: '版本说明（可选）', distributions: '备注 / 发行说明' })[props.kind])
const deployments = usePaginatedResource((query, options) => getLifecyclePage(props.productId, 'deployments', query, options), 'items')
function deploymentField(deployment) { return deployment.source === 'platform' ? 'platform_release_ids' : 'deployment_ids' }
function deploymentSelected(deployment) { return form.value[deploymentField(deployment)]?.includes(deployment.id) }
function setDeploymentSelected(deployment, selected) {
  const field = deploymentField(deployment), ids = form.value[field] || []
  form.value[field] = selected ? [...new Set([...ids, deployment.id])] : ids.filter(id => id !== deployment.id)
}
function loadDeployments() {
  const query = { keyword: deploymentKeyword.value }
  if (props.kind === 'distributions') {
    if (!form.value.environment_id) return deployments.reset()
    Object.assign(query, { environment_id: form.value.environment_id })
  } else if (props.record?.id) Object.assign(query, { version_id: props.record.id, include_available: true })
  return deployments.load(query)
}
function setEnvironment(value) {
  const environmentID = Number(value) || 0
  if (environmentID === form.value.environment_id) return
  form.value.environment_id = environmentID
  form.value.deployment_ids = []
  deploymentKeyword.value = ''
  deployments.reset()
}
watch([deploymentsOpen, deploymentKeyword, () => form.value.environment_id], ([expanded, , environmentID]) => {
  if (expanded && (props.kind !== 'distributions' || environmentID)) void loadDeployments()
})
watch(() => props.open, open => {
  deploymentsOpen.value = false
  githubOpen.value = false
  versionKeyword.value = ''
  if (!open) { deployments.cancel(); return }
  const defaults = {
    tasks: { title: '', kind: 'action', priority: 'normal', status: 'todo', version_id: '', due_date: '', details: '' },
    versions: { name: '', released_on: props.record ? '' : localDate(), release_unit_id: props.record ? 0 : props.releaseUnits.length < 2 ? (props.releaseUnits[0]?.id || 0) : '', notes: '' },
    distributions: { audience: 'testing', channel_id: 0, version_id: '', environment_id: 0, deployment_ids: [], platform_version: '', build_identifier: '', status: 'draft', notes: '' },
  }[props.kind]
  form.value = { ...defaults, ...JSON.parse(JSON.stringify(props.record || {})) }
  if (props.kind !== 'versions') form.value.version_id ??= ''
  if (props.kind === 'tasks') {
    form.value.version_ids ??= form.value.version_id ? [form.value.version_id] : []
    if (taskGroups.value.length) form.value.group_id ??= 0
  }
  if (props.kind === 'distributions') {
    form.value.deployment_ids ||= []
    channelFieldsOpen.value = Boolean(form.value.build_identifier || (form.value.platform_version && form.value.platform_version !== selectedVersionName.value))
    deployments.reset()
    deploymentKeyword.value = ''
    deploymentsOpen.value = false
  }
  if (props.kind === 'versions') {
    form.value.github_sources ||= []
    form.value.task_ids ||= []
    form.value.deployment_ids ||= []
    form.value.platform_release_ids ||= []
    deployments.reset()
    deploymentKeyword.value = ''
    deploymentsOpen.value = props.expandDeployments
    if (props.expandDeployments) void nextTick(() => deploymentSearch.value?.focus())
  }
}, { immediate: true })
function submit() {
  if (props.kind === 'versions' && (form.value.release_unit_id === '' || (!form.value.id && !form.value.released_on))) {
    emit('error', '请选择发布端并填写正式发版日期')
    return
  }
  if (props.kind === 'distributions' && (!form.value.channel_id || (!form.value.version_id && !form.value.platform_version))) {
    emit('error', '请选择发行渠道，并关联正式版本或填写渠道版本号')
    return
  }
  emit('save', {
    ...form.value,
    ...(props.kind === 'tasks' ? { version_id: form.value.version_ids[0] ?? null } : {}),
    ...(props.kind === 'distributions' ? { version_id: Number(form.value.version_id), channel_id: Number(form.value.channel_id), environment_id: Number(form.value.environment_id || 0), deployment_ids: form.value.deployment_ids || [] } : {}),
  })
}
</script>
<style scoped>
.lifecycle-fields { border: 0; padding: 0; margin: 0; min-width: 0; }
.lifecycle-extra-panel { margin-top: var(--space-12); }
.version-associations { display: grid; }
.version-associations > .form-group { margin-bottom: var(--space-12); }
.version-association { min-width: 0; border-top: 1px solid var(--border-muted); }
.version-association:last-child { border-bottom: 1px solid var(--border-muted); }
.version-association-toggle { display: flex; align-items: center; gap: var(--space-8); width: 100%; min-height: var(--button-height); padding: var(--space-8) 0; border: 0; background: transparent; color: var(--text-primary); font: inherit; font-size: var(--button-font-size); font-weight: 650; text-align: start; cursor: pointer; }
.version-association-toggle:hover:not(:disabled) { color: var(--action-primary-hover); }
.version-association-toggle:focus-visible { outline: 2px solid var(--focus); outline-offset: -2px; border-radius: var(--radius-control); }
.version-association-toggle:disabled { cursor: default; }
.version-association-toggle > .cell-secondary { margin-inline-start: auto; font-weight: 400; }
.version-association-toggle > svg { flex: none; margin-inline-start: auto; color: var(--text-secondary); }
.version-association-toggle > .cell-secondary + svg { margin-inline-start: 0; }
.version-association-toggle[aria-expanded="true"] > svg { transform: rotate(180deg); }
.version-association-body { padding: var(--space-4) 0 var(--space-12); }
.version-association-body :deep(.empty-state) { min-height: 72px; }
.version-deployment-tools { display: flex; align-items: center; gap: var(--space-8); }
.version-deployment-tools > input { min-width: 0; }
.task-editor-remove { margin-right: auto; }
</style>
