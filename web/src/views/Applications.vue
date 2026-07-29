<template>
  <div>
    <div class="page-header">
      <div><h1 class="page-title">{{ pageMeta.title }}</h1><p class="page-subtitle">{{ pageMeta.subtitle }}</p></div>
      <button v-if="section === 'applications'" class="btn btn-primary" @click="openCreateApplication">+ 创建应用</button>
      <button v-else-if="section === 'projects'" class="btn btn-primary" @click="showProjectModal = true">+ 新建项目</button>
    </div>
    <div v-if="error" class="k8s-banner k8s-banner-warn section-gap">⚠ {{ error }}</div>

    <template v-if="section === 'applications'">
      <div v-if="applicationsLoaded && applications.length > 0" class="card">
        <div class="table-wrap"><table class="data-table"><thead><tr><th>应用</th><th>项目</th><th>环境</th><th>命名空间</th><th>访问地址</th><th>操作</th></tr></thead><tbody>
          <tr v-for="app in applications" :key="app.id" class="clickable" @click="openDetails(app)"><td class="cell-primary">{{ app.name }}</td><td>{{ app.project?.name || '-' }}</td><td>{{ app.environment?.name || '-' }}</td><td>{{ app.environment?.namespace || '-' }}</td><td>{{ endpointLabel(app) }}</td><td><button class="btn btn-sm" @click.stop="openRelease(app)">发布版本</button></td></tr>
        </tbody></table></div>
      </div>

    </template>

    <template v-else-if="section === 'projects'">
      <div v-if="projectsLoaded && projects.length > 0" class="card">
        <div class="table-wrap"><table class="data-table"><thead><tr><th>项目</th><th>说明</th><th>默认镜像仓库</th><th>环境与命名空间</th><th>已关联应用</th><th>操作</th></tr></thead><tbody>
          <tr v-for="project in projects" :key="project.id" class="clickable" @click="openProjectEnvironments(project)"><td class="cell-primary">{{ project.name }}</td><td>{{ project.description || '-' }}</td><td>{{ project.default_image_registry?.name || '-' }}</td><td><div v-if="project.environments?.length" class="environment-links"><span v-for="environment in project.environments" :key="environment.id" class="environment-link"><strong>{{ environment.name }}</strong><small>{{ environment.namespace }}</small></span></div><span v-else>-</span></td><td>{{ applicationCount(project.id) }}</td><td class="action-cell" @click.stop><div class="btn-group"><button class="btn btn-sm" @click="openProjectEditor(project)">编辑</button><button class="btn btn-sm" @click="openProjectEnvironments(project)">管理环境</button><button class="btn btn-sm btn-danger" title="删除项目" @click="requestProjectDelete(project)">删除</button></div></td></tr>
        </tbody></table></div>
      </div>
    </template>

    <template v-else>
      <div v-if="releaseHistoryLoaded && releaseHistory.length > 0" class="card">
        <div class="table-wrap"><table class="data-table"><thead><tr><th>应用</th><th>环境</th><th>版本</th><th>镜像</th><th>状态</th><th>时间</th></tr></thead><tbody>
          <tr v-for="item in releaseHistory" :key="item.id" class="clickable" @click="openHistoryRelease(item)"><td class="cell-primary">{{ item.application.name }}</td><td>{{ item.application.environment?.name || '-' }}</td><td>#{{ item.sequence }}</td><td>{{ item.image }}</td><td><span class="badge" :class="releaseBadge(item.status)">{{ item.status }}</span></td><td>{{ formatTime(item.created_at) }}</td></tr>
        </tbody></table></div>
      </div>
    </template>

    <div v-if="showCreate" class="overlay" @click.self="showCreate = false"><div class="modal"><h2 class="modal-title">创建应用</h2><form @submit.prevent="createApplication">
      <div class="form-group"><label class="form-label">项目</label><select v-model.number="createForm.projectID" class="form-select" required @change="loadCreateEnvironments"><option :value="0" disabled>选择项目</option><option v-for="project in projects" :key="project.id" :value="project.id">{{ project.name }}</option></select></div>
      <div class="form-group"><label class="form-label">环境</label><select v-model.number="createForm.environmentID" class="form-select" required :disabled="createEnvironments.length === 0"><option :value="0" disabled>{{ createEnvironments.length ? '选择环境' : '请先创建环境' }}</option><option v-for="environment in createEnvironments" :key="environment.id" :value="environment.id">{{ environment.name }} · {{ environment.namespace }}</option></select></div>
      <div class="form-group"><label class="form-label">应用名</label><input v-model.trim="createForm.applicationName" class="form-input" required pattern="[a-z0-9]([-a-z0-9]*[a-z0-9])?" maxlength="63" title="仅支持小写字母、数字和连字符，且以字母或数字开头和结尾" placeholder="order-api" /><p class="form-hint">仅支持小写字母、数字和连字符，最长 63 位。</p></div>
      <div class="modal-actions"><button type="button" class="btn" @click="showCreate = false">取消</button><button class="btn btn-primary" :disabled="submitting">{{ submitting ? '创建中...' : '创建' }}</button></div>
    </form></div></div>

    <div v-if="showProjectModal" class="overlay" @click.self="closeProjectModal"><div class="modal"><h2 class="modal-title">{{ editingProject ? '编辑项目' : '新建项目' }}</h2><form @submit.prevent="saveProject">
      <div class="form-group"><label class="form-label">项目名称</label><input v-model="projectForm.name" class="form-input" required placeholder="commerce" :disabled="editingProject && applicationCount(editingProject.id) > 0" /></div>
      <div class="form-group"><label class="form-label">说明</label><textarea v-model="projectForm.description" class="form-input form-textarea" placeholder="订单服务及其部署环境" /></div>
      <div v-if="editingProject" class="form-group"><label class="form-label">默认镜像仓库</label><select v-model.number="projectForm.defaultImageRegistryID" class="form-select"><option :value="0">不设置默认仓库</option><option v-for="registry in projectRegistries(editingProject)" :key="registry.id" :value="registry.id">{{ registry.name }} · {{ registry.endpoint }}</option></select></div>
      <div class="modal-actions"><button type="button" class="btn" @click="closeProjectModal">取消</button><button class="btn btn-primary" :disabled="submitting">{{ submitting ? '保存中...' : editingProject ? '保存' : '创建项目' }}</button></div>
    </form></div></div>

    <div v-if="projectDeleteTarget" class="overlay" @click.self="projectDeleteTarget = null"><div class="modal"><h2 class="modal-title">删除项目</h2><p class="confirm-copy">确认删除项目“{{ projectDeleteTarget.name }}”吗？该操作不可撤销。</p><div class="modal-actions"><button class="btn" @click="projectDeleteTarget = null">取消</button><button class="btn btn-danger" :disabled="submitting" @click="deleteProject">删除</button></div></div></div>

    <Teleport to="body"><div v-if="releaseApp" class="overlay" @click.self="releaseApp = null"><div class="modal release-modal"><h2 class="modal-title">发布 {{ releaseApp.name }}</h2><form @submit.prevent="createRelease">
      <div class="form-group"><label class="form-label">镜像仓库</label><select v-model.number="releaseForm.registry_id" class="form-select"><option :value="0">自定义完整镜像地址</option><option v-for="registry in releaseRegistries" :key="registry.id" :value="registry.id">{{ registry.name }} · {{ registry.endpoint }}</option></select></div>
      <div class="form-group"><label class="form-label">{{ releaseForm.registry_id ? '镜像路径与版本' : '镜像' }}</label><input v-model="releaseForm.image" class="form-input" required :placeholder="releaseForm.registry_id ? 'commerce/order-api:1.0.0' : 'registry.example.com/order-api:1.0.0'" /></div>
      <div class="form-row"><div class="form-group"><label class="form-label">容器端口</label><input v-model.number="releaseForm.container_port" type="number" class="form-input" min="1" required /></div><div class="form-group"><label class="form-label">副本数</label><input v-model.number="releaseForm.replicas" type="number" class="form-input" min="1" required /></div></div>
      <div class="form-row"><div class="form-group"><label class="form-label">CPU 请求</label><div class="unit-input"><input v-model.number="releaseForm.resources.requests_cpu" type="number" min="1" class="form-input" required /><select v-model="releaseForm.resources.requests_cpu_unit" class="form-select"><option value="m">mCPU</option><option value="">核</option></select></div></div><div class="form-group"><label class="form-label">CPU 限制</label><div class="unit-input"><input v-model.number="releaseForm.resources.limits_cpu" type="number" min="1" class="form-input" required /><select v-model="releaseForm.resources.limits_cpu_unit" class="form-select"><option value="m">mCPU</option><option value="">核</option></select></div></div></div>
      <div class="form-row"><div class="form-group"><label class="form-label">内存请求</label><div class="unit-input"><input v-model.number="releaseForm.resources.requests_memory" type="number" min="1" class="form-input" required /><select v-model="releaseForm.resources.requests_memory_unit" class="form-select"><option value="Mi">Mi</option><option value="Gi">Gi</option></select></div></div><div class="form-group"><label class="form-label">内存限制</label><div class="unit-input"><input v-model.number="releaseForm.resources.limits_memory" type="number" min="1" class="form-input" required /><select v-model="releaseForm.resources.limits_memory_unit" class="form-select"><option value="Mi">Mi</option><option value="Gi">Gi</option></select></div></div></div>
      <div class="form-row"><div class="form-group"><label class="check-row"><input v-model="releaseForm.health.readiness_enabled" type="checkbox" /> 启用就绪检查</label><div v-if="releaseForm.health.readiness_enabled" class="form-row"><select v-model="releaseForm.health.readiness_type" class="form-select"><option value="http">HTTP</option><option value="tcp">TCP</option></select><input v-if="releaseForm.health.readiness_type === 'http'" v-model="releaseForm.health.readiness_path" class="form-input" /></div></div><div class="form-group"><label class="check-row"><input v-model="releaseForm.health.liveness_enabled" type="checkbox" /> 启用存活检查</label><div v-if="releaseForm.health.liveness_enabled" class="form-row"><select v-model="releaseForm.health.liveness_type" class="form-select"><option value="http">HTTP</option><option value="tcp">TCP</option></select><input v-if="releaseForm.health.liveness_type === 'http'" v-model="releaseForm.health.liveness_path" class="form-input" /></div></div></div>
      <div class="form-row"><div class="form-group"><label class="form-label">Service 端口</label><input v-model.number="releaseForm.service.port" type="number" class="form-input" required /></div><div class="form-group"><label class="form-label">Target Port</label><input v-model.number="releaseForm.service.target_port" type="number" class="form-input" :disabled="releaseForm.service.target_port_auto" required /><label class="check-row"><input v-model="releaseForm.service.target_port_auto" type="checkbox" /> 与容器端口同步</label></div></div>
      <div class="form-row"><div class="form-group"><label class="form-label">暴露方式</label><select v-model="releaseForm.endpoint.exposure" class="form-select"><option value="cluster">仅集群内</option><option value="public">公网域名</option></select></div><div v-if="releaseForm.endpoint.exposure === 'public'" class="form-group"><label class="form-label">受管域名</label><select v-model.number="releaseForm.endpoint.domain_id" class="form-select"><option :value="0">手动填写新域名</option><option v-for="domain in releaseDomains" :key="domain.id" :value="domain.id">{{ domain.hostname }}</option></select></div></div>
      <div v-if="releaseForm.endpoint.exposure === 'public' && !releaseForm.endpoint.domain_id" class="form-row"><div class="form-group"><label class="form-label">域名</label><input v-model="releaseForm.endpoint.domain" class="form-input" required placeholder="api.example.com" /></div><div class="form-group"><label class="form-label">Issuer</label><input v-model="releaseForm.endpoint.issuer_ref" class="form-input" placeholder="letsencrypt-prod" /></div></div>
      <label v-if="releaseForm.endpoint.exposure === 'public'" class="check-row"><input v-model="releaseForm.endpoint.tls_enabled" type="checkbox" /> 启用 HTTPS</label>
      <div class="modal-actions"><button type="button" class="btn" @click="releaseApp = null">取消</button><button class="btn btn-primary" :disabled="submitting">{{ submitting ? '提交中...' : '开始发布' }}</button></div>
    </form></div></div></Teleport>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api/index.js'

const props = defineProps({ section: { type: String, default: 'applications' } })
const router = useRouter()
const applications = ref([])
const projects = ref([])
const releaseHistory = ref([])
const applicationsLoaded = ref(false)
const projectsLoaded = ref(false)
const releaseHistoryLoaded = ref(false)
const error = ref('')
const submitting = ref(false)
const showCreate = ref(false)
const showProjectModal = ref(false)
const editingProject = ref(null)
const projectDeleteTarget = ref(null)
const releaseApp = ref(null)
const releaseRegistries = ref([])
const releaseDomains = ref([])
const imageRegistries = ref([])
const createEnvironments = ref([])
const createForm = ref(newApplicationForm())
const projectForm = ref({ name: '', description: '', defaultImageRegistryID: 0 })
const releaseForm = ref(newReleaseForm())

const pageMeta = computed(() => ({
  applications: { title: '应用', subtitle: '发布和运维平台托管的 Kubernetes 服务' },
  projects: { title: '项目与环境', subtitle: '定义应用归属、部署环境和默认命名空间' },
  releases: { title: '发布记录', subtitle: '查看所有应用的发布过程、状态和失败详情' },
}[props.section] || { title: '应用', subtitle: '发布和运维平台托管的 Kubernetes 服务' }))

function newApplicationForm() { return { projectID: 0, environmentID: 0, applicationName: '' } }
function newReleaseForm() { return { registry_id: 0, image: '', container_port: 8080, replicas: 1, resources: { requests_cpu: 100, requests_cpu_unit: 'm', requests_memory: 128, requests_memory_unit: 'Mi', limits_cpu: 500, limits_cpu_unit: 'm', limits_memory: 512, limits_memory_unit: 'Mi' }, health: { readiness_enabled: true, readiness_type: 'http', readiness_path: '/healthz', liveness_enabled: false, liveness_type: 'http', liveness_path: '/healthz' }, service: { port: 80, target_port: 8080, target_port_auto: true }, endpoint: { exposure: 'cluster', domain_id: 0, domain: '', path: '/', tls_enabled: false, issuer_ref: '' } } }
function endpointLabel(app) { const endpoint = app.endpoints?.[0]; return endpoint?.domain ? `https://${endpoint.domain}` : '集群内' }
function releaseBadge(status) { return status === 'succeeded' ? 'badge-online' : status === 'failed' ? 'badge-danger' : 'badge-offline' }
function formatTime(value) { return value ? new Date(value).toLocaleString() : '-' }
function applicationCount(projectID) { return applications.value.filter(app => app.project_id === projectID).length }

async function fetchApplications() { error.value = ''; try { applications.value = await api.get('/applications') || [] } catch (e) { error.value = e.message || '加载应用失败' } finally { applicationsLoaded.value = true } }
async function fetchProjects() { error.value = ''; try { projects.value = await api.get('/projects') || [] } catch (e) { error.value = e.message || '加载项目失败' } finally { projectsLoaded.value = true } }
async function fetchImageRegistries() { try { imageRegistries.value = await api.get('/image-registries') || [] } catch (e) { imageRegistries.value = []; error.value = e.message || '加载镜像仓库失败' } }
async function fetchEnvironments(projectID) { return await api.get(`/projects/${projectID}/environments`) || [] }
async function loadSection(section) { if (section === 'projects') { await Promise.all([fetchProjects(), fetchApplications(), fetchImageRegistries()]) } else if (section === 'releases') { await fetchReleaseHistory() } else { await fetchApplications() } }

async function openCreateApplication() {
  error.value = ''
  if (projects.value.length === 0) await fetchProjects()
  if (projects.value.length === 0) { error.value = '请先在“项目与环境”中创建项目和环境'; return }
  createForm.value = newApplicationForm()
  createForm.value.projectID = projects.value[0].id
  await loadCreateEnvironments()
  showCreate.value = true
}
async function loadCreateEnvironments() { createForm.value.environmentID = 0; createEnvironments.value = []; if (!createForm.value.projectID) return; try { createEnvironments.value = await fetchEnvironments(createForm.value.projectID) } catch (e) { error.value = e.message || '加载环境失败' } }
async function createApplication() { submitting.value = true; error.value = ''; try { await api.post('/applications', { project_id: createForm.value.projectID, environment_id: createForm.value.environmentID, name: createForm.value.applicationName }); showCreate.value = false; createForm.value = newApplicationForm(); await fetchApplications() } catch (e) { error.value = e.message || '创建应用失败' } finally { submitting.value = false } }
function projectRegistries(project) { return imageRegistries.value.filter(registry => registry.enabled && registry.projects?.some(allowedProject => allowedProject.id === project.id)) }
function openProjectEditor(project) { editingProject.value = project; projectForm.value = { name: project.name, description: project.description || '', defaultImageRegistryID: project.default_image_registry_id || 0 }; showProjectModal.value = true }
function closeProjectModal() { showProjectModal.value = false; editingProject.value = null; projectForm.value = { name: '', description: '', defaultImageRegistryID: 0 } }
async function saveProject() { const isEditing = !!editingProject.value; submitting.value = true; error.value = ''; try { if (isEditing) { await api.put(`/projects/${editingProject.value.id}`, { name: projectForm.value.name, description: projectForm.value.description, default_image_registry_id: projectForm.value.defaultImageRegistryID }) } else { await api.post('/projects', { name: projectForm.value.name, description: projectForm.value.description }) } closeProjectModal(); await Promise.all([fetchProjects(), fetchApplications()]) } catch (e) { error.value = e.message || (isEditing ? '更新项目失败' : '创建项目失败') } finally { submitting.value = false } }
function requestProjectDelete(project) { projectDeleteTarget.value = project }
async function deleteProject() { if (!projectDeleteTarget.value) return; submitting.value = true; error.value = ''; try { await api.delete(`/projects/${projectDeleteTarget.value.id}`); projectDeleteTarget.value = null; await Promise.all([fetchProjects(), fetchApplications()]) } catch (e) { error.value = e.message || '删除项目失败' } finally { submitting.value = false } }
async function openProjectEnvironments(project) { await router.push(`/applications/projects/${project.id}`) }
async function fetchReleaseHistory() { error.value = ''; try { await fetchApplications(); const details = await Promise.allSettled(applications.value.map(app => api.get(`/applications/${app.id}`))); releaseHistory.value = details.flatMap((result, index) => result.status === 'fulfilled' ? (result.value.releases || []).map(release => ({ ...release, application: applications.value[index] })) : []).sort((left, right) => new Date(right.created_at || 0) - new Date(left.created_at || 0)) } catch (e) { error.value = e.message || '加载发布记录失败' } finally { releaseHistoryLoaded.value = true } }
async function openHistoryRelease(item) { await router.push(`/applications/${item.application.id}/releases/${item.id}`) }

async function openRelease(app) { error.value = ''; releaseForm.value = newReleaseForm(); try { const [registries, domains] = await Promise.all([api.get(`/image-registries?project_id=${app.project_id}`), api.get('/domains')]); releaseRegistries.value = registries || []; releaseDomains.value = (domains || []).filter(domain => domain.enabled); const defaultRegistryID = app.project?.default_image_registry_id; if (releaseRegistries.value.some(registry => registry.id === defaultRegistryID && registry.enabled)) releaseForm.value.registry_id = defaultRegistryID } catch (e) { releaseRegistries.value = []; releaseDomains.value = []; error.value = e.message || '加载发布配置失败' } finally { releaseApp.value = app } }
function releasePayload() { const form = releaseForm.value; return { ...form, resources: { requests_cpu: `${form.resources.requests_cpu}${form.resources.requests_cpu_unit}`, requests_memory: `${form.resources.requests_memory}${form.resources.requests_memory_unit}`, limits_cpu: `${form.resources.limits_cpu}${form.resources.limits_cpu_unit}`, limits_memory: `${form.resources.limits_memory}${form.resources.limits_memory_unit}` }, service: { port: form.service.port, target_port: form.service.target_port } } }
async function createRelease() { submitting.value = true; error.value = ''; try { const applicationID = releaseApp.value.id; const release = await api.post(`/applications/${applicationID}/releases`, releasePayload()); releaseApp.value = null; await router.push(`/applications/${applicationID}/releases/${release.id}`); fetchApplications() } catch (e) { error.value = e.message || '创建发布失败' } finally { submitting.value = false } }
async function openDetails(app) { await router.push(`/applications/${app.id}`) }

watch(() => props.section, loadSection, { immediate: true })
watch(() => releaseForm.value.container_port, port => { if (releaseForm.value.service.target_port_auto) releaseForm.value.service.target_port = port })
</script>

<style scoped>
.page-header { display:flex; align-items:flex-start; justify-content:space-between; gap:var(--space-16); }
.page-subtitle { margin:var(--space-4) 0 0; color:var(--text-secondary); font-size:13px; }
.release-modal { width:min(680px, calc(100vw - 32px)); }
.compact-input { margin-top:6px; }.unit-input { display:grid; grid-template-columns:minmax(0, 1fr) 86px; gap:6px; }
.check-row { display:flex; gap:8px; align-items:center; margin:var(--space-12) 0; color:var(--text-secondary); font-size:13px; }
.form-textarea { min-height:82px; resize:vertical; }
.environment-links { display:flex; flex-wrap:wrap; gap:6px; }.environment-link { display:grid; min-width:96px; padding:4px 6px; border:1px solid var(--border-muted); border-radius:var(--radius-control); background:var(--surface-subtle); line-height:1.25; }.environment-link strong { color:var(--text-primary); font-size:11px; }.environment-link small { margin-top:2px; color:var(--text-muted); font-size:10px; }.confirm-copy { margin:0; color:var(--text-secondary); font-size:13px; }
@media (max-width:640px) { .page-header { align-items:stretch; flex-direction:column; }.page-header .btn { width:100%; } }
</style>
