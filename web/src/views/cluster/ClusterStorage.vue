<template>
  <div class="cluster-storage-page">
    <RouterLink class="back-link" to="/cluster?tab=list"><ArrowLeft :size="16" />返回集群列表</RouterLink>
    <SectionTabsHeader :title="cluster?.name || '存储卷'" :tabs="tabs" active-tab="claims" test-id-prefix="storage-tab" />
    <FeedbackBanner v-if="error" class="storage-error" tone="warning" :message="error" />
    <ClusterAgentGate v-if="cluster" :cluster-id="clusterID" @ready="loadStorage">
    <SurfaceCard class="storage-content" padding="none">
      <div class="storage-toolbar">
        <SelectMenu v-model="namespace" class="namespace-select" aria-label="命名空间">
          <option value="">全部命名空间</option>
          <option v-for="item in namespaces" :key="item" :value="item">{{ item }}</option>
        </SelectMenu>
        <div class="toolbar-actions">
          <button class="icon-button" type="button" title="刷新存储卷" aria-label="刷新存储卷" :disabled="loading" @click="loadClaims"><RefreshCw :size="15" :class="{ 'is-spinning': loading }" /></button>
          <button class="btn btn-primary" type="button" :disabled="!cluster" @click="openCreate">创建存储卷</button>
        </div>
      </div>
      <EmptyState v-if="!loaded" variant="loading" message="正在读取存储卷" />
      <EmptyState v-else-if="!claims.length" message="暂无存储卷" />
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead><tr><th>存储卷</th><th>命名空间</th><th>归属</th><th>容量</th><th>已用空间</th><th>StorageClass</th><th>状态</th><th>绑定节点</th><th>引用</th><th class="action-cell">操作</th></tr></thead>
          <tbody>
            <tr v-for="claim in claims" :key="`${claim.namespace}/${claim.name}`">
              <td class="cell-primary">{{ claim.name }}</td>
              <td>{{ claim.namespace }}</td>
              <td><span class="badge" :class="claim.managed ? 'badge-online' : 'badge-offline'">{{ claim.managed ? '平台托管' : '外部创建' }}</span></td>
              <td>{{ claim.storage || '-' }}</td>
              <td :title="usageFor(claim)?.message || ''">{{ usageFor(claim)?.status === 'available' ? formatBytes(usageFor(claim).used_bytes) : usageFor(claim)?.message || '读取中...' }}</td>
              <td>{{ claim.storage_class_name || '默认' }}</td>
              <td><span class="badge" :class="claim.phase === 'Bound' ? 'badge-online' : 'badge-deploying'">{{ claim.phase || 'Pending' }}</span></td>
              <td>{{ claim.bound_node || '-' }}</td>
              <td>{{ referenceCount(claim) }}</td>
              <td class="action-cell"><button v-if="canDelete(claim)" class="btn btn-sm btn-danger" type="button" :disabled="referencesLoading || !referencesLoaded || referenceCount(claim) > 0" @click="deleteTarget = claim">删除</button><span v-else class="cell-secondary">-</span></td>
            </tr>
          </tbody>
        </table>
      </div>
      <div v-if="total > pageSize" class="pagination"><button class="btn btn-sm" :disabled="page <= 1 || loading" @click="page--">上一页</button><span>{{ page }} / {{ Math.ceil(total / pageSize) }}</span><button class="btn btn-sm" :disabled="page * pageSize >= total || loading" @click="page++">下一页</button></div>
    </SurfaceCard>
    </ClusterAgentGate>

    <BaseModal :open="creating" title="创建存储卷" @close="creating = false">
      <form id="cluster-create-pvc" @submit.prevent="saveClaim">
        <div class="form-group"><label class="form-label" for="pvc-namespace">命名空间</label><SelectMenu id="pvc-namespace" v-model="form.namespace" required><option value="" disabled>选择命名空间</option><option v-for="item in namespaces" :key="item" :value="item">{{ item }}</option></SelectMenu></div>
        <div class="form-group"><label class="form-label" for="pvc-name">名称</label><input id="pvc-name" v-model.trim="form.name" class="form-input" required pattern="[a-z0-9]([-a-z0-9]*[a-z0-9])?" /></div>
        <div class="form-group"><label class="form-label" for="pvc-size">容量</label><div class="size-control"><input id="pvc-size" v-model.number="form.size" type="number" min="1" class="form-input" required /><SelectMenu v-model="form.unit" aria-label="容量单位"><option value="Mi">Mi</option><option value="Gi">Gi</option><option value="Ti">Ti</option></SelectMenu></div></div>
        <div class="form-group"><label class="form-label" for="pvc-class">StorageClass</label><SelectMenu id="pvc-class" v-model="form.storageClass"><option value="">使用默认 StorageClass</option><option v-for="item in storageClasses" :key="item.name" :value="item.name">{{ item.name }}</option></SelectMenu></div>
        <FeedbackBanner v-if="actionError" tone="warning" :message="actionError" />
      </form>
      <template #actions><button class="btn" :disabled="saving" @click="creating = false">取消</button><button class="btn btn-primary" type="submit" form="cluster-create-pvc" :disabled="saving || !form.namespace || !form.name || form.size < 1">{{ saving ? '创建中...' : '创建存储卷' }}</button></template>
    </BaseModal>
    <BaseModal :open="Boolean(deleteTarget)" title="删除存储卷" @close="deleteTarget = null">
      <p>确定删除 {{ deleteTarget?.namespace }}/{{ deleteTarget?.name }} 吗？底层数据是否删除取决于 PV 的回收策略。</p>
      <FeedbackBanner v-if="actionError" tone="warning" :message="actionError" />
      <template #actions><button class="btn" :disabled="saving" @click="deleteTarget = null">取消</button><button class="btn btn-danger" :disabled="saving" @click="removeClaim">确认删除</button></template>
    </BaseModal>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { ArrowLeft, RefreshCw } from 'lucide-vue-next'
import { getCluster } from '../../api/clusters.js'
import { createClusterPersistentVolumeClaim, deleteClusterPersistentVolumeClaim, getClusterPersistentVolumeInventory, getClusterPersistentVolumeReferences, getClusterPersistentVolumeUsage, getClusterStorageClasses, getClusterStorageNamespaces } from '../../api/storage.js'
import BaseModal from '../../components/BaseModal.vue'
import ClusterAgentGate from '../../components/ClusterAgentGate.vue'
import EmptyState from '../../components/EmptyState.vue'
import FeedbackBanner from '../../components/FeedbackBanner.vue'
import SectionTabsHeader from '../../components/SectionTabsHeader.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import SurfaceCard from '../../components/SurfaceCard.vue'
import { formatBytes } from '../../utils/formatters.js'

const props = defineProps({ clusterID: { type: String, required: true } })
const tabs = [{ id: 'claims', label: '存储卷' }]
const cluster = ref(null)
const namespace = ref('')
const namespaces = ref([])
const storageClasses = ref([])
const claims = ref([])
const references = ref(new Map())
const usage = ref(new Map())
const referencesLoading = ref(false)
const referencesLoaded = ref(false)
const loaded = ref(false)
const loading = ref(false)
const error = ref('')
const actionError = ref('')
const page = ref(1)
const pageSize = 20
const total = ref(0)
const creating = ref(false)
const saving = ref(false)
const deleteTarget = ref(null)
const form = ref({ namespace: '', name: '', size: 5, unit: 'Gi', storageClass: '' })
const scope = computed(() => String(props.clusterID))
let requestSequence = 0

watch(scope, async id => {
  cluster.value = null
  namespace.value = ''
  page.value = 1
  loaded.value = false
  error.value = ''
  try {
    const selectedCluster = await getCluster(id)
    if (id !== scope.value) return
    cluster.value = selectedCluster
  } catch (err) { if (id === scope.value) { error.value = err.message || '加载集群失败'; loaded.value = true } }
}, { immediate: true })

async function loadStorage() {
  const id = scope.value
  try {
    const [names, classes] = await Promise.all([getClusterStorageNamespaces(id), getClusterStorageClasses(id)])
    if (id !== scope.value) return
    namespaces.value = (names || []).map(item => typeof item === 'string' ? item : item.name).filter(Boolean).sort()
    storageClasses.value = classes || []
    await loadClaims()
  } catch (err) { if (id === scope.value) { error.value = err.message || '加载存储卷失败'; loaded.value = true } }
}
watch(namespace, () => {
  if (page.value !== 1) page.value = 1
  else if (cluster.value) void loadClaims()
})
watch(page, () => { if (cluster.value) void loadClaims() })

async function loadClaims() {
  const id = scope.value
  const request = ++requestSequence
  loading.value = true
  error.value = ''
  try {
    const result = await getClusterPersistentVolumeInventory(id, { page: page.value, size: pageSize, namespace: namespace.value })
    if (id !== scope.value || request !== requestSequence) return
    claims.value = result.items || []
    total.value = result.total || 0
    loaded.value = true
    references.value = new Map()
    referencesLoaded.value = false
    usage.value = new Map()
    if (!claims.value.length) { referencesLoaded.value = true; return }
    referencesLoading.value = true
    try {
      const [referenceResult, usageResult] = await Promise.allSettled([
        getClusterPersistentVolumeReferences(id, claims.value),
        getClusterPersistentVolumeUsage(id, claims.value),
      ])
      if (id !== scope.value || request !== requestSequence) return
      if (referenceResult.status === 'fulfilled') {
        references.value = new Map((referenceResult.value || []).map(item => [`${item.namespace}/${item.name}`, item.references || []]))
        referencesLoaded.value = true
      }
      else error.value = referenceResult.reason?.message || '读取存储卷引用失败'
      if (usageResult.status === 'fulfilled') usage.value = new Map((usageResult.value || []).map(item => [`${item.namespace}/${item.name}`, item]))
    } catch (err) { if (id === scope.value) error.value = err.message || '读取存储卷详情失败' }
    finally { if (request === requestSequence) referencesLoading.value = false }
  } catch (err) { if (id === scope.value && request === requestSequence) { error.value = err.message || '加载存储卷失败'; loaded.value = true } }
  finally { if (id === scope.value && request === requestSequence) loading.value = false }
}
function referenceCount(claim) { return references.value.get(`${claim.namespace}/${claim.name}`)?.length || 0 }
function usageFor(claim) { return usage.value.get(`${claim.namespace}/${claim.name}`) }
function canDelete(claim) { return claim.managed && !claim.environment_id && claim.owner_type !== 'infrastructure' }
function openCreate() { actionError.value = ''; form.value = { namespace: namespace.value || namespaces.value[0] || '', name: '', size: 5, unit: 'Gi', storageClass: '' }; creating.value = true }
async function saveClaim() {
  saving.value = true
  actionError.value = ''
  try {
    await createClusterPersistentVolumeClaim(scope.value, { namespace: form.value.namespace, name: form.value.name, storage: `${form.value.size}${form.value.unit}`, storage_class_name: form.value.storageClass })
    creating.value = false
    await loadClaims()
  } catch (err) { actionError.value = err.message || '创建存储卷失败' }
  finally { saving.value = false }
}
async function removeClaim() {
  if (!deleteTarget.value) return
  saving.value = true
  actionError.value = ''
  try {
    await deleteClusterPersistentVolumeClaim(scope.value, deleteTarget.value.name, { namespace: deleteTarget.value.namespace, confirm_data_delete: true })
    deleteTarget.value = null
    await loadClaims()
  } catch (err) { actionError.value = err.message || '删除存储卷失败' }
  finally { saving.value = false }
}
</script>

<style scoped>
.cluster-storage-page { min-width: 0; }
.back-link { display: inline-flex; align-items: center; gap: 6px; margin-bottom: var(--space-16); color: var(--text-secondary); font-size: 13px; font-weight: 600; text-decoration: none; }
.back-link:hover { color: var(--action-primary); }
.storage-content { margin-top: var(--tabbed-page-content-gap); }
.storage-error { margin-top: var(--space-12); }
.storage-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 8px; min-height: 62px; padding: 12px var(--space-20); }
.namespace-select { width: 190px; }
.toolbar-actions, .size-control, .pagination { display: flex; align-items: center; gap: 8px; }
.size-control .form-input { flex: 1; min-width: 0; }
.storage-content .table-wrap { padding: 0 var(--space-20) var(--space-12); }
.pagination { justify-content: flex-end; padding: var(--space-12) var(--space-20); }
.is-spinning { animation: spin .8s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
@media (max-width: 640px) { .storage-toolbar { padding: 12px 14px; }.namespace-select { width: min(45%, 190px); }.storage-content .table-wrap { padding: 0 14px 12px; } }
</style>
