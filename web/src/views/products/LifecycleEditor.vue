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
          <label class="form-group product-full-width"><span class="form-label">产品版本名称 *</span><input v-model.trim="form.name" class="form-input" aria-label="版本名称" maxlength="128" placeholder="例如：1.1.0" required /></label>
          <label class="form-group product-full-width"><span class="form-label">本次目标</span><textarea v-model="form.goal" class="form-input" aria-label="版本目标" maxlength="1024" /></label>
          <label class="form-group product-full-width"><span class="form-label">验收项与验证结果</span><textarea v-model="form.acceptance" class="form-input" aria-label="验收项" maxlength="8000" placeholder="记录需要验证什么，以及实际验证结果" /></label>
        </template>
        <template v-else>
          <p class="product-muted product-full-width">每条记录对应一次渠道发行。重新提交可新增记录，编辑会保留状态和备注历史。正式渠道进度仅汇总正式发行，测试发行单独保留。</p>
          <label class="form-group product-full-width"><span class="form-label">发行分类</span><SelectMenu v-model="form.audience" aria-label="发行分类"><option v-for="item in distributionAudiences" :key="item.id" :value="item.id">{{ item.label }}</option></SelectMenu></label>
          <label class="form-group"><span class="form-label">发行渠道 *</span><SelectMenu v-model="form.channel_id" aria-label="发行渠道" :disabled="Boolean(form.id)" required><option :value="0">请选择渠道</option><option v-for="channel in channels" :key="channel.id" :value="channel.id">{{ channel.name || labelFor(platforms, channel.platform) }}</option></SelectMenu></label>
          <label class="form-group"><span class="form-label">平台版本 *</span><input v-model.trim="form.platform_version" class="form-input" aria-label="平台版本" maxlength="128" required /></label>
          <label class="form-group product-full-width"><span class="form-label">Build 标识</span><input v-model.trim="form.build_identifier" class="form-input" aria-label="Build 标识" maxlength="128" placeholder="versionCode / CFBundleVersion / 小程序版本标识" /></label>
        </template>
        <label v-if="kind !== 'versions'" class="form-group"><span class="form-label">产品版本{{ kind === 'distributions' ? ' *' : '（可选）' }}</span><SelectMenu v-model="form.version_id" aria-label="关联产品版本" :disabled="kind === 'distributions' && Boolean(form.id)"><option :value="''">{{ kind === 'tasks' ? '暂不安排版本' : '请选择版本' }}</option><option v-for="version in versions" :key="version.id" :value="version.id">{{ version.name }}</option></SelectMenu></label>
        <label class="form-group"><span class="form-label">状态</span><SelectMenu v-model="form.status" aria-label="记录状态"><option v-for="item in statuses" :key="item.id" :value="item.id">{{ item.label }}</option></SelectMenu></label>
        <label v-if="kind !== 'distributions'" class="form-group"><span class="form-label">{{ kind === 'tasks' ? '截止日期' : '目标日期' }}（可选）</span><input v-model="form[dateField]" type="date" class="form-input" aria-label="计划日期" /></label>
        <template v-if="kind === 'versions'">
          <label v-for="field in sourceFields" :key="field.key" class="form-group" :class="{ 'product-full-width': field.key !== 'commit_sha' }"><span class="form-label">{{ field.label }}</span><input v-model.trim="form[field.key]" class="form-input" :type="field.key === 'commit_sha' ? 'text' : 'url'" :maxlength="field.key === 'commit_sha' ? 128 : 1024" :aria-label="field.label" /></label>
          <div class="product-full-width">
            <h3 class="product-section-title">关联部署与平台更新（可选）</h3>
            <p class="product-muted">仅列出此产品关联的项目、组件或当前管理平台的记录；选择记录不会触发部署。App 和小程序可留空。</p>
            <input v-model.trim="deploymentKeyword" class="form-input" aria-label="搜索部署记录" placeholder="搜索组件或版本" />
            <EmptyState v-if="deployments.loading.value" variant="loading" message="正在读取部署记录..." />
            <EmptyState v-else-if="deployments.error.value" message="部署记录读取失败"><template #action><button type="button" class="btn btn-sm" @click="loadDeployments">重试</button></template></EmptyState>
            <div v-else class="product-checklist"><label v-for="deployment in deployments.items.value" :key="`${deployment.source || 'application'}-${deployment.id}`"><input type="checkbox" :value="deployment.id" :checked="deploymentSelected(deployment)" @change="setDeploymentSelected(deployment, $event.target.checked)" />{{ deployment.application_name }} · {{ deployment.environment_name || '未命名环境' }}（{{ environmentKindLabel(deployment.environment_kind) }}） · {{ deployment.version || `#${deployment.id}` }} · {{ deployment.status }}</label><span v-if="!deployments.total.value" class="product-muted">暂无可关联的部署记录</span></div>
            <ResourcePagination :resource="deployments" label="可关联部署分页" />
            <div class="product-actions"><span class="product-muted">已选择 {{ deploymentCount(form) }} 条</span><button v-if="deploymentCount(form)" type="button" class="btn btn-sm" @click="form.deployment_ids = []; form.platform_release_ids = []">清空选择</button></div>
          </div>
        </template>
        <label class="form-group product-full-width"><span class="form-label">{{ kind === 'tasks' ? '说明与处理结果' : '备注 / 发行说明' }}</span><textarea v-model="form[kind === 'tasks' ? 'details' : 'notes']" class="form-input" aria-label="记录说明" maxlength="8000" /></label>
      </fieldset>
    </form>
    <template #actions><button class="btn" :disabled="busy" @click="$emit('close')">取消</button><button class="btn btn-primary" :disabled="busy" type="submit" :form="formID">{{ busy ? '保存中...' : `保存${noun}` }}</button></template>
  </BaseModal>
</template>
<script setup>
import { distributionAudiences, environmentKindLabel } from './environment-utils.js'
import { computed, ref, useId, watch } from 'vue'
import BaseModal from '../../components/BaseModal.vue'
import EmptyState from '../../components/EmptyState.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import ResourcePagination from '../../components/ResourcePagination.vue'
import { usePaginatedResource } from '../../composables/usePaginatedResource.js'
import { getLifecyclePage } from '../../api/product-lifecycle.js'
import { labelFor, platforms } from './product-utils.js'
import { taskKinds, taskStatuses, priorities, versionStatuses, distributionStatuses, deploymentCount } from './lifecycle-utils.js'
const props = defineProps({ open: Boolean, busy: Boolean, kind: { type: String, required: true }, productId: { type: Number, required: true }, record: { type: Object, default: null }, versions: { type: Array, default: () => [] }, channels: { type: Array, default: () => [] } })
const emit = defineEmits(['close', 'save', 'error'])
const formID = `lifecycle-${useId()}`
const form = ref({}), deploymentKeyword = ref('')
const noun = computed(() => ({ tasks: '事项', versions: '版本', distributions: '发行记录' })[props.kind])
const statuses = computed(() => ({ tasks: taskStatuses, versions: versionStatuses, distributions: distributionStatuses })[props.kind])
const dateField = computed(() => props.kind === 'tasks' ? 'due_date' : 'target_date')
const sourceFields = [{ key: 'repository_url', label: '代码仓库链接' }, { key: 'commit_sha', label: '提交 SHA' }, { key: 'build_url', label: '构建记录链接' }, { key: 'artifact_url', label: '产物链接（不含访问密钥）' }]
const deployments = usePaginatedResource((query, options) => getLifecyclePage(props.productId, 'deployments', query, options), 'items')
function deploymentField(deployment) { return deployment.source === 'platform' ? 'platform_release_ids' : 'deployment_ids' }
function deploymentSelected(deployment) { return form.value[deploymentField(deployment)]?.includes(deployment.id) }
function setDeploymentSelected(deployment, selected) {
  const field = deploymentField(deployment), ids = form.value[field] || []
  form.value[field] = selected ? [...new Set([...ids, deployment.id])] : ids.filter(id => id !== deployment.id)
}
function loadDeployments() { return deployments.load({ keyword: deploymentKeyword.value }) }
watch(deploymentKeyword, () => { if (props.open && props.kind === 'versions') void loadDeployments() })
watch(() => props.open, open => {
  if (!open) { deployments.cancel(); return }
  const defaults = props.kind === 'tasks' ? { title: '', kind: 'action', priority: 'normal', status: 'todo', version_id: '', due_date: '', details: '' } : props.kind === 'versions' ? { name: '', goal: '', acceptance: '', status: 'planning', target_date: '', repository_url: '', commit_sha: '', build_url: '', artifact_url: '', deployment_ids: [], platform_release_ids: [], notes: '' } : { audience: 'testing', channel_id: 0, version_id: '', platform_version: '', build_identifier: '', status: 'draft', notes: '' }
  form.value = { ...defaults, ...JSON.parse(JSON.stringify(props.record || {})) }
  if (props.kind !== 'versions') form.value.version_id ??= ''
  if (props.kind === 'versions') { form.value.deployment_ids ||= []; form.value.platform_release_ids ||= []; deployments.reset(); deploymentKeyword.value = ''; void loadDeployments() }
}, { immediate: true })
function submit() {
  if (props.kind === 'distributions' && (!form.value.version_id || !form.value.channel_id)) { emit('error', '请选择产品版本和发行渠道'); return }
  emit('save', { ...form.value, ...(props.kind !== 'versions' ? { version_id: form.value.version_id ? Number(form.value.version_id) : null } : {}), ...(props.kind === 'distributions' ? { channel_id: Number(form.value.channel_id) } : {}) })
}
</script>
<style scoped>
.lifecycle-fields { border: 0; padding: 0; margin: 0; min-width: 0; }
</style>
