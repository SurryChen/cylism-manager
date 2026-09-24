<template>
  <div class="configs-workspace">
    <TabbedWorkspaceCard>
      <template #meta>
        <div class="table-tabs" role="tablist" aria-label="配置类型">
          <button :class="['tab-btn', { 'tab-active': activeTab === 'configmaps' }]" type="button" role="tab" :aria-selected="activeTab === 'configmaps'" @click="selectTab('configmaps')">ConfigMaps</button>
          <button :class="['tab-btn', { 'tab-active': activeTab === 'secrets' }]" type="button" role="tab" :aria-selected="activeTab === 'secrets'" @click="selectTab('secrets')">Secrets</button>
        </div>
        <SelectMenu v-if="activeTab === 'secrets'" v-model="secretNamespace" class="secret-namespace-filter" aria-label="筛选 Secret 命名空间" @change="selectTab('secrets')"><option value="">全部命名空间</option><option v-for="item in namespaces" :key="item.name" :value="item.name">{{ item.name }}</option></SelectMenu>
        <span class="resource-count">{{ resourceCountLabel }}</span>
      </template>
      <template #actions>
        <button class="icon-button" type="button" title="刷新配置" aria-label="刷新配置" :disabled="loadingTab" @click="selectTab(activeTab)"><RefreshCw :size="16" :class="{ 'is-spinning': loadingTab }" /></button>
        <button class="btn btn-primary" type="button" data-testid="create-config-resource" @click="openCreate"><Plus :size="15" /> 新建 {{ activeTab === 'configmaps' ? 'ConfigMap' : 'Secret' }}</button>
      </template>

      <EmptyState v-if="loadingTab && !resources.length" message="正在读取配置..." variant="loading" />
      <EmptyState v-else-if="!resources.length" :message="`暂无 ${activeTab === 'configmaps' ? 'ConfigMap' : 'Secret'}`" />
      <div v-else class="table-wrap config-table-wrap">
        <table class="data-table config-table">
          <thead><tr><th>名称</th><th>命名空间</th><th v-if="activeTab === 'configmaps'">键数量</th><th>年龄</th><th class="action-cell">操作</th></tr></thead>
          <tbody><tr v-for="resource in resources" :key="resourceKey(resource)">
            <td class="cell-primary"><OverflowTooltip class="config-cell-truncate" :text="resource.name || '-'" /></td>
            <td><OverflowTooltip class="config-cell-truncate" :text="resource.namespace || '-'" /></td>
            <td v-if="activeTab === 'configmaps'">{{ keyCount(resource) }}</td><td>{{ resource.age || '-' }}</td>
            <td class="action-cell"><div class="config-row-actions"><button class="btn btn-sm" type="button" :data-testid="`view-config-resource-${resourceKey(resource)}`" @click="openDetail(resource)">查看</button><button class="icon-button" type="button" title="编辑资源" aria-label="编辑资源" @click="openEdit(resource)"><Pencil :size="16" /></button><button class="icon-button danger" type="button" title="删除资源" aria-label="删除资源" @click="removeResource(resource)"><Trash2 :size="16" /></button></div></td>
          </tr></tbody>
        </table>
      </div>
      <div v-if="activeTab === 'secrets' && secretContinue" class="secret-pagination"><button class="btn" type="button" :disabled="loadingTab" @click="loadMoreSecrets">{{ loadingTab ? '正在读取...' : '加载更多' }}</button></div>
    </TabbedWorkspaceCard>

    <BaseModal :open="showDetail" :title="detailTitle" size="large" @close="closeDetail">
      <div v-if="selectedResource" class="config-detail">
        <div v-if="detailLoading" class="detail-empty">正在读取配置详情...</div>
        <template v-else>
          <section v-if="activeTab === 'secrets'" class="detail-section secret-summary"><h3>基本信息</h3><dl><div><dt>类型</dt><dd>{{ detail?.type || '-' }}</dd></div><div><dt>键数量</dt><dd>{{ detailItems.length }}</dd></div></dl></section>
          <section class="detail-section"><h3>数据项</h3><div class="table-wrap detail-table-wrap"><table class="data-table detail-table"><thead><tr><th>键</th><th>值</th></tr></thead><tbody><tr v-for="item in detailItems" :key="item.key"><td class="cell-primary"><OverflowTooltip class="detail-cell" :text="item.key" /></td><td><OverflowTooltip class="detail-cell detail-code" :text="item.value" /></td></tr><tr v-if="!detailItems.length"><td colspan="2" class="detail-empty-cell">暂无数据项</td></tr></tbody></table></div></section>
          <section class="detail-section"><h3>引用工作负载</h3><div v-if="references.length" class="table-wrap detail-table-wrap"><table class="data-table detail-table"><thead><tr><th>类型</th><th>名称</th><th>命名空间</th></tr></thead><tbody><tr v-for="reference in references" :key="`${reference.kind}/${reference.namespace}/${reference.name}`"><td>{{ reference.kind || '-' }}</td><td class="cell-primary"><OverflowTooltip class="detail-cell" :text="reference.name || '-'" /></td><td><OverflowTooltip class="detail-cell" :text="reference.namespace || selectedResource.namespace || '-'" /></td></tr></tbody></table></div><p v-else class="detail-empty-copy">暂无工作负载引用</p></section>
        </template>
      </div>
    </BaseModal>

    <BaseModal :open="showEditor" :title="`${editing ? '编辑' : '新建'} ${resourceKind}`" size="large" @close="closeEditor">
      <p class="form-hint">{{ activeTab === 'secrets' ? 'Secret 值不会再次显示；编辑时留空会保留对应 key 的当前值。' : '修改会立即影响引用该 ConfigMap 的工作负载。' }}</p>
      <form class="resource-modal" @submit.prevent="saveResource">
        <div class="form-row"><div class="form-group"><label class="form-label">命名空间</label><input v-if="namespacesLoading" class="form-input" value="" placeholder="正在读取命名空间..." disabled /><SelectMenu v-else-if="namespaces.length" v-model="resourceForm.namespace" class="form-select" required><option value="" disabled>选择命名空间</option><option v-for="item in namespaces" :key="item.name" :value="item.name">{{ item.name }}</option></SelectMenu><input v-else v-model.trim="resourceForm.namespace" class="form-input" required /></div><div class="form-group"><label class="form-label">名称</label><input v-model.trim="resourceForm.name" class="form-input" required :disabled="editing" placeholder="app-config" /></div></div>
        <div class="form-group"><div class="resource-heading"><label class="form-label">数据项</label><button type="button" class="btn btn-sm" @click="addDataItem">+ 添加数据项</button></div><div class="key-value-list"><div v-for="(item, index) in resourceForm.data" :key="index" class="key-value-row"><input v-model.trim="item.key" class="form-input" required placeholder="Key" /><input v-model="item.value" :type="activeTab === 'secrets' ? 'password' : 'text'" class="form-input" :required="!editing || activeTab === 'configmaps'" :placeholder="activeTab === 'secrets' && editing ? '留空保持不变' : '值'" /><button type="button" class="icon-button" title="移除数据项" aria-label="移除数据项" @click="removeDataItem(index)"><Trash2 :size="16" /></button></div></div></div>
      </form>
      <template #actions><button type="button" class="btn" @click="closeEditor">取消</button><button class="btn btn-primary" :disabled="saving" type="button" @click="saveResource">{{ saving ? '保存中...' : '保存' }}</button></template>
    </BaseModal>
    <ErrorNoticeModal :open="Boolean(activeError)" :title="errorTitle" :message="activeError" @close="clearErrors" />
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { Pencil, Plus, RefreshCw, Trash2 } from 'lucide-vue-next'
import { createConfigMap, createSecret, deleteConfigMap, deleteSecret, getConfigMap, getConfigMapsForNamespace, getNamespaceNames, getSecret, getSecretMetadataPage, updateConfigMap, updateSecret } from '../../api/kubernetes.js'
import { useAsyncResource } from '../../composables/useAsyncResource.js'
import BaseModal from '../../components/BaseModal.vue'
import EmptyState from '../../components/EmptyState.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import OverflowTooltip from '../../components/OverflowTooltip.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import TabbedWorkspaceCard from '../../components/TabbedWorkspaceCard.vue'

const activeTab = ref('configmaps')
const configmaps = ref([])
const secrets = ref([])
const namespaces = ref([])
const secretNamespace = ref('')
const secretContinue = ref('')
const listResource = useAsyncResource(({ signal }, tab, continueToken = '') => tab === 'configmaps' ? getConfigMapsForNamespace('', { signal }) : getSecretMetadataPage(secretNamespace.value, { continueToken, signal }), null)
const detailResource = useAsyncResource(({ signal }, tab, namespace, name) => tab === 'configmaps' ? getConfigMap(namespace, name, { signal }) : getSecret(namespace, name, { signal }), null)
const loadingTab = listResource.loading
const namespacesLoading = ref(false)
const saving = ref(false)
const listError = ref('')
const detailError = ref('')
const mutationError = ref('')
const selectedResource = ref(null)
const detail = ref(null)
const showDetail = ref(false)
const showEditor = ref(false)
const editing = ref(false)
const resourceForm = ref(newResourceForm())

const resources = computed(() => activeTab.value === 'configmaps' ? configmaps.value : secrets.value)
const resourceCountLabel = computed(() => activeTab.value === 'secrets' ? `本页 ${resources.value.length} 条` : `${resources.value.length} 条`)
const resourceKind = computed(() => activeTab.value === 'configmaps' ? 'ConfigMap' : 'Opaque Secret')
const detailLoading = detailResource.loading
const detailTitle = computed(() => selectedResource.value ? `${activeTab.value === 'configmaps' ? 'ConfigMap' : 'Secret'} · ${selectedResource.value.namespace}/${selectedResource.value.name}` : '配置详情')
const detailItems = computed(() => {
  if (!selectedResource.value) return []
  if (activeTab.value === 'secrets') return Object.keys(detail.value?.data || {}).map(key => ({ key, value: '已隐藏' }))
  return Object.entries(detail.value?.data || {}).map(([key, value]) => ({ key, value: String(value ?? '') }))
})
const references = computed(() => detail.value?.used_by || [])
const activeError = computed(() => mutationError.value || detailError.value || listError.value)
const errorTitle = computed(() => mutationError.value ? '操作失败' : detailError.value ? '读取详情失败' : '读取配置失败')

function newResourceForm() { return { namespace: '', name: '', data: [{ key: '', value: '' }] } }
function resourceKey(resource) { return `${resource.namespace}/${resource.name}` }
function keyCount(resource) { return resource.keys_count ?? resource.keys?.length ?? 0 }
function dataMap() { return resourceForm.value.data.reduce((result, item) => { if (item.key) result[item.key] = item.value; return result }, {}) }
function addDataItem() { resourceForm.value.data.push({ key: '', value: '' }) }
function removeDataItem(index) { resourceForm.value.data.splice(index, 1); if (!resourceForm.value.data.length) addDataItem() }
function clearErrors() { listError.value = ''; detailError.value = ''; mutationError.value = '' }

onMounted(() => { void selectTab('configmaps') })
async function loadNamespaces() {
  namespacesLoading.value = true
  try { namespaces.value = await getNamespaceNames() || [] } catch (_) { namespaces.value = [] } finally { namespacesLoading.value = false }
}
async function selectTab(tab) {
  activeTab.value = tab
  closeDetail()
  listError.value = ''
  if (tab === 'secrets') {
    secretContinue.value = ''
    secrets.value = []
    if (!namespaces.value.length && !namespacesLoading.value) void loadNamespaces()
  }
  const result = await listResource.refresh(tab)
  if (!result) { if (listResource.error.value) listError.value = listResource.error.value.message || '加载失败，请检查集群连接'; return }
  if (tab === 'configmaps') configmaps.value = result || []
  else { secrets.value = result.items || []; secretContinue.value = result.continue || '' }
}
async function loadMoreSecrets() {
  if (!secretContinue.value || loadingTab.value) return
  listError.value = ''
  const result = await listResource.refresh('secrets', secretContinue.value)
  if (!result) { if (listResource.error.value) listError.value = listResource.error.value.message || '加载失败，请检查集群连接'; return }
  secrets.value = [...secrets.value, ...(result.items || [])]
  secretContinue.value = result.continue || ''
}
async function openDetail(resource) {
  selectedResource.value = resource
  detail.value = null
  detailError.value = ''
  showDetail.value = true
  const tab = activeTab.value
  const loaded = await detailResource.refresh(tab, resource.namespace, resource.name)
  if (loaded && selectedResource.value === resource) detail.value = loaded
  else if (detailResource.error.value) detailError.value = detailResource.error.value.message || `读取${tab === 'configmaps' ? 'ConfigMap' : 'Secret'}失败`
}
function closeDetail() { detailResource.cancel(); showDetail.value = false; selectedResource.value = null; detail.value = null; detailError.value = '' }
async function openCreate() {
  editing.value = false
  resourceForm.value = newResourceForm()
  showEditor.value = true
  await loadNamespaces()
  if (namespaces.value.length === 1) resourceForm.value.namespace = namespaces.value[0].name
}
async function openEdit(resource) {
  editing.value = true
  resourceForm.value = { namespace: resource.namespace, name: resource.name, data: [] }
  detailError.value = ''
  if (activeTab.value === 'configmaps') {
    const loaded = await detailResource.refresh('configmaps', resource.namespace, resource.name)
    if (!loaded) { detailError.value = detailResource.error.value?.message || '读取 ConfigMap 失败'; return }
    resourceForm.value.data = Object.entries(loaded.data || {}).map(([key, value]) => ({ key, value }))
  } else {
    const loaded = await detailResource.refresh('secrets', resource.namespace, resource.name)
    if (!loaded) { detailError.value = detailResource.error.value?.message || '读取 Secret 失败'; return }
    if (loaded.type !== 'Opaque') { mutationError.value = '仅支持编辑 Opaque Secret'; return }
    resourceForm.value.data = Object.keys(loaded.data || {}).map(key => ({ key, value: '' }))
  }
  if (!resourceForm.value.data.length) addDataItem()
  showEditor.value = true
}
function closeEditor() { showEditor.value = false; resourceForm.value = newResourceForm() }
async function saveResource() {
  saving.value = true
  mutationError.value = ''
  try {
    const payload = { namespace: resourceForm.value.namespace, name: resourceForm.value.name, data: dataMap() }
    if (activeTab.value === 'configmaps') { if (editing.value) await updateConfigMap(payload.namespace, payload.name, { data: payload.data }); else await createConfigMap(payload) }
    else if (editing.value) await updateSecret(payload.namespace, payload.name, { data: payload.data })
    else await createSecret(payload)
    closeEditor()
    await selectTab(activeTab.value)
  } catch (cause) { mutationError.value = cause.message || '保存资源失败' } finally { saving.value = false }
}
async function removeResource(resource) {
  mutationError.value = ''
  try {
    if (activeTab.value === 'secrets') {
      const loaded = await detailResource.refresh('secrets', resource.namespace, resource.name)
      if (!loaded) { detailError.value = detailResource.error.value?.message || '读取 Secret 失败'; return }
      if (loaded.type !== 'Opaque') { mutationError.value = '仅支持删除 Opaque Secret'; return }
    }
    if (!window.confirm(`删除 ${resource.name} 后无法恢复，是否继续？`)) return
    if (activeTab.value === 'configmaps') await deleteConfigMap(resource.namespace, resource.name)
    else await deleteSecret(resource.namespace, resource.name)
    await selectTab(activeTab.value)
  } catch (cause) { mutationError.value = cause.message || '删除资源失败' }
}
</script>

<style scoped>
.configs-workspace { margin-top: var(--space-20); }
.resource-count { color: var(--text-secondary); font-size: 12px; font-variant-numeric: tabular-nums; }
.secret-namespace-filter { width: 156px; flex: 0 0 156px; }
.secret-namespace-filter :deep(.select-menu) { width: 100%; }
.config-table { min-width: 760px; table-layout: fixed; }
.config-cell-truncate { max-width: 180px; }
.action-cell { white-space: nowrap; }
.config-row-actions { display: flex; align-items: center; justify-content: flex-end; gap: 6px; }
.icon-button.danger:not(:disabled) { color: var(--danger); }
.config-detail { display: grid; gap: var(--space-20); }
.detail-section { display: grid; gap: var(--space-8); }
.detail-section h3 { margin: 0; color: var(--text-primary); font-size: 13px; }
.secret-summary dl { display: flex; gap: var(--space-24); margin: 0; }.secret-summary dl > div { display: grid; gap: 4px; }.secret-summary dt { color: var(--text-muted); font-size: 12px; }.secret-summary dd { margin: 0; color: var(--text-primary); font-size: 13px; }
.detail-table-wrap { padding: 0; }
.detail-table { min-width: 500px; table-layout: fixed; }
.detail-cell { max-width: 300px; }
.detail-code { color: var(--text-secondary); font-family: var(--font-mono); font-size: 11px; }
.detail-empty, .detail-empty-cell, .detail-empty-copy { color: var(--text-muted); font-size: 12px; text-align: center; }
.detail-empty { padding: var(--space-24) 0; }
.detail-empty-cell { padding: var(--space-16); }
.detail-empty-copy { margin: 0; padding: var(--space-12) 0; text-align: left; }
.form-hint { margin: 0 0 var(--space-16); color: var(--text-secondary); font-size: 13px; line-height: 1.5; }
.resource-heading { display: flex; align-items: center; justify-content: space-between; gap: var(--space-12); margin-bottom: var(--space-12); }
.resource-heading .form-label { margin: 0; }
.key-value-list { display: grid; gap: 8px; }
.key-value-row { display: grid; grid-template-columns: minmax(0, .8fr) minmax(0, 1.4fr) 30px; gap: 8px; padding: 8px; border: 1px solid var(--border-muted); border-radius: var(--radius-control); background: var(--surface-subtle); }
.secret-pagination { display: flex; justify-content: center; padding-top: var(--space-16); }
@media(max-width:640px) { .secret-namespace-filter { width: min(100%, 220px); flex-basis: min(100%, 220px); }.config-row-actions { justify-content:flex-start; }.key-value-row { grid-template-columns:1fr 1fr 30px; } }
</style>
