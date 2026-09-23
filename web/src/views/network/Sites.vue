<template>
  <section class="sites-workspace">
    <div v-if="error" class="k8s-banner k8s-banner-warn">{{ error }}</div>

    <div
      v-if="controllerStatus && !controllerStatus.running"
      class="controller-banner controller-warn"
    >
      <span v-if="controllerStatus.type === 'Unknown'">未检测到 Ingress Controller，Ingress 规则可能无法生效</span>
      <span v-else>Ingress Controller: {{ controllerStatus.type }}（未运行）</span>
    </div>

    <SurfaceCard padding="none" class="ingress-card">
      <div class="ingress-toolbar">
        <span v-if="controllerStatus" class="badge" :class="controllerStatus.running ? 'badge-online' : 'badge-offline'">
          {{ controllerSummary }}
        </span>
        <div class="ingress-toolbar-actions">
          <div class="ingress-filter-control">
            <SelectMenu v-model="filterNamespace" class="ingress-namespace-filter" data-testid="ingress-namespace-filter">
              <option value="">全部命名空间</option>
              <option v-for="namespace in ingressNamespaces" :key="namespace" :value="namespace">{{ namespace }}</option>
            </SelectMenu>
          </div>
          <button
            class="icon-button"
            type="button"
            title="刷新 Ingress"
            aria-label="刷新 Ingress"
            :disabled="ingressesLoading"
            data-testid="refresh-ingresses"
            @click="fetchIngresses"
          >
            <RefreshCw :size="16" :class="{ 'is-spinning': ingressesLoading }" />
          </button>
          <button class="btn btn-primary" type="button" @click="showAddIngress = true">+ 添加 Ingress</button>
        </div>
      </div>

      <div v-if="ingressesLoading" class="empty-state">
        <span class="empty-text">加载 Ingress 中...</span>
      </div>
      <div v-else-if="filteredIngresses.length === 0" class="empty-state">
        <span class="empty-icon">⊞</span>
        <span class="empty-text">暂无标准 Ingress</span>
      </div>
      <div v-else class="table-wrap ingress-table-wrap">
        <table class="data-table ingress-table">
          <colgroup>
            <col class="ingress-name-column" />
            <col class="ingress-namespace-column" />
            <col class="ingress-host-column" />
            <col class="ingress-path-column" />
            <col class="ingress-tls-column" />
            <col class="ingress-controller-column" />
            <col class="ingress-age-column" />
            <col class="ingress-actions-column" />
          </colgroup>
          <thead><tr><th>名称</th><th>命名空间</th><th>Host</th><th>路径 → Service</th><th>TLS</th><th>Controller</th><th>年龄</th><th class="action-cell">操作</th></tr></thead>
          <tbody>
            <tr v-for="ingress in filteredIngresses" :key="ingress.namespace + '/' + ingress.name">
              <td class="cell-primary"><OverflowTooltip :text="ingress.name" /></td>
              <td><OverflowTooltip :text="ingress.namespace" /></td>
              <td><OverflowTooltip :text="(ingress.hosts || []).join(', ') || '-'" /></td>
              <td><OverflowTooltip :text="(ingress.paths || []).join('; ') || '-'" /></td>
              <td><OverflowTooltip :text="(ingress.tls || []).join(', ') || '-'" /></td>
              <td><OverflowTooltip :text="ingress.controller || '默认'" /></td>
              <td>{{ ingress.age }}</td>
              <td class="action-cell"><button class="btn btn-sm btn-danger" @click="confirmDeleteIngress(ingress)">删除</button></td>
            </tr>
          </tbody>
        </table>
      </div>
    </SurfaceCard>

    <BaseModal :open="showAddIngress" title="添加 Ingress" @close="showAddIngress = false">
      <form @submit.prevent="createIngress">
        <div class="form-row">
          <div class="form-group"><label class="form-label">名称</label><input v-model="ingressForm.name" class="form-input" required /></div>
          <div class="form-group"><label class="form-label">命名空间</label><input v-model="ingressForm.namespace" class="form-input" required /></div>
        </div>
        <div class="form-row">
          <div class="form-group"><label class="form-label">域名</label><input v-model="ingressForm.host" class="form-input" placeholder="example.com" required /></div>
          <div class="form-group"><label class="form-label">路径</label><input v-model="ingressForm.path" class="form-input" placeholder="/" /></div>
        </div>
        <div class="form-row">
          <div class="form-group"><label class="form-label">Service</label><input v-model="ingressForm.service_name" class="form-input" required /></div>
          <div class="form-group"><label class="form-label">Port</label><input v-model="ingressForm.service_port" class="form-input" placeholder="http" required /></div>
        </div>
        <div class="modal-actions"><button type="button" class="btn" @click="showAddIngress = false">取消</button><button type="submit" class="btn btn-primary">确认</button></div>
      </form>
    </BaseModal>

    <BaseModal :open="Boolean(deleteIngressTarget)" title="删除 Ingress" @close="deleteIngressTarget = null">
      <p class="modal-copy">确定删除 <strong>{{ deleteIngressTarget?.name }}</strong>？</p>
      <template #actions><button class="btn" @click="deleteIngressTarget = null">取消</button><button class="btn btn-danger" @click="removeIngress">确认删除</button></template>
    </BaseModal>
  </section>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { RefreshCw } from 'lucide-vue-next'
import { createIngress as createIngressRequest, deleteIngress, getIngressControllerStatus, getIngresses } from '../../api/sites.js'
import { useAsyncResource } from '../../composables/useAsyncResource.js'
import BaseModal from '../../components/BaseModal.vue'
import OverflowTooltip from '../../components/OverflowTooltip.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import SurfaceCard from '../../components/SurfaceCard.vue'

const controllerStatusCacheKey = 'cylism.ingress-controller.status'
const controllerStatusCacheTtl = 60 * 1000

const controllerStatus = ref(getCachedControllerStatus()?.status || null)
const ingresses = ref([])
const filterNamespace = ref('')
const ingressesLoading = ref(true)
const error = ref('')
const deleteIngressTarget = ref(null)
const showAddIngress = ref(false)
const ingressForm = ref(newIngressForm())
const ingressesResource = useAsyncResource(({ signal }) => getIngresses({ signal }), [])
const controllerResource = useAsyncResource(({ signal }) => getIngressControllerStatus({ signal }), null)

const ingressNamespaces = computed(() => [...new Set(ingresses.value.map(ingress => ingress.namespace).filter(Boolean))].sort())
const filteredIngresses = computed(() => (
  filterNamespace.value ? ingresses.value.filter(ingress => ingress.namespace === filterNamespace.value) : ingresses.value
))
const controllerSummary = computed(() => {
  if (!controllerStatus.value) return ''
  const { type, version, running } = controllerStatus.value
  return `${type || 'Ingress Controller'}${version ? ` ${version}` : ''} · ${running ? '运行中' : '未运行'}`
})

onMounted(() => {
  fetchControllerStatus()
  fetchIngresses()
})

async function fetchControllerStatus() {
  const cached = getCachedControllerStatus()
  if (cached?.status) controllerStatus.value = cached.status
  if (cached && cached.expiresAt > Date.now()) return
  try {
    const status = await controllerResource.refresh()
    if (status) {
      controllerStatus.value = status
      cacheControllerStatus(status)
    }
  } catch (_) {
    // Controller status should not block the Ingress list.
  }
}

async function fetchIngresses() {
  ingressesLoading.value = true
  error.value = ''
  try {
    const result = await ingressesResource.refresh()
    if (result !== undefined) ingresses.value = result || []
    else throw ingressesResource.error.value || new Error('加载 Ingress 失败')
  } catch (err) {
    error.value = err.message || '加载 Ingress 失败'
  } finally {
    ingressesLoading.value = false
  }
}

function getCachedControllerStatus() {
  try {
    const cached = JSON.parse(sessionStorage.getItem(controllerStatusCacheKey) || 'null')
    return cached?.status ? cached : null
  } catch (_) {
    return null
  }
}

function cacheControllerStatus(status) {
  try {
    sessionStorage.setItem(controllerStatusCacheKey, JSON.stringify({ status, expiresAt: Date.now() + controllerStatusCacheTtl }))
  } catch (_) {
    // Browsers with disabled storage can still use the live response.
  }
}

function newIngressForm() {
  return { name: '', namespace: 'default', host: '', path: '/', service_name: '', service_port: 'http' }
}

function confirmDeleteIngress(ingress) {
  deleteIngressTarget.value = ingress
}

async function removeIngress() {
  if (!deleteIngressTarget.value) return
  try {
    await deleteIngress(deleteIngressTarget.value.namespace, deleteIngressTarget.value.name)
    deleteIngressTarget.value = null
    await fetchIngresses()
  } catch (err) {
    error.value = err.message || '删除 Ingress 失败'
  }
}

async function createIngress() {
  try {
    await createIngressRequest(ingressForm.value)
    showAddIngress.value = false
    ingressForm.value = newIngressForm()
    await fetchIngresses()
  } catch (err) {
    error.value = err.message || '创建 Ingress 失败'
  }
}
</script>

<style scoped>
.sites-workspace { padding-top: var(--tabbed-page-content-gap); }
.k8s-banner, .controller-banner { margin-top: var(--space-20); }
.controller-banner { padding: 12px 16px; border: 1px solid var(--warning); border-radius: var(--radius-control); background: var(--warning-surface); color: var(--warning); font-size: 12px; }
.ingress-card { margin-top: var(--space-20); min-height: 260px; }
.ingress-toolbar { display: flex; min-height: 62px; align-items: center; justify-content: space-between; gap: var(--space-12); padding: 12px var(--space-16); }
.ingress-toolbar-actions { display: flex; min-width: 0; align-items: center; justify-content: flex-end; gap: 8px; }
.ingress-filter-control { width: 156px; flex: 0 0 156px; }
.ingress-filter-control :deep(.select-menu) { width: 100%; }
.ingress-table-wrap { padding: 0 var(--space-16); }
.ingress-table { min-width: 940px; table-layout: fixed; }
.ingress-name-column { width: 150px; }.ingress-namespace-column { width: 130px; }.ingress-host-column { width: 180px; }.ingress-path-column { width: 210px; }.ingress-tls-column { width: 150px; }.ingress-controller-column { width: 110px; }.ingress-age-column { width: 88px; }.ingress-actions-column { width: 72px; }
.is-spinning { animation: spin .8s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
@media (max-width: 640px) { .ingress-toolbar { align-items: stretch; flex-direction: column; }.ingress-toolbar-actions { width: 100%; }.ingress-filter-control { min-width: 0; flex: 1; width: auto; } }
</style>
