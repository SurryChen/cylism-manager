<template>
  <div>
    <div class="page-header"><h1 class="page-title">配置</h1><button class="btn btn-sm btn-primary" @click="openCreate"><Plus :size="15" /> 新建 {{ activeTab === 'configmaps' ? 'ConfigMap' : 'Secret' }}</button></div>
    <div v-if="listError" class="k8s-banner k8s-banner-warn page-error">{{ listError }}</div>
    <div v-if="detailError" class="k8s-banner k8s-banner-warn page-error">{{ detailError }}</div>
    <div v-if="mutationError" class="k8s-banner k8s-banner-warn page-error">{{ mutationError }}</div>

    <div class="card section-gap">
      <div class="table-tabs">
        <button :class="['tab-btn', { 'tab-active': activeTab === 'configmaps' }]" @click="selectTab('configmaps')">ConfigMaps</button>
        <button :class="['tab-btn', { 'tab-active': activeTab === 'secrets' }]" @click="selectTab('secrets')">Secrets</button>
      </div>
    </div>

    <div class="card">
      <div v-if="loadingTab" class="empty-state"><span class="empty-text">加载中...</span></div>
      <div v-else-if="resources.length === 0" class="empty-state"><span class="empty-text">暂无 {{ activeTab === 'configmaps' ? 'ConfigMap' : 'Secret' }}</span></div>
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead><tr><th>名称</th><th>命名空间</th><th v-if="activeTab === 'secrets'">类型</th><th>键</th><th>被引用</th><th>年龄</th><th>操作</th></tr></thead>
          <tbody>
            <template v-for="resource in resources" :key="resource.namespace + '/' + resource.name">
              <tr class="clickable" @click="toggleExpand(resource)">
                <td class="cell-primary">{{ resource.name }}</td><td>{{ resource.namespace }}</td>
                <td v-if="activeTab === 'secrets'"><span class="badge badge-deploying">{{ resource.type }}</span></td>
                <td>{{ (resource.keys || []).join(', ') || '-' }}</td>
                <td>{{ (resource.used_by || []).map(item => `${item.kind}/${item.name}`).join(', ') || '-' }}</td><td>{{ resource.age }}</td>
                <td class="action-cell" @click.stop><button class="icon-button" title="编辑资源" :disabled="!isEditable(resource)" @click="openEdit(resource)"><Pencil :size="16" /></button><button class="icon-button danger" title="删除资源" :disabled="!isEditable(resource) || (resource.used_by || []).length > 0" @click="removeResource(resource)"><Trash2 :size="16" /></button></td>
              </tr>
              <tr v-if="expandedKey === resourceKey(resource)" class="detail-row">
                <td :colspan="activeTab === 'secrets' ? 7 : 6"><div class="detail-panel">
                  <template v-if="activeTab === 'configmaps'"><div v-for="(value, key) in detail.data" :key="key" class="detail-grid"><span class="detail-label">{{ key }}</span><span class="detail-value">{{ value }}</span></div></template>
                  <template v-else><div v-for="key in resource.keys" :key="key" class="detail-grid"><span class="detail-label">{{ key }}</span><span class="detail-value secret-value">已隐藏</span></div></template>
                </div></td>
              </tr>
            </template>
          </tbody>
        </table>
      </div>
    </div>

    <Teleport to="body"><div v-if="showEditor" class="overlay" @click.self="closeEditor"><div class="modal resource-modal"><h2 class="modal-title">{{ editing ? '编辑' : '新建' }} {{ resourceKind }}</h2><p class="form-hint">{{ activeTab === 'secrets' ? 'Secret 值不会再次显示；编辑时留空会保留对应 key 的当前值。' : '修改会立即影响引用该 ConfigMap 的工作负载。' }}</p><form @submit.prevent="saveResource">
      <div class="form-row"><div class="form-group"><label class="form-label">命名空间</label><select v-if="namespaces.length" v-model="resourceForm.namespace" class="form-select" required><option value="" disabled>选择命名空间</option><option v-for="namespace in namespaces" :key="namespace.name" :value="namespace.name">{{ namespace.name }}</option></select><input v-else v-model.trim="resourceForm.namespace" class="form-input" required /></div><div class="form-group"><label class="form-label">名称</label><input v-model.trim="resourceForm.name" class="form-input" required :disabled="editing" placeholder="app-config" /></div></div>
      <div class="form-group"><div class="resource-heading"><label class="form-label">数据项</label><button type="button" class="btn btn-sm" @click="addDataItem">+ 添加数据项</button></div><div class="key-value-list"><div v-for="(item, index) in resourceForm.data" :key="index" class="key-value-row"><input v-model.trim="item.key" class="form-input" required placeholder="Key" /><input v-model="item.value" :type="activeTab === 'secrets' ? 'password' : 'text'" class="form-input" :required="!editing || activeTab === 'configmaps'" :placeholder="activeTab === 'secrets' && editing ? '留空保持不变' : '值'" /><button type="button" class="icon-button" title="移除数据项" @click="removeDataItem(index)"><Trash2 :size="16" /></button></div></div></div>
      <div class="modal-actions"><button type="button" class="btn" @click="closeEditor">取消</button><button class="btn btn-primary" :disabled="saving">{{ saving ? '保存中...' : '保存' }}</button></div>
    </form></div></div></Teleport>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { Pencil, Plus, Trash2 } from 'lucide-vue-next'
import { createConfigMap, createSecret, deleteConfigMap, deleteSecret, getConfigMap, getConfigMaps, getNamespaceNames, getSecrets, updateConfigMap, updateSecret } from '../api/kubernetes.js'
import { useAsyncResource } from '../composables/useAsyncResource.js'

const activeTab = ref('configmaps')
const configmaps = ref([])
const secrets = ref([])
const namespaces = ref([])
const listResource = useAsyncResource(({ signal }, tab) => tab === 'configmaps' ? getConfigMaps({ signal }) : getSecrets({ signal }), null)
const detailResource = useAsyncResource(({ signal }, kind, namespace, name) => kind === 'configmap' ? getConfigMap(namespace, name, { signal }) : Promise.resolve(null), null)
const loadingTab = listResource.loading
const saving = ref(false)
const listError = ref('')
const detailError = ref('')
const mutationError = ref('')
const expandedKey = ref('')
const detail = ref({})
const showEditor = ref(false)
const editing = ref(false)
const resourceForm = ref(newResourceForm())

const resources = computed(() => activeTab.value === 'configmaps' ? configmaps.value : secrets.value)
const resourceKind = computed(() => activeTab.value === 'configmaps' ? 'ConfigMap' : 'Opaque Secret')

function newResourceForm() { return { namespace: '', name: '', data: [{ key: '', value: '' }] } }
function resourceKey(resource) { return `${resource.namespace}/${resource.name}` }
function isEditable(resource) { return activeTab.value === 'configmaps' || resource.type === 'Opaque' }
function dataMap() { return resourceForm.value.data.reduce((result, item) => { if (item.key) result[item.key] = item.value; return result }, {}) }
function addDataItem() { resourceForm.value.data.push({ key: '', value: '' }) }
function removeDataItem(index) { resourceForm.value.data.splice(index, 1); if (!resourceForm.value.data.length) addDataItem() }

onMounted(async () => { await Promise.all([loadNamespaces(), selectTab('configmaps')]) })
async function loadNamespaces() { try { namespaces.value = await getNamespaceNames() || [] } catch (_) { namespaces.value = [] } }
async function selectTab(tab) {
  activeTab.value = tab
  expandedKey.value = ''
  listError.value = ''
  const result = await listResource.refresh(tab)
  if (!result) {
    if (listResource.error.value) listError.value = listResource.error.value.message || '加载失败，请检查集群连接'
    return
  }
  if (tab === 'configmaps') configmaps.value = result || []
  else secrets.value = result || []
}
async function toggleExpand(resource) {
  const key = resourceKey(resource)
  if (expandedKey.value === key) { expandedKey.value = ''; detailResource.cancel(); return }
  expandedKey.value = key
  detailError.value = ''
  if (activeTab.value === 'configmaps') {
    const result = await detailResource.refresh('configmap', resource.namespace, resource.name)
    if (result && expandedKey.value === key) detail.value = result
    if (!result && detailResource.error.value) detailError.value = detailResource.error.value.message || '读取 ConfigMap 失败'
  }
}
function openCreate() { editing.value = false; resourceForm.value = newResourceForm(); if (namespaces.value.length === 1) resourceForm.value.namespace = namespaces.value[0].name; showEditor.value = true }
async function openEdit(resource) {
  editing.value = true
  resourceForm.value = { namespace: resource.namespace, name: resource.name, data: [] }
  detailError.value = ''
  if (activeTab.value === 'configmaps') {
    const loaded = await detailResource.refresh('configmap', resource.namespace, resource.name)
    if (!loaded) { detailError.value = detailResource.error.value?.message || '读取 ConfigMap 失败'; return }
    resourceForm.value.data = Object.entries(loaded.data || {}).map(([key, value]) => ({ key, value }))
  } else resourceForm.value.data = (resource.keys || []).map(key => ({ key, value: '' }))
  if (!resourceForm.value.data.length) addDataItem()
  showEditor.value = true
}
function closeEditor() { showEditor.value = false; resourceForm.value = newResourceForm() }
async function saveResource() {
  saving.value = true
  mutationError.value = ''
  try {
    const payload = { namespace: resourceForm.value.namespace, name: resourceForm.value.name, data: dataMap() }
    if (activeTab.value === 'configmaps') {
      if (editing.value) await updateConfigMap(payload.namespace, payload.name, { data: payload.data })
      else await createConfigMap(payload)
    } else if (editing.value) await updateSecret(payload.namespace, payload.name, { data: payload.data })
    else await createSecret(payload)
    closeEditor()
    await selectTab(activeTab.value)
  } catch (e) { mutationError.value = e.message || '保存资源失败' } finally { saving.value = false }
}
async function removeResource(resource) {
  if (!window.confirm(`删除 ${resource.name} 后无法恢复，是否继续？`)) return
  mutationError.value = ''
  try {
    if (activeTab.value === 'configmaps') await deleteConfigMap(resource.namespace, resource.name)
    else await deleteSecret(resource.namespace, resource.name)
    await selectTab(activeTab.value)
  } catch (e) { mutationError.value = e.message || '删除资源失败' }
}
</script>

<style scoped>
.page-header{display:flex;align-items:center;justify-content:space-between;gap:var(--space-16);margin-bottom:var(--space-16)}.page-header .btn{display:inline-flex;align-items:center;gap:6px}.page-error{margin-bottom:var(--space-16)}.clickable{cursor:pointer}.action-cell{display:flex;gap:6px}.icon-button.danger:not(:disabled){color:var(--danger)}.detail-row td{padding:8px 12px;border-bottom:0;background:var(--surface-subtle)}.detail-panel{padding:var(--space-8) 0}.detail-grid{display:grid;grid-template-columns:180px minmax(0,1fr);gap:var(--space-12);padding:6px 0}.detail-label{font-family:var(--font-mono);font-size:12px;font-weight:700}.detail-value{font-family:var(--font-mono);font-size:12px;overflow-wrap:anywhere}.secret-value{color:var(--text-secondary)}.resource-modal{width:min(680px,calc(100vw - 32px))}.resource-heading{display:flex;align-items:center;justify-content:space-between;gap:var(--space-12);margin-bottom:var(--space-12)}.resource-heading .form-label{margin:0}.key-value-list{display:grid;gap:8px}.key-value-row{display:grid;grid-template-columns:minmax(0,.8fr) minmax(0,1.4fr) 30px;gap:8px;padding:8px;border:1px solid var(--border-muted);border-radius:var(--radius-control);background:var(--surface-subtle)}@media(max-width:640px){.page-header{align-items:stretch;flex-direction:column}.page-header .btn{justify-content:center}.detail-grid{grid-template-columns:1fr}.key-value-row{grid-template-columns:1fr 1fr 30px}}
</style>
