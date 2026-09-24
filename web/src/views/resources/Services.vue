<template>
  <div class="services-workspace">
    <TabbedWorkspaceCard>
      <template #meta><span class="resource-count">{{ filteredServices.length }} 个服务</span></template>
      <template #actions>
        <div class="service-namespace-filter">
          <SelectMenu v-model="namespace" aria-label="筛选命名空间" placeholder="全部命名空间">
            <option value="">全部命名空间</option>
            <option v-for="item in namespaces" :key="item" :value="item">{{ item }}</option>
          </SelectMenu>
        </div>
        <button class="icon-button" type="button" title="刷新服务" aria-label="刷新服务" :disabled="loading" @click="loadServices">
          <RefreshCw :size="16" :class="{ 'is-spinning': loading }" />
        </button>
      </template>

      <EmptyState v-if="loading && !services.length" message="正在读取 Service..." variant="loading" />
      <EmptyState v-else-if="!filteredServices.length" :message="namespace ? '当前命名空间暂无 Service' : '暂无 Service'" />
      <div v-else class="table-wrap service-table-wrap">
        <table class="data-table service-table">
          <thead><tr><th>名称</th><th>命名空间</th><th>类型</th><th>Cluster IP</th><th>端口</th><th>年龄</th><th class="action-cell">操作</th></tr></thead>
          <tbody><tr v-for="service in filteredServices" :key="serviceKey(service)">
            <td class="cell-primary"><OverflowTooltip class="service-cell-truncate" :text="service.name || '-'" /></td>
            <td><OverflowTooltip class="service-cell-truncate" :text="service.namespace || '-'" /></td>
            <td><span class="badge badge-online">{{ service.type || '-' }}</span></td>
            <td><OverflowTooltip class="service-cell-truncate service-code-cell" :text="service.cluster_ip || '-'" /></td>
            <td><OverflowTooltip class="service-cell-truncate service-code-cell" :text="formatPorts(service.ports)" /></td>
            <td>{{ service.age || '-' }}</td>
            <td class="action-cell"><button class="btn btn-sm" type="button" :data-testid="`view-service-endpoints-${serviceKey(service)}`" @click="openEndpoints(service)">查看端点</button></td>
          </tr></tbody>
        </table>
      </div>
    </TabbedWorkspaceCard>

    <BaseModal :open="showEndpoints" :title="endpointTitle" size="large" @close="closeEndpoints">
      <div v-if="selectedService" class="service-detail">
        <div class="service-detail-field"><span>Selector</span><OverflowTooltip class="service-selector" :text="formatSelector(selectedService.selector)" /></div>
        <div v-if="endpointLoading" class="endpoint-empty">正在读取端点...</div>
        <div v-else-if="endpointError" class="endpoint-empty endpoint-error">{{ endpointError }}</div>
        <div v-else-if="!endpointSlices.length" class="endpoint-empty">该服务暂无可用端点</div>
        <section v-for="slice in endpointSlices" v-else :key="slice.name" class="endpoint-slice">
          <div class="endpoint-slice-heading"><strong>{{ slice.name || 'EndpointSlice' }}</strong><span>{{ slice.address_type || '-' }}</span></div>
          <div class="table-wrap endpoint-table-wrap"><table class="data-table endpoint-table"><thead><tr><th>IP</th><th>节点</th><th>Pod</th><th>端口</th><th>状态</th></tr></thead><tbody>
            <tr v-for="endpoint in slice.endpoints || []" :key="endpoint.ip || endpoint.pod_name"><td class="cell-primary"><OverflowTooltip class="service-cell-truncate service-code-cell" :text="endpoint.ip || '-'" /></td><td><OverflowTooltip class="service-cell-truncate" :text="endpoint.node || '-'" /></td><td><OverflowTooltip class="service-cell-truncate" :text="endpoint.pod_name || '-'" /></td><td>{{ endpoint.port || '-' }}</td><td><span class="badge" :class="endpoint.ready ? 'badge-online' : 'badge-danger'">{{ endpoint.ready ? 'Ready' : 'NotReady' }}</span></td></tr>
          </tbody></table></div>
        </section>
      </div>
    </BaseModal>
    <ErrorNoticeModal :open="Boolean(error)" title="读取 Service 失败" :message="error" @close="error = ''" />
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { RefreshCw } from 'lucide-vue-next'
import { getLightweightServiceDiscovery, getServiceEndpoints } from '../../api/kubernetes.js'
import { useAsyncResource } from '../../composables/useAsyncResource.js'
import BaseModal from '../../components/BaseModal.vue'
import EmptyState from '../../components/EmptyState.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import OverflowTooltip from '../../components/OverflowTooltip.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import TabbedWorkspaceCard from '../../components/TabbedWorkspaceCard.vue'

const services = ref([])
const namespace = ref('')
const error = ref('')
const selectedService = ref(null)
const showEndpoints = ref(false)
const endpointSlices = ref([])
const endpointLoading = ref(false)
const endpointError = ref('')
const servicesResource = useAsyncResource(({ signal }) => getLightweightServiceDiscovery('', { signal }), [])

const loading = servicesResource.loading
const namespaces = computed(() => [...new Set(services.value.map(item => item?.namespace).filter(Boolean))].sort())
const filteredServices = computed(() => services.value.filter(item => item && (!namespace.value || item.namespace === namespace.value)))
const endpointTitle = computed(() => selectedService.value ? `服务端点 · ${selectedService.value.namespace}/${selectedService.value.name}` : '服务端点')

onMounted(loadServices)

function serviceKey(service) { return `${service.namespace}/${service.name}` }
function formatPorts(ports) { return Array.isArray(ports) && ports.length ? ports.join(', ') : '-' }
function formatSelector(selector) {
  const entries = Object.entries(selector || {})
  return entries.length ? entries.map(([key, value]) => `${key}=${value}`).join(', ') : '-'
}

async function loadServices() {
  error.value = ''
  const result = await servicesResource.refresh()
  if (result) services.value = result
  else if (servicesResource.error.value) error.value = servicesResource.error.value.message || '加载失败，请检查集群连接'
}

async function openEndpoints(service) {
  selectedService.value = service
  showEndpoints.value = true
  endpointSlices.value = []
  endpointError.value = ''
  endpointLoading.value = true
  try {
    endpointSlices.value = await getServiceEndpoints(service.namespace, service.name) || []
  } catch (cause) {
    endpointError.value = cause.message || '读取服务端点失败'
  } finally {
    endpointLoading.value = false
  }
}

function closeEndpoints() {
  showEndpoints.value = false
  selectedService.value = null
  endpointSlices.value = []
  endpointError.value = ''
}
</script>

<style scoped>
.services-workspace { margin-top: var(--space-20); }
.resource-count { color: var(--text-secondary); font-size: 12px; font-variant-numeric: tabular-nums; }
.service-namespace-filter { width: 156px; flex: 0 0 156px; }.service-namespace-filter :deep(.select-menu) { width: 100%; }
.service-table { min-width: 920px; table-layout: fixed; }.service-cell-truncate { max-width: 170px; }.service-code-cell { color: var(--text-secondary); font-family: var(--font-mono); font-size: 11px; }.action-cell { white-space: nowrap; }.service-detail { display: grid; gap: var(--space-16); }.service-detail-field { display: grid; grid-template-columns: 84px minmax(0, 1fr); gap: var(--space-12); align-items: start; color: var(--text-secondary); font-size: 12px; }.service-detail-field > span { color: var(--text-muted); font-weight: 700; }.service-selector { color: var(--text-primary); font-family: var(--font-mono); font-size: 11px; }.endpoint-empty { padding: var(--space-24) 0; color: var(--text-muted); font-size: 12px; text-align: center; }.endpoint-error { color: var(--danger); }.endpoint-slice { display: grid; gap: var(--space-8); }.endpoint-slice-heading { display: flex; align-items: baseline; gap: var(--space-8); color: var(--text-secondary); font-size: 11px; }.endpoint-slice-heading strong { color: var(--text-primary); font-size: 12px; }.endpoint-table-wrap { padding: 0; }.endpoint-table { min-width: 560px; table-layout: fixed; }
@media (max-width: 640px) { .service-namespace-filter { width: min(100%, 220px); flex-basis: min(100%, 220px); }.service-detail-field { grid-template-columns: 1fr; gap: 4px; } }
</style>
