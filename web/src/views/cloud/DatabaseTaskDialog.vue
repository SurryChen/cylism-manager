<template>
  <BaseModal :open="open && !selected" title="数据库任务" size="large" teleport @close="$emit('close')">
    <SurfaceCard padding="none">
      <div class="task-toolbar"><span class="cell-secondary">{{ resource ? '当前资源的任务' : '全部任务' }} · {{ total }} 条</span><div class="task-buttons"><SelectMenu v-model="status" aria-label="任务状态" :options="statusOptions" @update:model-value="offset = 0; load()" /><button class="icon-button" title="刷新任务" aria-label="刷新任务" :disabled="loading" @click="load"><RefreshCw :size="15" /></button></div></div>
      <EmptyState v-if="loading && !tasks.length" variant="loading" message="正在读取任务..." />
      <EmptyState v-else-if="!tasks.length" message="暂无数据库任务" />
      <div v-else class="table-wrap task-table-wrap"><table class="data-table task-table"><thead><tr><th>任务</th><th>源 → 目标</th><th>阶段</th><th>状态</th><th></th></tr></thead><tbody><tr v-for="task in tasks" :key="task.id"><td>#{{ task.id }} · {{ kindLabel(task.kind) }}</td><td><div class="task-cell" :title="resourceName(task.source) + ' → ' + resourceName(task.target)">{{ resourceName(task.source) }} → {{ resourceName(task.target) }}</div></td><td><div class="task-cell" :title="task.phase_name">{{ task.phase_name || '等待执行' }}</div></td><td><span class="badge" :class="task.status === 'succeeded' ? 'badge-online' : task.status === 'failed' ? 'badge-danger' : 'badge-offline'">{{ statusLabel(task) }}</span></td><td><button class="btn btn-sm" @click="select(task.id)">查看</button></td></tr></tbody></table></div>
      <div class="task-pagination"><span class="cell-secondary">共 {{ total }} 条</span><SelectMenu v-model="limit" aria-label="每页任务数" :options="[{ value: 10, label: '10 条 / 页' }, { value: 20, label: '20 条 / 页' }, { value: 50, label: '50 条 / 页' }]" @update:model-value="offset = 0; load()" /><div class="task-buttons"><button class="btn btn-sm" :disabled="loading || offset === 0" @click="page(-1)">上一页</button><span class="cell-secondary">{{ Math.floor(offset / limit) + 1 }} / {{ Math.max(1, Math.ceil(total / limit)) }}</span><button class="btn btn-sm" :disabled="loading || offset + limit >= total" @click="page(1)">下一页</button></div></div>
    </SurfaceCard>
  </BaseModal>
  <BaseModal :open="open && Boolean(selected)" :title="`任务 #${selected || ''} · ${detail ? kindLabel(detail.task.kind) : '详情'}`" size="large" teleport @close="$emit('close')">
    <div class="tasks-layout">
      <div class="task-buttons detail-navigation"><button class="btn btn-sm" @click="back"><ArrowLeft :size="15" />返回任务列表</button><button class="icon-button" title="刷新任务详情" aria-label="刷新任务详情" @click="select(selected)"><RefreshCw :size="15" /></button></div>
      <EmptyState v-if="!detail" variant="loading" message="正在读取任务详情..." />
      <template v-else-if="detail">
        <div class="detail-heading"><strong>#{{ detail.task.id }} · {{ kindLabel(detail.task.kind) }}</strong><div class="task-buttons"><button v-if="['queued', 'running'].includes(detail.task.status)" class="btn btn-sm" :disabled="acting || detail.task.pause_requested" @click="control('pause')">{{ detail.task.pause_requested ? '等待检查点暂停' : '暂停' }}</button><button v-if="detail.task.status === 'paused'" class="btn btn-sm" :disabled="acting" @click="control('resume')">继续</button><button v-if="!['succeeded', 'cancelled', 'cancelling'].includes(detail.task.status)" class="btn btn-sm" :disabled="acting" @click="confirmCancel = true">终止</button></div></div>
        <ol class="phase-list"><li v-for="(phase, index) in detail.phases" :key="phase" :class="{ completed: index < detail.task.phase, current: index === detail.task.phase }"><span>{{ index < detail.task.phase ? '✓' : index + 1 }}</span>{{ phase }}</li></ol>
        <p v-if="detail.task.progress" class="form-hint">{{ detail.task.progress }}</p>
        <p v-if="detail.task.last_error" class="task-error">{{ detail.task.last_error }}</p>
        <p v-if="detail.snapshot_pvc" class="form-hint">快照 PVC：<code>{{ detail.snapshot_pvc }}</code>（{{ clusterName(detail.task.cluster_id) }} / cylism-databases）。任务结束后保留，可在存储页确认不再需要后清理。</p>
        <div v-if="result" class="task-result"><strong>执行结果</strong><template v-if="result.schemas"><p>业务数据库：{{ schemaNames || '无' }}</p></template><p v-if="result.version">版本：{{ result.version }} · {{ result.read_only ? '只读' : '可写' }}</p><p v-if="typeof result.io_running === 'boolean'">复制接收线程：{{ result.io_running ? '运行中' : '未运行' }}；执行线程：{{ result.sql_running ? '运行中' : '未运行' }}；延迟：{{ result.lag_seconds == null ? '未知' : `${result.lag_seconds} 秒` }}。这是本次检查结果。</p><p v-if="result.application_switch_required">快照已导入，仍需配置账号、切换应用并验证。</p></div>
        <p v-else-if="detail.task.result && !detail.task.result.startsWith('{')" class="form-hint">{{ detail.task.result }}</p>
        <pre class="task-logs">{{ detail.logs.map(log => `${new Date(log.created_at).toLocaleString('zh-CN', { hour12: false })} ${log.message}`).join('\n') || '暂无阶段日志' }}</pre>
      </template>
    </div>
  </BaseModal>
  <Teleport to="body"><ConfirmDialog :open="confirmCancel" title="终止数据库任务" :message="detail?.task.kind === 'resize' ? '终止任务会停止等待就绪，已经提交的资源和参数保留；不会回滚或停止实例。' : '将停止执行 Pod 并核对已建立的复制通道，完成后释放资源锁。数据、实例和快照会保留；已建立的复制继续运行，可另行解除。初始化中断的目标需要检查后再使用。'" confirm-text="终止任务" :busy="acting" @cancel="confirmCancel = false" @confirm="control('cancel')" /></Teleport>
  <ErrorNoticeModal :open="Boolean(error)" :message="error" teleport @close="error = ''" />
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { ArrowLeft, RefreshCw } from 'lucide-vue-next'
import { controlDatabaseOperation, getDatabaseOperation, getDatabaseOperations } from '../../api/databases.js'
import SurfaceCard from '../../components/SurfaceCard.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import BaseModal from '../../components/BaseModal.vue'
import ConfirmDialog from '../../components/ConfirmDialog.vue'
import EmptyState from '../../components/EmptyState.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import { usePolling } from '../../composables/usePolling.js'

const props = defineProps({ open: Boolean, resource: { type: String, default: '' }, taskID: { type: Number, default: null }, resources: { type: Array, default: () => [] }, clusters: { type: Array, default: () => [] } })
const emit = defineEmits(['close', 'updated'])
const tasks = ref([]), total = ref(0), offset = ref(0), limit = ref(10), status = ref(''), detail = ref(null), selected = ref(null), loading = ref(false), acting = ref(false), error = ref(''), confirmCancel = ref(false)
let generation = 0, selectionGeneration = 0
const statusOptions = [{ value: '', label: '全部状态' }, { value: 'queued', label: '排队中' }, { value: 'running', label: '执行中' }, { value: 'paused', label: '已暂停' }, { value: 'cancelling', label: '终止中' }, { value: 'succeeded', label: '已完成' }, { value: 'failed', label: '失败' }, { value: 'cancelled', label: '已终止' }]
const kindLabel = kind => ({ resize: '调整 MySQL 资源', deploy: '部署 MySQL', inspect: '检查实例', migrate: '迁移数据', replicate: '建立复制', detach: '解除复制', replication_check: '检查复制', replication_discover: '发现已有复制', switch: '更新应用连接模板' }[kind] || kind)
const statusLabel = task => ({ queued: '排队中', running: task.pause_requested ? '待暂停' : '执行中', paused: '已暂停', failed: '失败 · 待处理', succeeded: '已完成', cancelling: '终止中', cancelled: '已终止' }[task.status] || task.status)
const resourceName = ref => !ref ? '—' : props.resources.find(item => item.ref === ref)?.name || ref
const clusterName = id => props.clusters.find(item => item.id === id)?.name || `集群 ${id}`
const result = computed(() => { try { return JSON.parse(detail.value?.task.result || 'null') } catch { return null } })
const schemaNames = computed(() => (Array.isArray(result.value?.schemas) ? result.value.schemas : String(result.value?.schemas || '').split(',')).filter(Boolean).join('、'))
async function load() {
  if (!props.open || loading.value) return
  const current = generation; loading.value = true
  try {
    if (selected.value) { await select(selected.value); return }
    const response = await getDatabaseOperations({ resource: props.resource, status: status.value, limit: limit.value, offset: offset.value })
    if (current !== generation) return
    tasks.value = response.items; total.value = response.total
    if (offset.value > 0 && offset.value >= total.value) { offset.value = Math.max(0, Math.ceil(total.value / limit.value) - 1) * limit.value; loading.value = false; await load() }
  } catch (cause) { if (current === generation) error.value = cause.message || '读取任务失败' }
  finally { if (current === generation) loading.value = false }
}
async function select(id) {
  if (!id || !props.open) return
  const current = generation, selection = ++selectionGeneration; if (selected.value !== id) detail.value = null; selected.value = id
  try { const response = await getDatabaseOperation(id); if (current === generation && selection === selectionGeneration) detail.value = response }
  catch (cause) { if (current === generation && selection === selectionGeneration) error.value = cause.message || '读取任务失败' }
}
async function control(action) {
  if (!detail.value || acting.value) return
  acting.value = true
  try { await controlDatabaseOperation(detail.value.task.id, action); confirmCancel.value = false; await load(); emit('updated') }
  catch (cause) { error.value = cause.message || '任务操作失败' }
  finally { acting.value = false }
}
function back() { ++selectionGeneration; selected.value = null; detail.value = null; confirmCancel.value = false; load() }
async function page(direction) { if (loading.value) return; offset.value = Math.max(0, offset.value + direction * limit.value); await load() }
const polling = usePolling(load, { interval: 5000 })
watch(() => [props.open, props.resource, props.taskID], ([open]) => {
  ++generation; ++selectionGeneration; loading.value = false; tasks.value = []; detail.value = null; selected.value = props.taskID; offset.value = 0; status.value = ''; error.value = ''; confirmCancel.value = false
  polling.stop(); if (open) { load(); polling.start() }
}, { immediate: true })
</script>

<style scoped>
.tasks-layout { display: grid; gap: var(--space-16); }
.task-toolbar, .detail-heading, .task-buttons { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.task-table { min-width: 660px; table-layout: fixed; }.task-table th:first-child { width: 155px; }.task-table th:nth-last-child(2) { width: 100px; }.task-table th:last-child { width: 70px; }.task-cell { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }.task-toolbar, .task-pagination { padding: 14px 20px; }.task-table-wrap { padding: 0 20px; }.task-pagination { display: flex; align-items: center; flex-wrap: wrap; gap: 12px; border-top: 1px solid var(--border-muted); }.task-pagination > .task-buttons { margin-left: auto; }.detail-navigation { justify-content: space-between; }.task-pagination :deep(.select-menu) { width: 125px; }
.detail-heading { padding-top: 16px; border-top: 1px solid var(--border-muted); font-size: 13px; }
.phase-list { list-style: none; display: grid; gap: 10px; padding: 0; margin: 0; color: var(--text-secondary); font-size: 12px; }
.phase-list li { display: flex; gap: 10px; align-items: center; }
.phase-list li span { width: 22px; height: 22px; border: 1px solid var(--border-muted); border-radius: 50%; display: grid; place-items: center; }
.phase-list .current { color: var(--action-primary); font-weight: 600; }
.phase-list .completed { color: var(--success); }
.task-logs { background: var(--surface-subtle); padding: 12px; max-height: 230px; overflow: auto; white-space: pre-wrap; overflow-wrap: anywhere; font-size: 11px; border-radius: var(--radius-card); }
.task-error { color: var(--danger); font-size: 12px; }
.task-result { border-top: 1px solid var(--border-muted); padding-top: 12px; font-size: 12px; }
.task-result strong { font-size: 13px; }
</style>
