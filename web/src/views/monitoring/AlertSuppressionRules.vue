<template>
  <SurfaceCard class="suppression-card" padding="none">
    <form class="suppression-toolbar" @submit.prevent="load">
      <input v-model.trim="node" class="form-input" aria-label="屏蔽规则节点筛选" placeholder="节点名称（精确匹配）" />
      <SelectMenu v-model="status" class="suppression-select" aria-label="屏蔽规则状态筛选" :options="statusOptions" />
      <button class="btn" :disabled="loading">筛选</button>
      <button type="button" class="icon-button" title="重置屏蔽规则筛选" aria-label="重置屏蔽规则筛选" @click="reset"><RotateCcw :size="15" /></button>
      <button type="button" class="btn btn-primary suppression-create" @click="open()"><Plus :size="15" />新增屏蔽规则</button>
    </form>
    <EmptyState v-if="loading && !items.length" variant="loading" message="正在读取屏蔽规则..." />
    <EmptyState v-else-if="!items.length" message="暂无屏蔽规则" description="指定节点和告警类型，可临时或长期停止通知。" />
    <div v-else class="table-wrap" :aria-busy="loading">
      <table class="data-table">
        <thead><tr><th>规则</th><th>节点</th><th>告警类型</th><th>状态</th><th>有效期</th><th>操作</th></tr></thead>
        <tbody><tr v-for="rule in items" :key="rule.id">
          <td><span class="cell-primary">{{ rule.name }}</span><span v-if="rule.comment" class="cell-secondary suppression-detail">{{ rule.comment }}</span></td>
          <td>{{ rule.node_name || '全部节点' }}</td>
          <td>{{ rule.alert_names?.length ? rule.alert_names.map(alertRuleLabel).join('、') : '全部类型' }}</td>
          <td><span class="badge" :class="ruleState(rule) === '生效中' ? 'badge-online' : 'badge-offline'">{{ ruleState(rule) }}</span></td>
          <td>{{ rule.ends_at ? formatTime(rule.ends_at) : '长期' }}<span v-if="rule.ends_at" class="cell-secondary suppression-detail">到期自动恢复通知</span></td>
          <td><div class="suppression-actions"><button class="btn btn-sm" :disabled="busy" @click="open(rule)">编辑</button><button class="btn btn-sm" :disabled="busy || (!rule.enabled && expired(rule))" @click="toggle(rule)">{{ rule.enabled ? '停用' : '启用' }}</button><button class="icon-button" :disabled="busy" :title="`删除规则：${rule.name}`" :aria-label="`删除规则：${rule.name}`" @click="deleting = rule"><Trash2 :size="15" /></button></div></td>
        </tr></tbody>
      </table>
    </div>
    <PaginationControls :page="page" :page-size="pageSize" :total="total" :loading="loading" label="屏蔽规则分页" @page-change="changePage" @page-size-change="changePageSize" />
    <p class="suppression-note">节点与类型同时设置时需同时命中；勾选的多种类型满足任意一种即可。只停止平台飞书和邮件通知，实时告警、历史记录和自动处置继续运行。停用或到期后，从下一次告警回调恢复通知。</p>
  </SurfaceCard>
  <BaseModal :open="!!form" :title="editingID ? '编辑屏蔽规则' : '新增屏蔽规则'" :show-close="!busy" :close-on-overlay="!busy" :close-on-escape="!busy" @close="form = null">
    <form v-if="form" id="suppression-rule-form" class="suppression-form" @submit.prevent="save">
      <label class="form-group"><span class="form-label">规则名称</span><input v-model.trim="form.name" class="form-input" aria-label="屏蔽规则名称" required maxlength="100" placeholder="例如 cloudcone 资源告警" /></label>
      <label class="form-group"><span class="form-label">节点</span><input v-model.trim="form.node_name" class="form-input" aria-label="屏蔽节点" maxlength="253" placeholder="精确匹配节点名称，留空表示全部节点" /></label>
      <fieldset class="suppression-types"><legend class="form-label">告警类型（可多选）</legend><label class="checkbox-label"><input v-model="allTypes" type="checkbox" />该节点全部类型</label><div v-if="!allTypes" class="suppression-type-grid"><label v-for="option in typeOptions" :key="option.id" class="checkbox-label"><input v-model="form.alert_names" type="checkbox" :value="option.id" />{{ option.label }}</label></div></fieldset>
      <label class="form-group"><span class="form-label">有效期</span><SelectMenu v-model="form.duration" aria-label="屏蔽有效期" :options="durationOptions" /></label>
      <label class="form-group"><span class="form-label">说明</span><input v-model.trim="form.comment" class="form-input" aria-label="屏蔽说明" maxlength="300" placeholder="屏蔽原因" /></label>
      <label class="checkbox-label"><input v-model="form.enabled" type="checkbox" />启用规则</label>
      <p class="suppression-preview">{{ scopePreview }}。{{ form.duration === '0' ? '长期生效，直到手动停用或删除。' : form.duration === 'keep' ? '保留当前到期时间。' : '保存后立即开始计时，到期自动恢复通知。' }}</p>
    </form>
    <template #actions><button class="btn" :disabled="busy" @click="form = null">取消</button><button class="btn btn-primary" form="suppression-rule-form" :disabled="busy || !valid" type="submit">{{ busy ? '正在保存...' : '保存规则' }}</button></template>
  </BaseModal>
  <ConfirmDialog :open="!!deleting" title="删除屏蔽规则" :message="`删除“${deleting?.name || ''}”后，该规则将不再屏蔽通知。`" :busy="busy" @cancel="deleting = null" @confirm="remove" />
  <ErrorNoticeModal :open="!!error || !!mutationError" title="屏蔽规则操作失败" :message="mutationError || error?.message || '操作失败'" @close="error = null; mutationError = ''" />
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { Plus, RotateCcw, Trash2 } from 'lucide-vue-next'
import { getAlertSuppressionRules, createAlertSuppressionRule, updateAlertSuppressionRule, setAlertSuppressionEnabled, deleteAlertSuppressionRule } from '../../api/alerting.js'
import { usePaginatedResource } from '../../composables/usePaginatedResource.js'
import { formatDateTime as formatTime } from '../../utils/formatters.js'
import { alertRuleOptions, alertRuleLabel } from './alertRules.js'
import SurfaceCard from '../../components/SurfaceCard.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import EmptyState from '../../components/EmptyState.vue'
import PaginationControls from '../../components/PaginationControls.vue'
import BaseModal from '../../components/BaseModal.vue'
import ConfirmDialog from '../../components/ConfirmDialog.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'

const props = defineProps({ clusterId: { type: [String, Number], default: '' } })
const node = ref(''), status = ref(''), form = ref(null), editingID = ref(null), allTypes = ref(false), busy = ref(false), deleting = ref(null), mutationError = ref('')
const statusOptions = [{ value: '', label: '全部状态' }, { value: 'active', label: '生效中' }, { value: 'disabled', label: '已停用' }, { value: 'expired', label: '已到期' }]
const durations = [{ value: '60', label: '1 小时' }, { value: '240', label: '4 小时' }, { value: '1440', label: '24 小时' }, { value: '10080', label: '7 天' }, { value: '43200', label: '30 天' }, { value: '0', label: '长期' }]
const durationOptions = computed(() => editingID.value ? [{ value: 'keep', label: '保留当前有效期' }, ...durations] : durations)
const typeOptions = computed(() => [...alertRuleOptions, ...(form.value?.alert_names || []).filter(name => !alertRuleOptions.some(option => option.id === name)).map(name => ({ id: name, label: name }))])
const pagination = usePaginatedResource(getAlertSuppressionRules, 'items')
const { items, total, page, pageSize, loading, error, changePage, changePageSize } = pagination
const valid = computed(() => form.value?.name && (form.value.node_name || (!allTypes.value && form.value.alert_names.length)) && (allTypes.value || form.value.alert_names.length))
const scopePreview = computed(() => !form.value ? '' : `${form.value.node_name || '全部节点'} · ${allTypes.value ? '全部类型' : form.value.alert_names.map(alertRuleLabel).join('、') || '请选择告警类型'}`)
function expired(rule) { return rule.ends_at && new Date(rule.ends_at).getTime() <= Date.now() }
function ruleState(rule) { return !rule.enabled ? '已停用' : expired(rule) ? '已到期' : '生效中' }
function load() { return pagination.load({ clusterID: props.clusterId, node: node.value, status: status.value }) }
function reset() { node.value = ''; status.value = ''; void load() }
function open(rule) {
  editingID.value = rule?.id || null
  allTypes.value = !!rule && !rule.alert_names?.length
  form.value = { name: rule?.name || '', node_name: rule?.node_name || '', alert_names: [...(rule?.alert_names || [])], comment: rule?.comment || '', enabled: rule?.enabled ?? true, duration: rule?.id ? 'keep' : '240' }
}
async function mutate(action) {
  if (busy.value) return
  busy.value = true; mutationError.value = ''
  try { await action(); await pagination.refresh() } catch (e) { mutationError.value = e.message || '操作失败' } finally { busy.value = false }
}
function save() {
  if (!valid.value) return
  const { duration, ...body } = form.value
  body.alert_names = allTypes.value ? [] : [...body.alert_names]
  if (duration !== 'keep') body.duration_minutes = Number(duration)
  const options = { clusterID: props.clusterId }
  return mutate(async () => { if (editingID.value) await updateAlertSuppressionRule(editingID.value, body, options); else await createAlertSuppressionRule(body, options); form.value = null })
}
function toggle(rule) { return mutate(() => setAlertSuppressionEnabled(rule.id, !rule.enabled, { clusterID: props.clusterId })) }
function remove() { const rule = deleting.value; return mutate(async () => { await deleteAlertSuppressionRule(rule.id, { clusterID: props.clusterId }); deleting.value = null }) }
watch(() => props.clusterId, () => { pagination.reset(); node.value = ''; status.value = ''; form.value = null; deleting.value = null; void load() }, { immediate: true })
defineExpose({ open, refresh: pagination.refresh, total, loading })
</script>

<style scoped>
.suppression-toolbar{display:flex;align-items:center;gap:var(--space-8);padding:14px var(--space-20)}.suppression-toolbar .form-input{width:220px;min-height:var(--button-height);font-size:12px}.suppression-select{width:150px}.suppression-create{margin-left:auto}.suppression-card>.table-wrap{padding:0 var(--space-20)}.suppression-card .data-table{min-width:780px}.suppression-actions{display:flex;align-items:center;gap:var(--space-8)}.suppression-detail{display:block;margin-top:4px}.suppression-note,.suppression-preview{margin:0;color:var(--text-secondary);font-size:12px;line-height:1.6}.suppression-note{padding:12px var(--space-20);border-top:1px solid var(--border-muted)}.suppression-preview{margin-top:var(--space-12)}.suppression-types{min-width:0;margin:0 0 var(--space-16);padding:0;border:0}.suppression-type-grid{display:grid;grid-template-columns:1fr 1fr;gap:var(--space-12);margin-top:var(--space-12)}@media(max-width:640px){.suppression-toolbar{flex-direction:column;align-items:stretch;padding:14px}.suppression-toolbar .form-input,.suppression-select{width:100%}.suppression-create{margin-left:0}.suppression-card>.table-wrap{padding:0 14px}.suppression-type-grid{grid-template-columns:1fr}.suppression-note{padding:12px 14px}}
</style>
