<template>
  <div class="storage-page" @click="advancedFiltersOpen = false; actionMenuKey = ''">
    <SectionTabsHeader title="存储卷" :tabs="pageTabs" active-tab="claims" test-id-prefix="storage-page" />
    <div v-if="error" class="k8s-banner k8s-banner-warn section-gap">{{ error }}</div>
    <div v-if="workflowError && (showCreate || deleteTarget || migrationTarget || cleanupTarget || backupTarget || importTarget)" class="k8s-banner k8s-banner-warn section-gap">{{ workflowError }}</div>
    <TabbedWorkspaceCard class="storage-workspace">
      <template #meta>
        <section class="storage-context" aria-label="存储卷筛选">
          <div class="storage-picker"><SelectMenu v-model="namespaceFilter" data-testid="storage-namespace-filter" class="form-select"><option value="">全部命名空间</option><option v-for="namespace in namespaces" :key="namespace" :value="namespace">{{ namespace }}</option></SelectMenu></div>
          <div class="storage-picker"><SelectMenu v-model.number="projectID" data-testid="storage-project-filter" class="form-select"><option :value="0">全部项目</option><option v-for="project in projects" :key="project.id" :value="project.id">{{ project.name }}</option></SelectMenu></div>
          <div class="storage-picker"><SelectMenu v-model.number="environmentID" data-testid="storage-environment-filter" class="form-select" :disabled="!selectedProject"><option :value="0">全部环境</option><option v-for="environment in selectedProject?.environments || []" :key="environment.id" :value="environment.id">{{ environment.name }} · {{ environment.namespace }}</option></SelectMenu></div>
        </section>
      </template>
      <template #actions>
        <div class="storage-filter-control" @click.stop>
          <button class="btn" :class="{ 'is-active': activeSecondaryFilterCount }" type="button" data-testid="storage-open-filters" :aria-expanded="advancedFiltersOpen" aria-controls="storage-secondary-filters" @click="advancedFiltersOpen = !advancedFiltersOpen"><Filter :size="15" />筛选<span v-if="activeSecondaryFilterCount" class="filter-count">{{ activeSecondaryFilterCount }}</span></button>
          <div v-if="advancedFiltersOpen" id="storage-secondary-filters" class="storage-filter-popover" @click.stop>
            <label class="storage-filter-field"><span class="form-label">状态</span><SelectMenu v-model="phaseFilter" data-testid="storage-phase-filter" class="form-select"><option value="">全部状态</option><option v-for="phase in phases" :key="phase" :value="phase">{{ phase }}</option></SelectMenu></label>
            <label class="storage-filter-field"><span class="form-label">StorageClass</span><SelectMenu v-model="storageClassFilter" data-testid="storage-class-filter" class="form-select"><option value="">全部 StorageClass</option><option v-for="storageClass in claimStorageClasses" :key="storageClass" :value="storageClass">{{ storageClass }}</option></SelectMenu></label>
          </div>
        </div>
        <button v-if="hasFilters" class="icon-button" type="button" title="重置筛选" aria-label="重置筛选" @click="resetFilters"><RotateCcw :size="16" /></button>
        <button class="icon-button" type="button" title="刷新已用空间" aria-label="刷新已用空间" :disabled="usageLoading" @click="loadUsage"><RefreshCw :size="16" :class="{ 'is-spinning': usageLoading }" /></button>
        <button class="btn btn-primary" type="button" @click="openCreate">创建存储卷</button>
      </template>
    <div v-if="loaded && !filteredClaims.length" class="empty-state"><span class="empty-icon">▣</span><span class="empty-text">没有匹配筛选条件的存储卷</span></div>
      <div v-else class="table-wrap">
        <table class="data-table storage-table">
          <thead><tr><th>存储卷</th><th>命名空间</th><th>归属</th><th>容量</th><th>已用空间</th><th>StorageClass</th><th>状态</th><th>绑定节点</th><th>引用</th><th>操作</th></tr></thead>
          <tbody><tr v-for="claim in filteredClaims" :key="claimKey(claim)"><td class="cell-primary"><span class="storage-cell-text" :title="claim.name">{{ claim.name }}</span></td><td><span class="storage-cell-text" :title="claim.namespace">{{ claim.namespace }}</span></td><td><span class="badge" :class="claim.owner_type === 'infrastructure' ? 'badge-deploying' : (claim.managed ? 'badge-online' : 'badge-offline')" :title="ownerDetail(claim)">{{ claim.owner_type === 'infrastructure' ? '基础设施' : (claim.managed ? '平台托管' : '外部创建') }}</span></td><td>{{ claim.storage || '-' }}</td><td><span v-if="usageFor(claim)?.status === 'available'" :title="usageDetail(usageFor(claim))">{{ formatBytes(usageFor(claim).used_bytes) }}</span><span v-else class="storage-cell-text" :title="usageFor(claim)?.message">{{ usageFor(claim)?.message || (usageLoading ? '读取中...' : '暂不可用') }}</span></td><td><span class="storage-cell-text" :title="claim.storage_class_name || '默认 StorageClass'">{{ claim.storage_class_name || '默认 StorageClass' }}</span></td><td><span class="badge" :class="claim.phase === 'Bound' ? 'badge-online' : 'badge-deploying'" :title="migrationFor(claim) ? migrationLabel(migrationFor(claim).status) : claim.phase || 'Pending'">{{ claim.phase || 'Pending' }}</span></td><td><span v-if="claim.bound_node_display_name" class="storage-cell-text" :title="claim.bound_node">{{ claim.bound_node_display_name }}</span><span v-else-if="claim.wait_for_first_consumer">首次挂载时决定</span><span v-else>-</span></td><td><span class="storage-cell-text" :title="claim.references?.join('、') || '无引用'">{{ referencesLoading ? '读取中...' : referenceLabel(claim) }}</span></td><td><a v-if="claim.owner_type === 'infrastructure'" class="btn btn-sm monitoring-link" :href="infrastructureLink(claim)">{{ infrastructureActionLabel(claim) }}</a><div v-else-if="claim.managed" class="claim-actions"><button v-if="hasManagedEnvironment(claim) && claim.is_local && claim.bound_node" class="btn btn-sm" @click="openBackup(claim)">备份</button><div class="claim-action-menu" @click.stop><button class="icon-button" type="button" title="更多存储卷操作" aria-label="更多存储卷操作" :aria-expanded="actionMenuKey === claimKey(claim)" @click="actionMenuKey = actionMenuKey === claimKey(claim) ? '' : claimKey(claim)"><MoreHorizontal :size="16" /></button><div v-if="actionMenuKey === claimKey(claim)" class="claim-action-menu-items"><button v-if="hasManagedEnvironment(claim) && claim.is_local && claim.bound_node && !migrationFor(claim)" type="button" @click="openMigration(claim); actionMenuKey = ''"><ArrowRightLeft :size="14" />迁移</button><button v-if="hasManagedEnvironment(claim) && claim.is_local && claim.bound_node && !migrationFor(claim)" type="button" data-testid="open-directory-import" @click="openImport(claim); actionMenuKey = ''"><FolderInput :size="14" />导入目录</button><button v-if="migrationFor(claim)?.status === 'cleanup_pending'" type="button" class="is-danger" @click="requestCleanup(migrationFor(claim)); actionMenuKey = ''"><Trash2 :size="14" />清理源卷</button><button type="button" class="is-danger" :disabled="referencesLoading || claim.references?.length || !!migrationFor(claim)" @click="requestDelete(claim); actionMenuKey = ''"><Trash2 :size="14" />删除</button></div></div></div><span v-else>-</span></td></tr></tbody>
        </table>
      </div>
      <div v-if="total > pageSize" class="pagination storage-pagination"><button class="btn btn-sm" type="button" :disabled="page === 1 || !loaded" @click="previousPage">上一页</button><span class="pagination-status">{{ page }} / {{ totalPages }}</span><button class="btn btn-sm" type="button" :disabled="page >= totalPages || !loaded" @click="nextPage">下一页</button></div>
    </TabbedWorkspaceCard>

    <div v-if="showCreate" class="overlay" @click.self="showCreate = false"><div class="modal"><h2 class="modal-title">创建存储卷</h2><form @submit.prevent="createClaim"><div class="form-group"><label class="form-label">命名空间</label><SelectMenu v-model="form.namespace" data-testid="storage-create-namespace" class="form-select" required><option value="" disabled>选择目标命名空间</option><option v-if="form.namespace&&!namespaces.includes(form.namespace)" :value="form.namespace">{{ form.namespace }}（尚未创建）</option><option v-for="namespace in namespaces" :key="namespace" :value="namespace">{{ namespace }}</option></SelectMenu><p class="form-hint">存储卷属于命名空间。应用挂载时需与该存储卷位于同一命名空间。</p><div v-if="form.namespace&&!namespaces.includes(form.namespace)" class="form-hint namespace-create-hint"><span>该命名空间尚不存在。</span><button type="button" data-testid="create-missing-namespace" class="btn btn-sm" :disabled="saving" @click="createNamespace">创建命名空间</button></div></div><div class="form-group"><label class="form-label">名称</label><input v-model.trim="form.name" class="form-input" required pattern="[a-z0-9]([-a-z0-9]*[a-z0-9])?" placeholder="karakeep-data" /></div><div class="form-group"><label class="form-label">容量</label><div class="quantity-input"><input v-model.number="form.storage_value" data-testid="storage-capacity-value" type="number" min="1" step="1" class="form-input" required placeholder="5" /><SelectMenu v-model="form.storage_unit" data-testid="storage-capacity-unit" class="form-select"><option value="Mi">Mi</option><option value="Gi">Gi</option><option value="Ti">Ti</option></SelectMenu></div><p class="form-hint">首期仅支持 ReadWriteOnce，挂载该卷的应用只能运行一个副本。</p></div><div class="form-group"><label class="form-label">StorageClass</label><SelectMenu v-model="form.storage_class_name" data-testid="storage-create-storage-class" class="form-select"><option value="">使用默认 StorageClass</option><option v-for="item in storageClasses" :key="item.name" :value="item.name">{{ item.name }}{{ item.is_default ? '（默认）' : '' }} · {{ item.volume_binding_mode || 'Immediate' }}</option></SelectMenu></div><div class="modal-actions"><button type="button" class="btn" @click="showCreate = false">取消</button><button class="btn btn-primary" :disabled="saving || !validStorage || !namespaces.includes(form.namespace)">{{ saving ? '创建中...' : '创建存储卷' }}</button></div></form></div></div>
    <div v-if="deleteTarget" class="overlay" @click.self="deleteTarget = null"><div class="modal"><h2 class="modal-title">删除存储卷</h2><p class="confirm-copy">将删除 PVC“{{ deleteTarget.name }}”。{{ deleteTarget.reclaim_policy === 'Delete' ? '该 PV 的回收策略为 Delete，底层数据可能一并被清除。' : '底层数据的保留行为由 PV 回收策略决定。' }}</p><div class="modal-actions"><button class="btn" @click="deleteTarget = null">取消</button><button class="btn btn-danger" :disabled="saving" @click="deleteClaim">确认删除</button></div></div></div>
    <div v-if="migrationTarget" class="overlay" @click.self="migrationTarget = null"><div class="modal"><h2 class="modal-title">迁移本地存储卷</h2><p class="confirm-copy">应用会在复制数据时短暂停止。目标工作负载就绪后，源卷将保留，待确认后再清理。</p><div class="form-group"><label class="form-label">目标节点</label><SelectMenu v-model="migrationNode" class="form-select" required><option value="" disabled>选择不同于当前绑定节点的服务器</option><option v-for="node in migrationNodes" :key="node.name" :value="node.name">{{ node.display_name || node.name }} · {{ node.name }}</option></SelectMenu></div><div class="modal-actions"><button class="btn" @click="migrationTarget = null">取消</button><button class="btn btn-primary" :disabled="saving || !migrationNode" @click="createMigration">开始迁移</button></div></div></div>
    <div v-if="cleanupTarget" class="overlay" @click.self="cleanupTarget = null"><div class="modal"><h2 class="modal-title">清理源存储卷</h2><p class="confirm-copy">目标工作负载已就绪。删除源 PVC 后，{{ cleanupTarget.source_pvc_name }} 的底层数据将按其 PV 回收策略处理，此操作不可恢复。</p><div class="modal-actions"><button class="btn" @click="cleanupTarget = null">取消</button><button class="btn btn-danger" :disabled="saving" @click="cleanupMigration">确认清理</button></div></div></div>
    <div v-if="backupTarget" class="overlay" @click.self="backupTarget = null"><div class="modal"><h2 class="modal-title">备份 {{ backupTarget.name }}</h2><p class="confirm-copy">备份会从绑定节点复制本地卷数据到指定服务器目录。</p><form @submit.prevent="createBackup"><div class="form-group"><label class="form-label">备份服务器</label><SelectMenu v-model.number="backupForm.backup_server_id" class="form-select" required><option :value="0" disabled>选择服务器</option><option v-for="server in backupServers" :key="server.id" :value="server.id">{{ server.name }} · {{ server.host }}</option></SelectMenu></div><div class="form-group"><label class="form-label">备份根目录</label><input v-model.trim="backupForm.backup_root" class="form-input" placeholder="/data/cylism-backups" required /></div><div v-if="backups.length" class="backup-list"><div v-for="backup in backups" :key="backup.id" class="backup-row"><span>{{ backup.backup_path }}</span><small>{{ backup.status }} · {{ backup.bytes || 0 }} bytes<span v-if="backup.restore_status"> · 恢复{{ backup.restore_status }}</span></small><button v-if="backup.status === 'succeeded' && backup.restore_status !== 'running'" type="button" class="btn btn-sm btn-danger" @click="restoreBackup(backup)">恢复</button></div></div><div class="modal-actions"><button type="button" class="btn" @click="backupTarget = null">关闭</button><button class="btn btn-primary" :disabled="saving">{{ saving ? '处理中...' : '开始备份' }}</button></div></form></div></div>
    <div v-if="importTarget" class="overlay" @click.self="importTarget = null"><div class="modal modal-wide"><h2 class="modal-title">导入宿主机目录到 {{ importTarget.name }}</h2><p class="confirm-copy">平台会停止引用该卷的应用，创建本地归档后复制并校验数据。源目录和 PVC 可位于不同节点；完成校验并恢复应用后，归档才可删除。</p><form class="import-form" @submit.prevent="createImport"><div class="form-group"><label class="form-label">源服务器</label><SelectMenu v-model.number="importForm.source_server_id" data-testid="import-source-server" class="form-select" required><option :value="0" disabled>选择已登记的 SSH 服务器</option><option v-for="server in backupServers" :key="server.id" :value="server.id">{{ server.name }} · {{ server.host }}</option></SelectMenu></div><div class="form-group"><label class="form-label">源目录</label><input v-model.trim="importForm.source_path" data-testid="import-source-path" class="form-input" required placeholder="/data/legacy/karakeep" /><p class="form-hint">不允许系统目录。归档由平台保存在源服务器或 PVC 节点的 /data/cylism-import-backups。</p></div><label class="check-row"><input v-model="importForm.confirm_data_replace" data-testid="import-confirm-replace" type="checkbox" />我确认若目标 PVC 已有数据，可先备份并覆盖</label><div v-if="imports.length" class="import-list"><div v-for="task in imports" :key="task.id" class="import-row"><div><strong>{{ importLabel(task.status) }}</strong><span>{{ task.source_path }} → {{ task.target_path }}</span><small v-if="task.detail">{{ task.detail }}</small><small v-if="task.source_checksum && task.target_checksum">校验摘要：{{ task.target_checksum.slice(0, 12) }}...</small></div><button v-if="task.status === 'succeeded' && task.verified_at && !task.backup_deleted_at" type="button" class="btn btn-sm btn-danger" @click="deleteImportBackup(task)">删除备份</button><small v-else-if="task.backup_deleted_at">备份已删除</small></div></div><div class="modal-actions"><button type="button" class="btn" @click="importTarget = null">关闭</button><button class="btn btn-primary" :disabled="saving || !importForm.source_server_id || !importForm.source_path">{{ saving ? '处理中...' : '开始导入' }}</button></div></form></div></div>
  </div>
</template>

<script setup>
import { computed, onUnmounted, ref, watch } from 'vue'
import { ArrowRightLeft, Filter, FolderInput, MoreHorizontal, RefreshCw, RotateCcw, Trash2 } from 'lucide-vue-next'
import SectionTabsHeader from '../../components/SectionTabsHeader.vue'
import TabbedWorkspaceCard from '../../components/TabbedWorkspaceCard.vue'
import { getProjects } from '../../api/applications.js'
import { getClusterNodes } from '../../api/cluster.js'
import { getNamespaceNames } from '../../api/kubernetes.js'
import { getServers } from '../../api/servers.js'
import {
  cleanupPersistentVolumeMigration,
  createNamespace as createNamespaceRequest,
  createPersistentVolumeBackup,
  createPersistentVolumeClaim,
  createPersistentVolumeImport,
  createPersistentVolumeMigration,
  deletePersistentVolumeClaim,
  deletePersistentVolumeImportBackup,
  getPersistentVolumeBackups,
  getPersistentVolumeImports,
  getPersistentVolumeInventory,
  getPersistentVolumeMigrations,
	getPersistentVolumeReferences,
  getPersistentVolumeStorageClasses,
  getPersistentVolumeUsage,
  restorePersistentVolumeBackup,
} from '../../api/storage.js'
import { useAsyncResource } from '../../composables/useAsyncResource.js'
import { usePolling } from '../../composables/usePolling.js'
import { formatBytes } from '../../utils/formatters.js'

const projects = ref([])
const pageTabs = [{ id: 'claims', label: '存储卷' }]
const claims = ref([])
const usage = ref([])
const usageLoading = ref(false)
const storageClasses = ref([])
const migrations = ref([])
const nodes = ref([])
const backupServers = ref([])
const backups = ref([])
const imports = ref([])
const namespaces = ref([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const projectID = ref(0)
const environmentID = ref(0)
const namespaceFilter = ref('')
const phaseFilter = ref('')
const storageClassFilter = ref('')
const advancedFiltersOpen = ref(false)
const actionMenuKey = ref('')
const loaded = ref(false)
const error = ref('')
const workflowError = ref('')
const saving = ref(false)
const showCreate = ref(false)
const deleteTarget = ref(null)
const migrationTarget = ref(null)
const migrationNode = ref('')
const cleanupTarget = ref(null)
const backupTarget = ref(null)
const backupForm = ref({ backup_server_id: 0, backup_root: '/data/cylism-backups' })
const importTarget = ref(null)
const importForm = ref({ source_server_id: 0, source_path: '', confirm_data_replace: false })
const form = ref(newClaimForm())
const migrationPolling = usePolling(loadClaims, { interval: 2500 })
const backupPolling = usePolling(refreshBackups, { interval: 2500 })
const importPolling = usePolling(refreshImports, { interval: 2500 })
const createQueryHandled = ref(false)
const inventoryResource = useAsyncResource(({ signal }, filters) => getPersistentVolumeInventory(filters, { signal }), null)
const usageResource = useAsyncResource(({ signal }, targetClaims) => getPersistentVolumeUsage(targetClaims, { signal }), [])
const referencesResource = useAsyncResource(({ signal }, targetClaims) => getPersistentVolumeReferences(targetClaims, { signal }), [])
const referencesLoading = referencesResource.loading
const migrationsResource = useAsyncResource(({ signal }) => getPersistentVolumeMigrations({ signal }), [])
const storageClassesResource = useAsyncResource(({ signal }) => getPersistentVolumeStorageClasses({ signal }), [])
const nodesResource = useAsyncResource(({ signal }) => getClusterNodes({ signal }), [])
const serversResource = useAsyncResource(({ signal }) => getServers({ signal }), [])
const namespacesResource = useAsyncResource(({ signal }) => getNamespaceNames({ signal }), [])
const backupsResource = useAsyncResource(({ signal }, name, targetEnvironmentID) => getPersistentVolumeBackups(name, targetEnvironmentID, { signal }), [])
const importsResource = useAsyncResource(({ signal }, name, targetEnvironmentID) => getPersistentVolumeImports(name, targetEnvironmentID, { signal }), [])
const selectedProject = computed(() => projects.value.find(project => project.id === projectID.value) || null)
const validStorage = computed(() => Boolean(form.value.namespace) && Number.isInteger(form.value.storage_value) && form.value.storage_value > 0 && ['Mi', 'Gi', 'Ti'].includes(form.value.storage_unit))
const migrationNodes = computed(() => nodes.value.filter(node => node.name && node.name !== migrationTarget.value?.bound_node && (node.status === 'Ready' || node.status === 'ready')))
const phases = computed(() => [...new Set(claims.value.map(claim => claim.phase).filter(Boolean))].sort())
const claimStorageClasses = computed(() => [...new Set(claims.value.map(claim => claim.storage_class_name).filter(Boolean))].sort())
const filteredClaims = computed(() => claims.value.filter(claim => {
  if (phaseFilter.value && claim.phase !== phaseFilter.value) return false
  return !storageClassFilter.value || claim.storage_class_name === storageClassFilter.value
}))
const hasFilters = computed(() => Boolean(namespaceFilter.value || projectID.value || environmentID.value || phaseFilter.value || storageClassFilter.value))
const activeSecondaryFilterCount = computed(() => Number(Boolean(phaseFilter.value)) + Number(Boolean(storageClassFilter.value)))
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))

function newClaimForm() { return { namespace: namespaceFilter.value || namespaces.value[0] || '', name: '', storage_value: 5, storage_unit: 'Gi', storage_class_name: '' } }
function claimKey(claim) { return `${claim.namespace}/${claim.name}` }
function resetFilters() { namespaceFilter.value = ''; projectID.value = 0; environmentID.value = 0; phaseFilter.value = ''; storageClassFilter.value = '' }
function hasManagedEnvironment(claim) { return claim.managed && Number(claim.environment_id) > 0 }
function claimEnvironmentID(claim) { return Number(claim?.environment_id) || 0 }
function infrastructureLink(claim) { return claim.owner === 'oci-registry' ? '#/delivery/registry' : '#/monitoring' }
function infrastructureActionLabel(claim) { return claim.owner === 'oci-registry' ? '查看制品库' : '查看监控' }
function usageFor(claim) { return usage.value.find(item => item.namespace === claim.namespace && item.name === claim.name) }
function ownerDetail(claim) { return claim.owner_name || [claim.project_name, claim.environment_name].filter(Boolean).join(' · ') || '未关联应用环境' }
function usageDetail(item) { const capacity = Number(item?.capacity_bytes) || 0; if (!capacity) return `已用 ${formatBytes(item?.used_bytes)}`; return `已用 ${formatBytes(item.used_bytes)} / ${formatBytes(capacity)}（${Math.min(100, Math.round((Number(item.used_bytes) / capacity) * 100))}%）` }
function referenceLabel(claim) { const count = claim.references?.length || 0; return count ? `${count} 个引用` : '-' }
async function loadProjects() { try { projects.value = await getProjects() || [] } catch (e) { error.value = e.message || '加载项目失败' } }
async function loadClaims() {
  loaded.value = false
  error.value = ''
  const result = await inventoryResource.refresh({ page: page.value, size: pageSize, namespace: namespaceFilter.value, project_id: projectID.value || undefined, environment_id: environmentID.value || undefined })
  if (!result) {
    error.value = inventoryResource.error.value?.message || '加载存储卷失败'
    loaded.value = true
    return
  }
  const inventory = Array.isArray(result) ? { items: result, total: result.length } : result
  claims.value = inventory.items || []
  total.value = inventory.total || 0
  namespaces.value = [...new Set([...namespaces.value, ...claims.value.map(item => item.namespace).filter(Boolean)])].sort()
  applyCreateQuery()
  loaded.value = true
  void loadMigrations()
	void loadReferences()
  void loadUsage()
}
async function loadUsage() { usageLoading.value = true; const result = await usageResource.refresh(claims.value.map(claim => ({ namespace: claim.namespace, name: claim.name }))); if (result) usage.value = result || []; else usage.value = []; usageLoading.value = false }
async function loadReferences() { const result = await referencesResource.refresh(claims.value.map(claim => ({ namespace: claim.namespace, name: claim.name }))); if (!result) return; const byClaim = new Map(result.map(item => [`${item.namespace}/${item.name}`, item.references || []])); claims.value = claims.value.map(claim => ({ ...claim, references: byClaim.get(`${claim.namespace}/${claim.name}`) || [] })) }
async function loadMigrations() { const result = await migrationsResource.refresh(); if (result) { migrations.value = result || []; syncMigrationPolling() } }
async function loadCreateOptions() {
  const [classes, namespaceList] = await Promise.all([storageClassesResource.refresh(), namespacesResource.refresh()])
  if (classes) storageClasses.value = classes || []
  if (namespaceList) namespaces.value = namespaceList.map(item => item.name).filter(Boolean).sort()
}
async function loadNodes() { const result = await nodesResource.refresh(); if (result) nodes.value = result || [] }
async function loadServers() { const result = await serversResource.refresh(); if (result) backupServers.value = result || [] }
async function openCreate() { workflowError.value = ''; form.value = newClaimForm(); showCreate.value = true; void loadCreateOptions() }
function applyCreateQuery() { if (createQueryHandled.value || typeof window === 'undefined') return; const query = new URLSearchParams(window.location.hash.split('?')[1] || ''); if (query.get('create') !== '1') return; createQueryHandled.value = true; const storage = query.get('storage') || '100Gi'; const matched = storage.match(/^(\d+)(Mi|Gi|Ti)$/); form.value = { namespace: query.get('namespace') || newClaimForm().namespace, name: query.get('name') || '', storage_value: matched ? Number(matched[1]) : 100, storage_unit: matched ? matched[2] : 'Gi', storage_class_name: query.get('storage_class_name') || 'local-path' }; showCreate.value = true; void loadCreateOptions() }
async function createNamespace() { if (!form.value.namespace || namespaces.value.includes(form.value.namespace)) return; saving.value = true; workflowError.value = ''; try { await createNamespaceRequest({ name: form.value.namespace }); namespaces.value = [...namespaces.value, form.value.namespace].sort() } catch (e) { workflowError.value = e.message || '创建命名空间失败' } finally { saving.value = false } }
async function createClaim() { if (!validStorage.value) return; saving.value = true; workflowError.value = ''; try { const { storage_value, storage_unit, ...claim } = form.value; await createPersistentVolumeClaim({ ...claim, storage: `${storage_value}${storage_unit}` }); showCreate.value = false; await loadClaims() } catch (e) { workflowError.value = e.message || '创建存储卷失败' } finally { saving.value = false } }
function requestDelete(claim) { workflowError.value = ''; deleteTarget.value = claim }
function migrationFor(claim) { return migrations.value.find(item => item.environment_id === claimEnvironmentID(claim) && item.source_pvc_name === claim.name && item.status !== 'cleaned') || null }
function migrationLabel(status) { return ({ pending: '等待预检', preflight: '预检中', provisioning_target: '预配目标卷', stopping_source: '停止源工作负载', copying: '复制数据中', cutover: '切换中', waiting_ready: '等待就绪', cleanup_pending: '等待清理源卷', failed: '迁移失败', rolled_back: '已回滚' })[status] || status }
function migrationRunning(task) { return !['cleanup_pending', 'failed', 'rolled_back', 'cleaned'].includes(task.status) }
function syncMigrationPolling() { if (migrations.value.some(migrationRunning)) migrationPolling.start(); else migrationPolling.stop() }
function stopMigrationPolling() { migrationPolling.stop() }
async function openMigration(claim) { workflowError.value = ''; migrationTarget.value = claim; migrationNode.value = ''; await loadNodes() }
async function createMigration() { if (!migrationTarget.value || !migrationNode.value) return; saving.value = true; workflowError.value = ''; try { await createPersistentVolumeMigration(migrationTarget.value.name, { environment_id: claimEnvironmentID(migrationTarget.value), target_node_name: migrationNode.value }); migrationTarget.value = null; await loadClaims() } catch (e) { workflowError.value = e.message || '创建存储卷迁移失败' } finally { saving.value = false } }
function requestCleanup(migration) { workflowError.value = ''; cleanupTarget.value = migration }
async function cleanupMigration() { if (!cleanupTarget.value) return; saving.value = true; workflowError.value = ''; try { await cleanupPersistentVolumeMigration(cleanupTarget.value.id); cleanupTarget.value = null; await loadClaims() } catch (e) { workflowError.value = e.message || '清理源存储卷失败' } finally { saving.value = false } }
function backupRunning(backup) { return ['accepted', 'running'].includes(backup.status) || backup.restore_status === 'running' }
function syncBackupPolling() { if (backupTarget.value && backups.value.some(backupRunning)) backupPolling.start(); else backupPolling.stop() }
function stopBackupPolling() { backupPolling.stop() }
async function refreshBackups() { const target = backupTarget.value; if (!target) return; const targetEnvironmentID = claimEnvironmentID(target); const result = await backupsResource.refresh(target.name, targetEnvironmentID); if (!backupTarget.value || backupTarget.value.name !== target.name || claimEnvironmentID(backupTarget.value) !== targetEnvironmentID) return; if (result) { backups.value = result || []; syncBackupPolling() } else { workflowError.value = backupsResource.error.value?.message || '读取备份记录失败'; stopBackupPolling() } }
async function openBackup(claim) { workflowError.value = ''; backupTarget.value = claim; await loadServers(); backupForm.value = { backup_server_id: backupServers.value[0]?.id || 0, backup_root: '/data/cylism-backups' }; await refreshBackups() }
async function createBackup() { if (!backupTarget.value) return; saving.value = true; workflowError.value = ''; try { await createPersistentVolumeBackup(backupTarget.value.name, { environment_id: claimEnvironmentID(backupTarget.value), ...backupForm.value }); await refreshBackups() } catch (e) { workflowError.value = e.message || '创建备份失败' } finally { saving.value = false } }
async function restoreBackup(backup) { if (!backupTarget.value || !window.confirm('恢复会覆盖当前 PVC 数据，并短暂停止引用它的工作负载，确定继续吗？')) return; saving.value = true; workflowError.value = ''; try { await restorePersistentVolumeBackup(backupTarget.value.name, backup.id, { environment_id: claimEnvironmentID(backupTarget.value), confirm_data_replace: true }); await refreshBackups() } catch (e) { workflowError.value = e.message || '恢复备份失败' } finally { saving.value = false } }
function importRunning(task) { return !['succeeded', 'failed'].includes(task.status) }
function importLabel(status) { return ({ pending: '等待执行', preflight: '预检中', stopping_workload: '停止应用中', backing_up: '创建备份中', copying: '复制数据中', verifying: '校验中', restoring_workload: '恢复应用中', succeeded: '导入完成', failed: '导入失败' })[status] || status }
function syncImportPolling() { if (importTarget.value && imports.value.some(importRunning)) importPolling.start(); else importPolling.stop() }
function stopImportPolling() { importPolling.stop() }
async function refreshImports() { const target = importTarget.value; if (!target) return; const targetEnvironmentID = claimEnvironmentID(target); const result = await importsResource.refresh(target.name, targetEnvironmentID); if (!importTarget.value || importTarget.value.name !== target.name || claimEnvironmentID(importTarget.value) !== targetEnvironmentID) return; if (result) { imports.value = result || []; syncImportPolling() } else { workflowError.value = importsResource.error.value?.message || '读取目录导入记录失败'; stopImportPolling() } }
async function openImport(claim) { workflowError.value = ''; importTarget.value = claim; await loadServers(); importForm.value = { source_server_id: backupServers.value[0]?.id || 0, source_path: '', confirm_data_replace: false }; await refreshImports() }
async function createImport() { if (!importTarget.value) return; saving.value = true; workflowError.value = ''; try { await createPersistentVolumeImport(importTarget.value.name, { environment_id: claimEnvironmentID(importTarget.value), ...importForm.value }); await refreshImports() } catch (e) { workflowError.value = e.message || '创建目录导入失败' } finally { saving.value = false } }
async function deleteImportBackup(task) { if (!importTarget.value || !window.confirm('删除后无法通过平台恢复本次导入前的数据，确定删除本地归档吗？')) return; saving.value = true; workflowError.value = ''; try { await deletePersistentVolumeImportBackup(importTarget.value.name, task.id, { environment_id: claimEnvironmentID(importTarget.value) }); await refreshImports() } catch (e) { workflowError.value = e.message || '删除导入备份失败' } finally { saving.value = false } }
async function deleteClaim() { if (!deleteTarget.value) return; saving.value = true; workflowError.value = ''; try { await deletePersistentVolumeClaim(deleteTarget.value.name, { environment_id: claimEnvironmentID(deleteTarget.value), namespace: deleteTarget.value.namespace, confirm_data_delete: true }); deleteTarget.value = null; await loadClaims() } catch (e) { workflowError.value = e.message || '删除存储卷失败' } finally { saving.value = false } }

watch(projectID, () => { const environments = selectedProject.value?.environments || []; if (!environments.some(item => item.id === environmentID.value)) environmentID.value = 0 })
watch([namespaceFilter, projectID, environmentID], () => { page.value = 1; void loadClaims() })
watch(backupTarget, target => { if (!target) { backupsResource.cancel(); stopBackupPolling() } })
watch(importTarget, target => { if (!target) { importsResource.cancel(); stopImportPolling() } })
onUnmounted(() => { stopMigrationPolling(); stopBackupPolling(); stopImportPolling(); backupsResource.cancel(); importsResource.cancel(); referencesResource.cancel() })
async function previousPage() { page.value = Math.max(1, page.value - 1); await loadClaims() }
async function nextPage() { page.value += 1; await loadClaims() }
loadProjects()
loadClaims()
</script>

<style scoped>
.storage-workspace { margin-top: var(--space-20); }
.storage-workspace :deep(.surface-card) { overflow: visible; }
.storage-workspace :deep(.tabbed-workspace-toolbar) { align-items: flex-end; }
.storage-context { display: flex; align-items: flex-end; flex-wrap: wrap; gap: 8px; width: 472px; margin: 0; padding: 0; border: 0; background: transparent; }
.storage-picker { min-width: 118px; flex: 0 1 152px; }
.storage-context .form-select { height: var(--button-height); min-height: var(--button-height); padding: 0 28px 0 9px; border-color: var(--border-muted); background: var(--surface-subtle); font-size: var(--button-font-size); }
.storage-context .form-select:hover:not(:disabled) { border-color: var(--focus); background: var(--surface-hover); }
.storage-filter-control { position: relative; z-index: 2; }
.storage-filter-popover { position: absolute; z-index: 40; top: calc(100% + 8px); right: 0; display: grid; width: 244px; gap: 12px; padding: 12px; border: 1px solid var(--border); border-radius: var(--radius-panel); background: var(--surface-raised); box-shadow: var(--shadow); }
.storage-filter-field { display: grid; gap: 5px; min-width: 0; }
.storage-filter-field .form-label { margin: 0; font-size: 11px; }
.storage-filter-popover :deep(.select-menu-options) { z-index: 50; }
.quantity-input { display: grid; grid-template-columns: minmax(0, 1fr) 88px; gap: 6px; }
.monitoring-link { text-decoration: none; }
.danger-action { color: var(--danger); }
.confirm-copy { margin: 0; color: var(--text-secondary); font-size: 13px; line-height: 1.6; }
.check-row { display: flex; align-items: flex-start; gap: 8px; margin: 12px 0; color: var(--text-secondary); font-size: 12px; line-height: 1.5; }
.namespace-create-hint { display: flex; align-items: center; gap: 8px; }
.modal-wide { width: min(620px, calc(100vw - 32px)); }
.backup-list, .import-list { display: grid; gap: 6px; margin: 12px 0; }
.backup-row, .import-row { display: grid; grid-template-columns: minmax(0, 1fr) auto; align-items: center; gap: 8px; padding: 8px; border: 1px solid var(--border-muted); border-radius: var(--radius-control); background: var(--surface-subtle); font-size: 11px; }
.backup-row span, .import-row span, .import-row small { overflow-wrap: anywhere; }
.backup-row small, .import-row small { color: var(--text-muted); }
.import-row > div { display: grid; gap: 3px; }
.import-row strong { font-size: 12px; }
.storage-table { min-width: 1230px; table-layout: fixed; }
.storage-table th, .storage-table td { padding-top: 7px; padding-bottom: 7px; }
.storage-table td { line-height: 16px; white-space: nowrap; }
.storage-table th:nth-child(1), .storage-table td:nth-child(1) { width: 148px; }
.storage-table th:nth-child(2), .storage-table td:nth-child(2) { width: 118px; }
.storage-table th:nth-child(3), .storage-table td:nth-child(3) { width: 90px; }
.storage-table th:nth-child(4), .storage-table td:nth-child(4) { width: 72px; }
.storage-table th:nth-child(5), .storage-table td:nth-child(5) { width: 92px; }
.storage-table th:nth-child(6), .storage-table td:nth-child(6) { width: 146px; }
.storage-table th:nth-child(7), .storage-table td:nth-child(7) { width: 82px; }
.storage-table th:nth-child(8), .storage-table td:nth-child(8) { width: 132px; }
.storage-table th:nth-child(9), .storage-table td:nth-child(9) { width: 82px; }
.storage-table th:last-child, .storage-table td:last-child { width: 128px; }
.claim-actions { display: flex; align-items: center; gap: 6px; }
.claim-action-menu { position: relative; }
.claim-action-menu-items { position: absolute; z-index: 4; top: calc(100% + 5px); right: 0; display: grid; min-width: 118px; overflow: hidden; border: 1px solid var(--border); border-radius: var(--radius-control); background: var(--surface-raised); box-shadow: var(--shadow); }
.claim-action-menu-items button { display: flex; align-items: center; gap: 7px; border: 0; padding: 8px 10px; background: transparent; color: var(--text-secondary); font: inherit; font-size: 12px; text-align: left; white-space: nowrap; cursor: pointer; }
.claim-action-menu-items button:hover:not(:disabled) { background: var(--surface-hover); color: var(--text-primary); }
.claim-action-menu-items button:disabled { opacity: .45; cursor: not-allowed; }
.claim-action-menu-items .is-danger { color: var(--danger); }
.storage-pagination { display: flex; align-items: center; justify-content: center; gap: 10px; padding: var(--space-16) 0 0; }
.storage-cell-text { display: block; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.filter-count { display: inline-flex; min-width: 16px; height: 16px; align-items: center; justify-content: center; border-radius: 50%; background: var(--action-primary); color: var(--action-contrast); font-size: 10px; line-height: 1; }
@media (max-width: 1100px) { .storage-picker { flex-basis: 118px; } }
@media (max-width: 640px) { .storage-context { display: grid; width: 100%; grid-template-columns: 1fr; }.storage-picker { min-width: 0; max-width: none; }.storage-filter-popover { width: min(244px, calc(100vw - 32px)); }.backup-row, .import-row { grid-template-columns: 1fr; } }
</style>
