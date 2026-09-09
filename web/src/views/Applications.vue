<template>
  <div @click="closeWorkspacePicker" @keydown.esc="closeWorkspacePicker">
    <div class="page-header">
      <div><h1 class="page-title">{{ pageMeta.title }}</h1><p class="page-subtitle">{{ pageMeta.subtitle }}</p></div>
      <div v-if="section === 'workspace'" class="page-actions"><div class="workspace-context" @click.stop><div class="workspace-picker"><span class="context-picker-label">项目</span><button data-testid="workspace-project-trigger" type="button" class="context-picker-trigger" :class="{ 'is-open': activeWorkspacePicker === 'project' }" :aria-expanded="activeWorkspacePicker === 'project'" aria-haspopup="listbox" @click="toggleWorkspacePicker('project')"><span class="context-picker-value">{{ workspaceProject?.name || '选择项目' }}</span><ChevronDown :size="15" /></button><div v-if="activeWorkspacePicker === 'project'" data-testid="workspace-project-menu" class="context-picker-menu" role="listbox" aria-label="项目"><button v-for="project in projects" :key="project.id" type="button" class="context-picker-option" :class="{ 'is-selected': project.id === workspaceProjectID }" role="option" :aria-selected="project.id === workspaceProjectID" @click="selectWorkspaceProject(project.id)"><span><strong>{{ project.name }}</strong><small>{{ project.description || '未设置项目说明' }}</small></span><Check v-if="project.id === workspaceProjectID" :size="15" /></button><div v-if="!projects.length" class="context-picker-empty">暂无项目</div></div></div><div class="workspace-picker"><span class="context-picker-label">环境</span><button data-testid="workspace-environment-trigger" type="button" class="context-picker-trigger" :class="{ 'is-open': activeWorkspacePicker === 'environment' }" :aria-expanded="activeWorkspacePicker === 'environment'" aria-haspopup="listbox" :disabled="!workspaceProject" @click="toggleWorkspacePicker('environment')"><span class="context-picker-value">{{ workspaceEnvironment ? `${workspaceEnvironment.name} · ${workspaceEnvironment.namespace}` : '选择环境' }}</span><ChevronDown :size="15" /></button><div v-if="activeWorkspacePicker === 'environment'" data-testid="workspace-environment-menu" class="context-picker-menu context-picker-menu--environment" role="listbox" aria-label="环境"><button v-for="environment in workspaceProject?.environments || []" :key="environment.id" type="button" class="context-picker-option" :class="{ 'is-selected': environment.id === workspaceEnvironmentID }" role="option" :aria-selected="environment.id === workspaceEnvironmentID" @click="selectWorkspaceEnvironment(environment.id)"><span><strong>{{ environment.name }}</strong><small>{{ environment.namespace }}</small></span><Check v-if="environment.id === workspaceEnvironmentID" :size="15" /></button><div v-if="!(workspaceProject?.environments || []).length" class="context-picker-empty">该项目暂无环境</div></div></div></div></div>
      <button v-else-if="section === 'projects'" class="btn btn-primary" @click="showProjectModal = true">+ 新建项目</button>
    </div>
    <div v-if="error" class="k8s-banner k8s-banner-warn section-gap">⚠ {{ error }}</div>

    <template v-if="section === 'workspace'">
      <div v-if="!workspaceReady && projectsLoaded" class="empty-state"><span class="empty-text">请选择项目与环境，或先创建部署目标</span><button class="btn btn-primary" @click="openProjectManagement">创建项目与环境</button></div>
      <template v-else-if="workspaceReady && workspaceOverview">
        <div class="overview-metrics workspace-metrics">
          <div class="metric"><span>应用</span><strong>{{ applications.length }}</strong></div><div class="metric"><span>运行中</span><strong>{{ workspaceRuntimeCounts.running }}</strong></div><div class="metric"><span>发布中</span><strong>{{ workspaceRuntimeCounts.deploying }}</strong></div><div class="metric"><span>异常服务</span><strong>{{ workspaceRuntimeCounts.attention }}</strong></div>
        </div>
        <section class="workspace-section"><div class="section-heading"><div><h2>应用</h2><p>当前环境中的服务与入口</p></div><button class="btn btn-sm btn-primary" @click="openCreateApplication">创建应用</button></div>
      <div v-if="applicationsLoaded && applications.length > 0" class="card">
        <div class="table-wrap"><table class="data-table"><thead><tr><th>应用</th><th>运行状态</th><th>运行版本</th><th>最近发布</th><th>访问地址</th><th>操作</th></tr></thead><tbody>
          <tr v-for="app in applications" :key="app.id" class="clickable" @click="openDetails(app)"><td class="cell-primary">{{ app.name }}<small>{{ app.workload_kind === 'statefulset' ? 'StatefulSet' : 'Deployment' }}</small></td><td><span class="badge" :class="runtimeBadge(app.runtime?.status)">{{ runtimeLabel(app.runtime?.status) }}</span><small v-if="app.runtime?.total_pods" class="runtime-count">{{ app.runtime.ready_pods }}/{{ app.runtime.total_pods }} Pods</small></td><td>{{ app.active_release?.version || '-' }}</td><td>{{ app.latest_release ? formatTime(app.latest_release.created_at) : '-' }}<small v-if="app.latest_release" :class="['release-state', releaseBadge(app.latest_release.status)]">{{ releaseLabel(app.latest_release.status) }}</small></td><td><a v-if="app.endpoint_url" class="endpoint-link" :href="app.endpoint_url" target="_blank" rel="noopener noreferrer" @click.stop>{{ app.endpoint_url }}</a><span v-else-if="app.endpoint_access_mode === 'protected_console'" class="endpoint-protected">受保护控制台（请在详情页打开）</span><small v-if="app.endpoint_count > 1" class="endpoint-more">另有 {{ app.endpoint_count - 1 }} 个地址</small><span v-else-if="!app.endpoint_url && app.endpoint_access_mode !== 'protected_console'">集群内</span></td><td><button class="btn btn-sm" @click.stop="openRelease(app)">发布版本</button></td></tr>
        </tbody></table></div>
      </div>
      <div v-else-if="applicationsLoaded" class="empty-state"><span class="empty-text">当前项目与环境下还没有应用</span></div></section>
        <div class="workspace-grid">
          <section class="workspace-section"><div class="section-heading"><div><h2>最近发布</h2><p>仅显示当前环境的最新 8 次发布</p></div></div><div v-if="workspaceRecentReleases.length" class="compact-list"><button v-for="release in workspaceRecentReleases" :key="release.id" class="list-row" @click="openWorkspaceRelease(release)"><span><strong>{{ release.application_name }}</strong><small>#{{ release.sequence }} · {{ release.image }} · {{ formatTime(release.created_at) }}</small></span><span class="badge" :class="releaseBadge(release.status)">{{ releaseLabel(release.status) }}</span></button></div><div v-else class="empty-inline">暂无发布记录</div></section>
          <section class="workspace-section"><div class="section-heading"><div><h2>受管域名</h2><p>证书和入口均归属当前环境</p></div><button class="btn btn-sm" @click="openDomains">管理域名</button></div><div v-if="workspaceDomains.length" class="compact-list"><button v-for="domain in workspaceDomains" :key="domain.id" class="list-row" @click="openDomains"><span><strong>{{ domain.hostname }}</strong><small>{{ domain.tls_secret_name || '等待 TLS Secret' }}</small></span><span class="badge" :class="domain.certificate?.status === 'Ready' ? 'badge-online' : 'badge-deploying'">{{ domain.certificate?.status === 'Ready' ? '已就绪' : '签发中' }}</span></button></div><div v-else class="empty-inline">尚未申请受管域名</div></section>
          <section class="workspace-section"><div class="section-heading"><div><h2>项目镜像仓库</h2><p>{{ workspaceDefaultRegistry ? `默认：${workspaceDefaultRegistry.name}` : '尚未设置默认镜像仓库' }}</p></div><button class="btn btn-sm" @click="openRegistries">管理仓库</button></div><div v-if="workspaceRegistries.length" class="compact-list"><button v-for="registry in workspaceRegistries" :key="registry.id" class="list-row" @click="openRegistries"><span><strong>{{ registry.name }}</strong><small>{{ registry.endpoint }}</small></span><span class="badge" :class="registry.id === workspaceDefaultRegistryID ? 'badge-online' : registry.enabled ? 'badge-deploying' : 'badge-offline'">{{ registry.id === workspaceDefaultRegistryID ? '默认' : registry.enabled ? '可用' : '停用' }}</span></button></div><div v-else class="empty-inline">当前项目还未授权镜像仓库</div></section>
        </div>
      </template>
    </template>

    <template v-else-if="section === 'projects'">
      <div v-if="projectsLoaded && projects.length > 0" class="card">
        <div class="table-wrap"><table class="data-table"><thead><tr><th>项目</th><th>说明</th><th>默认镜像仓库</th><th>已授权镜像仓库</th><th>环境与命名空间</th><th>已关联应用</th><th>操作</th></tr></thead><tbody>
          <tr v-for="project in projects" :key="project.id" class="clickable" @click="openProjectEnvironments(project)"><td class="cell-primary">{{ project.name }}</td><td>{{ project.description || '-' }}</td><td>{{ project.default_image_registry?.name || '-' }}</td><td><div v-if="authorizedProjectRegistries(project).length" class="registry-links"><span v-for="registry in authorizedProjectRegistries(project)" :key="registry.id" class="registry-link">{{ registry.name }}<small :class="registry.enabled ? '' : 'registry-disabled'">{{ registry.enabled ? registry.endpoint : '已停用' }}</small></span></div><span v-else>-</span></td><td><div v-if="project.environments?.length" class="environment-links"><span v-for="environment in project.environments" :key="environment.id" class="environment-link"><strong>{{ environment.name }}</strong><small>{{ environment.namespace }}</small></span></div><span v-else>-</span></td><td>{{ applicationCount(project.id) }}</td><td class="action-cell" @click.stop><div class="btn-group"><button class="btn btn-sm" @click="openProjectEditor(project)">编辑</button><button class="btn btn-sm" @click="openProjectEnvironments(project)">管理环境</button><button class="btn btn-sm btn-danger" title="删除项目" @click="requestProjectDelete(project)">删除</button></div></td></tr>
        </tbody></table></div>
      </div>
    </template>

    <template v-else>
      <div v-if="section === 'overview' && releaseHistoryLoaded" class="overview-metrics">
        <div class="metric"><span>跨项目应用</span><strong>{{ applications.length }}</strong></div><div class="metric"><span>失败发布</span><strong>{{ releaseHistory.filter(item => item.status === 'failed').length }}</strong></div><div class="metric"><span>证书告警</span><strong>{{ certificateAlerts.length }}</strong></div>
      </div>
      <section v-if="section === 'overview' && certificateAlerts.length" class="card section-gap"><div class="table-wrap"><table class="data-table"><thead><tr><th>域名</th><th>环境</th><th>证书状态</th><th>原因</th></tr></thead><tbody><tr v-for="domain in certificateAlerts" :key="domain.id"><td class="cell-primary">{{ domain.hostname }}</td><td>{{ domain.namespace || '-' }}</td><td><span class="badge badge-danger">{{ domain.certificate?.status || '未就绪' }}</span></td><td>{{ domain.certificate?.reason || domain.certificate_error || '-' }}</td></tr></tbody></table></div></section>
      <section v-if="section === 'overview' && unassignedDomains.length" class="k8s-banner k8s-banner-warn section-gap"><span>有 {{ unassignedDomains.length }} 个历史受管域名尚未归属环境，不能用于发布。</span><button class="btn btn-sm" @click="openUnassignedDomains">处理域名归属</button></section>
      <div v-if="releaseHistoryLoaded && releaseHistory.length > 0" class="card">
        <div class="table-wrap"><table class="data-table"><thead><tr><th>应用</th><th>环境</th><th>版本</th><th>镜像</th><th>状态</th><th>时间</th></tr></thead><tbody>
          <tr v-for="item in releaseHistory" :key="item.id" class="clickable" @click="openHistoryRelease(item)"><td class="cell-primary">{{ item.application.name }}</td><td>{{ item.application.environment?.name || '-' }}</td><td>#{{ item.sequence }}</td><td>{{ item.image }}</td><td><span class="badge" :class="releaseBadge(item.status)">{{ item.status }}</span></td><td>{{ formatTime(item.created_at) }}</td></tr>
        </tbody></table></div>
      </div>
    </template>

    <div v-if="showCreate" class="overlay" @click.self="showCreate = false"><div class="modal"><h2 class="modal-title">创建应用</h2><form @submit.prevent="createApplication">
      <div class="form-group"><label class="form-label">项目与环境</label><input class="form-input" :value="`${workspaceProject?.name || ''} · ${workspaceEnvironment?.name || ''} · ${workspaceEnvironment?.namespace || ''}`" disabled /></div>
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
      <div v-if="releaseTemplates.length" class="form-group"><label class="form-label">上线模板</label><select v-model.number="releaseForm.template_id" class="form-select" required><option v-for="template in releaseTemplates" :key="template.id" :value="template.id">{{ template.name }}{{ template.is_default ? '（默认）' : '' }}</option></select></div>
      <div v-if="selectedReleaseTemplate" class="template-summary"><span>模板 v{{ selectedReleaseTemplate.revision }}</span><strong>{{ selectedReleaseTemplate.spec.image }}</strong><small>{{ selectedReleaseTemplate.spec.replicas }} 副本 · Service {{ selectedReleaseTemplate.spec.service?.port }}</small></div>
      <p v-else class="template-intro">当前应用尚未配置上线模板，请先在应用详情中创建模板。</p>
      <div class="form-group"><label class="form-label">版本号</label><input v-model.trim="releaseForm.version" class="form-input" required placeholder="1.4.2" pattern="[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}" :disabled="!releaseTemplates.length" /><p class="form-hint">平台将使用所选模板的镜像路径与此版本号组合最终镜像。</p></div>
      <div class="modal-actions"><button type="button" class="btn" @click="releaseApp = null">取消</button><button v-if="!releaseTemplates.length" type="button" class="btn btn-primary" @click="openTemplateManagement(releaseApp)">管理模板</button><button v-else class="btn btn-primary" :disabled="submitting">{{ submitting ? '提交中...' : '开始发布' }}</button></div>
    </form></div></div></Teleport>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Check, ChevronDown } from 'lucide-vue-next'
import { api } from '../api/index.js'
import { getApplication, getApplications, getDeploymentTemplates, getDomains, getImageRegistries, getProjects, getWorkspace } from '../api/applications.js'
import { useAsyncResource } from '../composables/useAsyncResource.js'

const props = defineProps({ section: { type: String, default: 'workspace' } })
const router = useRouter()
const route = useRoute()
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
const releaseTemplates = ref([])
const imageRegistries = ref([])
const createForm = ref(newApplicationForm())
const projectForm = ref({ name: '', description: '', defaultImageRegistryID: 0 })
const releaseForm = ref(newReleaseForm())
const globalDomains = ref([])
const unassignedManagedDomains = ref([])
const workspaceOverview = ref(null)
const workspaceRegistries = ref([])
const activeWorkspacePicker = ref('')
const applicationsResource = useAsyncResource(({ signal }, scope = {}) => getApplications(scope, { signal }), [])
const projectsResource = useAsyncResource(({ signal }) => getProjects({ signal }), [])
const imageRegistriesResource = useAsyncResource(({ signal }, projectID) => getImageRegistries({ projectID }, { signal }), [])
const workspaceResource = useAsyncResource(async ({ signal }, projectID, environmentID) => {
  const [overview, registries] = await Promise.all([
    getWorkspace(projectID, environmentID, { signal }),
    getImageRegistries({ projectID }, { signal }),
  ])
  return { overview, registries }
}, null)
const releaseHistoryResource = useAsyncResource(async ({ signal }) => {
  const apps = await getApplications({}, { signal }) || []
  const details = await Promise.allSettled(apps.map(app => getApplication(app.id, { signal })))
  return {
    applications: apps,
    releases: details.flatMap((result, index) => result.status === 'fulfilled'
      ? (result.value.releases || []).map(release => ({ ...release, application: apps[index] }))
      : [])
      .sort((left, right) => new Date(right.created_at || 0) - new Date(left.created_at || 0)),
  }
}, null)
const overviewDomainsResource = useAsyncResource(async ({ signal }) => {
  const [domains, unassigned] = await Promise.all([
    getDomains({}, { signal }),
    getDomains({ unassigned: true }, { signal }),
  ])
  return { domains, unassigned }
}, null)
const releaseTemplatesResource = useAsyncResource(({ signal }, applicationID) => getDeploymentTemplates(applicationID, { signal }), [])
let sectionLoadID = 0

const pageMeta = computed(() => ({
  workspace: { title: '工作台', subtitle: '当前项目与环境下的应用、发布和运行状态' },
  projects: { title: '项目与环境', subtitle: '定义应用归属、部署环境和默认命名空间' },
  overview: { title: '全局概览', subtitle: '跨项目应用、失败发布与证书告警' },
}[props.section] || { title: '工作台', subtitle: '当前项目与环境下的应用、发布和运行状态' }))

const workspaceProjectID = computed(() => Number(route.query.project_id) || 0)
const workspaceEnvironmentID = computed(() => Number(route.query.environment_id) || 0)
const workspaceProject = computed(() => projects.value.find(item => item.id === workspaceProjectID.value) || null)
const workspaceEnvironment = computed(() => workspaceProject.value?.environments?.find(item => item.id === workspaceEnvironmentID.value) || null)
const workspaceReady = computed(() => !!workspaceProject.value && !!workspaceEnvironment.value)
const certificateAlerts = computed(() => globalDomains.value.filter(domain => domain.certificate?.status !== 'Ready'))
const unassignedDomains = computed(() => unassignedManagedDomains.value)
const workspaceDomains = computed(() => workspaceOverview.value?.domains || [])
const workspaceRecentReleases = computed(() => workspaceOverview.value?.recent_releases || [])
const workspaceDefaultRegistryID = computed(() => workspaceProject.value?.default_image_registry_id || 0)
const workspaceDefaultRegistry = computed(() => workspaceRegistries.value.find(registry => registry.id === workspaceDefaultRegistryID.value) || workspaceProject.value?.default_image_registry || null)
const selectedReleaseTemplate = computed(() => releaseTemplates.value.find(template => template.id === releaseForm.value.template_id) || null)
const workspaceRuntimeCounts = computed(() => (applications.value || []).reduce((counts, app) => {
  const status = app.runtime?.status
  if (status === 'running') counts.running++
  else if (status === 'deploying') counts.deploying++
  else if (status === 'degraded' || status === 'unavailable') counts.attention++
  return counts
}, { running: 0, deploying: 0, attention: 0 }))

function newApplicationForm() { return { projectID: 0, environmentID: 0, applicationName: '' } }
function newReleaseForm() { return { template_id: 0, version: '' } }
function releaseBadge(status) { return status === 'succeeded' ? 'badge-online' : status === 'failed' ? 'badge-danger' : 'badge-offline' }
function releaseLabel(status) { return status === 'succeeded' ? '成功' : status === 'failed' ? '失败' : '发布中' }
function runtimeBadge(status) { return status === 'running' ? 'badge-online' : status === 'deploying' ? 'badge-deploying' : status === 'not_released' || status === 'unknown' ? 'badge-offline' : 'badge-danger' }
function runtimeLabel(status) { return ({ running: '运行中', deploying: '发布中', degraded: '异常', unavailable: '不可用', not_released: '未发布', unknown: '未知' })[status] || '未知' }
function formatTime(value) { return value ? new Date(value).toLocaleString() : '-' }
function applicationCount(projectID) { return applications.value.filter(app => app.project_id === projectID).length }

async function fetchApplications(scoped = false) {
  error.value = ''
  const scope = scoped && workspaceReady.value ? { projectID: workspaceProjectID.value, environmentID: workspaceEnvironmentID.value } : {}
  const result = await applicationsResource.refresh(scope)
  if (result !== undefined) applications.value = result || []
  else if (applicationsResource.error.value) error.value = applicationsResource.error.value.message || '加载应用失败'
  applicationsLoaded.value = true
}

async function fetchProjects() {
  error.value = ''
  const result = await projectsResource.refresh()
  if (result !== undefined) projects.value = result || []
  else if (projectsResource.error.value) error.value = projectsResource.error.value.message || '加载项目失败'
  projectsLoaded.value = true
}

async function fetchImageRegistries(projectID) {
  const result = await imageRegistriesResource.refresh(projectID)
  if (result !== undefined) imageRegistries.value = result || []
  else if (imageRegistriesResource.error.value) {
    imageRegistries.value = []
    error.value = imageRegistriesResource.error.value.message || '加载镜像仓库失败'
  }
}

async function fetchWorkspace() {
  error.value = ''
  const result = await workspaceResource.refresh(workspaceProjectID.value, workspaceEnvironmentID.value)
  if (result !== undefined) {
    workspaceOverview.value = result.overview
    workspaceRegistries.value = result.registries || []
    applications.value = result.overview?.applications || []
  } else if (workspaceResource.error.value) {
    workspaceOverview.value = null
    workspaceRegistries.value = []
    error.value = workspaceResource.error.value.message || '加载工作台失败'
  }
  applicationsLoaded.value = true
}

async function fetchReleaseHistory() {
  error.value = ''
  const result = await releaseHistoryResource.refresh()
  if (result !== undefined) {
    applications.value = result.applications
    releaseHistory.value = result.releases
  } else if (releaseHistoryResource.error.value) error.value = releaseHistoryResource.error.value.message || '加载发布记录失败'
  releaseHistoryLoaded.value = true
}

async function fetchOverviewDomains() {
  const result = await overviewDomainsResource.refresh()
  if (result !== undefined) {
    globalDomains.value = result.domains || []
    unassignedManagedDomains.value = result.unassigned || []
  } else if (overviewDomainsResource.error.value) {
    globalDomains.value = []
    unassignedManagedDomains.value = []
    error.value = overviewDomainsResource.error.value.message || '加载证书概览失败'
  }
}

async function loadSection(section) {
  const loadID = ++sectionLoadID
  if (section === 'projects') {
    await Promise.all([fetchProjects(), fetchApplications(), fetchImageRegistries()])
    return
  }
  if (section === 'overview') {
    await Promise.all([fetchReleaseHistory(), fetchOverviewDomains()])
    return
  }
  if (workspaceProjectID.value && workspaceEnvironmentID.value) {
    await Promise.all([fetchProjects(), fetchWorkspace()])
    return
  }

  await fetchProjects()
  if (loadID !== sectionLoadID) return
  if (!workspaceProjectID.value) {
    const saved = JSON.parse(localStorage.getItem('cylism.application-workspace') || '{}')
    const savedProject = projects.value.find(project => project.id === Number(saved.project_id))
    const savedEnvironment = savedProject?.environments?.find(environment => environment.id === Number(saved.environment_id))
    if (savedProject && savedEnvironment) {
      await updateWorkspace(savedProject.id, savedEnvironment.id)
      return
    }
  }
  if (workspaceReady.value) await fetchWorkspace()
  else {
    workspaceOverview.value = null
    workspaceRegistries.value = []
    applications.value = []
    applicationsLoaded.value = true
  }
}

async function openCreateApplication() {
  error.value = ''
  if (!workspaceReady.value) { error.value = '请先在顶部选择项目与环境'; return }
  createForm.value = { projectID: workspaceProjectID.value, environmentID: workspaceEnvironmentID.value, applicationName: '' }
  showCreate.value = true
}
async function createApplication() { submitting.value = true; error.value = ''; try { await api.post('/applications', { project_id: createForm.value.projectID, environment_id: createForm.value.environmentID, name: createForm.value.applicationName }); showCreate.value = false; createForm.value = newApplicationForm(); await fetchWorkspace() } catch (e) { error.value = e.message || '创建应用失败' } finally { submitting.value = false } }
function projectRegistries(project) { return imageRegistries.value.filter(registry => registry.enabled && registry.projects?.some(allowedProject => allowedProject.id === project.id)) }
function authorizedProjectRegistries(project) { return imageRegistries.value.filter(registry => registry.projects?.some(allowedProject => allowedProject.id === project.id)) }
function openProjectEditor(project) { editingProject.value = project; projectForm.value = { name: project.name, description: project.description || '', defaultImageRegistryID: project.default_image_registry_id || 0 }; showProjectModal.value = true }
function closeProjectModal() { showProjectModal.value = false; editingProject.value = null; projectForm.value = { name: '', description: '', defaultImageRegistryID: 0 } }
async function saveProject() { const isEditing = !!editingProject.value; submitting.value = true; error.value = ''; try { if (isEditing) { await api.put(`/projects/${editingProject.value.id}`, { name: projectForm.value.name, description: projectForm.value.description, default_image_registry_id: projectForm.value.defaultImageRegistryID }) } else { await api.post('/projects', { name: projectForm.value.name, description: projectForm.value.description }) } closeProjectModal(); await Promise.all([fetchProjects(), fetchApplications()]) } catch (e) { error.value = e.message || (isEditing ? '更新项目失败' : '创建项目失败') } finally { submitting.value = false } }
function requestProjectDelete(project) { projectDeleteTarget.value = project }
async function deleteProject() { if (!projectDeleteTarget.value) return; submitting.value = true; error.value = ''; try { await api.delete(`/projects/${projectDeleteTarget.value.id}`); projectDeleteTarget.value = null; await Promise.all([fetchProjects(), fetchApplications()]) } catch (e) { error.value = e.message || '删除项目失败' } finally { submitting.value = false } }
async function openProjectEnvironments(project) { await router.push(`/applications/projects/${project.id}`) }
async function openHistoryRelease(item) { await router.push(`/applications/${item.application.id}/releases/${item.id}`) }
async function openUnassignedDomains() { await router.push({ path: '/applications/domains', query: { ...route.query, unassigned: 'true' } }) }
async function openProjectManagement() { await router.push('/applications/projects') }
async function updateWorkspace(projectID, environmentID) { const query = {}; if (projectID) query.project_id = String(projectID); if (environmentID) query.environment_id = String(environmentID); if (projectID && environmentID) localStorage.setItem('cylism.application-workspace', JSON.stringify({ project_id: projectID, environment_id: environmentID })); await router.push({ path: '/applications', query }) }
function closeWorkspacePicker() { activeWorkspacePicker.value = '' }
function toggleWorkspacePicker(picker) { activeWorkspacePicker.value = activeWorkspacePicker.value === picker ? '' : picker }
async function selectWorkspaceProject(value) { closeWorkspacePicker(); const projectID = Number(value) || 0; const project = projects.value.find(item => item.id === projectID); const environmentID = project?.environments?.length === 1 ? project.environments[0].id : 0; await updateWorkspace(projectID, environmentID) }
async function selectWorkspaceEnvironment(value) { closeWorkspacePicker(); await updateWorkspace(workspaceProjectID.value, Number(value) || 0) }
async function openDomains() { await router.push({ path: '/applications/domains', query: { project_id: workspaceProjectID.value, environment_id: workspaceEnvironmentID.value } }) }
async function openRegistries() { await router.push({ path: '/applications/registries', query: { project_id: workspaceProjectID.value, environment_id: workspaceEnvironmentID.value } }) }
async function openWorkspaceRelease(release) { await router.push(`/applications/${release.application_id}/releases/${release.id}`) }

async function openRelease(app) {
  error.value = ''
  releaseForm.value = newReleaseForm()
  releaseTemplates.value = []
  const result = await releaseTemplatesResource.refresh(app.id)
  if (result !== undefined) {
    releaseTemplates.value = (result || []).filter(template => template.enabled)
    const defaultTemplate = releaseTemplates.value.find(template => template.is_default) || releaseTemplates.value[0]
    releaseForm.value.template_id = defaultTemplate?.id || 0
  } else if (releaseTemplatesResource.error.value) error.value = releaseTemplatesResource.error.value.message || '加载上线模板失败'
  releaseApp.value = app
}
async function createRelease() { error.value = ''; if (!releaseForm.value.template_id || !releaseForm.value.version) { error.value = '请选择上线模板并填写版本号'; return } submitting.value = true; try { const applicationID = releaseApp.value.id; const release = await api.post(`/applications/${applicationID}/releases`, releaseForm.value); releaseApp.value = null; await router.push(`/applications/${applicationID}/releases/${release.id}`); fetchWorkspace() } catch (e) { error.value = e.message || '创建发布失败' } finally { submitting.value = false } }
async function openTemplateManagement(app) { releaseApp.value = null; await router.push(`/applications/${app.id}`) }
async function openDetails(app) { await router.push(`/applications/${app.id}`) }

watch(() => [props.section, route.query.project_id, route.query.environment_id], () => loadSection(props.section), { immediate: true })
</script>

<style scoped>
.page-header { display:flex; align-items:flex-start; justify-content:space-between; gap:var(--space-16); }
.page-subtitle { margin:var(--space-4) 0 0; color:var(--text-secondary); font-size:13px; }
.release-modal { width:min(680px, calc(100vw - 32px)); }.template-summary{display:grid;gap:4px;margin-bottom:var(--space-16);padding:12px;border:1px solid var(--border-muted);border-radius:var(--radius-control);background:var(--surface-subtle)}.template-summary span,.template-summary small,.template-intro{color:var(--text-secondary);font-size:12px}.template-summary strong{overflow-wrap:anywhere;font-size:13px}.template-intro{margin:0 0 var(--space-16)}
.compact-input { margin-top:6px; }.unit-input { display:grid; grid-template-columns:minmax(0, 1fr) 86px; gap:6px; }
.check-row { display:flex; gap:8px; align-items:center; margin:var(--space-12) 0; color:var(--text-secondary); font-size:13px; }
.form-textarea { min-height:82px; resize:vertical; }
.environment-links,.registry-links { display:flex; flex-wrap:wrap; gap:6px; }.environment-link,.registry-link { display:grid; min-width:96px; padding:4px 6px; border:1px solid var(--border-muted); border-radius:var(--radius-control); background:var(--surface-subtle); line-height:1.25; }.environment-link strong,.registry-link { color:var(--text-primary); font-size:11px; }.environment-link small,.registry-link small { margin-top:2px; color:var(--text-muted); font-size:10px; }.registry-disabled { color:var(--danger)!important; }.confirm-copy { margin:0; color:var(--text-secondary); font-size:13px; }
.overview-metrics{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:8px;margin-bottom:var(--space-16)}.metric{display:grid;gap:4px;padding:12px;border:1px solid var(--border-muted);border-radius:var(--radius-control);background:var(--surface-subtle)}.metric span{color:var(--text-secondary);font-size:12px}.metric strong{font-size:22px}
.page-actions{display:flex;align-items:center;justify-content:flex-end;flex-wrap:wrap;gap:8px}.workspace-context{display:flex;align-items:stretch;gap:8px}.workspace-picker{position:relative;display:grid;min-width:144px;gap:3px}.context-picker-label{padding-left:2px;color:var(--text-muted);font-size:10px;font-weight:700;line-height:1}.context-picker-trigger{display:flex;min-width:0;height:36px;align-items:center;gap:7px;padding:0 9px 0 10px;border:1px solid var(--border-muted);border-radius:var(--radius-control);background:var(--surface-raised);color:var(--text-primary);font:inherit;font-size:12px;text-align:left;cursor:pointer;transition:border-color .18s ease,background .18s ease,box-shadow .18s ease}.context-picker-trigger:hover:not(:disabled){border-color:var(--focus);background:var(--surface-hover)}.context-picker-trigger:focus-visible{outline:2px solid var(--focus);outline-offset:2px}.context-picker-trigger.is-open{border-color:var(--focus);box-shadow:0 0 0 2px color-mix(in srgb,var(--focus) 18%,transparent)}.context-picker-trigger:disabled{cursor:not-allowed;opacity:.55}.context-picker-value{min-width:0;overflow:hidden;flex:1;text-overflow:ellipsis;white-space:nowrap}.context-picker-trigger svg{flex:0 0 auto;color:var(--text-muted);transition:transform .18s ease}.context-picker-trigger.is-open svg{transform:rotate(180deg);color:var(--action-primary)}.context-picker-menu{position:absolute;z-index:25;top:calc(100% + 6px);left:0;display:grid;width:max-content;min-width:100%;max-width:min(310px,calc(100vw - 32px));gap:3px;padding:5px;border:1px solid var(--border);border-radius:var(--radius-control);background:var(--surface-raised);box-shadow:var(--shadow);backdrop-filter:blur(24px) saturate(140%)}.context-picker-menu--environment{min-width:230px}.context-picker-option{display:flex;min-width:0;align-items:center;justify-content:space-between;gap:18px;padding:8px;border:0;border-radius:6px;background:transparent;color:var(--text-primary);font:inherit;text-align:left;cursor:pointer}.context-picker-option:hover,.context-picker-option.is-selected{background:var(--surface-hover)}.context-picker-option span{display:grid;min-width:0;gap:2px}.context-picker-option strong,.context-picker-option small{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.context-picker-option strong{font-size:12px}.context-picker-option small{color:var(--text-muted);font-size:10px}.context-picker-option svg{flex:0 0 auto;color:var(--action-primary)}.context-picker-empty{padding:9px 8px;color:var(--text-muted);font-size:11px}.workspace-metrics{grid-template-columns:repeat(4,minmax(0,1fr))}.workspace-section{min-width:0;padding:var(--space-16) 0;border-top:1px solid var(--border-muted)}.section-heading{display:flex;align-items:flex-start;justify-content:space-between;gap:var(--space-12);margin-bottom:var(--space-12)}.section-heading h2{margin:0;font-size:15px}.section-heading p{margin:4px 0 0;color:var(--text-secondary);font-size:12px}.workspace-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:var(--space-16)}.compact-list{display:grid;gap:6px}.list-row{display:flex;align-items:center;justify-content:space-between;gap:8px;width:100%;padding:8px;border:1px solid var(--border-muted);border-radius:var(--radius-control);background:var(--surface-subtle);color:var(--text-primary);font:inherit;text-align:left;cursor:pointer}.list-row:hover{background:var(--surface-raised)}.list-row span:first-child{display:grid;min-width:0;gap:2px}.list-row strong,.list-row small{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.list-row small{color:var(--text-secondary);font-size:11px}.runtime-count,.release-state,.endpoint-more{display:block;margin-top:4px;font-size:10px}.endpoint-link{display:inline-block;max-width:260px;overflow:hidden;color:var(--action-primary);text-overflow:ellipsis;white-space:nowrap;text-decoration:none}.endpoint-link:hover{text-decoration:underline}.empty-inline{padding:14px 0;color:var(--text-muted);font-size:12px}
@media (max-width:760px){.workspace-grid{grid-template-columns:1fr}.workspace-metrics{grid-template-columns:repeat(2,minmax(0,1fr))}}@media (max-width:640px) { .page-header { align-items:stretch; flex-direction:column; }.page-header .btn { width:100%; }.page-actions{width:100%}.workspace-context{width:100%}.workspace-picker{flex:1;min-width:0}.context-picker-menu{max-width:calc(100vw - 32px)} }
</style>
