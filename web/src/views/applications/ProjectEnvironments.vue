<template>
  <div class="project-environments">
    <PageBackLink v-if="!embedded" to="/applications/projects">返回项目与环境</PageBackLink>
    <div v-if="!embedded" class="page-header">
      <div><h1 class="page-title">{{ project ? `${project.name} 的环境` : '项目环境' }}</h1><p class="page-subtitle">环境定义部署目标与默认命名空间</p></div>
    </div>

    <div v-if="namespaceConflicts.length" class="k8s-banner k8s-banner-warn section-gap"><strong>命名空间迁移待处理</strong><span v-for="conflict in namespaceConflicts" :key="`${conflict.cluster_id}/${conflict.namespace}`">{{ clusterName(conflict.cluster_id) }} / {{ conflict.namespace }}：{{ conflict.environments.map(item => `项目 ${item.project_id} / ${item.name}`).join('、') }}</span></div>
    <SurfaceCard as="div" class="product-table-card" padding="none">
      <div class="product-toolbar">
        <span class="cell-secondary" aria-live="polite">{{ listPage.total.value }} 个环境</span>
        <div class="environment-toolbar-actions">
          <router-link v-if="project?.transferred_environments?.length" class="btn" :to="{ path: '/applications', query: { project_id: props.projectID, environment_id: project.transferred_environments[0].id } }">查看迁入应用</router-link>
          <button class="icon-button" title="部署设置" aria-label="部署设置" :disabled="!project" @click="openProjectSettings"><Settings :size="16" aria-hidden="true" /></button>
          <button class="btn btn-primary" :disabled="!project" @click="openCreateEnvironment">新建环境</button>
        </div>
      </div>
      <EmptyState v-if="projectEnvironmentResource.loading.value" variant="loading" message="正在读取环境..." />
      <EmptyState v-else-if="!showEnvironmentList" message="暂无部署环境，需要部署时再创建" />
      <div v-else class="table-wrap environment-table">
        <DataTable>
          <thead>
            <tr>
              <th>环境</th>
              <th>部署位置</th>
              <th>命名空间状态</th>
              <th>关联应用</th>
              <TableActionCell header />
            </tr>
          </thead>
          <tbody>
            <tr v-for="environment in environments" :key="environment.id">
              <td>
                <div class="environment-name">
                  <router-link class="environment-link" :aria-label="'查看 ' + environment.name + ' 的应用与部署'" :to="{ path: '/applications', query: { project_id: props.projectID, environment_id: environment.id } }">{{ environment.name }}</router-link>
                  <span class="badge badge-offline">{{ environmentKindLabel(environment.kind) }}</span>
                </div>
              </td>
              <td><span>{{ clusterName(environment.cluster_id) }}</span><span class="cell-secondary product-line environment-namespace">{{ environment.namespace }}</span></td>
              <td>
                <div class="environment-namespace-state">
                <span class="badge" :class="namespaceStatusClass(environment.namespace_status)">
                  {{ environment.cluster_id ? namespaceStatusLabel(environment.namespace_status) : '待绑定' }}
                </span>
                  <button
                    v-if="environment.cluster_id && needsNamespaceSync(environment)"
                    class="icon-button"
                    title="同步命名空间"
                    aria-label="同步命名空间"
                    :disabled="syncingEnvironmentID === environment.id"
                    @click="syncNamespace(environment)"
                  ><RefreshCw :size="15" :class="{ 'is-spinning': syncingEnvironmentID === environment.id }" /></button>
                </div>
              </td>
              <td>{{ environmentApplicationCount(environment.id) }}</td>
              <TableActionCell>
                <button class="icon-button" title="编辑环境" :aria-label="'编辑环境 ' + environment.name" @click="openEnvironmentEditor(environment)"><Pencil :size="15" aria-hidden="true" /></button>
              </TableActionCell>
            </tr>
          </tbody>
        </DataTable>
      </div>
      <ResourcePagination :resource="listPage" label="项目环境分页" :hide-single-page="environmentCountOnly" />
    </SurfaceCard>

    <SurfaceCard v-if="transferredApplications.length && !project?.transferred_environments?.length" padding="none" class="product-table-card section-gap">
      <div class="product-toolbar"><h2 class="product-section-title">迁入的应用</h2></div>
      <div class="table-wrap">
        <DataTable>
          <thead><tr><th>应用</th><th>运行环境</th><th>运行版本</th><TableActionCell header /></tr></thead>
          <tbody>
            <tr v-for="app in transferredApplications" :key="app.id">
              <td class="cell-primary">{{ app.name }}</td>
              <td><router-link :to="{ path: '/applications', query: { project_id: app.deployment_project_id, environment_id: app.environment_id } }">{{ app.deployment_project }} / {{ app.environment }}</router-link><div class="cell-secondary">{{ app.namespace }}</div></td>
              <td><router-link v-if="app.active_release_id" :to="`/applications/${app.id}/releases/${app.active_release_id}`">{{ app.active_version || `#${app.active_release_id}` }}</router-link><span v-else>—</span></td>
              <TableActionCell><router-link class="btn btn-sm" :to="`/applications/${app.id}`">查看运行</router-link></TableActionCell>
            </tr>
          </tbody>
        </DataTable>
      </div>
    </SurfaceCard>

    <BaseModal teleport :open="showEnvironmentModal" :title="editingEnvironment ? '编辑环境' : '新建环境'" @close="closeEnvironmentModal"><form id="project-environment-form" @submit.prevent="saveEnvironment">
      <div class="form-group"><label class="form-label">环境分类</label><SelectMenu v-model="environmentForm.kind" aria-label="环境分类"><option v-for="item in environmentKinds" :key="item.id" :value="item.id">{{ item.label }}</option></SelectMenu></div>
      <div class="form-group"><label class="form-label">环境名称</label><input v-model.trim="environmentForm.name" class="form-input" required :pattern="editingEnvironment && environmentForm.name === editingEnvironment.name ? undefined : '[a-z0-9]([-a-z0-9]*[a-z0-9])?'" :maxlength="editingEnvironment && environmentForm.name === editingEnvironment.name ? undefined : Math.max(1, 54 - (project?.slug?.length || 0))" title="仅支持小写英文字母、数字和连字符" placeholder="production" :disabled="editingEnvironment && environmentHasApplications(editingEnvironment.id)" /></div>
      <div class="form-group"><label class="form-label">所属集群</label><SelectMenu v-model.number="environmentForm.cluster_id" class="form-select" :disabled="!!editingEnvironment?.cluster_id"><option :value="0" disabled>选择集群</option><option v-for="cluster in clusters" :key="cluster.id" :value="cluster.id">{{ cluster.name }}</option></SelectMenu></div>
      <div class="form-group"><label class="form-label">命名空间</label><input class="form-input" :value="namespacePreview" disabled /><p v-if="!project?.slug" class="form-hint">请先编辑项目，设置项目英文名。</p></div>
      </form><template #actions><button v-if="editingEnvironment" type="button" class="btn btn-danger environment-delete" :disabled="submitting" @click="requestEnvironmentDelete(editingEnvironment)">删除环境</button><button type="button" class="btn" :disabled="submitting" @click="closeEnvironmentModal">取消</button><button form="project-environment-form" type="submit" class="btn btn-primary" :disabled="submitting">{{ submitting ? '保存中...' : editingEnvironment ? '保存' : '创建环境' }}</button></template></BaseModal>

    <BaseModal teleport :open="settingsOpen" title="项目部署设置" @close="settingsOpen = false">
      <form id="project-deployment-settings" @submit.prevent="saveProjectSettings">
        <label class="form-group"><span class="form-label">项目英文名</span><input v-model.trim="settingsForm.slug" class="form-input" maxlength="40" pattern="[a-z0-9]([-a-z0-9]*[a-z0-9])?" :disabled="Boolean(environments.length)" required /></label>
        <label class="form-group"><span class="form-label">默认镜像仓库</span><SelectMenu v-model="settingsForm.default_image_registry_id" aria-label="默认镜像仓库"><option :value="0">不设置</option><option v-for="registry in registries.filter(item => item.enabled)" :key="registry.id" :value="registry.id">{{ registry.name }}</option></SelectMenu></label>
      </form>
      <template #actions><button class="btn" @click="settingsOpen = false">取消</button><button form="project-deployment-settings" type="submit" class="btn btn-primary" :disabled="submitting">保存</button></template>
    </BaseModal>
    <ConfirmDialog teleport :open="Boolean(environmentDeleteTarget)" title="删除环境" :message="'确认删除环境“' + (environmentDeleteTarget?.name || '') + '”？'" :busy="submitting" @cancel="environmentDeleteTarget = null" @confirm="deleteEnvironment" />
    <ErrorNoticeModal :open="Boolean(actionError)" :message="actionError" teleport @close="actionError = ''" />
  </div>
</template>

<script setup>
import DataTable from '../../components/DataTable.vue'
import TableActionCell from '../../components/TableActionCell.vue'
import BaseModal from '../../components/BaseModal.vue'
import ConfirmDialog from '../../components/ConfirmDialog.vue'
import EmptyState from '../../components/EmptyState.vue'
import { environmentKinds, environmentKindLabel } from '../products/environment-utils.js'
import ResourcePagination from '../../components/ResourcePagination.vue'
import { useListPage } from '../../composables/useListPage.js'
import { PAGE_SIZE_OPTIONS } from '../../composables/usePagination.js'
import { computed, ref, watch } from 'vue'
import { Pencil, RefreshCw, Settings } from 'lucide-vue-next'
import PageBackLink from '../../components/PageBackLink.vue'
import { getImageRegistries, updateProject, createProjectEnvironment, deleteProjectEnvironment, getProjectEnvironmentResources, syncProjectEnvironmentNamespace, updateProjectEnvironment } from '../../api/applications.js'
import { getClusters } from '../../api/clusters.js'
import { useAsyncResource } from '../../composables/useAsyncResource.js'
import SurfaceCard from '../../components/SurfaceCard.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import SelectMenu from '../../components/SelectMenu.vue'

const props = defineProps({ embedded: Boolean, projectID: { type: String, required: true }, transferredApplications: { type: Array, default: () => [] } })
const emit = defineEmits(['changed'])
const settingsOpen = ref(false), settingsForm = ref({}), registries = ref([])
const projects = ref([])
const environments = ref([])
const clusters = ref([])
const applications = ref([])
const namespaceConflicts = ref([])
const loadedProjectID = ref('')
const readError = ref('')
const actionError = ref('')
const showEnvironmentModal = ref(false)
const editingEnvironment = ref(null)
const environmentDeleteTarget = ref(null)
const submitting = ref(false)
const environmentForm = ref({ name: 'testing', kind: 'testing', cluster_id: 0 })
const syncingEnvironmentID = ref(null)
const projectEnvironmentResource = useAsyncResource(({ signal }, requestedProjectID) => Promise.all([getProjectEnvironmentResources(requestedProjectID, { signal }), getClusters({ signal })]), null)
const project = computed(() => projects.value.find(item => String(item.id) === props.projectID))
const showEnvironmentList = computed(() => loadedProjectID.value === props.projectID && !!project.value && environments.value.length > 0)
const namespacePreview = computed(() => editingEnvironment.value && environmentForm.value.name === editingEnvironment.value.name
  ? editingEnvironment.value.namespace
  : project.value?.slug ? `project-${project.value.slug}-${environmentForm.value.name}` : '')
function environmentApplicationCount(environmentID) { return applications.value.filter(application => application.environment_id === environmentID && (!application.management_project_id || String(application.management_project_id) === props.projectID)).length }
function environmentHasApplications(environmentID) { return applications.value.some(application => application.environment_id === environmentID) }
function clusterName(clusterID) { return clusters.value.find(cluster => cluster.id === Number(clusterID))?.name || (clusterID ? `集群 #${clusterID}` : '未绑定') }

const listPage = useListPage(() => '/projects/' + encodeURIComponent(props.projectID) + '/environments', environments, readError)
const environmentCountOnly = computed(() => listPage.pageSize.value === PAGE_SIZE_OPTIONS[0] && listPage.totalPages.value === 1)
async function loadProject() {
  const requestedProjectID = props.projectID
  readError.value = ''
  const result = await projectEnvironmentResource.refresh(requestedProjectID)
  if (!result) {
    if (projectEnvironmentResource.error.value) readError.value = projectEnvironmentResource.error.value.message || '加载项目环境失败'
    return
  }
  if (props.projectID !== requestedProjectID) return
  const [[projectList, applicationList, conflicts, environmentList], clusterList] = result
  projects.value = projectList || []
  applications.value = applicationList || []
  namespaceConflicts.value = conflicts || []
  if (!projects.value.some(item => String(item.id) === requestedProjectID)) return
  await listPage.refresh({ project_id: requestedProjectID }).catch(() => undefined)
  if (props.projectID !== requestedProjectID) return
  clusters.value = clusterList || []
  loadedProjectID.value = requestedProjectID
}

function namespaceStatusLabel(status) { return status === 'active' ? '就绪' : status === 'missing' ? '缺失' : status === 'pending' ? '创建中' : status === 'terminating' ? '删除中' : '未检测' }
function namespaceStatusClass(status) { return status === 'active' ? 'badge-online' : status === 'missing' ? 'badge-danger' : status === 'pending' || status === 'terminating' ? 'badge-deploying' : 'badge-offline' }
function needsNamespaceSync(environment) { return environment.namespace_status === 'missing' || environment.namespace_status === 'pending' }
function openCreateEnvironment() { editingEnvironment.value = null; environmentForm.value = { name: 'testing', kind: 'testing', cluster_id: clusters.value.length === 1 ? clusters.value[0].id : 0 }; showEnvironmentModal.value = true }
function openEnvironmentEditor(environment) { editingEnvironment.value = environment; environmentForm.value = { name: environment.name, kind: environment.kind || 'unclassified', cluster_id: environment.cluster_id || 0 }; showEnvironmentModal.value = true }
function closeEnvironmentModal() { showEnvironmentModal.value = false; editingEnvironment.value = null; environmentForm.value = { name: 'testing', kind: 'testing', cluster_id: 0 } }
async function saveEnvironment() {
  if (!project.value) return
	if (!environmentForm.value.cluster_id) { actionError.value = '请选择所属集群'; return }
	const isEditing = !!editingEnvironment.value
  submitting.value = true
  actionError.value = ''
  try {
    if (isEditing) { await updateProjectEnvironment(props.projectID, editingEnvironment.value.id, environmentForm.value) } else { await createProjectEnvironment(props.projectID, environmentForm.value) }
    closeEnvironmentModal()
    await loadProject()
    emit('changed')
  } catch (e) {
    actionError.value = e.message || (isEditing ? '更新环境失败' : '创建环境失败')
  } finally {
    submitting.value = false
  }
}

async function openProjectSettings() {
  try {
    registries.value = await getImageRegistries({ projectID: props.projectID })
    settingsForm.value = { slug: project.value.slug || '', default_image_registry_id: project.value.default_image_registry_id || 0 }
    settingsOpen.value = true
  } catch (cause) { actionError.value = cause.message }
}
async function saveProjectSettings() {
  submitting.value = true
  try {
    await updateProject(props.projectID, { name: project.value.name, description: project.value.description, ...settingsForm.value })
    settingsOpen.value = false
    await loadProject()
    emit('changed')
  } catch (cause) { actionError.value = cause.message }
  finally { submitting.value = false }
}
function requestEnvironmentDelete(environment) { closeEnvironmentModal(); environmentDeleteTarget.value = environment }
async function deleteEnvironment() { if (!environmentDeleteTarget.value) return; submitting.value = true; actionError.value = ''; try { await deleteProjectEnvironment(props.projectID, environmentDeleteTarget.value.id); environmentDeleteTarget.value = null; await loadProject() } catch (e) { actionError.value = e.message || '删除环境失败' } finally { submitting.value = false } }
async function syncNamespace(environment) { syncingEnvironmentID.value = environment.id; actionError.value = ''; try { const updated = await syncProjectEnvironmentNamespace(props.projectID, environment.id); const index = environments.value.findIndex(item => item.id === environment.id); if (index >= 0) environments.value[index] = updated } catch (e) { actionError.value = e.message || '同步命名空间失败' } finally { syncingEnvironmentID.value = null } }

watch(readError, value => { if (value) actionError.value = value })
watch(() => props.projectID, () => { listPage.reset(); environments.value = []; void loadProject() }, { immediate: true })
</script>

<style scoped>
.project-environments { min-width: 0; }
.environment-toolbar-actions { display: flex; align-items: center; justify-content: flex-end; flex-wrap: wrap; gap: var(--space-8); margin-inline-start: auto; }
.environment-name, .environment-namespace-state { display: flex; align-items: center; flex-wrap: wrap; gap: var(--space-8); }
.environment-link { font-weight: 650; text-decoration: none; }
.environment-link:hover { text-decoration: underline; }
.environment-link:focus-visible { outline: 2px solid var(--focus); outline-offset: 2px; }
.environment-namespace { overflow-wrap: anywhere; }
.environment-table { padding-bottom: var(--space-12); }
.environment-delete { margin-inline-end: auto; }
.page-header { display:flex; align-items:flex-start; justify-content:space-between; gap:var(--space-16); }
.page-subtitle { margin:var(--space-4) 0 0; color:var(--text-secondary); font-size:13px; }
.confirm-copy { margin:0; color:var(--text-secondary); font-size:13px; }
.form-hint { margin:6px 0 0; color:var(--text-muted); font-size:11px; line-height:1.5; }
.is-spinning { animation:spin .8s linear infinite; }
@keyframes spin { to { transform:rotate(360deg); } }
@media (max-width:640px) { .page-header { align-items:stretch; flex-direction:column; }.page-header .btn { width:100%; } }
</style>
