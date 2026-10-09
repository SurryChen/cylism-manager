<template>
  <div>
    <router-link class="back-link" to="/applications/projects"><ArrowLeft :size="16" />返回项目与环境</router-link>
    <div class="page-header">
      <div><h1 class="page-title">{{ project ? `${project.name} 的环境` : '项目环境' }}</h1><p class="page-subtitle">环境定义部署目标与默认命名空间</p></div>
      <button class="btn btn-primary" :disabled="!project" @click="openCreateEnvironment">+ 新建环境</button>
    </div>
    <div v-if="readError" class="k8s-banner k8s-banner-warn section-gap">{{ readError }}</div>
    <div v-if="namespaceConflicts.length" class="k8s-banner k8s-banner-warn section-gap"><strong>命名空间迁移待处理</strong><span v-for="conflict in namespaceConflicts" :key="`${conflict.cluster_id}/${conflict.namespace}`">{{ clusterName(conflict.cluster_id) }} / {{ conflict.namespace }}：{{ conflict.environments.map(item => `项目 ${item.project_id} / ${item.name}`).join('、') }}</span></div>
    <SurfaceCard v-if="showEnvironmentList" as="div">
      <div class="table-wrap"><table class="data-table"><thead><tr><th>环境</th><th>集群</th><th>命名空间</th><th>状态</th><th>关联应用</th><th>操作</th></tr></thead><tbody><tr v-for="environment in environments" :key="environment.id"><td class="cell-primary">{{ environment.name }}</td><td>{{ clusterName(environment.cluster_id) }}</td><td>{{ environment.namespace }}</td><td><span class="badge" :class="namespaceStatusClass(environment.namespace_status)">{{ environment.cluster_id ? namespaceStatusLabel(environment.namespace_status) : '待绑定' }}</span></td><td>{{ environmentApplicationCount(environment.id) }}</td><td class="action-cell"><div class="btn-group"><button v-if="environment.cluster_id && needsNamespaceSync(environment)" class="icon-button" title="同步命名空间" :disabled="syncingEnvironmentID === environment.id" @click="syncNamespace(environment)"><RefreshCw :size="16" :class="{ 'is-spinning': syncingEnvironmentID === environment.id }" /></button><button class="btn btn-sm" :disabled="environmentHasApplications(environment.id) && !!environment.cluster_id" :title="environmentHasApplications(environment.id) && environment.cluster_id ? '已有应用时不可修改部署目标' : '编辑环境'" @click="openEnvironmentEditor(environment)">编辑</button><button class="btn btn-sm btn-danger" title="删除环境" @click="requestEnvironmentDelete(environment)">删除</button></div></td></tr></tbody></table></div>
    </SurfaceCard>

    <div v-if="showEnvironmentModal" class="overlay" @click.self="closeEnvironmentModal"><div class="modal"><h2 class="modal-title">{{ editingEnvironment ? '编辑环境' : '新建环境' }}</h2><form @submit.prevent="saveEnvironment">
      <div class="form-group"><label class="form-label">环境名称</label><input v-model.trim="environmentForm.name" class="form-input" required :pattern="editingEnvironment && environmentForm.name === editingEnvironment.name ? undefined : '[a-z0-9]([-a-z0-9]*[a-z0-9])?'" :maxlength="editingEnvironment && environmentForm.name === editingEnvironment.name ? undefined : Math.max(1, 54 - (project?.slug?.length || 0))" title="仅支持小写英文字母、数字和连字符" placeholder="production" :disabled="editingEnvironment && environmentHasApplications(editingEnvironment.id)" /></div>
      <div class="form-group"><label class="form-label">所属集群</label><SelectMenu v-model.number="environmentForm.cluster_id" class="form-select" :disabled="!!editingEnvironment?.cluster_id"><option :value="0" disabled>选择集群</option><option v-for="cluster in clusters" :key="cluster.id" :value="cluster.id">{{ cluster.name }}</option></SelectMenu></div>
      <div class="form-group"><label class="form-label">命名空间</label><input class="form-input" :value="namespacePreview" disabled /><p v-if="!project?.slug" class="form-hint">请先编辑项目，设置项目英文名。</p></div>
      <div class="modal-actions"><button type="button" class="btn" @click="closeEnvironmentModal">取消</button><button class="btn btn-primary" :disabled="submitting">{{ submitting ? '保存中...' : editingEnvironment ? '保存' : '创建环境' }}</button></div>
    </form></div></div>

    <div v-if="environmentDeleteTarget" class="overlay" @click.self="environmentDeleteTarget = null"><div class="modal"><h2 class="modal-title">删除环境</h2><p class="confirm-copy">确认删除环境“{{ environmentDeleteTarget.name }}”吗？该操作不可撤销。</p><div class="modal-actions"><button class="btn" @click="environmentDeleteTarget = null">取消</button><button class="btn btn-danger" :disabled="submitting" @click="deleteEnvironment">删除</button></div></div></div>
    <ErrorNoticeModal :open="Boolean(actionError)" :message="actionError" teleport @close="actionError = ''" />
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { ArrowLeft, RefreshCw } from 'lucide-vue-next'
import { createProjectEnvironment, deleteProjectEnvironment, getProjectEnvironmentResources, syncProjectEnvironmentNamespace, updateProjectEnvironment } from '../../api/applications.js'
import { getClusters } from '../../api/clusters.js'
import { useAsyncResource } from '../../composables/useAsyncResource.js'
import SurfaceCard from '../../components/SurfaceCard.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import SelectMenu from '../../components/SelectMenu.vue'

const props = defineProps({ projectID: { type: String, required: true } })
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
const environmentForm = ref({ name: 'production', cluster_id: 0 })
const syncingEnvironmentID = ref(null)
const projectEnvironmentResource = useAsyncResource(({ signal }, requestedProjectID) => Promise.all([getProjectEnvironmentResources(requestedProjectID, { signal }), getClusters({ signal })]), null)
const project = computed(() => projects.value.find(item => String(item.id) === props.projectID))
const showEnvironmentList = computed(() => loadedProjectID.value === props.projectID && !!project.value && environments.value.length > 0)
const namespacePreview = computed(() => editingEnvironment.value && environmentForm.value.name === editingEnvironment.value.name
  ? editingEnvironment.value.namespace
  : project.value?.slug ? `project-${project.value.slug}-${environmentForm.value.name}` : '')
function environmentApplicationCount(environmentID) { return applications.value.filter(application => application.environment_id === environmentID).length }
function environmentHasApplications(environmentID) { return environmentApplicationCount(environmentID) > 0 }
function clusterName(clusterID) { return clusters.value.find(cluster => cluster.id === Number(clusterID))?.name || (clusterID ? `集群 #${clusterID}` : '未绑定') }

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
  environments.value = environmentList || []
  clusters.value = clusterList || []
  loadedProjectID.value = requestedProjectID
}

function namespaceStatusLabel(status) { return status === 'active' ? '就绪' : status === 'missing' ? '缺失' : status === 'pending' ? '创建中' : status === 'terminating' ? '删除中' : '未检测' }
function namespaceStatusClass(status) { return status === 'active' ? 'badge-online' : status === 'missing' ? 'badge-danger' : status === 'pending' || status === 'terminating' ? 'badge-deploying' : 'badge-offline' }
function needsNamespaceSync(environment) { return environment.namespace_status === 'missing' || environment.namespace_status === 'pending' }
function openCreateEnvironment() { editingEnvironment.value = null; environmentForm.value = { name: 'production', cluster_id: clusters.value.length === 1 ? clusters.value[0].id : 0 }; showEnvironmentModal.value = true }
function openEnvironmentEditor(environment) { editingEnvironment.value = environment; environmentForm.value = { name: environment.name, cluster_id: environment.cluster_id || 0 }; showEnvironmentModal.value = true }
function closeEnvironmentModal() { showEnvironmentModal.value = false; editingEnvironment.value = null; environmentForm.value = { name: 'production', cluster_id: 0 } }
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
  } catch (e) {
    actionError.value = e.message || (isEditing ? '更新环境失败' : '创建环境失败')
  } finally {
    submitting.value = false
  }
}

function requestEnvironmentDelete(environment) { environmentDeleteTarget.value = environment }
async function deleteEnvironment() { if (!environmentDeleteTarget.value) return; submitting.value = true; actionError.value = ''; try { await deleteProjectEnvironment(props.projectID, environmentDeleteTarget.value.id); environmentDeleteTarget.value = null; await loadProject() } catch (e) { actionError.value = e.message || '删除环境失败' } finally { submitting.value = false } }
async function syncNamespace(environment) { syncingEnvironmentID.value = environment.id; actionError.value = ''; try { const updated = await syncProjectEnvironmentNamespace(props.projectID, environment.id); const index = environments.value.findIndex(item => item.id === environment.id); if (index >= 0) environments.value[index] = updated } catch (e) { actionError.value = e.message || '同步命名空间失败' } finally { syncingEnvironmentID.value = null } }

watch(() => props.projectID, loadProject, { immediate: true })
</script>

<style scoped>
.back-link { display:inline-flex; align-items:center; gap:6px; margin-bottom:var(--space-16); color:var(--text-secondary); font-size:13px; font-weight:600; text-decoration:none; }
.back-link:hover { color:var(--action-primary); }
.back-link:focus-visible { outline:2px solid var(--focus); outline-offset:3px; }
.page-header { display:flex; align-items:flex-start; justify-content:space-between; gap:var(--space-16); }
.page-subtitle { margin:var(--space-4) 0 0; color:var(--text-secondary); font-size:13px; }
.confirm-copy { margin:0; color:var(--text-secondary); font-size:13px; }
.form-hint { margin:6px 0 0; color:var(--text-muted); font-size:11px; line-height:1.5; }
.is-spinning { animation:spin .8s linear infinite; }
@keyframes spin { to { transform:rotate(360deg); } }
@media (max-width:640px) { .page-header { align-items:stretch; flex-direction:column; }.page-header .btn { width:100%; } }
</style>
