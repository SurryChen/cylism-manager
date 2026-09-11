<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">工作负载</h1>
      <button class="icon-button" title="刷新工作负载" aria-label="刷新工作负载" :disabled="loading" @click="fetchData"><RefreshCw :size="16" :class="{ 'is-spinning': loading }" /></button>
    </div>
    <div v-if="error" class="k8s-banner k8s-banner-warn" style="margin-bottom:var(--space-16)">⚠ {{ error }}</div>
    <div v-if="detailError" class="k8s-banner k8s-banner-warn" style="margin-bottom:var(--space-16)">⚠ {{ detailError }}</div>

    <nav class="resource-switcher section-gap" aria-label="工作负载资源类型">
      <button :class="['resource-tab', { 'resource-tab-active': activeTab === 'pods' }]" :aria-selected="activeTab === 'pods'" @click="selectTab('pods')"><Box :size="16" /><span>Pods<small>实例</small></span><strong>{{ pods.length }}</strong></button>
      <button :class="['resource-tab', { 'resource-tab-active': activeTab === 'deployments' }]" :aria-selected="activeTab === 'deployments'" @click="selectTab('deployments')"><Layers3 :size="16" /><span>Deployments<small>无状态服务</small></span><strong>{{ deployments.length }}</strong></button>
      <button :class="['resource-tab', { 'resource-tab-active': activeTab === 'statefulsets' }]" :aria-selected="activeTab === 'statefulsets'" @click="selectTab('statefulsets')"><Database :size="16" /><span>StatefulSets<small>有状态服务</small></span><strong>{{ statefulsets.length }}</strong></button>
      <button :class="['resource-tab', { 'resource-tab-active': activeTab === 'daemonsets' }]" :aria-selected="activeTab === 'daemonsets'" @click="selectTab('daemonsets')"><Network :size="16" /><span>DaemonSets<small>节点服务</small></span><strong>{{ daemonsets.length }}</strong></button>
    </nav>

    <!-- Deployments -->
    <div v-if="activeTab === 'deployments'" class="card">
      <div v-if="deployments.length === 0" class="empty-state">
        <span class="empty-icon">⬡</span><span class="empty-text">暂无 Deployment</span>
      </div>
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead><tr><th>名称</th><th>命名空间</th><th>副本</th><th>镜像</th><th>挂载</th><th>CPU</th><th>内存</th><th>年龄</th><th></th></tr></thead>
          <tbody>
            <template v-for="d in safeDeployments" :key="d.namespace + '/' + d.name">
              <tr @click="toggleDeployExpand(d)" class="clickable">
                <td class="cell-primary">{{ d.name }}</td><td>{{ d.namespace }}</td>
                <td><span :class="d.ready === d.replicas ? 'status-success' : 'status-warning'">{{ d.ready }}/{{ d.replicas }}</span></td>
                <td>{{ d.images?.[0] || '-' }}</td>
                <td><div v-if="d.volume_mounts?.length" class="volume-mount-list"><span v-for="mount in d.volume_mounts" :key="`${mount.claim_name}-${mount.mount_path}`" class="volume-mount">{{ mount.claim_name }} -> {{ mount.mount_path }}</span></div><span v-else>-</span></td>
                <td>{{ d.cpu || '-' }}</td><td>{{ d.memory || '-' }}</td><td>{{ d.age }}</td>
                <td>
                  <div class="btn-group action-cell" @click.stop>
                    <button class="btn btn-sm" @click="openScaleDialog(d)">扩缩</button>
                    <button class="btn btn-sm" @click="openImageDialog(d)">镜像</button>
                    <button class="btn btn-sm" @click="openRollbackDialog(d)">回滚</button>
                  </div>
                </td>
              </tr>
              <!-- expanded pods -->
              <template v-if="expandedDeploy === d.namespace + '/' + d.name">
                <tr v-for="pod in (deployPods[d.namespace + '/' + d.name] || [])" :key="pod.name" class="pod-row">
                  <td colspan="9">
                    <div class="pod-subrow">↳ {{ pod.name }} <span :class="pod.status === 'Running' ? 'badge badge-online' : 'badge badge-offline'">{{ pod.status }}</span> {{ displayServerName(pod.node) }} · 重启 {{ pod.restarts }} · {{ pod.ip }}</div>
                  </td>
                </tr>
              </template>
            </template>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Pods -->
    <div v-if="activeTab === 'pods'" class="card">
      <div class="pod-list-header">
        <div><h2>Pod 实例</h2><p>查看实例运行状态、所在服务器与重启情况</p></div>
        <div class="pod-list-tools">
          <label class="pod-search"><Search :size="16" /><span class="sr-only">搜索 Pod 名称</span><input v-model.trim="podNameFilter" placeholder="搜索 Pod 名称" aria-label="搜索 Pod 名称" /></label>
          <button :class="['btn btn-sm pod-filter-trigger', { 'is-active': podFiltersOpen || hasPodFilters }]" :aria-expanded="podFiltersOpen" @click="podFiltersOpen = !podFiltersOpen"><Filter :size="15" />筛选<span v-if="activePodFilterCount" class="filter-count">{{ activePodFilterCount }}</span></button>
          <button v-if="hasPodFilters" class="icon-button" title="重置 Pod 筛选" aria-label="重置 Pod 筛选" @click="resetPodFilters"><RotateCcw :size="15" /></button>
        </div>
      </div>
      <div v-if="podFiltersOpen" class="pod-filter-panel">
        <div class="filter-control">
          <label class="form-label" for="pod-filter-namespace">命名空间</label>
          <select id="pod-filter-namespace" v-model="podNamespaceFilter" class="form-select pod-filter-namespace"><option value="">全部命名空间</option><option v-for="namespace in podNamespaces" :key="namespace" :value="namespace">{{ namespace }}</option></select>
        </div>
        <div class="filter-control">
          <label class="form-label" for="pod-filter-node">所在服务器</label>
          <select id="pod-filter-node" v-model="podNodeFilter" class="form-select pod-filter-node"><option value="">全部服务器</option><option value="__unscheduled__">未调度</option><option v-for="node in podNodes" :key="node.value" :value="node.value">{{ node.label }}</option></select>
        </div>
        <div class="filter-control">
          <label class="form-label" for="pod-filter-status">状态</label>
          <select id="pod-filter-status" v-model="podStatusFilter" class="form-select pod-filter-status"><option value="">全部状态</option><option v-for="status in podStatuses" :key="status" :value="status">{{ status }}</option></select>
        </div>
        <label class="checkbox-label pod-restarts-filter"><input v-model="podRestartsOnly" type="checkbox" /> 仅显示已重启</label>
      </div>
      <div v-if="hasPodFilters" class="active-filters">
        <span>已筛选</span>
        <button v-if="podNamespaceFilter" class="filter-chip" @click="clearPodFilter('namespace')">命名空间: {{ podNamespaceFilter }}<X :size="13" /></button>
        <button v-if="podNodeFilter" class="filter-chip" @click="clearPodFilter('node')">服务器: {{ podNodeFilter === '__unscheduled__' ? '未调度' : displayServerName(podNodeFilter) }}<X :size="13" /></button>
        <button v-if="podStatusFilter" class="filter-chip" @click="clearPodFilter('status')">状态: {{ podStatusFilter }}<X :size="13" /></button>
        <button v-if="podNameFilter" class="filter-chip" @click="clearPodFilter('name')">名称: {{ podNameFilter }}<X :size="13" /></button>
        <button v-if="podRestartsOnly" class="filter-chip" @click="clearPodFilter('restarts')">已重启<X :size="13" /></button>
      </div>
      <div v-if="pods.length > 0" class="pod-result-summary">显示 <strong>{{ safePods.length }}</strong> / {{ pods.length }} 个 Pod</div>
      <div v-if="pods.length === 0" class="empty-state">
        <span class="empty-icon">⬡</span><span class="empty-text">暂无 Pod</span>
      </div>
      <div v-else-if="safePods.length === 0" class="empty-state pod-filter-empty">
        <span class="empty-icon">⌕</span><span class="empty-text">没有匹配筛选条件的 Pod</span>
      </div>
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead><tr><th>实例</th><th>命名空间</th><th>状态</th><th>所在服务器</th><th>重启</th><th>年龄</th><th></th></tr></thead>
          <tbody>
            <tr v-for="pod in safePods" :key="pod.namespace + '/' + pod.name" class="pod-list-row">
              <td class="cell-primary">{{ pod.name }}<small>{{ pod.ip || '未分配 Pod IP' }}</small></td>
              <td>{{ pod.namespace }}</td>
              <td><span :class="['badge', podStatusClass(pod.status)]">{{ pod.status }}</span></td>
              <td>
                <span class="server-name">{{ displayServerName(pod.node) }}</span>
                <small v-if="hasMappedServer(pod.node)" class="node-name">{{ pod.node }}</small>
              </td>
              <td><span :class="Number(pod.restarts || 0) > 0 ? 'restart-warning' : ''">{{ pod.restarts }}</span></td>
              <td>{{ pod.age || '-' }}</td>
              <td class="action-cell"><button class="icon-button pod-terminal-action" :disabled="!canOpenPodTerminal(pod)" :title="canOpenPodTerminal(pod) ? '打开容器终端' : '仅运行中且包含普通容器的 Pod 可打开终端'" :aria-label="`打开 ${pod.name} 的容器终端`" @click="openPodTerminal(pod)"><SquareTerminal :size="16" /></button></td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <PodTerminal v-if="terminalPod" :pod="terminalPod" @close="terminalPod = null" />

    <!-- StatefulSets -->
    <div v-if="activeTab === 'statefulsets'" class="card">
      <div v-if="statefulsets.length === 0" class="empty-state">
        <span class="empty-icon">⬡</span><span class="empty-text">暂无 StatefulSet</span>
      </div>
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead><tr><th>名称</th><th>命名空间</th><th>副本</th><th>镜像</th><th>挂载</th><th>年龄</th><th></th></tr></thead>
          <tbody>
            <tr v-for="s in safeStatefulsets" :key="s.namespace + '/' + s.name">
              <td class="cell-primary">{{ s.name }}</td><td>{{ s.namespace }}</td>
              <td><span :class="s.ready === s.replicas ? 'status-success' : 'status-warning'">{{ s.ready }}/{{ s.replicas }}</span></td>
              <td>{{ s.images?.[0] || '-' }}</td>
              <td>
                <div v-if="s.volume_mounts?.length" class="volume-mount-list">
                  <span v-for="mount in s.volume_mounts" :key="`${mount.type}-${mount.claim_name}-${mount.mount_path}`" class="volume-mount">
                    {{ mount.claim_name }}<template v-if="mount.type === 'volume_claim_template'"> (卷声明模板)</template> -> {{ mount.mount_path }}
                  </span>
                </div>
                <span v-else>-</span>
              </td>
              <td>{{ s.age }}</td>
              <td><div class="btn-group action-cell"><button class="btn btn-sm" @click="openStsScaleDialog(s)">扩缩</button></div></td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- DaemonSets -->
    <div v-if="activeTab === 'daemonsets'" class="card">
      <div v-if="daemonsets.length === 0" class="empty-state">
        <span class="empty-icon">⬡</span><span class="empty-text">暂无 DaemonSet</span>
      </div>
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead><tr><th>名称</th><th>命名空间</th><th>就绪/期望</th><th>镜像</th><th>节点选择器</th><th>年龄</th></tr></thead>
          <tbody>
            <tr v-for="d in safeDaemonsets" :key="d.namespace + '/' + d.name">
              <td class="cell-primary">{{ d.name }}</td><td>{{ d.namespace }}</td>
              <td><span :class="d.ready === d.desired ? 'status-success' : 'status-warning'">{{ d.ready }}/{{ d.desired }}</span></td>
              <td>{{ d.images?.[0] || '-' }}</td><td>{{ Object.entries(d.node_selector || {}).map(([k,v]) => `${k}=${v}`).join(', ') || '-' }}</td><td>{{ d.age }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Scale Dialog -->
    <div v-if="scaleDialog" class="modal-overlay" @click.self="scaleDialog = null">
      <div class="modal"><div class="modal-body">
        <h3>扩缩容: {{ scaleDialog.name }}</h3>
        <p class="modal-copy">当前: {{ scaleDialog.current }} → <input type="number" v-model="scaleDialog.replicas" min="0" class="form-input" style="width:80px;display:inline" /></p>
        <p v-if="scaleError" class="form-error" role="alert">{{ scaleError }}</p>
        <div class="btn-group" style="margin-top:var(--space-16)"><button class="btn btn-primary" :disabled="scaleSubmitting" @click="doScale">{{ scaleSubmitting ? '提交中...' : '确认' }}</button><button class="btn" :disabled="scaleSubmitting" @click="scaleDialog = null">取消</button></div>
      </div></div>
    </div>

    <!-- Image Dialog -->
    <div v-if="imageDialog" class="modal-overlay" @click.self="imageDialog = null">
      <div class="modal"><div class="modal-body">
        <h3>更新镜像: {{ imageDialog.name }}</h3>
        <p class="modal-copy">容器: <select v-model="imageDialog.container" class="form-select" style="width:auto;display:inline"><option v-for="img in imageDialog.images" :key="img" :value="img.split(':')[0]">{{ img }}</option></select></p>
        <p class="modal-copy">新镜像: <input v-model="imageDialog.newImage" class="form-input" style="width:200px;display:inline" placeholder="nginx:1.25" /></p>
        <p v-if="imageError" class="form-error" role="alert">{{ imageError }}</p>
        <div class="btn-group" style="margin-top:var(--space-16)"><button class="btn btn-primary" :disabled="imageSubmitting" @click="doUpdateImage">{{ imageSubmitting ? '提交中...' : '确认' }}</button><button class="btn" :disabled="imageSubmitting" @click="imageDialog = null">取消</button></div>
      </div></div>
    </div>

    <!-- Rollback Dialog -->
    <div v-if="rollbackDialog" class="modal-overlay" @click.self="rollbackDialog = null">
      <div class="modal"><div class="modal-body">
        <h3>回滚: {{ rollbackDialog.name }}</h3>
        <div class="table-wrap" style="margin:var(--space-12) 0"><table class="data-table">
          <thead><tr><th>版本</th><th>镜像</th><th>时间</th><th></th></tr></thead>
          <tbody><tr v-for="r in rollbackDialog.revisions" :key="r.revision"><td>{{ r.revision }}</td><td>{{ r.image }}</td><td>{{ r.age }}</td><td><button class="btn btn-sm" :disabled="rollbackSubmitting" @click="doRollback(r.revision)">{{ rollbackSubmitting ? '提交中...' : '回滚到此' }}</button></td></tr></tbody>
        </table></div>
        <p v-if="rollbackError" class="form-error" role="alert">{{ rollbackError }}</p>
        <button class="btn" :disabled="rollbackSubmitting" @click="rollbackDialog = null">取消</button>
      </div></div>
    </div>
  </div>
</template>

<script setup>
import { computed, ref, onMounted, onErrorCaptured } from 'vue'
import { Box, Database, Filter, Layers3, Network, RefreshCw, RotateCcw, Search, SquareTerminal, X } from 'lucide-vue-next'
import { getWorkloadDaemonSets, getWorkloadDeploymentPods, getWorkloadDeploymentRevisions, getWorkloadDeployments, getWorkloadPods, getWorkloadServers, getWorkloadStatefulSets, rollbackWorkload, scaleWorkload, updateWorkloadImage } from '../../api/kubernetes.js'
import { useAsyncResource } from '../../composables/useAsyncResource.js'
import PodTerminal from './PodTerminal.vue'

const activeTab = ref('pods')
const deployments = ref([])
const statefulsets = ref([])
const daemonsets = ref([])
const pods = ref([])
const servers = ref([])
const inventoryResource = useAsyncResource(async ({ signal }) => Promise.allSettled([
  getWorkloadDeployments({ signal }),
  getWorkloadStatefulSets({ signal }),
  getWorkloadDaemonSets({ signal }),
  getWorkloadPods({ signal }),
  getWorkloadServers({ signal }),
]), null)
const deploymentPodsResource = useAsyncResource(({ signal }, namespace, name) => getWorkloadDeploymentPods(namespace, name, { signal }), null)
const revisionsResource = useAsyncResource(({ signal }, namespace, name) => getWorkloadDeploymentRevisions(namespace, name, { signal }), null)
const loading = inventoryResource.loading
const error = ref('')
const detailError = ref('')
const scaleError = ref('')
const imageError = ref('')
const rollbackError = ref('')
const scaleSubmitting = ref(false)
const imageSubmitting = ref(false)
const rollbackSubmitting = ref(false)
const expandedDeploy = ref('')
const deployPods = ref({})
const podNamespaceFilter = ref('')
const podNodeFilter = ref('')
const podStatusFilter = ref('')
const podNameFilter = ref('')
const podRestartsOnly = ref(false)
const podFiltersOpen = ref(false)
const terminalPod = ref(null)

const scaleDialog = ref(null)
const imageDialog = ref(null)
const rollbackDialog = ref(null)

const safeDeployments = computed(() => (deployments.value || []).filter(d => d != null))
const safeStatefulsets = computed(() => (statefulsets.value || []).filter(s => s != null))
const safeDaemonsets = computed(() => (daemonsets.value || []).filter(d => d != null))
const serverNamesByNode = computed(() => new Map(
  (servers.value || [])
    .filter(server => server?.k8s_node_name && server?.name)
    .map(server => [server.k8s_node_name.toLowerCase(), server.name]),
))
const podNamespaces = computed(() => [...new Set(
  (pods.value || []).map(pod => pod?.namespace).filter(Boolean),
)].sort())
const podNodes = computed(() => [...new Set(
  (pods.value || []).map(pod => pod?.node).filter(Boolean),
)].map(node => ({ value: node, label: displayServerName(node) }))
  .sort((a, b) => a.label.localeCompare(b.label, 'zh-CN')))
const podStatuses = computed(() => [...new Set(
  (pods.value || []).map(pod => pod?.status).filter(Boolean),
)].sort())
const safePods = computed(() => {
  const query = podNameFilter.value.toLowerCase()
  return (pods.value || []).filter(pod => {
    if (!pod) return false
    if (podNamespaceFilter.value && pod.namespace !== podNamespaceFilter.value) return false
    if (podNodeFilter.value === '__unscheduled__' && pod.node) return false
    if (podNodeFilter.value && podNodeFilter.value !== '__unscheduled__' && pod.node !== podNodeFilter.value) return false
    if (podStatusFilter.value && pod.status !== podStatusFilter.value) return false
    if (podRestartsOnly.value && Number(pod.restarts || 0) === 0) return false
    return !query || pod.name?.toLowerCase().includes(query)
  })
})
const hasPodFilters = computed(() => Boolean(
  podNamespaceFilter.value || podNodeFilter.value || podStatusFilter.value || podNameFilter.value || podRestartsOnly.value,
))
const activePodFilterCount = computed(() => [podNamespaceFilter.value, podNodeFilter.value, podStatusFilter.value, podNameFilter.value, podRestartsOnly.value].filter(Boolean).length)

// 组件级错误边界：捕获渲染异常，避免白屏
onErrorCaptured((err, instance, info) => {
  console.error('[Workloads] 渲染异常:', err, info)
  error.value = `页面渲染异常: ${err.message || err}`
  return false // 阻止向上冒泡
})

async function fetchData() {
  error.value = ''
  const result = await inventoryResource.refresh()
  if (!result) return
  const [deps, sts, ds, podList, serverList] = result
    if (deps.status === 'fulfilled') deployments.value = deps.value || []
    if (sts.status === 'fulfilled') statefulsets.value = sts.value || []
    if (ds.status === 'fulfilled') daemonsets.value = ds.value || []
    if (podList.status === 'fulfilled') pods.value = podList.value || []
    if (serverList.status === 'fulfilled') servers.value = serverList.value || []
    const failed = [deps, sts, ds, podList, serverList].filter(r => r.status === 'rejected')
  if (failed.length > 0) error.value = failed.map(r => r.reason?.message || '未知错误').join('\n')
}
onMounted(fetchData)

function displayServerName(nodeName) {
  if (!nodeName) return '-'
  return serverNamesByNode.value.get(nodeName.toLowerCase()) || nodeName
}

function hasMappedServer(nodeName) {
  return Boolean(nodeName && serverNamesByNode.value.has(nodeName.toLowerCase()))
}

function podStatusClass(status) {
  if (status === 'Running') return 'badge-online'
  if (status === 'Pending') return 'badge-warn'
  if (status === 'Failed' || status === 'Unknown') return 'badge-danger'
  return 'badge-offline'
}

function selectTab(tab) {
  activeTab.value = tab
  if (tab !== 'pods') podFiltersOpen.value = false
}

function resetPodFilters() {
  podNamespaceFilter.value = ''
  podNodeFilter.value = ''
  podStatusFilter.value = ''
  podNameFilter.value = ''
  podRestartsOnly.value = false
}

function clearPodFilter(filter) {
  if (filter === 'namespace') podNamespaceFilter.value = ''
  if (filter === 'node') podNodeFilter.value = ''
  if (filter === 'status') podStatusFilter.value = ''
  if (filter === 'name') podNameFilter.value = ''
  if (filter === 'restarts') podRestartsOnly.value = false
}

function canOpenPodTerminal(pod) {
  return pod.status === 'Running' && Array.isArray(pod.containers) && pod.containers.length > 0
}

function openPodTerminal(pod) {
  if (canOpenPodTerminal(pod)) terminalPod.value = pod
}

async function toggleDeployExpand(d) {
  const key = d.namespace + '/' + d.name
  if (expandedDeploy.value === key) { expandedDeploy.value = ''; return }
  expandedDeploy.value = key
  if (!deployPods.value[key]) {
    try {
      const result = await deploymentPodsResource.refresh(d.namespace, d.name)
      if (result !== undefined) {
        deployPods.value[key] = result || []
        detailError.value = ''
      } else if (deploymentPodsResource.error.value) {
        detailError.value = deploymentPodsResource.error.value.message || '读取工作负载 Pod 失败'
      }
    } catch(e) { detailError.value = e.message || '读取工作负载 Pod 失败'; deployPods.value[key] = [] }
  }
}

function openScaleDialog(d) {
  scaleError.value = ''
  scaleDialog.value = {
    namespace: d.namespace, name: d.name,
    current: d.replicas, replicas: d.replicas,
    kind: 'deployment'
  }
}
async function doScale() {
  const d = scaleDialog.value
  if (!d || scaleSubmitting.value) return
  scaleError.value = ''
  scaleSubmitting.value = true
  try {
    await scaleWorkload(d.kind, d.namespace, d.name, Number(d.replicas))
    scaleDialog.value = null
    await fetchData()
  } catch(e) { scaleError.value = e.message || '扩缩容失败' }
  finally { scaleSubmitting.value = false }
}

function openImageDialog(d) {
  imageError.value = ''
  imageDialog.value = {
    namespace: d.namespace, name: d.name,
    images: d.images || [], container: (d.images?.[0] || '').split(':')[0] || '',
    newImage: ''
  }
}
async function doUpdateImage() {
  const d = imageDialog.value
  if (!d || imageSubmitting.value) return
  imageError.value = ''
  imageSubmitting.value = true
  try {
    await updateWorkloadImage(d.namespace, d.name, { container: d.container, image: d.newImage })
    imageDialog.value = null
    await fetchData()
  } catch (e) { imageError.value = e.message || '更新镜像失败' }
  finally { imageSubmitting.value = false }
}

async function openRollbackDialog(d) {
  detailError.value = ''
  try {
    const result = await revisionsResource.refresh(d.namespace, d.name)
    if (result) rollbackDialog.value = { namespace: d.namespace, name: d.name, revisions: result || [] }
    else if (revisionsResource.error.value) detailError.value = revisionsResource.error.value.message || '读取回滚版本失败'
  } catch (e) { detailError.value = e.message || '读取回滚版本失败' }
}

async function doRollback(revision) {
  const d = rollbackDialog.value
  if (!d || rollbackSubmitting.value) return
  rollbackError.value = ''
  rollbackSubmitting.value = true
  try {
    await rollbackWorkload(d.namespace, d.name, revision)
    rollbackDialog.value = null
    await fetchData()
  } catch(e) { rollbackError.value = e.message || '回滚失败' }
  finally { rollbackSubmitting.value = false }
}

function openStsScaleDialog(s) {
  scaleError.value = ''
  scaleDialog.value = {
    namespace: s.namespace, name: s.name,
    current: s.replicas, replicas: s.replicas,
    kind: 'statefulset'
  }
}
</script>

<style scoped>
.page-header { align-items: center; }
.resource-switcher { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 8px; padding-bottom: var(--space-16); border-bottom: 1px solid var(--border-muted); }
.resource-tab { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; gap: 10px; min-height: 64px; padding: 10px 12px; border: 1px solid transparent; border-radius: var(--radius-control); background: transparent; color: var(--text-secondary); text-align: left; cursor: pointer; transition: background .18s ease, border-color .18s ease, color .18s ease; }
.resource-tab:hover { background: var(--surface-subtle); color: var(--text-primary); }.resource-tab-active { border-color: var(--border); background: var(--surface-raised); color: var(--action-primary); box-shadow: var(--shadow-soft); }.resource-tab span { display: grid; gap: 3px; min-width: 0; font-size: 12px; font-weight: 700; }.resource-tab small { overflow: hidden; color: var(--text-muted); font-size: 10px; font-weight: 500; text-overflow: ellipsis; white-space: nowrap; }.resource-tab strong { display: grid; min-width: 24px; min-height: 24px; place-items: center; border-radius: 50%; background: var(--surface-subtle); color: var(--text-secondary); font: 11px/1 var(--font-mono); }.resource-tab-active strong { background: var(--action-primary); color: var(--action-contrast); }
.clickable { cursor: pointer; }
.pod-row td { padding: 4px 12px; border-bottom: 0; background: var(--surface-subtle); }
.pod-subrow { display: flex; align-items: center; gap: 8px; padding: 6px 0; font-size: 11px; color: var(--text-secondary); }
.pod-subrow .badge { font-size: 9px; }
.node-name { display: block; margin-top: 3px; color: var(--text-muted); font-size: 10px; font-family: var(--font-mono); }
.pod-list-header { display: flex; align-items: center; justify-content: space-between; gap: var(--space-16); margin-bottom: var(--space-16); }.pod-list-header h2 { margin: 0; color: var(--text-primary); font-size: 15px; }.pod-list-header p { margin: 5px 0 0; color: var(--text-secondary); font-size: 12px; }.pod-list-tools { display: flex; align-items: center; gap: 8px; }.pod-search { display: flex; width: min(260px, 28vw); min-width: 180px; align-items: center; gap: 8px; min-height: 34px; padding: 0 10px; border: 1px solid var(--border-muted); border-radius: var(--radius-control); background: var(--surface-input); color: var(--text-muted); }.pod-search:focus-within { border-color: var(--focus); outline: 2px solid var(--focus); outline-offset: 2px; }.pod-search input { width: 100%; min-width: 0; border: 0; outline: 0; background: transparent; color: var(--text-primary); font: inherit; font-size: 12px; }.pod-search input::placeholder { color: var(--text-muted); }.pod-filter-trigger { gap: 5px; }.pod-filter-trigger.is-active { border-color: var(--action-primary); color: var(--action-primary); }.filter-count { display: grid; min-width: 16px; height: 16px; place-items: center; border-radius: 50%; background: var(--action-primary); color: var(--action-contrast); font: 9px/1 var(--font-mono); }
.pod-filter-panel { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)) auto; align-items: end; gap: var(--space-12); margin-bottom: var(--space-12); padding: var(--space-16); border: 1px solid var(--border-muted); border-radius: var(--radius-control); background: var(--surface-subtle); }.pod-filter-panel .form-label { margin-bottom: 6px; }.pod-restarts-filter { min-height: 38px; white-space: nowrap; }.active-filters { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; margin-bottom: var(--space-12); color: var(--text-muted); font-size: 11px; }.filter-chip { display: inline-flex; align-items: center; gap: 4px; padding: 4px 6px 4px 8px; border: 1px solid var(--border-muted); border-radius: var(--radius-control); background: var(--surface-subtle); color: var(--text-secondary); font-size: 11px; cursor: pointer; }.filter-chip:hover { border-color: var(--action-primary); color: var(--action-primary); }.pod-result-summary { margin-bottom: var(--space-8); color: var(--text-muted); font-size: 11px; }.pod-result-summary strong { color: var(--text-secondary); font-family: var(--font-mono); }
.pod-filter-empty { min-height: 150px; }
.pod-list-row .cell-primary small { display: block; margin-top: 3px; color: var(--text-muted); font: 10px/1 var(--font-mono); }.server-name { color: var(--text-primary); font-weight: 600; }.restart-warning { color: var(--warning); font-weight: 700; }.pod-terminal-action { width: 30px; height: 30px; }.is-spinning { animation: spin .8s linear infinite; }@keyframes spin { to { transform: rotate(360deg); } }.sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0, 0, 0, 0); white-space: nowrap; }
.volume-mount-list { display: grid; gap: 4px; min-width: 180px; max-width: 300px; }.volume-mount { overflow-wrap: anywhere; color: var(--text-secondary); font: 11px/1.45 var(--font-mono); }
.modal-overlay { position: fixed; z-index: 200; inset: 0; display: flex; align-items: center; justify-content: center; background: var(--overlay); backdrop-filter: blur(3px); }
.modal { min-width: 380px; max-width: 520px; padding: var(--space-20); border: 1px solid var(--border); border-radius: var(--radius-panel); background: var(--surface-raised); box-shadow: var(--shadow); }
.modal-body h3 { margin: 0 0 var(--space-12); color: var(--text-primary); font-size: 15px; }
@media (max-width: 780px) { .resource-switcher { grid-template-columns: repeat(2, minmax(0, 1fr)); }.pod-list-header { align-items: stretch; flex-direction: column; }.pod-list-tools { flex-wrap: wrap; }.pod-search { width: 100%; max-width: none; }.pod-filter-panel { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 640px) { .resource-tab { min-height: 54px; gap: 7px; padding: 8px; }.resource-tab small { display: none; }.pod-filter-panel { grid-template-columns: 1fr; }.pod-restarts-filter { min-height: auto; } }
</style>
