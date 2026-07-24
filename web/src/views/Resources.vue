<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">资源浏览</h1>
    </div>

    <div class="card" style="margin-bottom:16px">
      <div style="display:flex;align-items:center;gap:12px;flex-wrap:wrap;">
        <div class="table-tabs">
          <button :class="['tab-btn', { 'tab-active': activeTab==='pods' }]" @click="activeTab='pods'">Pods</button>
          <button :class="['tab-btn', { 'tab-active': activeTab==='services' }]" @click="activeTab='services'">Services</button>
          <button :class="['tab-btn', { 'tab-active': activeTab==='deployments' }]" @click="activeTab='deployments'">Deployments</button>
        </div>
        <div style="display:flex;align-items:center;gap:8px;margin-left:auto;">
          <label class="form-label" style="margin-bottom:0;white-space:nowrap;">命名空间：</label>
          <select v-model="filterNs" class="form-select" style="width:auto;min-width:140px;">
            <option value="">全部</option>
            <option v-for="ns in namespaces" :key="ns" :value="ns">{{ ns }}</option>
          </select>
        </div>
      </div>
    </div>

    <div v-if="k8sError" class="k8s-banner k8s-banner-warn">⚠ {{ k8sError }}</div>

    <!-- Pods Tab -->
    <div v-if="activeTab==='pods'" class="card">
      <div v-if="pods.length===0" class="empty-state">
        <span class="empty-icon">▤</span><span class="empty-text">暂无 Pod</span>
      </div>
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead><tr><th>名称</th><th>命名空间</th><th>状态</th><th>节点</th><th>IP</th><th>重启</th></tr></thead>
          <tbody>
            <tr v-for="p in pods" :key="p.namespace+'/'+p.name">
              <td style="font-weight:600;">{{ p.name }}</td>
              <td>{{ p.namespace }}</td>
              <td><span class="badge" :class="p.status==='Running'?'badge-online':'badge-warn'">{{ p.status }}</span></td>
              <td>{{ p.node || '-' }}</td>
              <td>{{ p.ip || '-' }}</td>
              <td>{{ p.restarts }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Services Tab -->
    <div v-if="activeTab==='services'" class="card">
      <div v-if="services.length===0" class="empty-state">
        <span class="empty-icon">◎</span><span class="empty-text">暂无 Service</span>
      </div>
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead><tr><th>名称</th><th>命名空间</th><th>ClusterIP</th><th>类型</th><th>端口</th><th></th></tr></thead>
          <tbody>
            <tr v-for="s in services" :key="s.namespace+'/'+s.name">
              <td style="font-weight:600;">{{ s.name }}</td>
              <td>{{ s.namespace }}</td>
              <td>{{ s.cluster_ip || '-' }}</td>
              <td>{{ s.type }}</td>
              <td>{{ s.ports?.join(', ') || '-' }}</td>
              <td>
                <button class="btn btn-sm" @click="viewService(s)">详情</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Deployments Tab -->
    <div v-if="activeTab==='deployments'" class="card">
      <div v-if="deployments.length===0" class="empty-state">
        <span class="empty-icon">▥</span><span class="empty-text">暂无 Deployment</span>
      </div>
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead><tr><th>名称</th><th>命名空间</th><th>副本</th><th>就绪</th><th>镜像</th></tr></thead>
          <tbody>
            <tr v-for="d in deployments" :key="d.namespace+'/'+d.name">
              <td style="font-weight:600;">{{ d.name }}</td>
              <td>{{ d.namespace }}</td>
              <td>{{ d.replicas }}</td>
              <td><span :style="{color:d.ready===d.replicas?'var(--success)':'var(--warn)'}">{{ d.ready }}</span></td>
              <td style="font-size:12px;color:var(--text-secondary);">{{ d.images?.join(', ') }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Service Detail Modal -->
    <div v-if="serviceDetail" class="overlay" @click.self="serviceDetail=null">
      <div class="modal">
        <h2 class="modal-title">{{ serviceDetail.name }}</h2>
        <div style="display:grid;grid-template-columns:auto 1fr;gap:8px 16px;font-size:13px;">
          <span style="color:var(--text-muted)">命名空间：</span><span>{{ serviceDetail.namespace }}</span>
          <span style="color:var(--text-muted)">ClusterIP：</span><span>{{ serviceDetail.cluster_ip || '-' }}</span>
          <span style="color:var(--text-muted)">类型：</span><span>{{ serviceDetail.type }}</span>
          <span style="color:var(--text-muted)">端口：</span><span>{{ serviceDetail.ports?.join(', ') || '-' }}</span>
          <span v-if="serviceDetail.selectors?.length" style="color:var(--text-muted)">Selector：</span>
          <span v-if="serviceDetail.selectors?.length">{{ serviceDetail.selectors?.join(', ') }}</span>
        </div>
        <div class="modal-actions">
          <button class="btn" @click="serviceDetail=null">关闭</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import { api } from '../api/index.js'

const activeTab = ref('pods')
const filterNs = ref('')
const pods = ref([])
const services = ref([])
const deployments = ref([])
const k8sError = ref('')
const serviceDetail = ref(null)

const namespaces = computed(() => {
  const ns = new Set()
  pods.value.forEach(p => ns.add(p.namespace))
  services.value.forEach(s => ns.add(s.namespace))
  deployments.value.forEach(d => ns.add(d.namespace))
  return [...ns].sort()
})

onMounted(() => { fetchAll() })

async function fetchAll() {
  await Promise.all([fetchPods(), fetchServices(), fetchDeployments()])
}

async function fetchPods() {
  try {
    const ns = filterNs.value ? '?namespace=' + filterNs.value : ''
    const r = await api.get('/k8s/pods' + ns)
    const json = await r.json()
    if (json.error) { k8sError.value = json.error; pods.value = [] }
    else { pods.value = json.data || [] }
  } catch(e) { console.error(e) }
}

async function fetchServices() {
  try {
    const ns = filterNs.value ? '?namespace=' + filterNs.value : ''
    const r = await api.get('/k8s/services' + ns)
    const json = await r.json()
    if (json.error) { k8sError.value = json.error; services.value = [] }
    else { services.value = json.data || [] }
  } catch(e) { console.error(e) }
}

async function fetchDeployments() {
  try {
    const ns = filterNs.value ? '?namespace=' + filterNs.value : ''
    const r = await api.get('/k8s/deployments' + ns)
    const json = await r.json()
    if (json.error) { k8sError.value = json.error; deployments.value = [] }
    else { deployments.value = json.data || [] }
  } catch(e) { console.error(e) }
}

async function viewService(svc) {
  try {
    const r = await api.get(`/k8s/services/${svc.namespace}/${svc.name}`)
    const json = await r.json()
    if (json.error) { alert(json.error); return }
    serviceDetail.value = {
      ...json,
      selectors: json.selector ? Object.entries(json.selector).map(([k,v])=>k+'='+v) : [],
      ports: json.ports?.map(p => p.port + '/' + p.protocol)
    }
  } catch(e) { console.error(e) }
}
</script>
