<template>
  <div>
    <router-link class="back-link" to="/applications/projects"><ArrowLeft :size="16" />返回项目与环境</router-link>
    <div class="page-header">
      <div><h1 class="page-title">{{ project ? `${project.name} 的环境` : '项目环境' }}</h1><p class="page-subtitle">环境定义部署目标与默认命名空间</p></div>
      <button class="btn btn-primary" :disabled="!project" @click="showEnvironmentModal = true">+ 新建环境</button>
    </div>
    <div v-if="error" class="k8s-banner k8s-banner-warn section-gap">⚠ {{ error }}</div>
    <div v-if="showEnvironmentList" class="card">
      <div class="table-wrap"><table class="data-table"><thead><tr><th>环境</th><th>命名空间</th><th>关联应用</th><th>操作</th></tr></thead><tbody><tr v-for="environment in environments" :key="environment.id"><td class="cell-primary">{{ environment.name }}</td><td>{{ environment.namespace }}</td><td>{{ environmentApplicationCount(environment.id) }}</td><td class="action-cell"><div class="btn-group"><button class="btn btn-sm" :disabled="environmentHasApplications(environment.id)" :title="environmentHasApplications(environment.id) ? '已有应用时不可修改部署目标' : '编辑环境'" @click="openEnvironmentEditor(environment)">编辑</button><button class="btn btn-sm btn-danger" :disabled="environmentHasApplications(environment.id)" :title="environmentHasApplications(environment.id) ? '请先删除关联应用' : '删除环境'" @click="requestEnvironmentDelete(environment)">删除</button></div></td></tr></tbody></table></div>
    </div>

    <div v-if="showEnvironmentModal" class="overlay" @click.self="closeEnvironmentModal"><div class="modal"><h2 class="modal-title">{{ editingEnvironment ? '编辑环境' : '新建环境' }}</h2><form @submit.prevent="saveEnvironment">
      <div class="form-group"><label class="form-label">环境名称</label><input v-model="environmentForm.name" class="form-input" required placeholder="production" :disabled="editingEnvironment && environmentHasApplications(editingEnvironment.id)" /></div>
      <div class="form-group"><label class="form-label">命名空间</label><input v-model="environmentForm.namespace" class="form-input" required placeholder="commerce-prod" :disabled="editingEnvironment && environmentHasApplications(editingEnvironment.id)" /></div>
      <div class="modal-actions"><button type="button" class="btn" @click="closeEnvironmentModal">取消</button><button class="btn btn-primary" :disabled="submitting">{{ submitting ? '保存中...' : editingEnvironment ? '保存' : '创建环境' }}</button></div>
    </form></div></div>

    <div v-if="environmentDeleteTarget" class="overlay" @click.self="environmentDeleteTarget = null"><div class="modal"><h2 class="modal-title">删除环境</h2><p class="confirm-copy">确认删除环境“{{ environmentDeleteTarget.name }}”吗？该操作不可撤销。</p><div class="modal-actions"><button class="btn" @click="environmentDeleteTarget = null">取消</button><button class="btn btn-danger" :disabled="submitting" @click="deleteEnvironment">删除</button></div></div></div>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { ArrowLeft } from 'lucide-vue-next'
import { api } from '../api/index.js'

const props = defineProps({ projectID: { type: String, required: true } })
const projects = ref([])
const environments = ref([])
const applications = ref([])
const loadedProjectID = ref('')
const error = ref('')
const showEnvironmentModal = ref(false)
const editingEnvironment = ref(null)
const environmentDeleteTarget = ref(null)
const submitting = ref(false)
const environmentForm = ref({ name: 'production', namespace: '' })
const project = computed(() => projects.value.find(item => String(item.id) === props.projectID))
const showEnvironmentList = computed(() => loadedProjectID.value === props.projectID && !!project.value && environments.value.length > 0)
function environmentApplicationCount(environmentID) { return applications.value.filter(application => application.environment_id === environmentID).length }
function environmentHasApplications(environmentID) { return environmentApplicationCount(environmentID) > 0 }

async function loadProject() {
  const requestedProjectID = props.projectID
  error.value = ''
  try {
    const [projectList, applicationList] = await Promise.all([api.get('/projects'), api.get('/applications')])
    projects.value = projectList || []
    applications.value = applicationList || []
    if (!project.value) return
    const result = await api.get(`/projects/${requestedProjectID}/environments`) || []
    if (props.projectID === requestedProjectID) {
      environments.value = result
      loadedProjectID.value = requestedProjectID
    }
  } catch (e) {
    error.value = e.message || '加载项目环境失败'
  }
}

function openEnvironmentEditor(environment) { editingEnvironment.value = environment; environmentForm.value = { name: environment.name, namespace: environment.namespace }; showEnvironmentModal.value = true }
function closeEnvironmentModal() { showEnvironmentModal.value = false; editingEnvironment.value = null; environmentForm.value = { name: 'production', namespace: '' } }
async function saveEnvironment() {
  if (!project.value) return
	const isEditing = !!editingEnvironment.value
  submitting.value = true
  error.value = ''
  try {
    if (isEditing) { await api.put(`/projects/${props.projectID}/environments/${editingEnvironment.value.id}`, environmentForm.value) } else { await api.post(`/projects/${props.projectID}/environments`, environmentForm.value) }
    closeEnvironmentModal()
    await loadProject()
  } catch (e) {
    error.value = e.message || (isEditing ? '更新环境失败' : '创建环境失败')
  } finally {
    submitting.value = false
  }
}

function requestEnvironmentDelete(environment) { environmentDeleteTarget.value = environment }
async function deleteEnvironment() { if (!environmentDeleteTarget.value) return; submitting.value = true; error.value = ''; try { await api.delete(`/projects/${props.projectID}/environments/${environmentDeleteTarget.value.id}`); environmentDeleteTarget.value = null; await loadProject() } catch (e) { error.value = e.message || '删除环境失败' } finally { submitting.value = false } }

watch(() => props.projectID, loadProject, { immediate: true })
</script>

<style scoped>
.back-link { display:inline-flex; align-items:center; gap:6px; margin-bottom:var(--space-16); color:var(--text-secondary); font-size:13px; font-weight:600; text-decoration:none; }
.back-link:hover { color:var(--action-primary); }
.back-link:focus-visible { outline:2px solid var(--focus); outline-offset:3px; }
.page-header { display:flex; align-items:flex-start; justify-content:space-between; gap:var(--space-16); }
.page-subtitle { margin:var(--space-4) 0 0; color:var(--text-secondary); font-size:13px; }
.confirm-copy { margin:0; color:var(--text-secondary); font-size:13px; }
@media (max-width:640px) { .page-header { align-items:stretch; flex-direction:column; }.page-header .btn { width:100%; } }
</style>
