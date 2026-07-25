<template>
  <div>
    <div class="page-header">
    <div v-if="loading" class="k8s-banner" style="margin-bottom:var(--space-16)">加载中...</div>
    <div v-if="error" class="k8s-banner k8s-banner-warn" style="margin-bottom:var(--space-16)">⚠ {{ error }}</div>
      <h1 class="page-title">工作负载</h1>
    </div>

    <div class="card section-gap">
      <div class="table-tabs">
        <button :class="['tab-btn', { 'tab-active': activeTab === 'deployments' }]" @click="activeTab = 'deployments'">Deployments</button>
        <button :class="['tab-btn', { 'tab-active': activeTab === 'statefulsets' }]" @click="activeTab = 'statefulsets'">StatefulSets</button>
        <button :class="['tab-btn', { 'tab-active': activeTab === 'daemonsets' }]" @click="activeTab = 'daemonsets'">DaemonSets</button>
      </div>
    </div>

    <!-- Deployments -->
    <div v-if="activeTab === 'deployments'" class="card">
      <div v-if="deployments.length === 0" class="empty-state">
        <span class="empty-icon">⬡</span><span class="empty-text">暂无 Deployment</span>
      </div>
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead><tr><th>名称</th><th>命名空间</th><th>副本</th><th>镜像</th><th>CPU</th><th>内存</th><th>年龄</th><th></th></tr></thead>
          <tbody>
            <tr v-for="d in deployments" :key="d.namespace + '/' + d.name" @click="toggleDeployExpand(d)" class="clickable">
              <td class="cell-primary">{{ d.name }}</td><td>{{ d.namespace }}</td>
              <td><span :class="d.ready === d.replicas ? 'status-success' : 'status-warning'">{{ d.ready }}/{{ d.replicas }}</span></td>
              <td>{{ d.images?.[0] || '-' }}</td><td>{{ d.cpu || '-' }}</td><td>{{ d.memory || '-' }}</td><td>{{ d.age }}</td>
              <td>
                <div class="btn-group action-cell" @click.stop>
                  <button class="btn btn-sm" @click="openScaleDialog(d)">扩缩</button>
                  <button class="btn btn-sm" @click="openImageDialog(d)">镜像</button>
                  <button class="btn btn-sm" @click="openRollbackDialog(d)">回滚</button>
                </div>
              </td>
            </tr>
            <!-- expanded pods -->
            <tr v-if="expandedDeploy === d.namespace + '/' + d.name" v-for="pod in deployPods[d.namespace + '/' + d.name]" :key="pod.name" class="pod-row">
              <td colspan="8">
                <div class="pod-subrow">↳ {{ pod.name }} <span :class="pod.status === 'Running' ? 'badge badge-online' : 'badge badge-offline'">{{ pod.status }}</span> {{ pod.node }} · 重启 {{ pod.restarts }} · {{ pod.ip }}</div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- StatefulSets -->
    <div v-if="activeTab === 'statefulsets'" class="card">
      <div v-if="statefulsets.length === 0" class="empty-state">
        <span class="empty-icon">⬡</span><span class="empty-text">暂无 StatefulSet</span>
      </div>
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead><tr><th>名称</th><th>命名空间</th><th>副本</th><th>镜像</th><th>年龄</th><th></th></tr></thead>
          <tbody>
            <tr v-for="s in statefulsets" :key="s.namespace + '/' + s.name">
              <td class="cell-primary">{{ s.name }}</td><td>{{ s.namespace }}</td>
              <td><span :class="s.ready === s.replicas ? 'status-success' : 'status-warning'">{{ s.ready }}/{{ s.replicas }}</span></td>
              <td>{{ s.images?.[0] || '-' }}</td><td>{{ s.age }}</td>
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
            <tr v-for="d in daemonsets" :key="d.namespace + '/' + d.name">
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
        <div class="btn-group" style="margin-top:var(--space-16)"><button class="btn btn-primary" @click="doScale">确认</button><button class="btn" @click="scaleDialog = null">取消</button></div>
      </div></div>
    </div>

    <!-- Image Dialog -->
    <div v-if="imageDialog" class="modal-overlay" @click.self="imageDialog = null">
      <div class="modal"><div class="modal-body">
        <h3>更新镜像: {{ imageDialog.name }}</h3>
        <p class="modal-copy">容器: <select v-model="imageDialog.container" class="form-select" style="width:auto;display:inline"><option v-for="img in imageDialog.images" :key="img" :value="img.split(':')[0]">{{ img }}</option></select></p>
        <p class="modal-copy">新镜像: <input v-model="imageDialog.newImage" class="form-input" style="width:200px;display:inline" placeholder="nginx:1.25" /></p>
        <div class="btn-group" style="margin-top:var(--space-16)"><button class="btn btn-primary" @click="doUpdateImage">确认</button><button class="btn" @click="imageDialog = null">取消</button></div>
      </div></div>
    </div>

    <!-- Rollback Dialog -->
    <div v-if="rollbackDialog" class="modal-overlay" @click.self="rollbackDialog = null">
      <div class="modal"><div class="modal-body">
        <h3>回滚: {{ rollbackDialog.name }}</h3>
        <div class="table-wrap" style="margin:var(--space-12) 0"><table class="data-table">
          <thead><tr><th>版本</th><th>镜像</th><th>时间</th><th></th></tr></thead>
          <tbody><tr v-for="r in rollbackDialog.revisions" :key="r.revision"><td>{{ r.revision }}</td><td>{{ r.image }}</td><td>{{ r.age }}</td><td><button class="btn btn-sm" @click="doRollback(r.revision)">回滚到此</button></td></tr></tbody>
        </table></div>
        <button class="btn" @click="rollbackDialog = null">取消</button>
      </div></div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../api/index.js'

const activeTab = ref('deployments')
const deployments = ref([])
const statefulsets = ref([])
const daemonsets = ref([])
const loading = ref(true)
const error = ref('')
const expandedDeploy = ref('')
const deployPods = ref({})

const scaleDialog = ref(null)
const imageDialog = ref(null)
const rollbackDialog = ref(null)

async function fetchData() {
  loading.value = true
  error.value = ''
  try {
    try { deployments.value = await api.get('/k8s/deployments') || [] } catch(e) { console.error(e) }
    try { statefulsets.value = await api.get('/k8s/statefulsets') || [] } catch(e) { console.error(e) }
    try { daemonsets.value = await api.get('/k8s/daemonsets') || [] } catch(e) { console.error(e) }
  } catch(e) { error.value = '加载失败，请检查集群连接' }
  finally { loading.value = false }
}
onMounted(fetchData)

async function toggleDeployExpand(d) {
  const key = d.namespace + '/' + d.name
  if (expandedDeploy.value === key) { expandedDeploy.value = ''; return }
  expandedDeploy.value = key
  if (!deployPods.value[key]) {
    try {
      deployPods.value[key] = await api.get(`/k8s/deployments/${d.namespace}/${d.name}/pods`) || []
    } catch(e) { deployPods.value[key] = [] }
  }
}

function openScaleDialog(d) {
  scaleDialog.value = {
    namespace: d.namespace, name: d.name,
    current: d.replicas, replicas: d.replicas,
    kind: 'deployment'
  }
}
async function doScale() {
  const d = scaleDialog.value
  const isSts = d.kind === 'statefulset'
  const path = isSts
    ? `/k8s/statefulsets/${d.namespace}/${d.name}/scale`
    : `/k8s/deployments/${d.namespace}/${d.name}/scale`
  try {
    await api.patch(path, { replicas: Number(d.replicas) })
    scaleDialog.value = null
    fetchData()
  } catch(e) { console.error(e) }
}

function openImageDialog(d) {
  imageDialog.value = {
    namespace: d.namespace, name: d.name,
    images: d.images || [], container: (d.images?.[0] || '').split(':')[0] || '',
    newImage: ''
  }
}
function doUpdateImage() {
  const d = imageDialog.value
  api.patch(`/k8s/deployments/${d.namespace}/${d.name}/image`, { container: d.container, image: d.newImage }).then(() => { imageDialog.value = null; fetchData() })
}

async function openRollbackDialog(d) {
  const result = await api.get(`/k8s/deployments/${d.namespace}/${d.name}/revisions`)
  rollbackDialog.value = { namespace: d.namespace, name: d.name, revisions: result || [] }
}

function openStsScaleDialog(s) {
  scaleDialog.value = {
    namespace: s.namespace, name: s.name,
    current: s.replicas, replicas: s.replicas,
    kind: 'statefulset'
  }
}
</script>

<style scoped>
.clickable { cursor: pointer; }
.pod-row td { padding: 4px 12px; border-bottom: 0; background: var(--surface-subtle); }
.pod-subrow { display: flex; align-items: center; gap: 8px; padding: 6px 0; font-size: 11px; color: var(--text-secondary); }
.pod-subrow .badge { font-size: 9px; }
.modal-overlay { position: fixed; z-index: 200; inset: 0; display: flex; align-items: center; justify-content: center; background: var(--overlay); backdrop-filter: blur(3px); }
.modal { min-width: 380px; max-width: 520px; padding: var(--space-20); border: 1px solid var(--border); border-radius: var(--radius-panel); background: var(--surface-raised); box-shadow: var(--shadow); }
.modal-body h3 { margin: 0 0 var(--space-12); color: var(--text-primary); font-size: 15px; }
</style>
