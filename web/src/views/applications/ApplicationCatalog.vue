<template>
  <div class="catalog-page">
    <SectionTabsHeader title="应用商城" :tabs="[{ id: 'catalog', label: '应用目录' }]" active-tab="catalog">
      <template #actions><span class="catalog-count">共 {{ items.length }} 个应用</span></template>
    </SectionTabsHeader>
    <FeedbackBanner v-if="readError" tone="warning" :message="readError" />
    <SurfaceCard padding="none">
      <div class="catalog-toolbar">
        <div class="catalog-toolbar-actions">
          <button class="btn" @click="openSourceImport">导入应用</button>
          <button class="btn btn-primary" @click="showCreate = true"><Plus :size="15" />新增商城应用</button>
        </div>
      </div>
      <EmptyState v-if="loading" variant="loading" message="加载中..." />
      <EmptyState v-else-if="items.length === 0" message="商城中还没有应用" />
      <div v-else class="table-wrap">
        <table class="data-table catalog-table">
          <thead><tr><th>商城应用</th><th>部署组件</th><th>状态</th><th>配置版本</th><th class="catalog-actions">操作</th></tr></thead>
          <tbody><tr v-for="item in items" :key="item.id"><td><button class="catalog-name" @click="openDetail(item)">{{ item.name }}</button></td><td>{{ item.components.length }} 个{{ item.components.length ? ` · ${item.components.map(component => component.name).join('、')}` : '' }}</td><td><span class="badge" :class="item.enabled ? 'badge-online' : 'badge-offline'">{{ item.enabled ? '已启用' : '未启用' }}</span></td><td>v{{ item.revision }}</td><td class="catalog-actions"><button class="icon-button" type="button" title="编辑商城应用" :aria-label="`编辑 ${item.name}`" @click="openDetail(item)"><Pencil :size="15" /></button><button class="icon-button" type="button" title="删除商城应用" :aria-label="`删除 ${item.name}`" @click="deleteTarget = item"><Trash2 :size="15" /></button></td></tr></tbody>
        </table>
      </div>
    </SurfaceCard>

    <Teleport to="body"><div v-if="showCreate" class="overlay" @click.self="showCreate = false"><div class="modal"><h2 class="modal-title">新增商城应用</h2><form @submit.prevent="createItem">
      <div class="form-group"><label class="form-label">应用名称</label><input v-model.trim="createForm.name" class="form-input" required maxlength="128" placeholder="订单系统" /></div>
      <div class="form-group"><label class="form-label">应用标识</label><input v-model.trim="createForm.slug" class="form-input" required maxlength="63" pattern="[a-z0-9]([-a-z0-9]*[a-z0-9])?" placeholder="orders" /></div>
      <div class="form-group"><label class="form-label">说明</label><input v-model.trim="createForm.description" class="form-input" maxlength="512" placeholder="可选" /></div>
      <div class="modal-actions"><button type="button" class="btn" @click="showCreate = false">取消</button><button class="btn btn-primary" :disabled="saving">{{ saving ? '创建中...' : '创建并配置' }}</button></div>
    </form></div></div></Teleport>

    <Teleport to="body"><div v-if="showSourceImport" class="overlay" @click.self="showSourceImport = false"><div class="modal catalog-import"><h2 class="modal-title">导入现有应用</h2>
      <div class="catalog-import-fields">
        <div><label class="form-label">来源项目</label><SelectMenu :model-value="sourceProjectID" aria-label="来源项目" :options="projectOptions" placeholder="选择项目" @update:model-value="selectSourceProject" /></div>
        <div><label class="form-label">来源环境</label><SelectMenu :model-value="sourceEnvironmentID" aria-label="来源环境" :options="sourceEnvironmentOptions" placeholder="选择环境" :disabled="!sourceProjectID" @update:model-value="selectSourceEnvironment" /></div>
      </div>
      <div class="catalog-source-list">
        <span class="form-label">部署应用</span>
        <span v-if="sourceLoading" class="catalog-source-empty">加载中...</span>
        <span v-else-if="!sourceEnvironmentID" class="catalog-source-empty">请选择来源环境</span>
        <span v-else-if="sourceApplications.length === 0" class="catalog-source-empty">当前环境没有部署应用</span>
        <label v-for="app in sourceApplications" :key="app.id" class="catalog-import-option"><input v-model="sourceSelectedIDs" type="checkbox" :value="app.id" :disabled="!app.default_deployment_template_id || app.workload_kind === 'statefulset'" @change="syncSourceEntry" /><span>{{ app.name }}</span><span class="cell-secondary">{{ app.workload_kind === 'statefulset' ? '暂不支持 StatefulSet' : app.default_deployment_template_id ? '默认上线模板' : '未设置默认上线模板' }}</span></label>
      </div>
      <div class="catalog-source-fields">
        <div class="form-group"><label class="form-label">商城应用名称</label><input v-model.trim="sourceForm.name" class="form-input" maxlength="128" placeholder="例如：订单系统" /></div>
        <div class="form-group"><label class="form-label">应用标识</label><input v-model.trim="sourceForm.slug" class="form-input" maxlength="63" placeholder="例如：orders" /></div>
        <div class="form-group catalog-source-wide"><label class="form-label">入口应用</label><SelectMenu v-model.number="sourceEntryID" aria-label="入口应用" :options="sourceEntryOptions" placeholder="选择入口应用" :disabled="sourceSelectedIDs.length < 2" /></div>
      </div>
      <div class="modal-actions"><button class="btn" @click="showSourceImport = false">取消</button><button class="btn btn-primary" :disabled="saving || !sourceReady || sourceSelectedIDs.length < 2 || !sourceEntryID || !sourceForm.name || !validSourceSlug" @click="importFromEnvironment">{{ saving ? '导入中...' : '创建商城草稿' }}</button></div>
    </div></div></Teleport>

    <ConfirmDialog :open="!!deleteTarget" title="删除商城应用" :message="deleteTarget ? `确认删除“${deleteTarget.name}”吗？已导入项目的组件不受影响。` : ''" confirm-text="删除" :busy="saving" @cancel="deleteTarget = null" @confirm="deleteItem" />
    <ErrorNoticeModal :open="!!mutationError" :message="mutationError" teleport @close="mutationError = ''" />
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Pencil, Plus, Trash2 } from 'lucide-vue-next'
import { createApplicationCatalogFromEnvironment, createApplicationCatalogItem, deleteApplicationCatalogItem, getApplicationCatalog, getApplications, getProjects } from '../../api/applications.js'
import ConfirmDialog from '../../components/ConfirmDialog.vue'
import EmptyState from '../../components/EmptyState.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import FeedbackBanner from '../../components/FeedbackBanner.vue'
import SectionTabsHeader from '../../components/SectionTabsHeader.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import SurfaceCard from '../../components/SurfaceCard.vue'

const route = useRoute()
const router = useRouter()
const items = ref([])
const projects = ref([])
const loading = ref(true)
const saving = ref(false)
const readError = ref('')
const mutationError = ref('')
const showCreate = ref(false)
const showSourceImport = ref(false)
const sourceReady = ref(false)
const sourceLoading = ref(false)
const sourceProjectID = ref(Number(route.query.project_id) || 0)
const sourceEnvironmentID = ref(Number(route.query.environment_id) || 0)
const sourceApplications = ref([])
const sourceSelectedIDs = ref([])
const sourceEntryID = ref(0)
const sourceForm = ref({ name: '', slug: '', description: '' })
const deleteTarget = ref(null)
let sourceRequestID = 0
const createForm = ref({ name: '', slug: '', description: '' })
const projectOptions = computed(() => projects.value.map(item => ({ value: item.id, label: item.name })))
const sourceProject = computed(() => projects.value.find(item => item.id === sourceProjectID.value))
const sourceEnvironmentOptions = computed(() => (sourceProject.value?.environments || []).map(item => ({ value: item.id, label: item.name, description: item.namespace })))
const sourceEntryOptions = computed(() => sourceApplications.value.filter(app => sourceSelectedIDs.value.includes(app.id)).map(app => ({ value: app.id, label: app.name })))
const validSourceSlug = computed(() => /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/.test(sourceForm.value.slug))

function openDetail(item) { router.push(`/applications/catalog/${item.id}`) }
async function load() {
  loading.value = true; readError.value = ''
  try {
    const [catalog, projectList] = await Promise.all([getApplicationCatalog(), getProjects()])
    items.value = catalog || []
    projects.value = projectList || []
  } catch (error) { readError.value = error.message || '加载应用商城失败' }
  finally { loading.value = false }
}
async function createItem() {
  saving.value = true; mutationError.value = ''
  try {
    const item = await createApplicationCatalogItem(createForm.value)
    showCreate.value = false
    await router.push(`/applications/catalog/${item.id}`)
  } catch (error) { mutationError.value = error.message || '创建商城应用失败' }
  finally { saving.value = false }
}
async function deleteItem() {
  saving.value = true; mutationError.value = ''
  try { await deleteApplicationCatalogItem(deleteTarget.value.id); deleteTarget.value = null; await load() }
  catch (error) { mutationError.value = error.message || '删除商城应用失败' }
  finally { saving.value = false }
}
function selectSourceProject(value) {
  sourceProjectID.value = Number(value) || 0
  sourceEnvironmentID.value = 0
  sourceApplications.value = []
  sourceSelectedIDs.value = []
  sourceEntryID.value = 0
  sourceReady.value = false
  sourceLoading.value = false
  sourceRequestID++
}
function selectSourceEnvironment(value) {
  sourceEnvironmentID.value = Number(value) || 0
  sourceSelectedIDs.value = []
  sourceEntryID.value = 0
  loadSourceApplications()
}
function syncSourceEntry() {
  if (!sourceSelectedIDs.value.includes(sourceEntryID.value)) sourceEntryID.value = 0
}
async function loadSourceApplications() {
  const requestID = ++sourceRequestID
  sourceApplications.value = []
  sourceReady.value = false
  sourceLoading.value = false
  if (!sourceProjectID.value || !sourceEnvironmentID.value) return
  sourceLoading.value = true
  try {
    const applications = await getApplications({ projectID: sourceProjectID.value, environmentID: sourceEnvironmentID.value })
    if (requestID !== sourceRequestID) return
    sourceApplications.value = applications || []
    sourceReady.value = true
  } catch (error) {
    if (requestID === sourceRequestID) mutationError.value = error.message || '读取来源环境应用失败'
  } finally {
    if (requestID === sourceRequestID) sourceLoading.value = false
  }
}
function openSourceImport() {
  mutationError.value = ''
  sourceForm.value = { name: '', slug: '', description: '' }
  sourceSelectedIDs.value = []
  sourceEntryID.value = 0
  showSourceImport.value = true
  loadSourceApplications()
}
async function importFromEnvironment() {
  if (!sourceReady.value || sourceSelectedIDs.value.length < 2 || !sourceEntryID.value || !validSourceSlug.value) return
  saving.value = true; mutationError.value = ''
  try {
    const item = await createApplicationCatalogFromEnvironment({ project_id: sourceProjectID.value, environment_id: sourceEnvironmentID.value, application_ids: sourceSelectedIDs.value, entry_application_id: sourceEntryID.value, ...sourceForm.value })
    showSourceImport.value = false
    await router.push(`/applications/catalog/${item.id}`)
  } catch (error) { mutationError.value = error.message || '导入现有应用失败' }
  finally { saving.value = false }
}
onMounted(load)
</script>

<style scoped>
.catalog-page { display: grid; gap: var(--space-4); }
.catalog-count { color: var(--text-secondary); font-size: 13px; }
.catalog-toolbar, .catalog-toolbar-actions { display: flex; align-items: center; gap: 8px; }
.catalog-toolbar { justify-content: flex-end; padding: 14px var(--space-20); }
.catalog-table { min-width: 850px; }
.catalog-table th:last-child, .catalog-table td:last-child { text-align: right; white-space: nowrap; }
.catalog-table td:last-child .icon-button { margin-left: 5px; }
.table-wrap { padding: 0 var(--space-20) var(--space-8); overflow-x: auto; }
.catalog-name { border: 0; padding: 0; background: transparent; color: var(--action-primary); font: inherit; cursor: pointer; }
.catalog-name:hover { text-decoration: underline; }
.catalog-table td:first-child { white-space: nowrap; }
.catalog-import { width: min(580px, 100%); }
.catalog-import-fields { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; margin-bottom: 15px; }
.catalog-source-list { max-height: 190px; overflow-y: auto; border-top: 1px solid var(--border-muted); border-bottom: 1px solid var(--border-muted); }
.catalog-source-list > .form-label { margin: 10px 0 0; }
.catalog-source-empty { display: block; padding: 12px 0; color: var(--text-muted); font-size: 12px; }
.catalog-source-list .catalog-import-option:last-child { border-bottom: 0; }
.catalog-source-fields { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0 12px; margin-top: 15px; }
.catalog-source-wide { grid-column: 1 / -1; }
.catalog-import-option { display: flex; align-items: center; gap: 10px; min-height: 42px; border-bottom: 1px solid var(--border-muted); font-size: 12px; cursor: pointer; }
.catalog-import-option .cell-secondary { margin-left: auto; }
@media (max-width: 760px) { .catalog-toolbar { padding: 12px 14px; }.catalog-toolbar-actions { width: 100%; }.catalog-toolbar-actions .btn { flex: 1; }.table-wrap { padding: 0 14px 8px; } }
@media (max-width: 480px) { .catalog-import-fields, .catalog-source-fields { grid-template-columns: 1fr; }.catalog-toolbar-actions { flex-wrap: wrap; }.catalog-toolbar-actions .btn { flex: 1 1 auto; } }
</style>
