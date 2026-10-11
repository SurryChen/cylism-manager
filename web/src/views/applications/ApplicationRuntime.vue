<template>
  <section class="application-runtime">
    <SurfaceCard as="div" padding="none">
      <div class="runtime-toolbar">
        <div v-if="loaded" class="runtime-summary">
          <span :class="['badge', statusClass]">{{ statusLabel }}</span>
          <span>就绪副本 {{ runtime?.ready_replicas ?? 0 }}/{{ runtime?.desired_replicas ?? 0 }}</span>
          <span>运行版本 {{ activeVersion }}</span>
          <span v-if="!metricsAvailable && pods.length" class="runtime-metrics-note">资源指标不可用</span>
        </div>
        <div class="runtime-actions"><slot name="actions" /><button class="icon-button" title="刷新运行状态" aria-label="刷新运行状态" :disabled="loading" @click="loadRuntime"><RefreshCw :size="16" :class="{ 'is-spinning': loading }" /></button></div>
      </div>
      <FeedbackBanner v-if="error" tone="warning" :message="error" />
      <EmptyState v-if="!loaded && !error" variant="loading" message="正在读取运行状态" />
      <EmptyState v-else-if="loaded && !pods.length" :message="runtime?.status === 'not_released' ? '尚未发布，当前没有 Pod' : '当前没有关联的 Pod'" />
      <div v-else-if="loaded" class="table-wrap runtime-table-wrap">
        <table class="data-table runtime-pods-table">
          <thead><tr><th>Pod</th><th>状态</th><th>节点</th><th>重启</th><th>CPU</th><th>内存</th><th>操作</th></tr></thead>
          <tbody><tr v-for="pod in pods" :key="pod.name">
            <td class="cell-primary">{{ pod.name }}</td>
            <td><span :class="['badge', pod.ready ? 'badge-online' : 'badge-danger']">{{ pod.ready ? '运行中' : pod.reason || pod.phase || '未知' }}</span></td>
            <td>{{ pod.node || '-' }}</td>
            <td>{{ pod.restarts }}</td>
            <td>{{ formatCPU(pod.cpu_millicores) }}</td>
            <td>{{ formatMemory(pod.memory_bytes) }}</td>
            <td class="action-cell"><div class="pod-actions">
              <button class="icon-button" title="查看日志" :aria-label="`查看 ${pod.name} 的日志`" :disabled="!pod.containers?.length" @click="openLogs(pod)"><ScrollText :size="16" /></button>
              <button class="icon-button" :title="pod.phase === 'Running' ? '进入容器终端' : '仅运行中的 Pod 可进入终端'" :aria-label="`进入 ${pod.name} 的容器终端`" :disabled="pod.phase !== 'Running' || !pod.containers?.length" @click="terminalPod = pod"><SquareTerminal :size="16" /></button>
            </div></td>
          </tr></tbody>
        </table>
      </div>
    </SurfaceCard>

    <Teleport to="body"><div v-if="logPod" class="overlay" @click.self="closeLogs"><div class="modal pod-log-modal" role="dialog" aria-modal="true" aria-label="Pod 日志">
      <div class="log-header"><div><h2 class="modal-title">容器日志</h2><p>{{ logPod.name }}</p></div><button class="icon-button" title="关闭日志" aria-label="关闭日志" @click="closeLogs"><X :size="16" /></button></div>
      <div class="log-controls"><SelectMenu v-model="logContainer" aria-label="选择日志容器" :options="logPod.containers.map(name => ({ value: name, label: name }))" />
        <label v-if="logPod.restarts > 0" class="log-checkbox"><input v-model="previousLogs" type="checkbox" /> 上次容器</label>
        <label class="log-checkbox"><input v-model="followLogs" type="checkbox" /> 自动刷新</label>
        <button class="icon-button" title="刷新日志" aria-label="刷新日志" :disabled="logLoading" @click="loadLogs"><RefreshCw :size="15" :class="{ 'is-spinning': logLoading }" /></button>
      </div>
      <p v-if="logError" class="log-error">{{ logError }}</p>
      <pre class="log-output">{{ logLoading && !logText ? '正在读取日志...' : logText || '暂无日志' }}</pre>
    </div></div></Teleport>
    <PodTerminal v-if="terminalPod" :pod="terminalPod" :cluster-id="String(clusterId)" @close="terminalPod = null" />
  </section>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { RefreshCw, ScrollText, SquareTerminal, X } from 'lucide-vue-next'
import { getApplicationPodLogs, getApplicationPods, getApplicationRuntime } from '../../api/applications.js'
import EmptyState from '../../components/EmptyState.vue'
import FeedbackBanner from '../../components/FeedbackBanner.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import SurfaceCard from '../../components/SurfaceCard.vue'
import PodTerminal from '../resources/PodTerminal.vue'

const props = defineProps({ applicationId: { type: [String, Number], required: true }, clusterId: { type: [String, Number], required: true }, releases: { type: Array, default: () => [] } })
const runtime = ref(null)
const pods = ref([])
const metricsAvailable = ref(false)
const loading = ref(false)
const loaded = ref(false)
const error = ref('')
const terminalPod = ref(null)
const logPod = ref(null)
const logContainer = ref('')
const logText = ref('')
const logError = ref('')
const logLoading = ref(false)
const previousLogs = ref(false)
const followLogs = ref(true)
let runtimeTimer
let logTimer
let logRequest = 0

const statusLabel = computed(() => ({ running: '运行中', degraded: '异常', unavailable: '不可用', not_released: '未发布', deploying: '发布中' })[runtime.value?.status] || '未知')
const statusClass = computed(() => runtime.value?.status === 'running' ? 'badge-online' : runtime.value?.status === 'not_released' ? 'badge-offline' : 'badge-danger')
const activeVersion = computed(() => [...props.releases].filter(release => release.status === 'succeeded').sort((a, b) => b.sequence - a.sequence)[0]?.version || '-')

function formatCPU(value) { return value == null ? '-' : `${value} m` }
function formatMemory(value) { return value == null ? '-' : `${Math.round(value / 1024 / 1024)} MiB` }

async function loadRuntime() {
  if (loading.value) return
  loading.value = true
  try {
    const [summary, result] = await Promise.all([getApplicationRuntime(props.applicationId), getApplicationPods(props.applicationId)])
    runtime.value = summary
    pods.value = result?.pods || []
    metricsAvailable.value = !!result?.metrics_available
    error.value = ''
    loaded.value = true
  } catch (cause) {
    error.value = cause.message || '读取运行状态失败'
  } finally { loading.value = false }
}

function openLogs(pod) {
  logPod.value = pod
  logContainer.value = pod.containers[0]
  logText.value = ''
  logError.value = ''
  previousLogs.value = false
  followLogs.value = true
  loadLogs()
}
function closeLogs() { logRequest++; logPod.value = null; logText.value = ''; logError.value = ''; logLoading.value = false }
async function loadLogs() {
  if (!logPod.value || !logContainer.value || logLoading.value) return
  const request = ++logRequest
  logLoading.value = true
  try {
    const response = await getApplicationPodLogs(props.applicationId, logPod.value.name, { container: logContainer.value, previous: previousLogs.value, tail: 200 })
    if (request === logRequest) { logText.value = response?.logs || ''; logError.value = '' }
  } catch (cause) {
    if (request === logRequest) logError.value = cause.message || '读取日志失败'
  } finally { if (request === logRequest) logLoading.value = false }
}
watch([logContainer, previousLogs], () => { if (logPod.value) { logRequest++; logLoading.value = false; logText.value = ''; loadLogs() } })
onMounted(() => {
  loadRuntime()
  runtimeTimer = setInterval(loadRuntime, 15000)
  logTimer = setInterval(() => { if (logPod.value && followLogs.value) loadLogs() }, 3000)
})
watch(() => props.applicationId, () => { runtime.value = null; pods.value = []; loaded.value = false; loadRuntime() })
onBeforeUnmount(() => { clearInterval(runtimeTimer); clearInterval(logTimer); logRequest++ })
</script>

<style scoped>
.application-runtime{min-width:0;margin-top:var(--space-16)}
.runtime-toolbar,.runtime-summary,.runtime-actions,.log-header,.log-controls,.pod-actions{display:flex;align-items:center;gap:12px}
.runtime-toolbar{min-height:62px;justify-content:space-between;padding:12px 20px}
.runtime-summary{flex-wrap:wrap;color:var(--text-secondary);font-size:12px}
.runtime-actions{flex:none;gap:8px}
.runtime-metrics-note{color:var(--text-muted)}
.runtime-table-wrap{padding:0 20px 12px}.runtime-pods-table{min-width:760px}
.runtime-pods-table th:last-child,.runtime-pods-table td:last-child{text-align:right}
.pod-actions{justify-content:flex-end;gap:4px}
.pod-log-modal{width:min(980px,calc(100vw - 32px));max-height:calc(100vh - 48px);display:flex;flex-direction:column}
.log-header{justify-content:space-between}.log-header p{margin:3px 0 0;color:var(--text-secondary);font-size:12px}
.log-controls{flex-wrap:wrap;margin:16px 0 10px}.log-controls .select-menu{min-width:160px}.log-checkbox{display:flex;align-items:center;gap:6px;font-size:12px;color:var(--text-secondary)}
.log-output{min-height:240px;max-height:65vh;overflow:auto;margin:0;padding:14px;border-radius:var(--radius-control);background:var(--terminal-background);color:var(--terminal-foreground);font-size:12px;line-height:1.55;white-space:pre-wrap;overflow-wrap:anywhere}
.log-error{color:var(--danger);font-size:12px}
@media(max-width:640px){.runtime-table-wrap{padding:0 14px 10px}.runtime-toolbar{flex-wrap:wrap;padding:12px 14px}.runtime-actions{margin-left:auto}}
</style>
