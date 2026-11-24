<template>
  <BaseModal teleport :open="open" :title="`${form.id ? '编辑' : '新增'}${noun}`" size="large" :show-close="!busy" :close-on-overlay="!busy" :close-on-escape="!busy" @close="$emit('close')">
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
          <label class="form-group"><span class="form-label">发行渠道 *</span><SelectMenu v-model="form.channel_id" aria-label="发行渠道" :disabled="Boolean(form.id)" @change="form.version_id = ''"><option :value="0">请选择渠道</option><option v-for="channel in channels" :key="channel.id" :value="channel.id">{{ channel.name || labelFor(platforms, channel.platform) }}</option></SelectMenu></label>
          <label class="form-group">
            <span class="form-label">关联正式版本</span>
            <SelectMenu v-model="form.version_id" aria-label="关联产品版本" :disabled="Boolean(record?.version_id) || !form.channel_id">
              <option value="">暂不关联</option>
              <option v-for="version in channelVersions" :key="version.id" :value="version.id">{{ versionLabel(releaseUnits, version) }}</option>
            </SelectMenu>
          </label>
          <label class="form-group"><span class="form-label">发行分类</span><SelectMenu v-model="form.audience" aria-label="发行分类"><option v-for="item in distributionAudiences" :key="item.id" :value="item.id">{{ item.label }}</option></SelectMenu></label>
        </template>
        <label v-if="kind !== 'versions'" class="form-group"><span class="form-label">状态</span><SelectMenu v-model="form.status" aria-label="记录状态"><option v-for="item in statuses" :key="item.id" :value="item.id">{{ item.label }}</option></SelectMenu></label>
        <label v-if="kind === 'tasks'" class="form-group"><span class="form-label">截止日期（可选）</span><input v-model="form.due_date" type="date" class="form-input" aria-label="计划日期" /></label>
        <template v-if="kind === 'versions'">
          <VersionTaskPicker
            v-if="open"
            v-model="form.task_ids"
            :product-id="productId"
            :version-id="form.id"
            :disabled="busy"
          />
        </template>
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
        <template v-if="kind === 'versions'">
          <div class="product-full-width">
            <div class="product-actions">
              <button type="button" class="btn btn-sm" :aria-expanded="deploymentsOpen" :aria-controls="deploymentPanelID" @click="deploymentsOpen = !deploymentsOpen">关联部署记录</button>
              <span v-if="deploymentCount(form)" class="product-muted">已选择 {{ deploymentCount(form) }} 条</span>
              <button v-if="deploymentsOpen && deploymentCount(form)" type="button" class="btn btn-sm" @click="form.deployment_ids = []; form.platform_release_ids = []">清空选择</button>
            </div>
            <div v-show="deploymentsOpen" :id="deploymentPanelID" class="lifecycle-extra-panel">
              <template v-if="deploymentsOpen">
                <input ref="deploymentSearch" v-model.trim="deploymentKeyword" class="form-input" aria-label="搜索部署记录" placeholder="搜索组件或版本" />
                <EmptyState v-if="deployments.loading.value" variant="loading" message="正在读取部署记录..." />
                <EmptyState v-else-if="deployments.error.value" message="部署记录读取失败"><template #action><button type="button" class="btn btn-sm" @click="loadDeployments">重试</button></template></EmptyState>
                <div v-else class="product-checklist"><label v-for="deployment in deployments.items.value" :key="`${deployment.source || 'application'}-${deployment.id}`"><input type="checkbox" :value="deployment.id" :checked="deploymentSelected(deployment)" @change="setDeploymentSelected(deployment, $event.target.checked)" />{{ deployment.application_name }} · {{ deployment.environment_name || '未命名环境' }}（{{ environmentKindLabel(deployment.environment_kind) }}） · {{ deployment.version || `#${deployment.id}` }} · {{ deployment.status }}</label><span v-if="!deployments.total.value" class="product-muted">暂无可关联的部署记录</span></div>
                <ResourcePagination v-if="deployments.total.value" :resource="deployments" label="可关联部署分页" :show-error="false" />
              </template>
            </div>
          </div>
        </template>
      </fieldset>
    </form>
    <template #actions><button class="btn" :disabled="busy" @click="$emit('close')">取消</button><button class="btn btn-primary" :disabled="busy" type="submit" :form="formID">{{ busy ? '保存中...' : `保存${noun}` }}</button></template>
  </BaseModal>
</template>
<script setup>
import { distributionAudiences, environmentKindLabel } from './environment-utils.js'
import { computed, nextTick, ref, useId, watch } from 'vue'
import BaseModal from '../../components/BaseModal.vue'
import EmptyState from '../../components/EmptyState.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import ResourcePagination from '../../components/ResourcePagination.vue'
import VersionTaskPicker from './VersionTaskPicker.vue'
import { usePaginatedResource } from '../../composables/usePaginatedResource.js'
import { getLifecyclePage } from '../../api/product-lifecycle.js'
import { labelFor, localDate, platforms } from './product-utils.js'
import { taskKinds, taskStatuses, priorities, distributionStatuses, deploymentCount, versionLabel } from './lifecycle-utils.js'
const props = defineProps({ open: Boolean, busy: Boolean, expandDeployments: Boolean, kind: { type: String, required: true }, productId: { type: Number, required: true }, record: { type: Object, default: null }, versions: { type: Array, default: () => [] }, channels: { type: Array, default: () => [] }, releaseUnits: { type: Array, default: () => [] } })
const emit = defineEmits(['close', 'save', 'error'])
const formID = `lifecycle-${useId()}`
const deploymentPanelID = `${formID}-deployments`
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
const deploymentSearch = ref(null)
const deploymentKeyword = ref('')
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
  if (props.record?.id) Object.assign(query, { version_id: props.record.id, include_available: true })
  return deployments.load(query)
}
watch([deploymentsOpen, deploymentKeyword], ([expanded]) => {
  if (expanded) void loadDeployments()
})
watch(() => props.open, open => {
  deploymentsOpen.value = false
  versionKeyword.value = ''
  if (!open) { deployments.cancel(); return }
  const defaults = {
    tasks: { title: '', kind: 'action', priority: 'normal', status: 'todo', version_id: '', due_date: '', details: '' },
    versions: { name: '', released_on: props.record ? '' : localDate(), release_unit_id: props.record ? 0 : props.releaseUnits.length < 2 ? (props.releaseUnits[0]?.id || 0) : '', notes: '' },
    distributions: { audience: 'testing', channel_id: 0, version_id: '', platform_version: '', build_identifier: '', status: 'draft', notes: '' },
  }[props.kind]
  form.value = { ...defaults, ...JSON.parse(JSON.stringify(props.record || {})) }
  if (props.kind !== 'versions') form.value.version_id ??= ''
  if (props.kind === 'tasks') form.value.version_ids ??= form.value.version_id ? [form.value.version_id] : []
  if (props.kind === 'distributions') {
    channelFieldsOpen.value = Boolean(form.value.build_identifier || (form.value.platform_version && form.value.platform_version !== selectedVersionName.value))
  }
  if (props.kind === 'versions') {
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
    ...(props.kind === 'distributions' ? { version_id: Number(form.value.version_id), channel_id: Number(form.value.channel_id) } : {}),
  })
}
</script>
<style scoped>
.lifecycle-fields { border: 0; padding: 0; margin: 0; min-width: 0; }
.lifecycle-extra-panel { margin-top: var(--space-12); }
</style>
