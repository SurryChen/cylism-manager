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
        <div class="table-wrap"><table class="data-table"><thead><tr><th>应用</th><th>环境</th><th>命名空间</th><th>访问地址</th><th>操作</th></tr></thead><tbody>
          <tr v-for="app in applications" :key="app.id" class="clickable" @click="openDetails(app)"><td class="cell-primary">{{ app.name }}</td><td>{{ app.environment?.name || '-' }}</td><td>{{ app.environment?.namespace || '-' }}</td><td>{{ endpointLabel(app) }}</td><td><button class="btn btn-sm" @click.stop="openRelease(app)">发布版本</button></td></tr>
        </tbody></table></div>
      </div>

      <div v-if="applicationDetail" class="card section-gap"><div class="detail-header"><div><h2 class="section-title">{{ applicationDetail.application.name }} 发布记录</h2><p class="section-copy">{{ applicationDetail.application.environment?.namespace }}</p></div><button class="icon-button" title="关闭发布记录" @click="closeDetails">×</button></div>
        <div v-if="applicationDetail.releases.length === 0" class="empty-state"><span class="empty-text">暂无发布记录</span></div>
        <div v-else class="table-wrap"><table class="data-table"><thead><tr><th>版本</th><th>镜像</th><th>状态</th><th>时间</th><th></th></tr></thead><tbody><tr v-for="release in applicationDetail.releases" :key="release.id" class="clickable" @click="openReleaseDetail(release)"><td>#{{ release.sequence }}</td><td class="cell-primary">{{ release.image }}</td><td><span class="badge" :class="releaseBadge(release.status)">{{ release.status }}</span></td><td>{{ formatTime(release.created_at) }}</td><td class="btn-group action-cell" @click.stop><button v-if="release.status === 'failed'" class="btn btn-sm" @click="retryRelease(release)">重试</button><button class="btn btn-sm" @click="rollbackRelease(release)">回滚</button></td></tr></tbody></table></div>
        <div v-if="releaseDetail" class="release-steps"><strong>Release #{{ releaseDetail.sequence }} 步骤</strong><div v-for="operation in releaseDetail.operations" :key="operation.id" class="step-row"><span :class="['status-dot', operation.status]"></span><span>{{ operation.step }}</span><span>{{ operation.status }}</span><small>{{ operation.detail || '-' }}</small></div></div>
      </div>
    </template>

    <template v-else-if="section === 'projects'">
      <div v-if="projectsLoaded && projects.length > 0" class="card">
        <div class="table-wrap"><table class="data-table"><thead><tr><th>项目</th><th>说明</th><th>环境与命名空间</th><th>已关联应用</th><th>操作</th></tr></thead><tbody>
          <tr v-for="project in projects" :key="project.id" class="clickable" @click="openProjectEnvironments(project)"><td class="cell-primary">{{ project.name }}</td><td>{{ project.description || '-' }}</td><td><div v-if="project.environments?.length" class="environment-links"><span v-for="environment in project.environments" :key="environment.id" class="environment-link"><strong>{{ environment.name }}</strong><small>{{ environment.namespace }}</small></span></div><span v-else>-</span></td><td>{{ applicationCount(project.id) }}</td><td class="action-cell" @click.stop><div class="btn-group"><button class="btn btn-sm" @click="openProjectEditor(project)">编辑</button><button class="btn btn-sm" @click="openProjectEnvironments(project)">管理环境</button><button class="btn btn-sm btn-danger" :disabled="projectHasDependents(project)" :title="projectHasDependents(project) ? '请先删除关联环境和应用' : '删除项目'" @click="requestProjectDelete(project)">删除</button></div></td></tr>
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
      <div class="form-group"><label class="form-label">应用名</label><input v-model="createForm.applicationName" class="form-input" required placeholder="order-api" /></div>
      <div class="modal-actions"><button type="button" class="btn" @click="showCreate = false">取消</button><button class="btn btn-primary" :disabled="submitting">{{ submitting ? '创建中...' : '创建' }}</button></div>
    </form></div></div>

    <div v-if="showProjectModal" class="overlay" @click.self="closeProjectModal"><div class="modal"><h2 class="modal-title">{{ editingProject ? '编辑项目' : '新建项目' }}</h2><form @submit.prevent="saveProject">
      <div class="form-group"><label class="form-label">项目名称</label><input v-model="projectForm.name" class="form-input" required placeholder="commerce" :disabled="editingProject && applicationCount(editingProject.id) > 0" /></div>
      <div class="form-group"><label class="form-label">说明</label><textarea v-model="projectForm.description" class="form-input form-textarea" placeholder="订单服务及其部署环境" /></div>
      <div class="modal-actions"><button type="button" class="btn" @click="closeProjectModal">取消</button><button class="btn btn-primary" :disabled="submitting">{{ submitting ? '保存中...' : editingProject ? '保存' : '创建项目' }}</button></div>
    </form></div></div>

    <div v-if="projectDeleteTarget" class="overlay" @click.self="projectDeleteTarget = null"><div class="modal"><h2 class="modal-title">删除项目</h2><p class="confirm-copy">确认删除项目“{{ projectDeleteTarget.name }}”吗？该操作不可撤销。</p><div class="modal-actions"><button class="btn" @click="projectDeleteTarget = null">取消</button><button class="btn btn-danger" :disabled="submitting" @click="deleteProject">删除</button></div></div></div>

    <div v-if="releaseApp" class="overlay" @click.self="releaseApp = null"><div class="modal release-modal"><h2 class="modal-title">发布 {{ releaseApp.name }}</h2><form @submit.prevent="createRelease">
      <div class="form-group"><label class="form-label">镜像</label><input v-model="releaseForm.image" class="form-input" required placeholder="registry.example.com/order-api:1.0.0" /></div>
      <div class="form-row"><div class="form-group"><label class="form-label">容器端口</label><input v-model.number="releaseForm.container_port" type="number" class="form-input" min="1" required /></div><div class="form-group"><label class="form-label">副本数</label><input v-model.number="releaseForm.replicas" type="number" class="form-input" min="1" required /></div></div>
      <div class="form-row"><div class="form-group"><label class="form-label">CPU 请求/限制</label><input v-model="releaseForm.resources.requests_cpu" class="form-input" required /><input v-model="releaseForm.resources.limits_cpu" class="form-input compact-input" required /></div><div class="form-group"><label class="form-label">内存 请求/限制</label><input v-model="releaseForm.resources.requests_memory" class="form-input" required /><input v-model="releaseForm.resources.limits_memory" class="form-input compact-input" required /></div></div>
      <div class="form-row"><div class="form-group"><label class="form-label">就绪检查</label><input v-model="releaseForm.health.readiness_path" class="form-input" required /></div><div class="form-group"><label class="form-label">存活检查</label><input v-model="releaseForm.health.liveness_path" class="form-input" required /></div></div>
      <div class="form-row"><div class="form-group"><label class="form-label">Service 端口</label><input v-model.number="releaseForm.service.port" type="number" class="form-input" required /></div><div class="form-group"><label class="form-label">暴露方式</label><select v-model="releaseForm.endpoint.exposure" class="form-select"><option value="cluster">仅集群内</option><option value="public">公网域名</option></select></div></div>
      <div v-if="releaseForm.endpoint.exposure === 'public'" class="form-row"><div class="form-group"><label class="form-label">域名</label><input v-model="releaseForm.endpoint.domain" class="form-input" required placeholder="api.example.com" /></div><div class="form-group"><label class="form-label">Issuer</label><input v-model="releaseForm.endpoint.issuer_ref" class="form-input" placeholder="letsencrypt-prod" /></div></div>
      <label v-if="releaseForm.endpoint.exposure === 'public'" class="check-row"><input v-model="releaseForm.endpoint.tls_enabled" type="checkbox" /> 启用 HTTPS</label>
      <div class="modal-actions"><button type="button" class="btn" @click="releaseApp = null">取消</button><button class="btn btn-primary" :disabled="submitting">{{ submitting ? '提交中...' : '开始发布' }}</button></div>
    </form></div></div>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, ref, watch } from 'vue'
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
const applicationDetail = ref(null)
const releaseDetail = ref(null)
const createEnvironments = ref([])
let releasePoller = null
const createForm = ref(newApplicationForm())
const projectForm = ref({ name: '', description: '' })
const releaseForm = ref(newReleaseForm())

const pageMeta = computed(() => ({
  applications: { title: '应用', subtitle: '发布和运维平台托管的 Kubernetes 服务' },
  projects: { title: '项目与环境', subtitle: '定义应用归属、部署环境和默认命名空间' },
  releases: { title: '发布记录', subtitle: '查看所有应用的发布过程、状态和失败详情' },
}[props.section] || { title: '应用', subtitle: '发布和运维平台托管的 Kubernetes 服务' }))

function newApplicationForm() { return { projectID: 0, environmentID: 0, applicationName: '' } }
function newReleaseForm() { return { image: '', container_port: 8080, replicas: 1, resources: { requests_cpu: '100m', requests_memory: '128Mi', limits_cpu: '500m', limits_memory: '512Mi' }, health: { readiness_path: '/healthz', liveness_path: '/healthz' }, service: { port: 80, target_port: 8080 }, endpoint: { exposure: 'cluster', domain: '', path: '/', tls_enabled: false, issuer_ref: '' } } }
function endpointLabel(app) { const endpoint = app.endpoints?.[0]; return endpoint?.domain ? `https://${endpoint.domain}` : '集群内' }
function releaseBadge(status) { return status === 'succeeded' ? 'badge-online' : status === 'failed' ? 'badge-danger' : 'badge-offline' }
function formatTime(value) { return value ? new Date(value).toLocaleString() : '-' }
function applicationCount(projectID) { return applications.value.filter(app => app.project_id === projectID).length }
function projectHasDependents(project) { return applicationCount(project.id) > 0 || (project.environments?.length || 0) > 0 }

async function fetchApplications() { error.value = ''; try { applications.value = await api.get('/applications') || [] } catch (e) { error.value = e.message || '加载应用失败' } finally { applicationsLoaded.value = true } }
async function fetchProjects() { error.value = ''; try { projects.value = await api.get('/projects') || [] } catch (e) { error.value = e.message || '加载项目失败' } finally { projectsLoaded.value = true } }
async function fetchEnvironments(projectID) { return await api.get(`/projects/${projectID}/environments`) || [] }
async function loadSection(section) { closeDetails(); if (section === 'projects') { await Promise.all([fetchProjects(), fetchApplications()]) } else if (section === 'releases') { await fetchReleaseHistory() } else { await fetchApplications() } }

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
function openProjectEditor(project) { editingProject.value = project; projectForm.value = { name: project.name, description: project.description || '' }; showProjectModal.value = true }
function closeProjectModal() { showProjectModal.value = false; editingProject.value = null; projectForm.value = { name: '', description: '' } }
async function saveProject() { const isEditing = !!editingProject.value; submitting.value = true; error.value = ''; try { if (isEditing) { await api.put(`/projects/${editingProject.value.id}`, projectForm.value) } else { await api.post('/projects', projectForm.value) } closeProjectModal(); await Promise.all([fetchProjects(), fetchApplications()]) } catch (e) { error.value = e.message || (isEditing ? '更新项目失败' : '创建项目失败') } finally { submitting.value = false } }
function requestProjectDelete(project) { projectDeleteTarget.value = project }
async function deleteProject() { if (!projectDeleteTarget.value) return; submitting.value = true; error.value = ''; try { await api.delete(`/projects/${projectDeleteTarget.value.id}`); projectDeleteTarget.value = null; await Promise.all([fetchProjects(), fetchApplications()]) } catch (e) { error.value = e.message || '删除项目失败' } finally { submitting.value = false } }
async function openProjectEnvironments(project) { await router.push(`/applications/projects/${project.id}`) }
async function fetchReleaseHistory() { error.value = ''; try { await fetchApplications(); const details = await Promise.allSettled(applications.value.map(app => api.get(`/applications/${app.id}`))); releaseHistory.value = details.flatMap((result, index) => result.status === 'fulfilled' ? (result.value.releases || []).map(release => ({ ...release, application: applications.value[index] })) : []).sort((left, right) => new Date(right.created_at || 0) - new Date(left.created_at || 0)) } catch (e) { error.value = e.message || '加载发布记录失败' } finally { releaseHistoryLoaded.value = true } }
async function openHistoryRelease(item) { await router.push('/applications'); await openDetails(item.application); await openReleaseDetail(item) }

function openRelease(app) { releaseApp.value = app; releaseForm.value = newReleaseForm() }
async function createRelease() { submitting.value = true; error.value = ''; try { const applicationID = releaseApp.value.id; const release = await api.post(`/applications/${applicationID}/releases`, releaseForm.value); releaseApp.value = null; await openDetails({ id: applicationID }); await openReleaseDetail(release); fetchApplications() } catch (e) { error.value = e.message || '创建发布失败' } finally { submitting.value = false } }
async function openDetails(app) { try { applicationDetail.value = await api.get(`/applications/${app.id}`); releaseDetail.value = null } catch (e) { error.value = e.message || '加载发布记录失败' } }
function closeDetails() { applicationDetail.value = null; releaseDetail.value = null; stopPolling() }
async function openReleaseDetail(release) { if (!applicationDetail.value) return; try { releaseDetail.value = await api.get(`/applications/${applicationDetail.value.application.id}/releases/${release.id}`); pollRelease() } catch (e) { error.value = e.message || '加载发布步骤失败' } }
function pollRelease() { stopPolling(); if (!releaseDetail.value || ['succeeded', 'failed', 'rolled_back'].includes(releaseDetail.value.status)) return; releasePoller = window.setInterval(async () => { try { releaseDetail.value = await api.get(`/applications/${applicationDetail.value.application.id}/releases/${releaseDetail.value.id}`); if (['succeeded', 'failed', 'rolled_back'].includes(releaseDetail.value.status)) { stopPolling(); openDetails(applicationDetail.value.application) } } catch (e) { stopPolling() } }, 2000) }
function stopPolling() { if (releasePoller) { window.clearInterval(releasePoller); releasePoller = null } }
async function retryRelease(release) { try { const next = await api.post(`/applications/${applicationDetail.value.application.id}/releases/${release.id}/retry`); await openDetails(applicationDetail.value.application); await openReleaseDetail(next) } catch (e) { error.value = e.message || '重试失败' } }
async function rollbackRelease(release) { try { const next = await api.post(`/applications/${applicationDetail.value.application.id}/releases/${release.id}/rollback`); await openDetails(applicationDetail.value.application); await openReleaseDetail(next) } catch (e) { error.value = e.message || '回滚失败' } }

watch(() => props.section, loadSection, { immediate: true })
onBeforeUnmount(stopPolling)
</script>

<style scoped>
.page-header { display:flex; align-items:flex-start; justify-content:space-between; gap:var(--space-16); }
.page-subtitle { margin:var(--space-4) 0 0; color:var(--text-secondary); font-size:13px; }
.release-modal { width:min(680px, calc(100vw - 32px)); }
.compact-input { margin-top:6px; }
.check-row { display:flex; gap:8px; align-items:center; margin:var(--space-12) 0; color:var(--text-secondary); font-size:13px; }
.detail-header { display:flex; justify-content:space-between; gap:var(--space-12); }
.section-title { margin:0; font-size:15px; }
.section-copy { margin:4px 0 0; color:var(--text-secondary); font-size:12px; }
.release-steps { margin-top:var(--space-16); display:grid; gap:8px; }
.step-row { display:grid; grid-template-columns:10px 110px 72px minmax(0, 1fr); align-items:center; gap:8px; color:var(--text-secondary); font-size:12px; }
.status-dot { width:7px; height:7px; border-radius:50%; background:var(--text-muted); }.status-dot.success { background:var(--success); }.status-dot.failed { background:var(--danger); }.status-dot.running { background:var(--warning); }
.form-textarea { min-height:82px; resize:vertical; }
.environment-links { display:flex; flex-wrap:wrap; gap:6px; }.environment-link { display:grid; min-width:96px; padding:4px 6px; border:1px solid var(--border-muted); border-radius:var(--radius-control); background:var(--surface-subtle); line-height:1.25; }.environment-link strong { color:var(--text-primary); font-size:11px; }.environment-link small { margin-top:2px; color:var(--text-muted); font-size:10px; }.confirm-copy { margin:0; color:var(--text-secondary); font-size:13px; }
@media (max-width:640px) { .page-header, .detail-header { align-items:stretch; flex-direction:column; }.page-header .btn { width:100%; }.step-row { grid-template-columns:10px minmax(0, 1fr); }.step-row > :nth-child(3), .step-row > :nth-child(4) { grid-column:2; } }
</style>
