<template>
  <SurfaceCard class="cluster-list-card" padding="none">
    <div class="cluster-list-toolbar">
      <span class="cell-secondary">共 {{ clusters.length }} 个集群</span>
      <div class="cluster-list-actions">
        <button class="icon-button" type="button" title="刷新集群列表" aria-label="刷新集群列表" :disabled="loading" @click="load">
          <RefreshCw :size="15" :class="{ 'is-spinning': loading }" />
        </button>
        <button class="btn btn-primary" type="button" @click="openCreate">新建集群</button>
      </div>
    </div>
    <FeedbackBanner v-if="error" class="cluster-list-error" tone="warning" :message="error" />
    <EmptyState v-if="!loaded" variant="loading" message="加载中..." />
    <EmptyState v-else-if="!clusters.length && !error" message="暂无集群" />
    <div v-else-if="clusters.length" class="table-wrap">
      <table class="data-table">
        <thead><tr><th>集群名称</th><th>类型</th><th>状态</th><th>创建时间</th><th class="action-cell">操作</th></tr></thead>
        <tbody>
          <tr v-for="cluster in clusters" :key="cluster.id">
            <td class="cell-primary"><RouterLink class="cluster-name-link" :to="`/cluster/${cluster.id}`">{{ cluster.name }}</RouterLink></td>
            <td>{{ distributionLabel(cluster.distribution) }}</td>
            <td><span class="badge" :class="statusClass(cluster.status)">{{ statusLabel(cluster.status) }}</span></td>
            <td>{{ formatTime(cluster.created_at) }}</td>
            <td class="action-cell"><div class="cluster-row-actions">
              <button class="icon-button icon-button--table" type="button" :title="`编辑 ${cluster.name}`" :aria-label="`编辑 ${cluster.name}`" @click="openEdit(cluster)"><Pencil :size="14" /></button>
              <button class="icon-button icon-button--table" type="button" :title="`删除 ${cluster.name}`" :aria-label="`删除 ${cluster.name}`" @click="openDelete(cluster)"><Trash2 :size="14" /></button>
              <RouterLink class="icon-button icon-button--table" :to="`/cluster/${cluster.id}`" :title="`打开 ${cluster.name}`" :aria-label="`打开 ${cluster.name}`"><ArrowRight :size="14" /></RouterLink>
            </div></td>
          </tr>
        </tbody>
      </table>
    </div>
  </SurfaceCard>
  <BaseModal :open="creating" title="新建集群" size="small" @close="closeCreate">
    <form id="create-cluster-form" @submit.prevent="saveCluster">
      <div class="form-group">
        <label class="form-label" for="cluster-name">集群名称</label>
        <input id="cluster-name" v-model.trim="name" class="form-input" maxlength="128" required placeholder="例如 生产集群" />
      </div>
      <div class="form-group">
        <label class="form-label" for="cluster-distribution">集群类型</label>
        <SelectMenu id="cluster-distribution" v-model="distribution" :options="distributionOptions" placeholder="请选择集群类型" aria-label="集群类型" required />
        <p class="cluster-type-hint">目前仅支持 K3s；K8s 将在后续开放。</p>
      </div>
      <FeedbackBanner v-if="createError" tone="warning" :message="createError" />
    </form>
    <template #actions>
      <button class="btn" type="button" :disabled="saving" @click="closeCreate">取消</button>
      <button class="btn btn-primary" type="submit" form="create-cluster-form" :disabled="saving || !name.trim() || distribution !== 'k3s'">{{ saving ? '创建中...' : '创建集群' }}</button>
    </template>
  </BaseModal>
  <BaseModal :open="Boolean(editingCluster)" title="编辑集群" size="small" @close="closeEdit">
    <form id="edit-cluster-form" @submit.prevent="saveEdit">
      <div class="form-group"><label class="form-label" for="edit-cluster-name">集群名称</label><input id="edit-cluster-name" v-model.trim="editName" class="form-input" maxlength="128" required /></div>
      <p class="cluster-type-hint">集群类型创建后不可更改。</p>
      <FeedbackBanner v-if="editError" tone="warning" :message="editError" />
    </form>
    <template #actions><button class="btn" type="button" :disabled="savingEdit" @click="closeEdit">取消</button><button class="btn btn-primary" type="submit" form="edit-cluster-form" :disabled="savingEdit || !editName.trim()">{{ savingEdit ? '保存中...' : '保存修改' }}</button></template>
  </BaseModal>
  <BaseModal :open="Boolean(deletingCluster)" title="删除集群" size="small" @close="closeDelete">
    <p>确定删除集群“{{ deletingCluster?.name }}”吗？请先在集群拓扑中卸载全部节点。任务审计记录会保留。</p>
    <FeedbackBanner v-if="deleteError" tone="warning" :message="deleteError" />
    <template #actions><button class="btn" type="button" :disabled="deleting" @click="closeDelete">取消</button><button class="btn btn-danger" type="button" :disabled="deleting" @click="confirmDelete">{{ deleting ? '删除中...' : '删除集群' }}</button></template>
  </BaseModal>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ArrowRight, Pencil, RefreshCw, Trash2 } from 'lucide-vue-next'
import { createCluster, deleteCluster, getClusters, updateCluster } from '../../api/clusters.js'
import { useAsyncResource } from '../../composables/useAsyncResource.js'
import BaseModal from '../../components/BaseModal.vue'
import EmptyState from '../../components/EmptyState.vue'
import FeedbackBanner from '../../components/FeedbackBanner.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import SurfaceCard from '../../components/SurfaceCard.vue'
import { formatShortDateTime as formatTime } from '../../utils/formatters.js'

const clusters = ref([])
const loaded = ref(false)
const error = ref('')
const creating = ref(false)
const name = ref('')
const distribution = ref('')
const distributionOptions = [
  { value: 'k3s', label: 'K3s', description: '当前支持' },
  { value: 'kubernetes', label: 'K8s', description: '暂未支持', disabled: true },
]
const saving = ref(false)
const createError = ref('')
const editingCluster = ref(null)
const editName = ref('')
const editError = ref('')
const savingEdit = ref(false)
const deletingCluster = ref(null)
const deleteError = ref('')
const deleting = ref(false)
const router = useRouter()
const resource = useAsyncResource(({ signal }) => getClusters({ signal }), [])
const loading = resource.loading

onMounted(() => { void load() })

async function load() {
  error.value = ''
  const result = await resource.refresh()
  if (result) clusters.value = result
  else if (resource.error.value) error.value = resource.error.value.message || '加载集群列表失败'
  loaded.value = true
}

function openCreate() {
  name.value = ''
  distribution.value = ''
  createError.value = ''
  creating.value = true
}

function closeCreate() {
  if (saving.value) return
  creating.value = false
  createError.value = ''
}

async function saveCluster() {
  const trimmedName = name.value.trim()
  if (!trimmedName || distribution.value !== 'k3s' || saving.value) return
  saving.value = true
  createError.value = ''
  try {
    const cluster = await createCluster(trimmedName, distribution.value)
    creating.value = false
    await router.push(`/cluster/${cluster.id}`)
  } catch (err) {
    createError.value = err.message || '创建集群失败'
  } finally {
    saving.value = false
  }
}

function openEdit(cluster) { editingCluster.value = cluster; editName.value = cluster.name; editError.value = '' }
function closeEdit() { if (!savingEdit.value) editingCluster.value = null }
async function saveEdit() {
  if (!editingCluster.value || !editName.value.trim() || savingEdit.value) return
  savingEdit.value = true
  editError.value = ''
  try {
    const updated = await updateCluster(editingCluster.value.id, editName.value.trim())
    clusters.value = clusters.value.map(item => item.id === updated.id ? updated : item)
    editingCluster.value = null
  } catch (err) { editError.value = err.message || '修改集群失败' }
  finally { savingEdit.value = false }
}

function openDelete(cluster) { deletingCluster.value = cluster; deleteError.value = '' }
function closeDelete() { if (!deleting.value) deletingCluster.value = null }
async function confirmDelete() {
  if (!deletingCluster.value || deleting.value) return
  deleting.value = true
  deleteError.value = ''
  try {
    const id = deletingCluster.value.id
    await deleteCluster(id)
    clusters.value = clusters.value.filter(item => item.id !== id)
    deletingCluster.value = null
  } catch (err) { deleteError.value = err.message || '删除集群失败' }
  finally { deleting.value = false }
}

function distributionLabel(value) {
  return ({ k3s: 'K3s', kubernetes: 'K8s' })[value] || value || '-'
}

function statusLabel(value) {
  return ({ draft: '待部署', deploying: '部署中', ready: '运行中', failed: '异常' })[value] || value || '-'
}

function statusClass(value) {
  return ({ deploying: 'badge-deploying', ready: 'badge-online', failed: 'badge-danger' })[value] || 'badge-offline'
}
</script>

<style scoped>
.cluster-list-toolbar { display: flex; align-items: center; justify-content: space-between; min-height: 62px; padding: 12px var(--space-20); }
.cluster-list-actions { display: flex; align-items: center; gap: 8px; }
.cluster-name-link { color: var(--text-primary); font-weight: 700; text-decoration: none; }
.cluster-name-link:hover { color: var(--action-primary); text-decoration: underline; }
.cluster-name-link:focus-visible { outline: 2px solid var(--focus); outline-offset: 2px; }
.cluster-row-actions { display: flex; justify-content: flex-end; gap: 4px; }
.cluster-list-card > .table-wrap { padding: 0 var(--space-20) var(--space-12); }
.cluster-list-error { margin: 0 var(--space-20) var(--space-12); }
.cluster-type-hint { margin: 6px 0 0; color: var(--text-muted); font-size: 11px; }
.cluster-list-card > .empty-state { padding: var(--space-24); }
@media (max-width: 760px) {
  .cluster-list-toolbar { padding: 12px 14px; }
  .cluster-list-card > .table-wrap { padding-right: 14px; padding-left: 14px; }
  .cluster-list-error { margin-right: 14px; margin-left: 14px; }
}
</style>
