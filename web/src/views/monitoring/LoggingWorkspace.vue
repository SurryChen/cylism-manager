<template>
  <section v-if="statusError" class="k8s-banner k8s-banner-warn section-gap" role="alert">{{ statusError }}</section>

  <SurfaceCard v-if="loading && !status" class="logging-wait"><div class="empty-state"><span class="empty-icon">◌</span><span class="empty-text">正在读取日志采集状态</span></div></SurfaceCard>

  <SurfaceCard v-else-if="status?.state === 'not_installed'" class="logging-install-card">
    <template #header><div><h2 class="card-title">容器日志未启用</h2><p class="status-copy">平台将使用 Loki 保存日志，并在每个节点运行 Alloy 采集容器标准输出。</p></div></template><template #actions><span class="badge badge-offline">未安装</span></template>
    <form class="install-form" @submit.prevent="install">
      <div class="form-group"><label class="form-label">数据节点</label><SelectMenu v-model="installForm.node_name" class="form-select" required><option value="" disabled>选择就绪节点</option><option v-for="node in readyNodes" :key="node.name" :value="node.name">{{ displayNode(node) }}</option></SelectMenu><p class="form-hint">Loki 数据卷会绑定到所选节点；Alloy 会在所有可调度节点采集容器日志。</p></div>
      <div class="form-row"><label class="form-group"><span class="form-label">存储容量</span><input v-model.trim="installForm.storage" class="form-input" required placeholder="10Gi" /></label><label class="form-group"><span class="form-label">StorageClass</span><SelectMenu v-model="installForm.storage_class_name" class="form-select"><option value="">使用集群默认 StorageClass</option><option v-for="item in storageClasses" :key="item.name" :value="item.name">{{ item.name }}{{ item.is_default ? '（默认）' : '' }}</option></SelectMenu></label><label class="form-group"><span class="form-label">日志保留天数</span><input v-model.number="installForm.retention_days" class="form-input" type="number" min="1" max="365" required /></label></div>
      <p class="form-hint">平台会自动创建 <code>cylism-loki-data</code>。该存储卷由日志组件管理，卸载采集器不会删除已有日志。</p>
      <p v-if="installError" class="form-error" role="alert">{{ installError }}</p>
      <div class="modal-actions status-actions"><button class="btn btn-primary" :disabled="installing || !installForm.node_name">{{ installing ? '正在提交...' : '启用日志采集' }}</button><button type="button" class="btn" :disabled="installing" @click="refresh">重新检测</button></div>
    </form>
  </SurfaceCard>

  <template v-else-if="status">
    <section class="metric-grid logging-summary section-gap">
      <article class="metric"><span>日志状态</span><strong><span class="badge" :class="statusBadge">{{ statusLabel }}</span></strong><small>{{ status.message }}</small></article>
      <article class="metric"><span>Loki</span><strong>{{ status.loki_ready || 0 }} / 1</strong><small>{{ status.node_name || '等待分配数据节点' }}</small></article>
      <article class="metric"><span>采集节点</span><strong>{{ status.alloy_ready || 0 }} / {{ status.alloy_desired || 0 }}</strong><small>Alloy 已就绪</small></article>
      <article class="metric"><span>日志保留</span><strong>{{ status.retention_days || '-' }} 天</strong><small>{{ status.storage || '-' }}{{ status.storage_class_name ? ` · ${status.storage_class_name}` : '' }}</small></article>
    </section>

    <SurfaceCard v-if="!logsAvailable" class="logging-wait section-gap"><div class="empty-state"><span class="empty-icon">◌</span><span class="empty-text">{{ status.message || '等待 Loki 存储实例就绪' }}</span></div><div class="modal-actions status-actions"><button class="icon-button" data-testid="logging-settings" title="日志设置" aria-label="日志设置" @click="openSettings"><Settings2 :size="16" /></button><button class="btn" :disabled="loading" @click="refresh">重新检测</button></div></SurfaceCard>

    <template v-else>
      <div v-if="status.state === 'degraded'" class="k8s-banner k8s-banner-warn section-gap">{{ status.message }}</div>
      <section class="logging-section-heading section-gap"><div><h2>日志检索</h2><p>按容器标准输出检索。日志不会在打开页面时自动加载。</p></div><div class="icon-actions"><button class="icon-button" data-testid="logging-settings" title="日志设置" aria-label="日志设置" @click="openSettings"><Settings2 :size="16" /></button><button class="icon-button" title="刷新日志状态与筛选项" aria-label="刷新日志状态与筛选项" :disabled="loading" @click="refresh"><RefreshCw :size="16" :class="{ 'is-spinning': loading }" /></button></div></section>

      <SurfaceCard as="div" class="logging-query section-gap"><form class="logging-query" @submit.prevent="queryLogs">
        <p v-if="filterError" class="form-error">{{ filterError }}</p>
        <div class="logging-filter-grid">
          <label class="form-group"><span class="form-label">时间范围</span><SelectMenu v-model="queryForm.range" class="form-select" data-testid="log-time-range"><option value="1h">最近 1 小时</option><option value="6h">最近 6 小时</option><option value="24h">最近 24 小时</option><option value="custom">精确范围</option></SelectMenu></label>
          <label v-if="queryForm.range === 'custom'" class="form-group"><span class="form-label">开始时间</span><input v-model="queryForm.start_time" class="form-input" data-testid="log-start-time" type="datetime-local" step="1" required /></label>
          <label v-if="queryForm.range === 'custom'" class="form-group"><span class="form-label">结束时间</span><input v-model="queryForm.end_time" class="form-input" data-testid="log-end-time" type="datetime-local" step="1" required /></label>
          <label class="form-group"><span class="form-label">返回条数</span><SelectMenu v-model.number="queryForm.limit" class="form-select" data-testid="log-result-limit"><option :value="50">50 条</option><option :value="100">100 条</option><option :value="200">200 条</option><option :value="500">500 条</option></SelectMenu></label>
          <label class="form-group"><span class="form-label">项目</span><SelectMenu v-model.number="queryForm.project_id" class="form-select"><option :value="0">全部项目</option><option v-for="project in projects" :key="project.id" :value="project.id">{{ project.name }}</option></SelectMenu></label>
          <label class="form-group"><span class="form-label">环境</span><SelectMenu v-model.number="queryForm.environment_id" class="form-select"><option :value="0">全部环境</option><option v-for="environment in scopedEnvironments" :key="environment.id" :value="environment.id">{{ environment.name }}</option></SelectMenu></label>
          <label class="form-group"><span class="form-label">应用</span><SelectMenu v-model.number="queryForm.application_id" class="form-select"><option :value="0">全部应用</option><option v-for="application in scopedApplications" :key="application.id" :value="application.id">{{ application.name }}</option></SelectMenu></label>
          <label class="form-group"><span class="form-label">命名空间</span><SelectMenu v-model="queryForm.namespace" class="form-select"><option value="">全部命名空间</option><option v-for="namespace in namespaceOptions" :key="namespace" :value="namespace">{{ namespace }}</option></SelectMenu></label>
          <label class="form-group"><span class="form-label">Pod</span><SelectMenu v-model="queryForm.pod" class="form-select"><option value="">全部 Pod</option><option v-for="pod in scopedPods" :key="`${pod.namespace}/${pod.name}`" :value="pod.name">{{ pod.name }}</option></SelectMenu></label>
          <label class="form-group"><span class="form-label">容器</span><SelectMenu v-model="queryForm.container" class="form-select"><option value="">全部容器</option><option v-for="container in containerOptions" :key="container" :value="container">{{ container }}</option></SelectMenu></label>
          <label class="form-group"><span class="form-label">节点</span><SelectMenu v-model="queryForm.node" class="form-select"><option value="">全部节点</option><option v-for="node in filterOptions.nodes" :key="node" :value="node">{{ node }}</option></SelectMenu></label>
        </div>
        <div class="logging-content-filter"><label class="form-label" for="log-keyword">日志内容</label><div class="logging-search-row"><input id="log-keyword" v-model.trim="queryForm.keyword" class="form-input" maxlength="256" placeholder="按日志内容筛选，例如 error 或 connection refused" /><button class="btn btn-primary" data-testid="query-logs" :disabled="querying" type="submit">{{ querying ? '查询中...' : '查询日志' }}</button></div></div>
      </form></SurfaceCard>

      <SurfaceCard v-if="querying" class="logging-results section-gap"><div class="empty-inline">正在查询日志...</div></SurfaceCard>
      <p v-if="queryError" class="form-error section-gap">{{ queryError }}</p>
      <SurfaceCard v-if="queried && !lines.length" class="logging-results section-gap"><div class="empty-inline">当前筛选范围内没有匹配日志</div></SurfaceCard>
      <SurfaceCard v-if="lines.length" class="logging-results section-gap"><div class="logging-results-header"><span>已返回 {{ lines.length }} 行</span><small v-if="hasMore">结果已达到本次查询上限，请缩小筛选范围</small></div><div class="logging-lines"><article v-for="(entry, index) in lines" :key="`${entry.timestamp}-${index}`" class="logging-line"><time>{{ formatTime(entry.timestamp) }}</time><div class="logging-line-copy"><span class="logging-labels">{{ lineContext(entry.labels) }}</span><pre>{{ entry.line }}</pre></div></article></div></SurfaceCard>
    </template>
  </template>

  <Teleport to="body">
    <div v-if="settingsOpen" class="overlay logging-settings-overlay" @click.self="settingsOpen = false"><form class="modal logging-settings-modal" @submit.prevent="saveSettings"><header class="drawer-header"><div><h2>日志设置</h2><p>调整日志保留策略，不会直接修改系统管理的存储卷。</p></div><button class="icon-button" type="button" title="关闭日志设置" aria-label="关闭日志设置" @click="settingsOpen = false"><X :size="16" /></button></header><section class="drawer-section"><div class="settings-field"><span>数据节点</span><strong>{{ status?.node_name || '-' }}</strong></div><div class="settings-field"><span>系统存储卷</span><strong class="metric-code">{{ status?.pvc_name || 'cylism-loki-data' }}</strong><small>{{ status?.storage || '-' }}</small></div><label class="form-group settings-retention"><span class="form-label">日志保留天数</span><input v-model.number="settingsForm.retention_days" class="form-input" type="number" min="1" max="365" required /><span class="form-hint">超过新保留周期的日志由 Loki 后台自动清理。</span></label></section><p v-if="settingsError" class="form-error" role="alert">{{ settingsError }}</p><footer class="drawer-footer"><button class="btn" type="button" @click="settingsOpen = false">取消</button><button class="btn btn-primary" :disabled="saving">{{ saving ? '保存中...' : '保存设置' }}</button></footer></form></div>
  </Teleport>

  <div v-if="confirmUninstall" class="overlay" @click.self="confirmUninstall = false"><div class="modal logging-uninstall-modal"><h2 class="modal-title">卸载日志采集</h2><p class="confirm-copy">将删除 Loki、Alloy 与采集配置，但保留 <code>cylism-loki-data</code> 中已有日志。</p><p v-if="uninstallError" class="form-error" role="alert">{{ uninstallError }}</p><div class="modal-actions"><button class="btn" @click="confirmUninstall = false">取消</button><button class="btn btn-danger" :disabled="uninstalling" @click="uninstall">{{ uninstalling ? '卸载中...' : '确认卸载' }}</button></div></div></div>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { RefreshCw, Settings2, X } from 'lucide-vue-next'
import { getApplications, getProjects } from '../../api/applications.js'
import { getLoggingFilters, getLoggingStatus, installLogging, queryLogs as requestLogs, saveLoggingConfig, uninstallLogging } from '../../api/logging.js'
import { useAsyncResource } from '../../composables/useAsyncResource.js'
import SurfaceCard from '../../components/SurfaceCard.vue'
import { formatDateTime as formatTime } from '../../utils/formatters.js'

const props = defineProps({ nodes: { type: Array, default: () => [] }, storageClasses: { type: Array, default: () => [] } })

const status = ref(null)
const statusError = ref('')
const filterError = ref('')
const queryError = ref('')
const installError = ref('')
const settingsError = ref('')
const uninstallError = ref('')
const loading = ref(false)
const installing = ref(false)
const querying = ref(false)
const saving = ref(false)
const uninstalling = ref(false)
const settingsOpen = ref(false)
const confirmUninstall = ref(false)
const queried = ref(false)
const hasMore = ref(false)
const lines = ref([])
const filterOptions = ref({ namespaces: [], pods: [], nodes: [] })
const applications = ref([])
const projects = ref([])
const installForm = ref({ node_name: '', storage: '10Gi', storage_class_name: '', retention_days: 14 })
const settingsForm = ref({ retention_days: 14 })
const queryForm = ref({ range: '1h', start_time: '', end_time: '', limit: 200, keyword: '', project_id: 0, environment_id: 0, application_id: 0, namespace: '', pod: '', container: '', node: '' })
const statusResource = useAsyncResource(({ signal }) => getLoggingStatus({ signal }), null)
const filtersResource = useAsyncResource(async ({ signal }) => {
  const [filters, apps, projectList] = await Promise.all([getLoggingFilters({ signal }), getApplications({}, { signal }), getProjects({ signal })])
  return { filters, apps, projectList }
}, null)
const queryResource = useAsyncResource(({ signal }, payload) => requestLogs(payload, { signal }), null)

const readyNodes = computed(() => props.nodes.filter(node => node.ready))
const logsAvailable = computed(() => Number(status.value?.loki_ready) > 0)
const statusLabel = computed(() => ({ ready: '已就绪', installing: '启动中', degraded: '部分降级' }[status.value?.state] || '未安装'))
const statusBadge = computed(() => ({ ready: 'badge-online', installing: 'badge-deploying', degraded: 'badge-warn' }[status.value?.state] || 'badge-offline'))
const scopedApplications = computed(() => applications.value.filter(application => (!queryForm.value.project_id || application.project_id === queryForm.value.project_id) && (!queryForm.value.environment_id || application.environment_id === queryForm.value.environment_id)))
const scopedEnvironments = computed(() => {
  const environments = new Map()
  applications.value.filter(application => !queryForm.value.project_id || application.project_id === queryForm.value.project_id).forEach(application => {
    if (application.environment?.id) environments.set(application.environment.id, application.environment)
  })
  return [...environments.values()].sort((left, right) => left.name.localeCompare(right.name))
})
const namespaceOptions = computed(() => {
  const values = new Set(filterOptions.value.namespaces || [])
  scopedApplications.value.forEach(application => { if (application.environment?.namespace) values.add(application.environment.namespace) })
  return [...values].sort()
})
const scopedPods = computed(() => (filterOptions.value.pods || []).filter(pod => !queryForm.value.namespace || pod.namespace === queryForm.value.namespace))
const containerOptions = computed(() => {
  const values = new Set()
  scopedPods.value.filter(pod => !queryForm.value.pod || pod.name === queryForm.value.pod).forEach(pod => (pod.containers || []).forEach(container => values.add(container)))
  return [...values].sort()
})

onMounted(refresh)

watch(() => queryForm.value.project_id, () => { queryForm.value.environment_id = 0; queryForm.value.application_id = 0 })
watch(() => queryForm.value.environment_id, () => { queryForm.value.application_id = 0 })
watch(() => queryForm.value.application_id, applicationID => {
  const application = applications.value.find(item => item.id === applicationID)
  if (application?.environment?.namespace) queryForm.value.namespace = application.environment.namespace
})
watch(() => queryForm.value.namespace, () => { queryForm.value.pod = ''; queryForm.value.container = '' })
watch(() => queryForm.value.pod, () => { queryForm.value.container = '' })

function displayNode(node) { return node.display_name || node.name }

async function refresh() {
  loading.value = true
  statusError.value = ''
  const result = await statusResource.refresh()
  if (result !== undefined) {
    status.value = result
    if (!installForm.value.node_name) installForm.value.node_name = readyNodes.value[0]?.name || ''
    if (logsAvailable.value) await loadFilters()
  } else if (statusResource.error.value) statusError.value = statusResource.error.value.message || '读取日志采集状态失败'
  loading.value = false
}

async function loadFilters() {
  filterError.value = ''
  const result = await filtersResource.refresh()
  if (result !== undefined) {
    filterOptions.value = { namespaces: result.filters?.namespaces || [], pods: result.filters?.pods || [], nodes: result.filters?.nodes || [] }
    applications.value = result.apps || []
    projects.value = result.projectList || []
  } else if (filtersResource.error.value) filterError.value = filtersResource.error.value.message || '读取日志筛选项失败'
}

async function install() {
  installing.value = true
  installError.value = ''
  try { status.value = await installLogging(installForm.value); await refresh() } catch (e) { installError.value = e.message || '启用日志采集失败' } finally { installing.value = false }
}

async function queryLogs() {
  querying.value = true
  queryError.value = ''
  try {
    const payload = buildLogQueryPayload()
    if (!payload) return
    const result = await queryResource.refresh(payload)
    if (result === undefined) { if (queryResource.error.value) queryError.value = queryResource.error.value.message || '查询日志失败'; return }
    lines.value = result?.lines || []
    hasMore.value = Boolean(result?.has_more)
    queried.value = true
  } catch (e) { queryError.value = e.message || '查询日志失败' } finally { querying.value = false }
}

function buildLogQueryPayload() {
  const payload = { ...queryForm.value }
  if (payload.range !== 'custom') {
    payload.start_time = ''
    payload.end_time = ''
    return payload
  }
  const start = new Date(payload.start_time)
  const end = new Date(payload.end_time)
  if (Number.isNaN(start.valueOf()) || Number.isNaN(end.valueOf())) {
    queryError.value = '请选择有效的开始和结束时间'
    return null
  }
  payload.start_time = start.toISOString()
  payload.end_time = end.toISOString()
  return payload
}

function openSettings() { settingsForm.value = { retention_days: status.value?.retention_days || 14 }; settingsOpen.value = true }
async function saveSettings() {
  saving.value = true
  settingsError.value = ''
  try { status.value = await saveLoggingConfig({ node_name: status.value.node_name, retention_days: settingsForm.value.retention_days }); settingsOpen.value = false; await refresh() } catch (e) { settingsError.value = e.message || '保存日志设置失败' } finally { saving.value = false }
}
async function uninstall() {
  uninstalling.value = true
  uninstallError.value = ''
  try { await uninstallLogging(); confirmUninstall.value = false; lines.value = []; queried.value = false; await refresh() } catch (e) { uninstallError.value = e.message || '卸载日志采集失败' } finally { uninstalling.value = false }
}
function lineContext(labels = {}) { return [labels.namespace, labels.pod, labels.container].filter(Boolean).join(' / ') || '容器日志' }
</script>

<style scoped>
.status-copy,.form-hint,.metric small,.logging-section-heading p,.drawer-header p,.confirm-copy{margin:5px 0 0;color:var(--text-secondary);font-size:12px}.install-form{margin-top:var(--space-20)}.status-actions{justify-content:flex-start;margin-top:var(--space-16)}.logging-summary{grid-template-columns:repeat(4,minmax(0,1fr))}.metric{display:grid;min-width:0;gap:5px;padding:14px;border:1px solid var(--border-muted);border-radius:var(--radius-control);background:var(--surface-subtle)}.metric>span{color:var(--text-secondary);font-size:11px}.metric strong{min-width:0;font-size:18px}.metric small{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.logging-section-heading{display:flex;align-items:center;justify-content:space-between;gap:var(--space-16)}.logging-section-heading h2,.drawer-header h2{margin:0;color:var(--text-primary);font-size:16px}.icon-actions{display:flex;gap:6px}.logging-filter-grid{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:var(--space-12)}.logging-filter-grid .form-group{margin:0}.logging-content-filter{margin-top:var(--space-20);padding-top:var(--space-16);border-top:1px solid var(--border-muted)}.logging-content-filter>.form-label{display:block;margin-bottom:6px}.logging-search-row{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:8px}.logging-results{padding:0}.logging-results-header{display:flex;justify-content:space-between;gap:12px;padding:12px 14px;border-bottom:1px solid var(--border-muted);color:var(--text-secondary);font-size:12px}.logging-results-header small{color:var(--warning)}.logging-lines{max-height:560px;overflow:auto}.logging-line{display:grid;grid-template-columns:170px minmax(0,1fr);gap:12px;padding:11px 14px;border-bottom:1px solid var(--border-muted)}.logging-line:last-child{border-bottom:0}.logging-line time{color:var(--text-muted);font:11px/1.5 var(--font-mono)}.logging-line-copy{display:grid;min-width:0;gap:5px}.logging-labels{overflow:hidden;color:var(--text-secondary);font:11px/1.3 var(--font-mono);text-overflow:ellipsis;white-space:nowrap}.logging-line pre{margin:0;overflow:auto;color:var(--text-primary);font:12px/1.55 var(--font-mono);white-space:pre-wrap;overflow-wrap:anywhere}.logging-wait .empty-state{min-height:150px}.empty-inline{padding:22px;color:var(--text-muted);font-size:12px}.logging-settings-overlay{z-index:2000;align-items:center;justify-content:center}.logging-settings-modal{width:min(560px,calc(100vw - 32px));max-height:calc(100dvh - 32px)}.drawer-header{display:flex;align-items:flex-start;justify-content:space-between;gap:12px}.drawer-section{padding:var(--space-20) 0;border-bottom:1px solid var(--border-muted)}.settings-field{display:grid;gap:3px;padding-bottom:var(--space-16)}.settings-field>span{color:var(--text-secondary);font-size:11px}.settings-field strong{font-size:13px}.settings-field small{color:var(--text-muted);font:10px/1.4 var(--font-mono)}.metric-code{font-family:var(--font-mono)}.settings-retention{display:grid;gap:6px;margin:0}.drawer-footer{display:flex;justify-content:flex-end;gap:8px;padding-top:var(--space-20)}.logging-uninstall-modal{width:min(440px,calc(100vw - 32px))}@media(max-width:900px){.logging-filter-grid{grid-template-columns:repeat(2,minmax(0,1fr))}}@media(max-width:640px){.logging-summary,.logging-filter-grid{grid-template-columns:1fr}.logging-section-heading{align-items:flex-start;flex-direction:column}.logging-search-row{grid-template-columns:1fr}.logging-search-row .btn{width:100%}.logging-line{grid-template-columns:1fr;gap:5px}.logging-results-header{align-items:flex-start;flex-direction:column}}
</style>
