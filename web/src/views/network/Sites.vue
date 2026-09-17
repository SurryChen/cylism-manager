<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">路由</h1>
    </div>
    <div v-if="error" class="k8s-banner k8s-banner-warn" style="margin-bottom:var(--space-16)">⚠ {{ error }}</div>

    <!-- Traefik Controller Banner -->
    <div v-if="controllerStatus" class="controller-banner" :class="controllerStatus.running ? 'controller-ok' : 'controller-warn'">
      <span v-if="controllerStatus.type === 'Traefik'">Ingress Controller: Traefik {{ controllerStatus.version || '' }}（{{ controllerStatus.running ? '运行中' : '未运行' }}）</span>
      <span v-else-if="controllerStatus.type === 'Unknown'">未检测到 Ingress Controller，Ingress 规则可能无法生效</span>
      <span v-else>Ingress Controller: {{ controllerStatus.type }}（{{ controllerStatus.running ? '运行中' : '未运行' }}）</span>
    </div>

    <!-- Tab switcher -->
    <SurfaceCard as="div" class="section-gap">
      <div class="table-tabs">
        <button :class="['tab-btn', { 'tab-active': activeTab === 'ingressroute' }]" @click="activeTab = 'ingressroute'">IngressRoute</button>
        <button :class="['tab-btn', { 'tab-active': activeTab === 'ingress' }]" @click="activeTab = 'ingress'">标准 Ingress</button>
      </div>
    </SurfaceCard>

    <!-- IngressRoute Tab -->
    <SurfaceCard v-if="activeTab === 'ingressroute'" as="div">
      <div class="filter-bar" style="margin-bottom:var(--space-12)">
        <div class="filter-control"><label class="form-label">命名空间：</label>
        <SelectMenu v-model="filterNs" class="form-select">
          <option value="">全部</option>
          <option v-for="ns in namespaces" :key="ns" :value="ns">{{ ns }}</option>
        </SelectMenu></div>
      </div>
      <div v-if="routesLoading" class="empty-state">
        <span class="empty-text">加载 IngressRoute 中...</span>
      </div>
      <div v-else-if="filteredRoutes.length === 0" class="empty-state">
        <span class="empty-icon">⊞</span><span class="empty-text">暂无 IngressRoute</span>
      </div>
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead><tr><th>名称</th><th>命名空间</th><th>域名</th><th>TLS</th><th>年龄</th><th></th></tr></thead>
          <tbody>
            <tr v-for="route in filteredRoutes" :key="route.namespace + '/' + route.name">
              <td class="cell-primary">{{ route.name }}</td><td>{{ route.namespace }}</td>
              <td>{{ route.domain || '-' }}</td>
              <td><span class="badge" :class="route.tls && route.tls !== 'false' ? 'badge-online' : 'badge-offline'">{{ route.tls && route.tls !== 'false' ? 'HTTPS' : 'HTTP' }}</span></td>
              <td>{{ route.created_at }}</td>
              <td><div class="btn-group action-cell"><button class="btn btn-sm btn-danger" @click="confirmDeleteRoute(route)">删除</button></div></td>
            </tr>
          </tbody>
        </table>
      </div>
    </SurfaceCard>

    <!-- 标准 Ingress Tab -->
    <SurfaceCard v-if="activeTab === 'ingress'" as="div">
      <div class="filter-bar" style="margin-bottom:var(--space-12)"><button class="btn btn-primary" @click="showAddIngress = true">+ 添加 Ingress</button></div>
      <div v-if="ingressesLoading" class="empty-state">
        <span class="empty-text">加载 Ingress 中...</span>
      </div>
      <div v-else-if="ingresses.length === 0" class="empty-state">
        <span class="empty-icon">⊞</span><span class="empty-text">暂无标准 Ingress</span>
      </div>
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead><tr><th>名称</th><th>命名空间</th><th>Host</th><th>路径 → Service</th><th>TLS</th><th>Controller</th><th>年龄</th><th></th></tr></thead>
          <tbody>
            <tr v-for="ing in ingresses" :key="ing.namespace + '/' + ing.name">
              <td class="cell-primary">{{ ing.name }}</td><td>{{ ing.namespace }}</td>
              <td>{{ (ing.hosts || []).join(', ') }}</td>
              <td>{{ (ing.paths || []).join('; ') }}</td>
              <td>{{ (ing.tls || []).join(', ') || '-' }}</td>
              <td>{{ ing.controller || '默认' }}</td><td>{{ ing.age }}</td>
              <td><div class="btn-group action-cell"><button class="btn btn-sm btn-danger" @click="confirmDeleteIngress(ing)">删除</button></div></td>
            </tr>
          </tbody>
        </table>
      </div>
    </SurfaceCard>

    <!-- Add Ingress modal -->
    <div v-if="showAddIngress" class="overlay" @click.self="showAddIngress = false">
      <div class="modal">
        <h2 class="modal-title">添加 Ingress</h2>
        <form @submit.prevent="createIngress">
          <div class="form-row"><div class="form-group"><label class="form-label">名称</label><input v-model="ingressForm.name" class="form-input" required /></div>
          <div class="form-group"><label class="form-label">命名空间</label><input v-model="ingressForm.namespace" class="form-input" required /></div></div>
          <div class="form-row"><div class="form-group"><label class="form-label">域名</label><input v-model="ingressForm.host" class="form-input" placeholder="example.com" required /></div>
          <div class="form-group"><label class="form-label">路径</label><input v-model="ingressForm.path" class="form-input" placeholder="/" /></div></div>
          <div class="form-row"><div class="form-group"><label class="form-label">Service</label><input v-model="ingressForm.service_name" class="form-input" required /></div>
          <div class="form-group"><label class="form-label">Port</label><input v-model="ingressForm.service_port" class="form-input" placeholder="http" required /></div></div>
          <div class="modal-actions"><button type="button" class="btn" @click="showAddIngress = false">取消</button><button type="submit" class="btn btn-primary">确认</button></div>
        </form>
      </div>
    </div>

    <!-- Delete IngressRoute confirm -->
    <div v-if="deleteRouteTarget" class="overlay" @click.self="deleteRouteTarget = null">
      <div class="modal"><h2 class="modal-title">删除 IngressRoute</h2><p class="modal-copy">确定删除 <strong>{{ deleteRouteTarget.name }}</strong>？</p>
      <div class="modal-actions"><button class="btn" @click="deleteRouteTarget = null">取消</button><button class="btn btn-danger" @click="removeRoute">确认删除</button></div></div>
    </div>

    <!-- Delete Ingress confirm -->
    <div v-if="deleteIngressTarget" class="overlay" @click.self="deleteIngressTarget = null">
      <div class="modal"><h2 class="modal-title">删除 Ingress</h2><p class="modal-copy">确定删除 <strong>{{ deleteIngressTarget.name }}</strong>？</p>
      <div class="modal-actions"><button class="btn" @click="deleteIngressTarget = null">取消</button><button class="btn btn-danger" @click="removeIngress">确认删除</button></div></div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import { createIngress as createIngressRequest, deleteIngress, deleteRoute, getIngressControllerStatus, getIngresses, getRoutes } from '../../api/sites.js'
import { useAsyncResource } from '../../composables/useAsyncResource.js'
import SurfaceCard from '../../components/SurfaceCard.vue'

const controllerStatusCacheKey = 'cylism.ingress-controller.status'
const controllerStatusCacheTtl = 60 * 1000

const activeTab = ref('ingressroute')
const controllerStatus = ref(getCachedControllerStatus()?.status || null)
const routes = ref([])
const filterNs = ref('')
const ingresses = ref([])
const routesLoading = ref(true)
const ingressesLoading = ref(true)
const error = ref('')
const deleteRouteTarget = ref(null)
const deleteIngressTarget = ref(null)
const showAddIngress = ref(false)
const ingressForm = ref({ name: '', namespace: 'default', host: '', path: '/', service_name: '', service_port: 'http' })
const routesResource = useAsyncResource(({ signal }) => getRoutes({ signal }), [])
const ingressesResource = useAsyncResource(({ signal }) => getIngresses({ signal }), [])
const controllerResource = useAsyncResource(({ signal }) => getIngressControllerStatus({ signal }), null)

const namespaces = computed(() => [...new Set(routes.value.map(r => r.namespace))].sort())
const filteredRoutes = computed(() =>
  filterNs.value ? routes.value.filter(r => r.namespace === filterNs.value) : routes.value
)

onMounted(() => {
  error.value = ''
  fetchControllerStatus()
  fetchRoutes()
  fetchIngresses()
})

async function fetchControllerStatus() {
  const cached = getCachedControllerStatus()
  if (cached?.status) controllerStatus.value = cached.status
  if (cached && cached.expiresAt > Date.now()) return
  try {
    const status = await controllerResource.refresh()
    if (status) { controllerStatus.value = status; cacheControllerStatus(status) }
  } catch(e) {}
}

async function fetchRoutes() {
  routesLoading.value = true
  try {
    const result = await routesResource.refresh()
    if (result) routes.value = result || []
  } finally { routesLoading.value = false }
}

async function fetchIngresses() {
  ingressesLoading.value = true
  try {
    const result = await ingressesResource.refresh()
    if (result) ingresses.value = result || []
  } finally { ingressesLoading.value = false }
}

function getCachedControllerStatus() {
  try {
    const cached = JSON.parse(sessionStorage.getItem(controllerStatusCacheKey) || 'null')
    return cached?.status ? cached : null
  } catch(e) {
    return null
  }
}

function cacheControllerStatus(status) {
  try {
    sessionStorage.setItem(controllerStatusCacheKey, JSON.stringify({
      status,
      expiresAt: Date.now() + controllerStatusCacheTtl,
    }))
  } catch(e) {}
}

function confirmDeleteRoute(route) { deleteRouteTarget.value = route }
function confirmDeleteIngress(ing) { deleteIngressTarget.value = ing }

async function removeRoute() {
  try { await deleteRoute(deleteRouteTarget.value.namespace, deleteRouteTarget.value.name); deleteRouteTarget.value = null; fetchRoutes() } catch(e) { error.value = e.message || '删除 IngressRoute 失败' }
}

async function removeIngress() {
  try { await deleteIngress(deleteIngressTarget.value.namespace, deleteIngressTarget.value.name); deleteIngressTarget.value = null; fetchIngresses() } catch(e) { error.value = e.message || '删除 Ingress 失败' }
}

async function createIngress() {
  try {
    await createIngressRequest(ingressForm.value)
    showAddIngress.value = false
    ingressForm.value = { name: '', namespace: 'default', host: '', path: '/', service_name: '', service_port: 'http' }
    fetchIngresses()
  } catch(e) { error.value = e.message || '创建 Ingress 失败' }
}
</script>

<style scoped>
.controller-banner { padding: 12px 16px; border-radius: var(--radius-control); font-size: 12px; margin-bottom: var(--space-16); }
.controller-ok { background: var(--success-surface); color: var(--success); border: 1px solid var(--success); }
.controller-warn { background: var(--warning-surface); color: var(--warning); border: 1px solid var(--warning); }
</style>
