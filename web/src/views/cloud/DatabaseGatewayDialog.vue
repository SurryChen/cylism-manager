<template>
  <BaseModal :open="Boolean(instance)" :title="`私网网关 · ${instance?.name || ''}`" size="large" teleport @close="emit('close')">
    <EmptyState v-if="loading" variant="loading" message="正在读取私网网关..." />
    <div v-else class="gateway-content">
      <div class="gateway-summary"><span class="mono">{{ instance?.host }}:{{ instance?.port }}</span><span class="badge" :class="gateway?.status === 'ready' ? 'badge-online' : gateway?.status === 'error' || gateway?.status === 'degraded' ? 'badge-danger' : 'badge-offline'">{{ stateLabel(gateway?.status) }}</span></div>
      <dl v-if="gateway" class="gateway-facts"><div><dt>应用连接地址</dt><dd class="mono">{{ view.service_host }}:3306</dd></div><div><dt>TLS 校验名称</dt><dd class="mono">{{ view.tls_server_name }}</dd></div><div v-if="gateway.last_verified_at"><dt>网络验证时间</dt><dd>{{ new Date(gateway.last_verified_at).toLocaleString() }}</dd></div><div v-if="gateway.last_checked_at"><dt>服务检查时间</dt><dd>{{ new Date(gateway.last_checked_at).toLocaleString() }}</dd></div></dl>
      <p v-if="gateway?.last_error" class="form-hint">{{ gateway.last_error }}</p>
      <form class="gateway-form" @submit.prevent="save">
        <fieldset :disabled="busy || submitting || loading || readFailed">
          <div class="gateway-row"><label class="form-group"><span class="form-label">网关服务器</span><SelectMenu v-model.number="form.server_id" :disabled="Boolean(gateway)" :options="serverOptions" aria-label="网关服务器" /></label><label class="form-group"><span class="form-label">应用所在集群</span><SelectMenu v-model.number="form.cluster_id" :disabled="Boolean(gateway)" :options="clusterOptions" aria-label="应用所在集群" /></label></div>
          <div class="gateway-row"><label class="form-group"><span class="form-label">网关 Tailscale IP</span><input v-model.trim="form.listen_ip" data-testid="gateway-listen-ip" class="form-input" :disabled="Boolean(gateway)" required placeholder="100.x.x.x" /></label><label class="form-group"><span class="form-label">监听端口</span><input v-model.number="form.listen_port" class="form-input" :disabled="Boolean(gateway)" required type="number" min="1024" max="65535" /></label></div>
          <label class="form-group"><span class="form-label">允许访问的来源 IP / CIDR</span><textarea v-model="sources" data-testid="gateway-sources" class="form-input mono" required rows="3" placeholder="100.80.1.2/32&#10;100.80.1.3/32" /></label>
          <details><summary>网络要求与诊断镜像</summary><p class="form-hint">网关需能访问 RDS 内网地址，通常选择同地域、同 VPC 的 ECS；需要 Python 3.7+、systemd、Tailscale 和免交互 sudo。平台不修改 RDS 白名单或组网策略。请填写真实来源地址：节点进行 SNAT 时通常是节点的 Tailscale IP；保留 Pod 源地址时需要对应 Pod CIDR。云内链路如需 TLS，应在应用中启用证书校验并使用上面的原始数据库域名。</p><label class="form-group"><span class="form-label">诊断镜像（需包含 Python 3）</span><input v-model.trim="form.probe_image" class="form-input" placeholder="python:3.12-alpine" /></label><p class="form-hint">部署后从各个 Ready 且可调度节点的临时 Pod 连续读取 3 次 MySQL 握手；只验证网络，不验证账号认证或 TLS。单个网关仍存在单点故障。</p></details>
        </fieldset>
      </form>
      <section v-if="tasks.length" class="gateway-tasks" aria-label="网关任务">
        <div class="gateway-row"><SelectMenu v-model.number="selectedTaskID" aria-label="网关任务" :options="taskOptions" @change="selectLogs" /><span v-if="currentTask" class="badge" :class="currentTask.status === 'succeeded' ? 'badge-online' : currentTask.status === 'failed' ? 'badge-danger' : 'badge-offline'">{{ taskState(currentTask.status) }}</span></div>
        <p v-if="currentTask" class="form-hint" aria-live="polite">{{ currentTask.progress }}</p>
        <pre v-if="logs.length" class="gateway-logs" aria-label="网关任务日志">{{ logs.map(item => `${new Date(item.created_at).toLocaleTimeString()} ${item.message}`).join('\n') }}</pre>
      </section>
    </div>
    <template #actions>
      <button class="btn" type="button" @click="emit('close')">关闭</button>
      <button v-if="gateway && gateway.status !== 'disabled'" class="btn btn-danger" data-testid="gateway-disable" type="button" :disabled="busy || submitting || readFailed" @click="confirmDisable = true">停用</button>
      <button v-if="gateway && gateway.status !== 'disabled'" class="btn" data-testid="gateway-check" type="button" :disabled="busy || submitting || readFailed" @click="act('check')">重新测试</button>
      <button class="btn btn-primary" data-testid="gateway-save" type="button" :disabled="busy || submitting || loading || readFailed" @click="save">{{ submitting ? '提交中...' : busy ? '后台执行中' : gateway?.status === 'disabled' ? '保存并启用' : '保存并部署' }}</button>
    </template>
  </BaseModal>
  <Teleport to="body"><ConfirmDialog :open="confirmDisable" title="停用私网网关" message="将停止转发并删除集群 Service；使用此网关的应用会无法连接数据库。外部数据库不会被删除。" confirm-text="停用" :busy="submitting" @cancel="confirmDisable = false" @close="confirmDisable = false" @confirm="act('disable')" /></Teleport>
  <ErrorNoticeModal :open="Boolean(error)" title="私网网关操作失败" :message="error" teleport @close="error = ''" />
</template>

<script setup>
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { getServers } from '../../api/servers.js'
import { getClusters } from '../../api/clusters.js'
import { configureDatabaseGateway, getDatabaseGateway, getDatabaseGatewayTaskLogs, runDatabaseGatewayAction } from '../../api/databases.js'
import BaseModal from '../../components/BaseModal.vue'
import ConfirmDialog from '../../components/ConfirmDialog.vue'
import EmptyState from '../../components/EmptyState.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import { usePolling } from '../../composables/usePolling.js'

const props = defineProps({ instance: { type: Object, default: null } })
const emit = defineEmits(['close', 'updated'])
const defaults = () => ({ server_id: 0, cluster_id: 0, listen_ip: '', listen_port: 13306, probe_image: 'python:3.12-alpine' })
const form = reactive(defaults())
const sources = ref(''), view = ref({}), servers = ref([]), clusters = ref([]), logs = ref([]), selectedTaskID = ref(0)
const loading = ref(false), readFailed = ref(false), submitting = ref(false), confirmDisable = ref(false), error = ref('')
const gateway = computed(() => view.value.gateway)
const tasks = computed(() => view.value.tasks || [])
const busy = computed(() => tasks.value.some(task => ['queued', 'running'].includes(task.status)))
const currentTask = computed(() => tasks.value.find(task => task.id === selectedTaskID.value))
const stateLabel = state => ({ pending: '待部署', checking: '测试中', ready: '网络已验证', degraded: '服务检查异常', disabling: '停用中', disabled: '已停用', error: '任务失败' }[state] || '尚未配置')
const taskState = state => ({ queued: '排队中', running: '执行中', succeeded: '成功', failed: '失败' }[state] || state)
const taskKind = kind => ({ deploy: '部署', check: '测试', disable: '停用' }[kind] || kind)
const taskOptions = computed(() => tasks.value.map(item => ({ value: item.id, label: `任务 #${item.id} · ${taskKind(item.kind)} · ${taskState(item.status)}` })))
const serverOptions = computed(() => [{ value: 0, label: '选择能访问 RDS 私网的服务器' }, ...servers.value.map(item => ({ value: item.id, label: `${item.name} · ${item.host}` }))])
const clusterOptions = computed(() => [{ value: 0, label: '选择应用所在集群' }, ...clusters.value.filter(item => item.status === 'ready').map(item => ({ value: item.id, label: item.name }))])
const polling = usePolling(refresh, { interval: 2500 })
let generation = 0
const isCurrent = (id, version) => props.instance?.id === id && generation === version
onBeforeUnmount(() => { generation++ })

async function loadLogs() {
  const id = props.instance?.id, taskID = selectedTaskID.value, version = generation
  if (!id || !taskID) { logs.value = []; return }
  const result = await getDatabaseGatewayTaskLogs(id, taskID)
  if (isCurrent(id, version) && selectedTaskID.value === taskID) logs.value = result
}
async function selectLogs() {
  const id = props.instance?.id, version = generation
  try { await loadLogs() }
  catch (failure) { if (isCurrent(id, version)) error.value = failure.message || '读取任务日志失败' }
}
async function refresh() {
  const id = props.instance?.id, version = generation
  if (!id) return
  try {
    const result = await getDatabaseGateway(id)
    if (!isCurrent(id, version)) return
    const previousLatest = tasks.value[0]?.id
    view.value = result
    if (!selectedTaskID.value || selectedTaskID.value === previousLatest) selectedTaskID.value = tasks.value[0]?.id || 0
    await loadLogs()
    if (!isCurrent(id, version)) return
    readFailed.value = false
    emit('updated')
  } catch (failure) { if (isCurrent(id, version)) { error.value = failure.message || '读取网关失败'; readFailed.value = true; polling.stop() } }
}
async function save() {
  const id = props.instance?.id, version = generation
  if (!id || loading.value || busy.value || submitting.value || readFailed.value) return
  const allowed_sources = sources.value.split(/[\s,，]+/).filter(Boolean)
  if (!form.server_id || !form.cluster_id || !form.listen_ip || !allowed_sources.length) { error.value = '请选择服务器、应用集群，并填写监听 IP 和允许来源'; return }
  submitting.value = true
  try {
    await configureDatabaseGateway(id, { ...form, allowed_sources })
    if (!isCurrent(id, version)) return
    await refresh()
    if (isCurrent(id, version) && !readFailed.value) polling.start()
  }
  catch (failure) { if (isCurrent(id, version)) error.value = failure.message || '保存私网网关失败' }
  finally { if (isCurrent(id, version)) submitting.value = false }
}
async function act(action) {
  const id = props.instance?.id, version = generation
  if (!id || loading.value || readFailed.value || busy.value || submitting.value) return
  submitting.value = true
  try {
    await runDatabaseGatewayAction(id, action)
    if (!isCurrent(id, version)) return
    confirmDisable.value = false
    await refresh()
    if (isCurrent(id, version) && !readFailed.value) polling.start()
  }
  catch (failure) { if (isCurrent(id, version)) error.value = failure.message || '提交网关任务失败' }
  finally { if (isCurrent(id, version)) submitting.value = false }
}
watch(() => props.instance?.id, async id => {
  const currentGeneration = ++generation
  polling.stop(); loading.value = false; submitting.value = false; error.value = ''; confirmDisable.value = false; view.value = {}; logs.value = []; selectedTaskID.value = 0; sources.value = ''; readFailed.value = false; Object.assign(form, defaults())
  if (!id) return
  loading.value = true
  try {
    const results = await Promise.all([getServers(), getClusters(), getDatabaseGateway(id)])
    if (currentGeneration !== generation) return
    servers.value = results[0]; clusters.value = results[1]; view.value = results[2]
    if (gateway.value) { Object.keys(defaults()).forEach(key => { form[key] = gateway.value[key] }); sources.value = gateway.value.allowed_sources.join('\n') }
    selectedTaskID.value = tasks.value[0]?.id || 0
    await loadLogs()
    if (currentGeneration === generation) polling.start()
  } catch (failure) { if (currentGeneration === generation) { error.value = failure.message || '读取私网网关失败'; readFailed.value = true } }
  finally { if (currentGeneration === generation) loading.value = false }
}, { immediate: true })
</script>

<style scoped>
.gateway-content, .gateway-form fieldset { display: grid; gap: var(--space-16); }
.gateway-form fieldset { border: 0; padding: 0; margin: 0; min-width: 0; }
.gateway-summary, .gateway-row { display: flex; align-items: center; gap: var(--space-12); }
.gateway-summary { justify-content: space-between; }
.gateway-row > .form-group { flex: 1; min-width: 0; margin: 0; }
.gateway-facts { display: grid; gap: 8px; margin: 0; font-size: 12px; }
.gateway-facts > div { display: grid; grid-template-columns: 100px 1fr; gap: 12px; }
.gateway-facts dt { color: var(--text-secondary); }.gateway-facts dd { margin: 0; }
.mono { font-family: var(--font-mono); overflow-wrap: anywhere; }
.gateway-tasks { border-top: 1px solid var(--border-muted); padding-top: var(--space-16); }
.gateway-logs { white-space: pre-wrap; overflow-wrap: anywhere; max-height: 220px; overflow: auto; padding: var(--space-12); background: var(--surface-subtle); font-size: 12px; border-radius: var(--radius-control); }
summary { cursor: pointer; font-size: 12px; color: var(--text-secondary); }
@media (max-width: 600px) { .gateway-row, .gateway-summary { flex-direction: column; align-items: stretch; }.gateway-facts > div { grid-template-columns: 1fr; gap: 4px; } }
</style>
