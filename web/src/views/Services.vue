<template>
  <div>
    <div class="page-header"><h1 class="page-title">服务发现</h1></div>
    <div v-if="error" class="k8s-banner k8s-banner-warn" style="margin-bottom:var(--space-16)">⚠ {{ error }}</div>

    <div class="card">
      <div v-if="services.length === 0" class="empty-state">
        <span class="empty-icon">⬡</span><span class="empty-text">暂无 Service</span>
      </div>
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead><tr><th>名称</th><th>命名空间</th><th>类型</th><th>Cluster IP</th><th>端口</th><th>端点</th><th>年龄</th></tr></thead>
          <tbody>
            <template v-for="s in safeServices" :key="s.namespace + '/' + s.name">
              <tr class="clickable" @click="toggleExpand(s)">
                <td class="cell-primary">{{ s.name }}</td><td>{{ s.namespace }}</td>
                <td><span class="badge badge-online">{{ s.type }}</span></td>
                <td>{{ s.cluster_ip || '-' }}</td>
                <td>{{ (s.ports || []).join(', ') }}</td>
                <td><span :class="s.endpoint_count > 0 ? 'status-success' : 'status-warning'">{{ s.endpoint_count }}</span></td>
                <td>{{ s.age }}</td>
              </tr>
              <tr v-if="expandedSvc === s.namespace + '/' + s.name" class="endpoint-detail-row">
                <td colspan="7">
                  <div class="endpoint-panel">
                    <div class="detail-grid">
                      <span class="detail-label">Selector</span>
                      <span>{{ formatSelector(s.selector) }}</span>
                    </div>
                    <div v-for="slice in endpointSlices[s.namespace + '/' + s.name]" :key="slice.name" style="margin-top:var(--space-12)">
                      <strong style="font-size:11px;color:var(--text-secondary)">EndpointSlice: {{ slice.name }} ({{ slice.address_type }})</strong>
                      <div class="table-wrap" style="margin-top:6px"><table class="data-table">
                        <thead><tr><th>IP</th><th>节点</th><th>Pod</th><th>端口</th><th>状态</th></tr></thead>
                        <tbody><tr v-for="ep in slice.endpoints" :key="ep.ip">
                          <td class="cell-primary">{{ ep.ip }}</td><td>{{ ep.node || '-' }}</td><td>{{ ep.pod_name || '-' }}</td><td>{{ ep.port || '-' }}</td>
                          <td><span class="badge" :class="ep.ready ? 'badge-online' : 'badge-danger'">{{ ep.ready ? 'Ready' : 'NotReady' }}</span></td>
                        </tr></tbody>
                      </table></div>
                    </div>
                    <div v-if="!endpointSlices[s.namespace + '/' + s.name]?.length" style="margin-top:var(--space-8);color:var(--text-muted);font-size:11px">无端点</div>
                  </div>
                </td>
              </tr>
            </template>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, ref, onMounted } from 'vue'
import { getServiceDiscovery, getServiceEndpoints } from '../api/kubernetes.js'
import { useAsyncResource } from '../composables/useAsyncResource.js'

const services = ref([])
const loading = ref(true)
const error = ref('')
const expandedSvc = ref('')
const endpointSlices = ref({})
const servicesResource = useAsyncResource(({ signal }) => getServiceDiscovery({ signal }), [])

const safeServices = computed(() => (services.value || []).filter(s => s != null))

onMounted(loadServices)

async function loadServices() {
  loading.value = true
  error.value = ''
  const result = await servicesResource.refresh()
  if (result) services.value = result || []
  else if (servicesResource.error.value) error.value = servicesResource.error.value.message || '加载失败，请检查集群连接'
  loading.value = false
}

async function toggleExpand(s) {
  const key = s.namespace + '/' + s.name
  if (expandedSvc.value === key) { expandedSvc.value = ''; return }
  expandedSvc.value = key
  if (!endpointSlices.value[key]) {
    try {
      endpointSlices.value[key] = await getServiceEndpoints(s.namespace, s.name) || []
    } catch(e) { console.error(e) }
  }
}

function formatSelector(sel) {
  if (!sel) return '-'
  return Object.entries(sel).map(([k,v]) => `${k}=${v}`).join(', ')
}
</script>

<style scoped>
.clickable { cursor: pointer; }
.endpoint-detail-row td { padding: 8px 12px; border-bottom: 0; background: var(--surface-subtle); }
.endpoint-panel { padding: var(--space-12) 0; }
</style>
