<template>
  <section class="settings-section">
    <SurfaceCard>
      <div class="cloud-provider-toolbar">
        <button class="btn btn-primary" type="button" data-testid="cloud-provider-open-create" @click="openCreate">添加云提供商</button>
      </div>

      <div class="cloud-provider-table">
        <header class="cloud-provider-table-header">
          <span>连接名</span>
          <span>提供商</span>
          <span>状态</span>
          <span>操作</span>
        </header>

        <p v-if="loading" class="cloud-provider-row cloud-provider-row--message">正在读取云连接...</p>
        <template v-else-if="connections.length">
          <div v-for="item in connections" :key="item.id" class="cloud-provider-row" :data-testid="'cloud-provider-card-' + item.id">
            <span class="cloud-provider-name">{{ item.name }}</span>
            <span class="cloud-provider-value">{{ providerLabel(item.provider) }}<small>{{ credentialState(item) }}</small></span>
            <span><span class="badge" :class="statusBadgeClass(item)">{{ connectionState(item) }}</span></span>
            <span class="cloud-provider-actions">
              <button v-if="item.provider === 'aliyun'" class="btn btn-sm" type="button" :data-testid="'cloud-provider-permissions-' + item.id" @click="openPermissions(item)">查看权限</button>
              <button class="btn btn-sm" type="button" @click="openEdit(item)">编辑</button>
              <button class="btn btn-sm btn-danger" type="button" @click="remove(item)">删除</button>
            </span>
          </div>
        </template>
        <p v-else class="cloud-provider-row cloud-provider-row--message">还没有云提供商连接，添加后即可在域名管理和对象存储中使用对应服务。</p>
      </div>

      <p v-if="error" class="settings-copy endpoint-error">{{ error }}</p>
    </SurfaceCard>

    <BaseModal :open="formOpen" :title="editing ? '编辑云连接' : '添加云连接'" size="medium" :show-close="!saving" :close-on-overlay="!saving" :close-on-escape="!saving" @close="closeForm">
      <form id="cloud-provider-form" class="cloud-provider-form" autocomplete="off" @submit.prevent="save">
        <div class="cloud-provider-form-grid cloud-provider-form-grid--identity">
          <div class="form-group">
            <label class="form-label" for="cloud-provider-name">连接名称</label>
            <input id="cloud-provider-name" v-model.trim="form.name" class="form-input" :disabled="saving" required />
          </div>
          <div class="form-group">
            <label class="form-label" for="cloud-provider-provider">云提供商</label>
            <SelectMenu id="cloud-provider-provider" v-model="form.provider" :options="providerOptions" aria-label="云提供商" :disabled="saving || Boolean(editing)" required @change="selectProvider" />
            <small v-if="currentProvider" class="cloud-provider-field-hint">{{ currentProvider.description }}{{ currentProvider.implemented ? '' : '，当前尚未接通，暂不能保存。' }}</small>
          </div>
        </div>
        <section class="cloud-provider-form-section">
          <div class="cloud-provider-form-section-heading">
            <div><h3>访问凭据</h3><p>密钥会加密保存，编辑时不会回显。</p></div>
            <span v-if="editing" class="badge badge-online">凭据已保存</span>
          </div>
          <div class="cloud-provider-form-grid">
            <div v-for="field in credentialFields" :key="field.key" class="form-group">
              <label class="form-label" :for="'cloud-provider-credential-' + field.key">{{ field.label }}</label>
              <input :id="'cloud-provider-credential-' + field.key" v-model="credentialValues[field.key]" class="form-input" :type="field.type" autocomplete="new-password" :disabled="saving" :required="field.required && !editing" :placeholder="editing ? '留空表示保留已保存值' : field.placeholder" />
            </div>
          </div>
        </section>
        <p v-if="!currentProvider?.implemented" class="settings-copy form-hint">该供应商适配器尚未接入，暂不能保存连接。</p>
      </form>
      <template #actions>
        <button class="btn btn-sm" type="button" :disabled="saving" @click="closeForm">取消</button>
        <button class="btn btn-sm btn-primary" type="button" data-testid="cloud-provider-save" :disabled="saving || !currentProvider?.implemented" @click="save">{{ saving ? '保存中...' : '保存连接' }}</button>
      </template>
    </BaseModal>

    <BaseModal :open="Boolean(permissionTarget)" :title="permissionTarget ? permissionTarget.name + ' · 权限' : '连接权限'" size="medium" @close="closePermissions">
      <div class="cloud-permission-view" aria-live="polite">
        <p v-if="permissionLoading" class="cloud-permission-message">正在读取 RAM 授权策略...</p>
        <p v-else-if="permissionError" class="cloud-permission-message endpoint-error">{{ permissionError }}</p>
        <template v-else-if="permissionResult">
          <div class="cloud-permission-summary">
            <span class="badge" :class="permissionStatusClass(permissionResult.status)">{{ permissionStatusLabel(permissionResult.status) }}</span>
            <time v-if="permissionResult.inspected_at" :datetime="permissionResult.inspected_at">{{ formatInspectionTime(permissionResult.inspected_at) }}</time>
          </div>
          <div v-if="permissionResult.identity" class="cloud-permission-identity">
            <span>调用身份 · {{ permissionResult.identity.type }}</span>
            <strong>{{ permissionResult.identity.arn }}</strong>
          </div>
          <p v-if="permissionResult.message" class="cloud-permission-message" :class="{ 'endpoint-error': permissionResult.status !== 'complete' }">{{ permissionResult.message }}</p>
          <div v-if="permissionResult.policies?.length" class="cloud-permission-policies">
            <h3>已附加策略</h3>
            <div v-for="(policy, index) in permissionResult.policies" :key="policy.source + ':' + policy.group_name + ':' + policy.name + ':' + index" class="cloud-permission-policy">
              <span><strong>{{ policy.name }}</strong><small>{{ policy.type === 'System' ? '系统策略' : '自定义策略' }}</small></span>
              <small>{{ policy.source === 'group' ? '用户组 · ' + policy.group_name : '直接授权' }}</small>
            </div>
          </div>
          <p v-else-if="permissionResult.status === 'complete'" class="cloud-permission-message">未附加策略</p>
          <p class="cloud-permission-caveat">已附加策略不等于实际有效权限；资源范围、条件和显式拒绝仍可能限制操作。</p>
        </template>
      </div>
      <template #actions>
        <button class="btn btn-sm" type="button" :disabled="permissionLoading" @click="loadPermissions">刷新</button>
        <button class="btn btn-sm btn-primary" type="button" @click="closePermissions">关闭</button>
      </template>
    </BaseModal>
  </section>
</template>

<script setup>
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { createCloudConnection, deleteCloudConnection, getCloudConnectionPermissions, getCloudConnections, getCloudProviders, updateCloudConnection } from '../../api/cloud-resources.js'
import BaseModal from '../../components/BaseModal.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import SurfaceCard from '../../components/SurfaceCard.vue'

const connections = ref([])
const loading = ref(true)
const saving = ref(false)
const error = ref('')
const formOpen = ref(false)
const editing = ref(null)
const permissionTarget = ref(null)
const permissionLoading = ref(false)
const permissionResult = ref(null)
const permissionError = ref('')
let permissionController = null
const providers = ref([])
const credentialValues = reactive({})
const fallbackProviders = [
  { id: 'aliyun', name: '阿里云', description: 'Alibaba Cloud DNS 与 OSS', implemented: true, capabilities: ['dns', 'object_storage'], credential_fields: [{ key: 'access_key_id', label: 'AccessKey ID', type: 'text', required: true }, { key: 'access_key_secret', label: 'AccessKey Secret', type: 'password', required: true }], configuration_fields: [] },
  { id: 'tencent', name: '腾讯云', description: 'Tencent Cloud DNSPod 与 COS', implemented: false, capabilities: ['dns', 'object_storage'], credential_fields: [], configuration_fields: [] },
  { id: 'cloudcone', name: 'CloudCone', description: 'CloudCone API 与 S3 兼容对象存储', implemented: false, capabilities: ['object_storage'], credential_fields: [], configuration_fields: [] },
]
const defaults = () => ({ name: '', provider: 'aliyun' })
const form = reactive(defaults())
const assign = value => Object.assign(form, value)
const currentProvider = computed(() => providers.value.find(item => item.id === form.provider))
const providerOptions = computed(() => providers.value.map(item => ({ value: item.id, label: item.name + (item.implemented ? '' : '（待接入）'), disabled: !item.implemented })))
const credentialFields = computed(() => currentProvider.value?.credential_fields || [])
const providerLabel = provider => providers.value.find(item => item.id === provider)?.name || provider
const connectionState = item => item.credential_configured ? '已配置' : '待配置'
const statusBadgeClass = item => item.credential_configured ? 'badge-online' : 'badge-warn'
const credentialState = item => item.credential_configured ? '凭据已配置' : '凭据待配置'
const permissionStatusLabel = status => ({ complete: '查询完整', partial: '部分可读', unavailable: '无法读取', unsupported: '暂不支持' })[status] || '查询异常'
const permissionStatusClass = status => status === 'complete' ? 'badge-online' : 'badge-warn'
function formatInspectionTime(value) { const date = new Date(value); return Number.isNaN(date.getTime()) ? '' : date.toLocaleString('zh-CN') }
function openPermissions(item) { permissionTarget.value = item; loadPermissions() }
function closePermissions() { permissionController?.abort(); permissionController = null; permissionTarget.value = null; permissionResult.value = null; permissionError.value = '' }
async function loadPermissions() {
  if (!permissionTarget.value) return
  permissionController?.abort()
  const controller = new AbortController()
  permissionController = controller
  permissionLoading.value = true
  permissionResult.value = null
  permissionError.value = ''
  try {
    const result = await getCloudConnectionPermissions(permissionTarget.value.id, { signal: controller.signal })
    if (permissionController === controller) permissionResult.value = result
  } catch (err) {
    if (permissionController === controller) permissionError.value = err.message || '读取权限信息失败'
  } finally {
    if (permissionController === controller) permissionLoading.value = false
  }
}
async function refresh() { loading.value = true; error.value = ''; try { connections.value = await getCloudConnections() } catch (err) { error.value = err.message || '读取云连接失败' } finally { loading.value = false } }
function clearValues(target) { Object.keys(target).forEach(key => delete target[key]) }
function selectProvider() {
  clearValues(credentialValues)
}
function openCreate() { editing.value = null; assign(defaults()); selectProvider(); formOpen.value = true }
function openEdit(item) {
  editing.value = item
  assign({ ...defaults(), ...item })
  clearValues(credentialValues)
  formOpen.value = true
}
function closeForm() { if (saving.value) return; formOpen.value = false }
async function save() {
  saving.value = true
  try {
    const body = { ...form, credentials: JSON.stringify(credentialValues), configuration: editing.value?.configuration || '{}' }
    if (editing.value && !Object.values(credentialValues).some(Boolean)) delete body.credentials
    if (editing.value) await updateCloudConnection(editing.value.id, body); else await createCloudConnection(body)
    formOpen.value = false; await refresh()
  } catch (err) { error.value = err.message || '保存云连接失败' } finally { saving.value = false }
}
async function remove(item) { if (!window.confirm('删除云连接 ' + item.name + '？')) return; try { await deleteCloudConnection(item.id); await refresh() } catch (err) { error.value = err.message || '删除云连接失败' } }
onMounted(async () => {
  try { providers.value = await getCloudProviders() } catch (err) { providers.value = fallbackProviders }
  await refresh()
})
onUnmounted(() => permissionController?.abort())
defineExpose({ refresh })
</script>

<style scoped src="./SystemSettings.shared.css"></style>
