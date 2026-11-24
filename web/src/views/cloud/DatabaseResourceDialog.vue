<template>
  <BaseModal :open="Boolean(instance)" title="调整 MySQL 资源" size="medium" teleport @close="close">
    <div class="resource-form">
      <p class="resource-instance">{{ instance?.name }}<span class="cell-secondary">当前 {{ instance?.cpu_limit || '2' }} 核 / {{ instance?.memory_limit || '2Gi' }}</span></p>
      <label class="form-group"><span class="form-label">资源规格</span><SelectMenu v-model="preset" aria-label="资源规格" :options="presets" @change="applyPreset" /></label>
      <div class="resource-fields"><label class="form-group"><span class="form-label">CPU 上限</span><input v-model.trim="form.cpu_limit" class="form-input" aria-label="CPU 上限" placeholder="2 或 500m" @input="preset = 'custom'"></label><label class="form-group"><span class="form-label">内存上限</span><input v-model.trim="form.memory_limit" class="form-input" aria-label="内存上限" placeholder="2Gi" @input="preset = 'custom'"></label></div>
      <p class="form-hint">缓冲池按内存的 25% 设置，并同步调整连接数、会话缓冲和临时表参数。</p>
      <p class="resource-impact">保存后创建后台任务并重启 MySQL，连接会短暂中断。已有数据、复制范围和只读配置会保留；从库恢复后继续复制。</p>
    </div>
    <template #actions><button class="btn" :disabled="saving" @click="close">取消</button><button class="btn btn-primary" :disabled="saving || !form.cpu_limit || !form.memory_limit" @click="save">{{ saving ? '提交中...' : '保存并重启' }}</button></template>
  </BaseModal>
  <ErrorNoticeModal :open="Boolean(error)" :message="error" teleport @close="error = ''" />
</template>
<script setup>
import { reactive, ref, watch } from 'vue'
import { updateDatabaseResources } from '../../api/databases.js'
import BaseModal from '../../components/BaseModal.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
const props = defineProps({ instance: { type: Object, default: null } })
const emit = defineEmits(['close', 'created'])
const form = reactive({ cpu_limit: '', memory_limit: '' }), preset = ref('custom'), saving = ref(false), error = ref('')
const presets = [{ value: '1/1Gi', label: '1 核 / 1 GiB' }, { value: '2/2Gi', label: '2 核 / 2 GiB' }, { value: '2/4Gi', label: '2 核 / 4 GiB' }, { value: 'custom', label: '自定义' }]
function applyPreset(value) { if (value !== 'custom') [form.cpu_limit, form.memory_limit] = value.split('/') }
function close() { if (!saving.value) emit('close') }
async function save() {
  if (saving.value || !props.instance) return
  saving.value = true
  try { const task = await updateDatabaseResources(props.instance.id, { ...form }); emit('created', task.id) }
  catch (cause) { error.value = cause.message || '调整资源失败' }
  finally { saving.value = false }
}
watch(() => props.instance, item => { if (item) { form.cpu_limit = item.cpu_limit || '2'; form.memory_limit = item.memory_limit || '2Gi'; const value = `${form.cpu_limit}/${form.memory_limit}`; preset.value = presets.some(item => item.value === value) ? value : 'custom'; error.value = '' } }, { immediate: true })
</script>
<style scoped>
.resource-form { display: grid; gap: var(--space-16); }.resource-form .form-group { margin: 0; }.resource-instance { margin: 0; font-weight: 600; }.resource-instance span { display: block; margin-top: 6px; font-weight: 400; }.resource-fields { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }.resource-impact { font-size: 12px; line-height: 1.7; margin: 0; color: var(--text-secondary); border-top: 1px solid var(--border-muted); padding-top: 16px; }
@media (max-width:600px) { .resource-fields { grid-template-columns: 1fr; } }
</style>
