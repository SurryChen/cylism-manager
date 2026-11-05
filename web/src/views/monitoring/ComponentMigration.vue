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
        <dl v-if="latest.target_pvc_name" class="migration-volumes"><dt>源卷</dt><dd>{{ latest.source_pvc_name }}</dd><dt>目标卷</dt><dd>{{ latest.target_pvc_name }}</dd><dt v-if="latest.bytes_copied">已传输</dt><dd v-if="latest.bytes_copied">{{ (latest.bytes_copied / 1024 / 1024).toFixed(1) }} MiB</dd></dl>
        <pre v-if="logs.length" class="migration-logs" aria-label="迁移任务日志">{{ logs.map(entry => `${entry.created_at ? new Date(entry.created_at).toLocaleTimeString() : ''} ${entry.message}`).join('\n') }}</pre>
        <RouterLink :to="`/cluster/${clusterId}?tab=tasks`" @click="open = false">在集群任务中查看历史</RouterLink>
      </section>
      <template #actions>
        <button class="btn" :disabled="submitting" @click="open = false">{{ busy ? '关闭（后台继续）' : '关闭' }}</button>
        <button v-if="!busy" class="btn btn-primary" data-testid="start-component-migration" :disabled="submitting || !target || loading" @click="start">{{ submitting ? '正在创建任务...' : '确认迁移' }}</button>
      </template>
    </BaseModal>
    <ErrorNoticeModal :open="!!error" title="迁移操作失败" :message="error" teleport @close="error = ''" />
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import BaseModal from '../../components/BaseModal.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import { getComponentMigrations, startComponentMigration } from '../../api/componentMigrations.js'
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
const terminal = status => ['succeeded', 'failed', 'rolled_back'].includes(status)
const busy = computed(() => !!latest.value && !terminal(latest.value.status))
const statusLabel = computed(() => ({ pending: '等待执行', preflight: '预检中', provisioning_target: '准备目标卷', stopping_source: '停止源组件', copying: '复制数据', verifying: '校验数据', cutover: '切换存储', waiting_ready: '等待目标就绪', finalizing: '完成记录', rolling_back: '恢复源组件', rollback_failed: '等待恢复重试', succeeded: '迁移成功', failed: '迁移失败', rolled_back: '已恢复源节点' }[latest.value?.status] || '迁移中'))
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
  refresh()
}, { immediate: true })
onBeforeUnmount(() => { generation++; controller?.abort() })

async function refresh() {
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
.migration-volumes { display: grid; grid-template-columns: auto 1fr; gap: var(--space-8) var(--space-16); font-size: 12px; }
.migration-volumes dt { color: var(--text-secondary); }
.migration-volumes dd { margin: 0; overflow-wrap: anywhere; }
.migration-logs { max-height: 260px; overflow: auto; padding: var(--space-12); background: var(--surface-subtle); border-radius: var(--radius-control); font-size: 12px; line-height: 1.7; white-space: pre-wrap; overflow-wrap: anywhere; }
@media (max-width: 640px) { .migration-fields { grid-template-columns: 1fr; gap: var(--space-8); } }
</style>
