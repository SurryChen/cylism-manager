<template>
  <div class="component-migration">
    <button class="btn btn-sm" data-testid="open-component-migration" :disabled="disabled || loading" @click="openDialog">{{ busy ? '查看迁移进度' : '迁移节点' }}</button>
    <span v-if="busy" class="badge badge-deploying">{{ statusLabel }}</span>
    <BaseModal :open="open" :title="`${label} 节点迁移`" size="large" teleport :close-on-overlay="!submitting" :show-close="!submitting" @close="open = false">
      <template v-if="!busy">
        <div class="migration-fields">
          <div><span class="form-label">当前数据节点</span><p class="migration-node">{{ currentNode || '未分配' }}</p></div>
          <label class="form-group"><span class="form-label">目标节点</span><SelectMenu v-model="target" aria-label="迁移目标节点" placeholder="选择目标节点" :options="targets" :disabled="submitting" /></label>
        </div>
        <p class="form-hint">迁移会暂停{{ label === 'Loki' ? '日志存储' : '指标采集与查询' }}，复制并校验历史数据后在目标节点启动。Service 地址不变，原 PVC 保留；失败时自动恢复源节点。目标节点需能通过平台 SSH 连接，并有足够磁盘空间。</p>
        <p v-if="!targets.length" class="form-hint">当前没有其他就绪且可调度的节点。</p>
      </template>
      <section v-if="latest" class="migration-task" aria-live="polite">
        <div class="migration-task-heading"><strong>任务 #{{ latest.task_id }}</strong><span class="badge" :class="busy ? 'badge-deploying' : latest.status === 'succeeded' ? 'badge-online' : 'badge-danger'">{{ statusLabel }}</span></div>
        <p class="cell-secondary">{{ latest.source_node_name }} → {{ latest.target_node_name }}</p>
        <p>{{ latest.detail }}</p>
        <div v-if="latest.bytes_total > 0" class="migration-progress">
          <div class="migration-task-heading"><strong>数据传输 {{ progressPercent }}%</strong><span class="cell-secondary">{{ formatBytes(latest.bytes_copied) }} / 约 {{ formatBytes(latest.bytes_total) }}</span></div>
          <progress :value="progressPercent" max="100" aria-label="数据传输进度" />
          <div class="migration-task-heading cell-secondary"><span>{{ transferRate }}</span><span>{{ remainingTime }}</span></div>
          <p v-if="progressStale" class="form-hint">进度暂未更新，正在等待任务连接或状态恢复。</p>
        </div>
        <p v-if="paused" class="migration-pause-notice">{{ latest.can_resume ? '传输已暂停或正在暂停，已复制数据保留。源服务仍处于停写状态；需要恢复服务时，请选择取消并恢复源服务。' : '平台重启或传输连接已中断，无法沿用原连接继续。已复制数据保留，请取消并恢复源服务后重新迁移。' }}</p>
        <dl v-if="latest.target_pvc_name" class="migration-volumes"><dt>源卷</dt><dd>{{ latest.source_pvc_name }}</dd><dt>目标卷</dt><dd>{{ latest.target_pvc_name }}</dd><dt v-if="latest.bytes_copied">已传输</dt><dd v-if="latest.bytes_copied">{{ (latest.bytes_copied / 1024 / 1024).toFixed(1) }} MiB</dd></dl>
        <pre v-if="logs.length" class="migration-logs" aria-label="迁移任务日志">{{ logs.map(entry => `${entry.created_at ? new Date(entry.created_at).toLocaleTimeString() : ''} ${entry.message}`).join('\n') }}</pre>
        <RouterLink :to="`/cluster/${clusterId}?tab=tasks`" @click="open = false">在集群任务中查看历史</RouterLink>
      </section>
      <template #actions>
        <button v-if="latest?.can_cancel" class="btn btn-danger" data-testid="cancel-component-migration" :disabled="submitting" @click="confirmCancel = true">取消并恢复源服务</button>
        <button v-if="latest?.can_pause" class="btn" data-testid="pause-component-migration" :disabled="submitting" @click="control('pause')">暂停传输</button>
        <button v-if="latest?.can_resume" class="btn btn-primary" data-testid="resume-component-migration" :disabled="submitting" @click="control('resume')">继续传输</button>
        <button class="btn" :disabled="submitting" @click="open = false">{{ paused ? '关闭（保持暂停）' : busy ? '关闭（后台继续）' : '关闭' }}</button>
        <button v-if="!busy" class="btn btn-primary" data-testid="start-component-migration" :disabled="submitting || !target || loading" @click="start">{{ submitting ? '正在创建任务...' : '确认迁移' }}</button>
      </template>
    </BaseModal>
    <ConfirmDialog :open="confirmCancel" title="取消迁移并恢复源服务？" message="将停止当前迁移，并恢复源节点和原 PVC。源卷和已复制的目标卷都会保留；恢复可能需要几分钟。" confirm-text="取消迁移并恢复" :busy="submitting" @cancel="confirmCancel = false" @confirm="control('cancel')" />
    <ErrorNoticeModal :open="!!error" title="迁移操作失败" :message="error" teleport @close="error = ''" />
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import BaseModal from '../../components/BaseModal.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import ConfirmDialog from '../../components/ConfirmDialog.vue'
import { getComponentMigrations, startComponentMigration, controlComponentMigration } from '../../api/componentMigrations.js'
import { getClusterDeploymentLogs } from '../../api/clusters.js'
import { usePolling } from '../../composables/usePolling.js'

const props = defineProps({ clusterId: { type: String, required: true }, component: { type: String, required: true }, currentNode: { type: String, default: '' }, nodes: { type: Array, default: () => [] }, disabled: Boolean })
const emit = defineEmits(['busy', 'changed'])
const label = computed(() => props.component === 'loki' ? 'Loki' : 'VictoriaMetrics')
const open = ref(false)
const target = ref('')
const latest = ref(null)
const logs = ref([])
const error = ref('')
const loading = ref(false)
const submitting = ref(false)
const confirmCancel = ref(false)
const now = ref(Date.now())
const terminal = status => ['succeeded', 'failed', 'rolled_back', 'canceled'].includes(status)
const busy = computed(() => !!latest.value && !terminal(latest.value.status))
const paused = computed(() => busy.value && ['copying', 'paused'].includes(latest.value?.status) && latest.value?.control === 'pause')
const statusLabel = computed(() => latest.value?.control === 'cancel' && latest.value?.can_cancel === false && busy.value ? '取消并恢复中' : paused.value ? latest.value.status === 'paused' ? '已暂停' : '正在暂停' : ({ pending: '等待执行', preflight: '预检中', provisioning_target: '准备目标卷', stopping_source: '停止源组件', copying: '复制数据', verifying: '校验数据', cutover: '切换存储', waiting_ready: '等待目标就绪', finalizing: '完成记录', rolling_back: '恢复源组件', rollback_failed: '等待恢复重试', succeeded: '迁移成功', failed: '迁移失败', rolled_back: '已恢复源节点', canceled: '已取消' }[latest.value?.status] || '迁移中'))
const progressPercent = computed(() => {
  if (!latest.value?.bytes_total) return 0
  const copying = ['copying', 'paused', 'stopping_source', 'provisioning_target'].includes(latest.value.status)
  return Math.min(copying ? 99 : 100, Math.floor((latest.value.bytes_copied || 0) * 100 / latest.value.bytes_total))
})
const progressStale = computed(() => latest.value?.status === 'copying' && !paused.value && latest.value.progress_at && now.value - new Date(latest.value.progress_at).getTime() > 15000)
const transferRate = computed(() => paused.value ? '传输暂停' : progressStale.value ? '等待进度更新' : latest.value?.status === 'copying' ? `${formatBytes(latest.value.bytes_per_second || 0)}/s` : '数据传输已结束')
const remainingTime = computed(() => {
  if (paused.value || progressStale.value || latest.value?.status !== 'copying' || !latest.value.bytes_per_second) return '剩余时间待估算'
  const seconds = Math.max(0, Math.ceil((latest.value.bytes_total - latest.value.bytes_copied) / latest.value.bytes_per_second))
  return seconds < 60 ? '预计不足 1 分钟' : `预计还需 ${Math.ceil(seconds / 60)} 分钟`
})
function formatBytes(value) {
  const size = Number(value) || 0
  return size >= 1024 ** 3 ? `${(size / 1024 ** 3).toFixed(2)} GiB` : `${(size / 1024 ** 2).toFixed(1)} MiB`
}
const targets = computed(() => props.nodes.filter(node => node.name !== props.currentNode && node.ready && !node.unschedulable && !node.evicted && !node.default_no_schedule && !(node.taints || []).some(t => ['NoSchedule', 'NoExecute'].includes(t.effect))).map(node => ({ value: node.name, label: node.display_name || node.name })))
let controller
let generation = 0
let refreshID = 0
const polling = usePolling(refresh, { interval: 3000 })
watch([busy, loading, submitting], () => emit('busy', busy.value || loading.value || submitting.value), { immediate: true })
watch(() => [props.clusterId, props.component], () => {
  generation++
  controller?.abort()
  controller = new AbortController()
  polling.stop()
  latest.value = null
  logs.value = []
  open.value = false
  target.value = ''
  error.value = ''
  submitting.value = false
  confirmCancel.value = false
  refresh()
}, { immediate: true })
onBeforeUnmount(() => { generation++; controller?.abort() })

async function refresh() {
  now.value = Date.now()
  if (!props.clusterId) return
  const version = generation
  const requestID = ++refreshID
  const previous = latest.value
  loading.value = !previous
  try {
    const items = await getComponentMigrations(props.clusterId, props.component, { signal: controller.signal })
    if (version !== generation || requestID !== refreshID) return
    latest.value = Array.isArray(items) ? items[0] || null : null
    if (busy.value) polling.start()
    else polling.stop()
    if (previous && !terminal(previous.status) && latest.value && terminal(latest.value.status)) {
      emit('changed')
      if (latest.value.status !== 'succeeded') error.value = latest.value.detail || '迁移未完成，请查看任务日志'
    }
    if (open.value) await loadLogs(version)
  } catch (e) {
    if (version === generation && e.name !== 'AbortError') error.value = e.message || '读取迁移任务失败'
  } finally { if (version === generation) loading.value = false }
}
async function control(action) {
  if (submitting.value || !latest.value) return
  const version = generation
  submitting.value = true
  error.value = ''
  try {
    const item = await controlComponentMigration(props.clusterId, props.component, latest.value.id, action, { signal: controller.signal })
    if (version !== generation) return
    latest.value = item
    confirmCancel.value = false
    await refresh()
  } catch (e) {
    if (version === generation && e.name !== 'AbortError') { error.value = e.message || '迁移操作失败'; await refresh() }
  } finally { if (version === generation) submitting.value = false }
}
async function loadLogs(version) {
  if (!latest.value) return
  const taskId = latest.value.task_id
  const entries = await getClusterDeploymentLogs(props.clusterId, taskId, 0, { signal: controller.signal })
  if (version === generation && taskId === latest.value?.task_id) logs.value = Array.isArray(entries) ? entries : entries?.items || []
}
function openDialog() { open.value = true; refresh() }
async function start() {
  if (busy.value || submitting.value || !targets.value.some(node => node.value === target.value)) return
  const version = generation
  submitting.value = true
  error.value = ''
  try {
    const item = await startComponentMigration(props.clusterId, props.component, target.value, { signal: controller.signal })
    if (version !== generation) return
    latest.value = item
    logs.value = []
    target.value = ''
    polling.start()
    emit('changed')
    await refresh()
  } catch (e) {
    if (version === generation && e.name !== 'AbortError') {
      error.value = e.message || '创建迁移任务失败'
      await refresh() // A lost response may still have created a durable task.
    }
  } finally { if (version === generation) submitting.value = false }
}
</script>

<style scoped>
.component-migration { display: flex; align-items: center; gap: var(--space-8); }
.migration-fields { display: grid; grid-template-columns: 1fr 1fr; gap: var(--space-16); }
.migration-node { margin: var(--space-8) 0; font-size: 13px; overflow-wrap: anywhere; }
.migration-task { margin-top: var(--space-20); border-top: 1px solid var(--border-muted); padding-top: var(--space-16); font-size: 13px; }
.migration-task-heading { display: flex; justify-content: space-between; gap: var(--space-8); }
.migration-progress { display: grid; gap: var(--space-8); margin: var(--space-16) 0; font-size: 12px; }
.migration-progress progress { width: 100%; height: 8px; accent-color: var(--accent); }
.migration-pause-notice { color: var(--warning); line-height: 1.6; }
.migration-volumes { display: grid; grid-template-columns: auto 1fr; gap: var(--space-8) var(--space-16); font-size: 12px; }
.migration-volumes dt { color: var(--text-secondary); }
.migration-volumes dd { margin: 0; overflow-wrap: anywhere; }
.migration-logs { max-height: 260px; overflow: auto; padding: var(--space-12); background: var(--surface-subtle); border-radius: var(--radius-control); font-size: 12px; line-height: 1.7; white-space: pre-wrap; overflow-wrap: anywhere; }
@media (max-width: 640px) { .migration-fields { grid-template-columns: 1fr; gap: var(--space-8); } }
</style>
