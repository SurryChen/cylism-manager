<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">资源浏览</h1>
    </div>

    <div class="card section-gap">
      <div class="filter-bar">
        <div class="table-tabs">
          <button :class="['tab-btn', { 'tab-active': activeTab==='pods' }]" @click="activeTab='pods'">Pods</button>
          <button :class="['tab-btn', { 'tab-active': activeTab==='services' }]" @click="activeTab='services'">Services</button>
          <button :class="['tab-btn', { 'tab-active': activeTab==='deployments' }]" @click="activeTab='deployments'">Deployments</button>
        </div>
        <div class="filter-control filter-control--push">
          <label class="form-label">命名空间：</label>
          <select v-model="filterNs" class="form-select">
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
          <thead><tr><th>名称</th><th>命名空间</th><th>状态</th><th>节点</th><th>IP</th><th>重启</th><th>年龄</th></tr></thead>
          <tbody>
            <tr v-for="p in pods" :key="p.namespace+'/'+p.name">
              <td class="cell-primary">{{ p.name }}</td>
              <td>{{ p.namespace }}</td>
              <td><span class="badge" :class="p.status==='Running'?'badge-online':'badge-warn'">{{ p.status }}</span></td>
              <td>{{ p.node || '-' }}</td>
              <td>{{ p.ip || '-' }}</td>
              <td>{{ p.restarts }}</td>
              <td>{{ p.age || '-' }}</td>
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
              <td class="cell-primary">{{ s.name }}</td>
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
              <td class="cell-primary">{{ d.name }}</td>
              <td>{{ d.namespace }}</td>
              <td>{{ d.replicas }}</td>
              <td><span :class="d.ready===d.replicas ? 'status-success' : 'status-warning'">{{ d.ready }}</span></td>
              <td class="cell-secondary">{{ d.images?.join(', ') }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Service Detail Modal -->
    <div v-if="serviceDetail" class="overlay" @click.self="serviceDetail=null">
      <div class="modal">
        <h2 class="modal-title">{{ serviceDetail.name }}</h2>
        <div class="detail-grid">
          <span class="detail-label">命名空间：</span><span>{{ serviceDetail.namespace }}</span>
          <span class="detail-label">ClusterIP：</span><span>{{ serviceDetail.cluster_ip || '-' }}</span>
          <span class="detail-label">类型：</span><span>{{ serviceDetail.type }}</span>
          <span class="detail-label">端口：</span><span>{{ serviceDetail.ports?.join(', ') || '-' }}</span>
          <span v-if="serviceDetail.selectors?.length" class="detail-label">Selector：</span>
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
import { ref, onMounted, computed, watch } from 'vue'
import { getResourceInventory, getResourceService } from '../../api/kubernetes.js'
import { useAsyncResource } from '../../composables/useAsyncResource.js'

const activeTab = ref('pods')
const filterNs = ref('')
const pods = ref([])
const services = ref([])
const deployments = ref([])
const k8sError = ref('')
const serviceDetail = ref(null)
const resourceResource = useAsyncResource(({ signal }, namespace) => getResourceInventory(namespace, { signal }), [[], [], []])

const namespaces = computed(() => {
  const ns = new Set()
  pods.value.forEach(p => ns.add(p.namespace))
  services.value.forEach(s => ns.add(s.namespace))
  deployments.value.forEach(d => ns.add(d.namespace))
  return [...ns].sort()
})

onMounted(() => { refreshResources() })
watch(filterNs, refreshResources)

async function refreshResources() {
  k8sError.value = ''
  const result = await resourceResource.refresh(filterNs.value)
  if (result) {
    pods.value = result[0] || []
    services.value = result[1] || []
    deployments.value = result[2] || []
  } else if (resourceResource.error.value) {
    k8sError.value = resourceResource.error.value.message || 'K8s 集群未连接，资源数据不可用'
  }
}

async function viewService(svc) {
  try {
    const json = await getResourceService(svc.namespace, svc.name)
    serviceDetail.value = {
      ...json,
      selectors: json.selector ? Object.entries(json.selector).map(([k,v])=>k+'='+v) : [],
      ports: json.ports?.map(p => p.port + '/' + p.protocol)
    }
  } catch(e) { console.error(e) }
}
</script>
