<template>
  <SurfaceCard as="div" padding="none" class="speed-test-card">
    <form class="speed-test-toolbar" @submit.prevent="runTest">
      <SelectMenu v-model="sourceID" :options="serverOptions" placeholder="来源节点" aria-label="来源节点" :disabled="busy" />
      <ArrowRight :size="16" class="direction-icon" aria-hidden="true" />
      <SelectMenu v-model="targetID" :options="targetOptions" placeholder="目标节点" aria-label="目标节点" :disabled="busy" />
      <SelectMenu v-model="address" :options="addressOptions" aria-label="目标地址" :disabled="busy" />
      <SelectMenu v-model="protocol" :options="protocolOptions" aria-label="测速协议" :disabled="busy" />
      <SelectMenu v-model="direction" :options="directionOptions" aria-label="传输方向" :disabled="busy" />
      <button class="btn btn-primary" type="submit" :disabled="!canRun"><LoaderCircle v-if="busy" :size="15" class="is-spinning" /><Gauge v-else :size="15" />{{ checking ? '检测中...' : running ? '测速中...' : installingID !== null ? '安装中...' : '开始测速' }}</button>
    </form>

    <section class="speed-test-output" aria-label="测速结果">
      <div class="speed-section-heading"><h2>测速结果</h2><span v-if="results.length" class="cell-secondary">{{ results.length === maxResults ? '最近 ' : '' }}{{ results.length }} 条记录</span></div>
      <div v-if="running" class="speed-test-placeholder speed-progress"><LoaderCircle :size="16" class="is-spinning" />正在测试节点间传输</div>
      <div v-if="results.length" class="table-wrap speed-results-table-wrap">
        <table class="data-table speed-results-table">
          <thead><tr><th>时间</th><th>传输方向</th><th>连接方式</th><th>发送速率</th><th>接收速率</th><th>传输质量</th></tr></thead>
          <tbody><tr v-for="(entry, index) in pagedResults" :key="`${entry.measuredAt}-${index}`">
            <td class="speed-result-time">{{ formatDateTime(entry.measuredAt) }}</td>
            <td class="speed-result-route"><span class="speed-result-path">{{ serverName(entry.direction === 'forward' ? entry.source_id : entry.target_id) }} <ArrowRight :size="13" aria-hidden="true" /> {{ serverName(entry.direction === 'forward' ? entry.target_id : entry.source_id) }}</span></td>
            <td class="speed-result-connection"><span class="cell-primary">{{ entry.protocol.toUpperCase() }} · {{ entry.address === 'tailscale' ? 'Tailscale' : '登记地址' }}</span><span class="cell-secondary speed-result-address">{{ entry.target_address }}</span></td>
            <td class="speed-result-sent" data-label="发送速率">{{ formatMBps(entry.sender_mbps) }}</td>
            <td class="speed-result-received" data-label="接收速率"><strong>{{ formatMBps(entry.receiver_mbps) }}</strong></td>
            <td class="speed-result-quality">{{ entry.protocol === 'tcp' ? `重传 ${entry.retransmits} 次` : `丢包 ${entry.lost_packets} / ${entry.total_packets} (${Number(entry.loss_percent).toFixed(1)}%)` }}</td>
          </tr></tbody>
        </table>
      </div>
      <div v-if="results.length > pageSize" class="speed-results-pagination"><button class="btn btn-sm" type="button" :disabled="page === 1" @click="page--">上一页</button><span>{{ page }} / {{ totalPages }}</span><button class="btn btn-sm" type="button" :disabled="page === totalPages" @click="page++">下一页</button></div>
      <div v-if="!running && !results.length" class="speed-test-placeholder">{{ servers.length < 2 ? '至少需要两台服务器' : '暂无测速结果' }}</div>
    </section>
  </SurfaceCard>
  <BaseModal :open="installPromptOpen" title="安装测速工具" size="small" :show-close="installingID === null" :close-on-overlay="installingID === null" :close-on-escape="installingID === null" @close="closeInstallPrompt">
    <p class="speed-install-intro">以下节点缺少测速所需工具：</p>
    <ul class="speed-install-list">
      <li v-for="node in missingNodes" :key="node.id"><strong>{{ node.name }}</strong><span class="cell-secondary">{{ node.role }}</span><span>{{ missingTools(node) }}</span></li>
    </ul>
    <p v-if="installingID !== null" class="speed-install-progress"><LoaderCircle :size="15" class="is-spinning" />正在安装 {{ serverName(installingID) }} · {{ formatElapsed(installElapsed) }}</p>
    <p v-if="installError" class="speed-install-error" role="alert">{{ installError }}</p>
    <template #actions>
      <button class="btn" type="button" :disabled="installingID !== null" @click="closeInstallPrompt">取消</button>
      <button class="btn btn-primary" type="button" :disabled="installingID !== null" @click="installTools"><Download :size="15" />安装并测速</button>
    </template>
  </BaseModal>
  <ErrorNoticeModal :open="Boolean(error)" :message="error" teleport @close="error = ''" />
</template>

<script setup>
import { computed, onUnmounted, ref, watch } from 'vue'
import { ArrowRight, Download, Gauge, LoaderCircle } from 'lucide-vue-next'
import { getServerSpeedDependencies, installServerSpeedDependencies, runServerSpeedTest } from '../../api/servers.js'
import BaseModal from '../../components/BaseModal.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import SurfaceCard from '../../components/SurfaceCard.vue'
import { formatDateTime } from '../../utils/formatters.js'

const resultsStorageKey = 'cylism.server-speed-test.results.v1'
const maxResults = 100
const pageSize = 10
const props = defineProps({ servers: { type: Array, default: () => [] } })
const sourceID = ref('')
const targetID = ref('')
const address = ref('registered')
const protocol = ref('tcp')
const direction = ref('forward')
const checking = ref(false)
const running = ref(false)
const installingID = ref(null)
const installPromptOpen = ref(false)
const installError = ref('')
const results = ref(loadResults())
const page = ref(1)
const error = ref('')
const dependencies = ref({})
const installElapsed = ref(0)
let installTimer = null
let checkController = null
const busy = computed(() => checking.value || running.value || installingID.value !== null)
const selectedNodes = computed(() => [
  { id: Number(sourceID.value), role: '来源' },
  { id: Number(targetID.value), role: '目标' },
].filter(node => node.id && props.servers.some(server => server.id === node.id)).map(node => ({ ...node, name: serverName(node.id) })))
const canRun = computed(() => !busy.value && !installPromptOpen.value && selectedNodes.value.length === 2)
const missingNodes = computed(() => selectedNodes.value.filter(node => !nodeReady(node)))
const totalPages = computed(() => Math.ceil(results.value.length / pageSize))
const pagedResults = computed(() => results.value.slice((page.value - 1) * pageSize, page.value * pageSize))
const serverOptions = computed(() => props.servers.map(server => ({ value: String(server.id), label: server.name, description: server.host })))
const targetOptions = computed(() => serverOptions.value.map(option => ({ ...option, disabled: option.value === sourceID.value })))
const addressOptions = [
  { value: 'registered', label: '登记地址' },
  { value: 'tailscale', label: 'Tailscale IP' },
]
const protocolOptions = [
  { value: 'tcp', label: 'TCP' },
  { value: 'udp', label: 'UDP · 0.125 MB/s' },
]
const directionOptions = [
  { value: 'forward', label: '来源 → 目标' },
  { value: 'reverse', label: '目标 → 来源' },
]

watch(sourceID, value => { if (value === targetID.value) targetID.value = '' })
watch(() => props.servers, servers => {
  if (!servers.some(server => String(server.id) === sourceID.value)) sourceID.value = ''
  if (!servers.some(server => String(server.id) === targetID.value)) targetID.value = ''
})
onUnmounted(() => {
  checkController?.abort()
  if (installTimer) clearInterval(installTimer)
})

function closeInstallPrompt() {
  if (installingID.value !== null) return
  installPromptOpen.value = false
  installError.value = ''
}

function nodeReady(node) {
  const status = dependencies.value[node.id]
  return Boolean(status?.iperf3_installed && (node.role !== '目标' || status.timeout_installed))
}

function missingTools(node) {
  const status = dependencies.value[node.id]
  const missing = []
  if (!status?.iperf3_installed) missing.push('iperf3')
  if (node.role === '目标' && !status?.timeout_installed) missing.push('timeout')
  return missing.join('、')
}

function formatElapsed(seconds) {
  return `${String(Math.floor(seconds / 60)).padStart(2, '0')}:${String(seconds % 60).padStart(2, '0')}`
}

async function installTools() {
  if (!installPromptOpen.value || busy.value) return
  installError.value = ''
  try {
    for (const node of [...missingNodes.value]) {
      installingID.value = node.id
      installElapsed.value = 0
      installTimer = setInterval(() => { installElapsed.value += 1 }, 1000)
      try {
        const status = await installServerSpeedDependencies(node.id)
        dependencies.value = { ...dependencies.value, [node.id]: status }
        if (!nodeReady(node)) throw new Error(`${node.name} 安装后仍缺少 ${missingTools(node)}`)
      } finally {
        clearInterval(installTimer)
        installTimer = null
      }
    }
    installPromptOpen.value = false
    await executeTest()
  } catch (cause) {
    installError.value = cause?.message || '安装测速工具失败'
  } finally {
    installingID.value = null
  }
}

function serverName(id) {
  return props.servers.find(server => server.id === id)?.name || `#${id}`
}

function formatMBps(value) {
  const megabytes = Number(value || 0) / 8
  const digits = megabytes > 0 && megabytes < 1 ? 3 : 2
  const factor = 10 ** digits
  return `${(Math.round((megabytes + Number.EPSILON) * factor) / factor).toFixed(digits)} MB/s`
}

function loadResults() {
  try {
    const stored = JSON.parse(sessionStorage.getItem(resultsStorageKey) || '[]')
    return Array.isArray(stored) ? stored.filter(entry => entry?.measuredAt && typeof entry?.protocol === 'string' && entry?.source_id && entry?.target_id).slice(0, maxResults) : []
  } catch {
    return []
  }
}

function addResult(report) {
  results.value = [{ ...report, measuredAt: new Date().toISOString() }, ...results.value].slice(0, maxResults)
  page.value = 1
  try { sessionStorage.setItem(resultsStorageKey, JSON.stringify(results.value)) } catch { /* Browser storage can be unavailable. */ }
}

async function runTest() {
  if (!canRun.value) return
  checking.value = true
  checkController = new AbortController()
  try {
    const statuses = await Promise.all(selectedNodes.value.map(node => getServerSpeedDependencies(node.id, { signal: checkController.signal })))
    dependencies.value = Object.fromEntries(statuses.map(status => [status.server_id, status]))
    if (missingNodes.value.length) {
      installError.value = ''
      installPromptOpen.value = true
      return
    }
    checking.value = false
    await executeTest()
  } catch (cause) {
    if (!checkController.signal.aborted) error.value = cause?.message || '检测测速工具失败'
  } finally {
    checking.value = false
  }
}

async function executeTest() {
  running.value = true
  try {
    const report = await runServerSpeedTest({
      source_id: Number(sourceID.value),
      target_id: Number(targetID.value),
      address: address.value,
      protocol: protocol.value,
      direction: direction.value,
    })
    if (!report?.protocol || !report?.source_id || !report?.target_id) throw new Error('测速结果无效')
    addResult(report)
  } catch (cause) {
    error.value = cause?.message || '测速失败'
  } finally {
    running.value = false
  }
}
</script>

<style scoped>
.speed-test-toolbar { display: flex; align-items: center; flex-wrap: wrap; gap: var(--space-8); padding: 14px var(--space-16); border-bottom: 1px solid var(--border-muted); }
.speed-test-toolbar .select-menu { min-width: 140px; flex: 1 1 140px; }
.speed-test-toolbar .btn { flex: 0 0 auto; }
.direction-icon { flex: 0 0 auto; color: var(--text-muted); }
.speed-section-heading { display: flex; min-height: 36px; align-items: center; gap: 8px; padding: 0 var(--space-16); }
.speed-section-heading h2 { margin: 0; color: var(--text-primary); font-size: 13px; font-weight: 700; }
.speed-install-intro { margin: 0 0 12px; color: var(--text-secondary); font-size: 12px; }
.speed-install-list { display: grid; gap: 0; margin: 0; padding: 0; list-style: none; }
.speed-install-list li { display: flex; min-height: 40px; align-items: center; gap: 8px; border-top: 1px solid var(--border-muted); font-size: 12px; }
.speed-install-list li strong { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.speed-install-list li > span:last-child { margin-left: auto; color: var(--warning); white-space: nowrap; }
.speed-install-progress { display: flex; align-items: center; gap: 7px; margin: 12px 0 0; color: var(--text-secondary); font-size: 12px; }
.speed-install-error { margin: 12px 0 0; color: var(--danger); font-size: 12px; overflow-wrap: anywhere; }
.speed-test-placeholder.speed-progress { display: flex; align-items: center; gap: 7px; }
.speed-test-output { padding: 8px 0 10px; }
.speed-results-table-wrap { padding: 0 var(--space-16); }
.speed-results-table { min-width: 850px; }
.speed-results-table th, .speed-results-table td { padding: 10px 12px; }
.speed-result-time { white-space: nowrap; font-variant-numeric: tabular-nums; }
.speed-result-path { display: inline-flex; align-items: center; gap: 6px; white-space: nowrap; }
.speed-result-path svg { flex: 0 0 auto; color: var(--text-muted); }
.speed-result-address { display: block; margin-top: 2px; white-space: nowrap; }
.speed-results-table td:nth-child(4), .speed-results-table td:nth-child(5) { white-space: nowrap; font-variant-numeric: tabular-nums; }
.speed-results-table td:last-child { white-space: nowrap; }
.speed-results-pagination { display: flex; align-items: center; justify-content: flex-end; gap: 8px; padding: 12px var(--space-16) 2px; color: var(--text-secondary); font-size: 11px; }
.speed-test-placeholder { padding: 14px var(--space-16); color: var(--text-muted); font-size: 12px; }
@media (max-width: 640px) {
  .speed-test-toolbar { flex-direction: column; align-items: stretch; }
  .speed-test-toolbar .select-menu { flex: none; width: 100%; }
  .speed-test-toolbar .btn { justify-content: center; }
  .direction-icon { align-self: center; transform: rotate(90deg); }
  .speed-install-list li { flex-wrap: wrap; gap: 4px 8px; padding: 8px 0; }
  .speed-install-list li > span:last-child { flex-basis: 100%; margin-left: 0; }
  .speed-results-table { display: block; min-width: 0; }
  .speed-results-table thead { display: none; }
  .speed-results-table tbody { display: block; }
  .speed-results-table tbody tr { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); column-gap: 12px; padding: 12px 0; border-bottom: 1px solid var(--border-muted); }
  .speed-results-table tbody tr:last-child { border-bottom: 0; }
  .speed-results-table td { display: block; min-width: 0; padding: 2px 0; border: 0; }
  .speed-results-table .speed-result-route { grid-column: 1 / -1; grid-row: 1; }
  .speed-result-path { flex-wrap: wrap; white-space: normal; overflow-wrap: anywhere; }
  .speed-results-table .speed-result-time { grid-column: 1 / -1; grid-row: 2; color: var(--text-muted); font-size: 11px; }
  .speed-results-table .speed-result-connection { grid-column: 1 / -1; grid-row: 3; }
  .speed-result-address { display: inline; margin: 0 0 0 6px; overflow-wrap: anywhere; white-space: normal; }
  .speed-results-table .speed-result-sent, .speed-results-table .speed-result-received { padding-top: 10px; }
  .speed-results-table td[data-label]::before { display: block; content: attr(data-label); color: var(--text-muted); font-size: 10px; }
  .speed-results-table .speed-result-quality { grid-column: 1 / -1; grid-row: 5; padding-top: 8px; }
}
</style>
