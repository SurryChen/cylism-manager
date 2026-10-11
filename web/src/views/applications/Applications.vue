<template>
  <div @click="closeWorkspacePicker" @keydown.esc="closeWorkspacePicker">
    <SectionTabsHeader v-if="section === 'workspace'" title="工作台" :tabs="workspaceTabs" :active-tab="activeWorkspaceTab" test-id-prefix="workspace-tab" @select="selectWorkspaceTab" />
    <PageHeader v-else :title="pageMeta.title" :description="pageMeta.subtitle" sticky><template #actions><button class="btn btn-primary" @click="showProjectModal = true">+ 新建项目</button></template></PageHeader>
    <FeedbackBanner v-if="readError" data-testid="applications-read-error" tone="warning" :message="readError" />

    <template v-if="section === 'workspace'">
      <SurfaceCard as="div" padding="none" class="workspace-list" data-testid="workspace-list">
        <div class="workspace-toolbar">
          <div class="workspace-context">
            <div class="workspace-picker">
              <span class="context-picker-label">项目</span>
              <SelectMenu data-testid="workspace-project-picker" class="context-picker-select" :model-value="workspaceProjectID" :options="workspaceProjectOptions" placeholder="选择项目" aria-label="项目" @update:model-value="selectWorkspaceProject" />
            </div>
            <div class="workspace-picker">
              <span class="context-picker-label">环境</span>
              <SelectMenu data-testid="workspace-environment-picker" class="context-picker-select context-picker-select--environment" :model-value="workspaceEnvironmentID" :options="workspaceEnvironmentOptions" placeholder="选择环境" aria-label="环境" :disabled="!workspaceProject" @update:model-value="selectWorkspaceEnvironment" />
            </div>
          </div>
          <div class="workspace-toolbar-actions">
            <span v-if="workspaceReady && workspaceOverview" class="workspace-count">{{ workspaceCount }} 条记录</span>
            <template v-if="activeWorkspaceTab === 'runtime'">
              <button class="btn btn-sm" :disabled="!workspaceOverview" @click="openCatalogImport">从商城导入</button>
              <button class="btn btn-sm btn-primary" :disabled="!workspaceOverview" @click="openCreateApplication">创建部署组件</button>
            </template>
          </div>
        </div>
        <EmptyState v-if="!projectsLoaded || (workspaceReady && !workspaceOverview && !readError && workspaceEnvironment?.cluster_id !== 0)" variant="loading" message="正在读取工作台..." />
        <EmptyState v-else-if="workspaceEnvironment?.cluster_id === 0" variant="actionable" message="当前环境尚未绑定集群">
          <template #action><button class="btn btn-primary" @click="openProjectEnvironments(workspaceProject)">绑定集群</button></template>
        </EmptyState>
        <EmptyState v-else-if="!workspaceReady" variant="actionable" message="请选择项目与环境，或先创建部署目标">
          <template #action><button class="btn btn-primary" @click="openProjectManagement">创建项目与环境</button></template>
        </EmptyState>
        <template v-else-if="workspaceOverview">
          <template v-if="activeWorkspaceTab === 'runtime'">
            <div v-if="applicationsLoaded && applications.length > 0" class="workspace-table-content">
              <div class="table-wrap workspace-applications-table-wrap"><table class="data-table workspace-applications-table"><thead><tr><th>部署组件</th><th>运行状态</th><th>Pod 就绪</th><th>运行版本</th><th>最近发布</th><th>操作</th></tr></thead><tbody>
                <tr v-for="app in applications" :key="app.id" class="clickable" @click="openDetails(app)"><td class="cell-primary">{{ app.name }}</td><td><span class="badge" :class="runtimeBadge(app.runtime?.status)">{{ runtimeLabel(app.runtime?.status) }}</span></td><td>{{ app.runtime?.status === 'not_released' ? '-' : `${app.runtime?.ready_pods ?? 0}/${app.runtime?.total_pods ?? 0}` }}</td><td>{{ app.active_release?.version || '-' }}</td><td><span v-if="app.latest_release" :class="['badge', releaseBadge(app.latest_release.status)]">{{ releaseLabel(app.latest_release.status) }}</span><span v-else>-</span></td><td class="action-cell" @click.stop><div class="btn-group"><button class="btn btn-sm" @click="openDetails(app)">查看运行</button><button class="btn btn-sm" @click="openRelease(app)">发布版本</button><button v-if="!app.latest_release && !app.endpoint_count" class="btn btn-sm btn-danger" :title="`删除部署组件 ${app.name}`" @click="deleteApplicationTarget = app">删除</button></div></td></tr>
              </tbody></table></div>
            </div>
            <EmptyState v-else-if="applicationsLoaded" message="当前项目与环境下还没有部署组件" />
          </template>
        <template v-else-if="activeWorkspaceTab === 'releases'">
          <div v-if="workspaceReleases.length" class="workspace-table-content"><div class="table-wrap workspace-applications-table-wrap"><table class="data-table workspace-applications-table"><thead><tr><th>部署组件</th><th>版本</th><th>镜像</th><th>状态</th><th>发布时间</th><th>操作</th></tr></thead><tbody>
            <tr v-for="release in workspaceReleases" :key="release.id"><td class="cell-primary">{{ release.application_name }}</td><td>{{ release.version || `#${release.sequence}` }}</td><td>{{ release.image || '-' }}</td><td><span :class="['badge', releaseBadge(release.status)]">{{ releaseLabel(release.status) }}</span></td><td>{{ formatWorkspaceTime(release.created_at) }}</td><td class="action-cell"><button class="btn btn-sm" @click="openReleaseDetails(release)">查看发布</button></td></tr>
          </tbody></table></div></div>
          <EmptyState v-else message="当前环境还没有发布记录" />
        </template>
        <template v-else-if="activeWorkspaceTab === 'config'">
          <FeedbackBanner v-if="workspaceTemplatesError" tone="warning" :message="workspaceTemplatesError" />
          <EmptyState v-if="workspaceTemplatesLoading" variant="loading" message="正在读取部署配置" />
          <div v-else-if="applications.length" class="workspace-table-content"><div class="table-wrap workspace-applications-table-wrap"><table class="data-table workspace-applications-table"><thead><tr><th>部署组件</th><th>默认模板</th><th>镜像</th><th>默认版本</th><th>副本</th><th>操作</th></tr></thead><tbody>
            <tr v-for="app in applications" :key="app.id"><td class="cell-primary">{{ app.name }}</td><td>{{ defaultWorkspaceTemplate(app.id)?.name || '未配置' }}</td><td>{{ defaultWorkspaceTemplate(app.id)?.spec?.image || '-' }}</td><td>{{ defaultWorkspaceTemplate(app.id)?.default_version || '-' }}</td><td>{{ defaultWorkspaceTemplate(app.id)?.spec?.replicas ?? '-' }}</td><td class="action-cell"><button class="btn btn-sm" @click="openApplicationTab(app, 'config')">管理配置</button></td></tr>
          </tbody></table></div></div>
          <EmptyState v-else message="当前环境还没有部署组件" />
        </template>
        <template v-else>
          <div v-if="applications.length" class="workspace-table-content"><div class="table-wrap workspace-applications-table-wrap"><table class="data-table workspace-applications-table"><thead><tr><th>部署组件</th><th>访问域名</th><th>路径</th><th>协议</th><th>Service 端口</th><th>操作</th></tr></thead><tbody>
            <tr v-for="entry in workspaceEndpointRows" :key="entry.key"><td class="cell-primary">{{ entry.app.name }}</td><td>{{ entry.endpoint?.domain || '未配置' }}</td><td>{{ entry.endpoint?.path || '-' }}</td><td>{{ entry.endpoint ? (entry.endpoint.ingress_enabled === false ? entry.endpoint.protocol || 'TCP' : entry.endpoint.tls_enabled ? 'HTTPS' : 'HTTP') : '-' }}</td><td>{{ entry.endpoint?.service_port || '-' }}</td><td class="action-cell"><button class="btn btn-sm" @click="openApplicationTab(entry.app, 'access')">管理入口</button></td></tr>
          </tbody></table></div></div>
          <EmptyState v-else message="当前环境还没有部署组件" />
        </template>
        </template>
      </SurfaceCard>
    </template>

    <template v-else-if="section === 'projects'">
      <SurfaceCard v-if="projectsLoaded && projects.length > 0" as="div">
        <div class="table-wrap"><table class="data-table project-table"><thead><tr><th>项目</th><th>英文名</th><th>说明</th><th>已关联应用</th><th>操作</th></tr></thead><tbody>
          <tr v-for="project in projects" :key="project.id" class="clickable" @click="openProjectEnvironments(project)">
            <td class="cell-primary">{{ project.name }}</td>
            <td>{{ project.slug || '-' }}</td>
            <td>{{ project.description || '-' }}</td>
            <td>{{ applicationCount(project.id) }}</td>
            <td class="action-cell" @click.stop><div class="btn-group"><button class="btn btn-sm" @click="openProjectEditor(project)">编辑</button><button class="btn btn-sm" @click="openProjectEnvironments(project)">管理环境</button><button class="btn btn-sm btn-danger" title="删除项目" @click="requestProjectDelete(project)">删除</button></div></td>
          </tr>
        </tbody></table></div>
      </SurfaceCard>
    </template>

    <div v-if="showCreate" class="overlay" @click.self="showCreate = false"><div class="modal"><h2 class="modal-title">创建应用</h2><form @submit.prevent="createApplication">
      <div class="form-group"><label class="form-label">部署目标</label><input class="form-input" :value="`${workspaceProject?.name || ''} · ${workspaceEnvironment?.name || ''} · ${clusterName(workspaceEnvironment?.cluster_id)} · ${workspaceEnvironment?.namespace || ''}`" disabled /></div>
      <div class="form-group"><label class="form-label">应用名</label><input v-model.trim="createForm.applicationName" class="form-input" required pattern="[a-z0-9]([-a-z0-9]*[a-z0-9])?" maxlength="63" title="仅支持小写字母、数字和连字符，且以字母或数字开头和结尾" placeholder="order-api" /><p class="form-hint">仅支持小写字母、数字和连字符，最长 63 位。</p></div>
      <div class="modal-actions"><button type="button" class="btn" @click="showCreate = false">取消</button><button class="btn btn-primary" :disabled="submitting">{{ submitting ? '创建中...' : '创建' }}</button></div>
    </form></div></div>

    <Teleport to="body"><div v-if="showCatalogImport" class="overlay" @click.self="showCatalogImport = false"><div class="modal workspace-import-modal"><h2 class="modal-title">从商城导入</h2>
      <p class="workspace-import-target">{{ workspaceProject?.name }} · {{ workspaceEnvironment?.name }} · {{ workspaceEnvironment?.namespace }}</p>
      <EmptyState v-if="catalogLoading" variant="loading" message="加载中..." />
      <EmptyState v-else-if="enabledCatalogItems.length === 0" message="商城中没有已启用的应用" />
      <div v-else class="workspace-import-options"><label v-for="item in enabledCatalogItems" :key="item.id" class="workspace-import-option"><input v-model="selectedCatalogIDs" type="checkbox" :value="item.id" /><span>{{ item.name }}</span><span class="cell-secondary">{{ item.components.length }} 个组件</span></label></div>
      <div v-if="selectedCatalogItems.length" class="workspace-import-selection"><div v-for="item in selectedCatalogItems" :key="item.id" class="workspace-import-package"><strong>{{ item.name }}</strong><div v-for="component in item.components" :key="component.application_name" class="workspace-import-component"><span>{{ component.name }} <small>{{ component.application_name }}:{{ component.spec.container_port }}</small></span><span v-if="catalogConflicts.includes(component.application_name)" class="badge badge-danger">名称冲突</span><span v-else class="badge badge-online">将创建默认模板</span></div></div></div>
      <div class="modal-actions"><button class="btn" @click="showCatalogImport = false">取消</button><button class="btn btn-primary" :disabled="submitting || catalogLoading || selectedCatalogItems.length === 0 || catalogConflicts.length > 0" @click="importCatalogItems">{{ submitting ? '导入中...' : `确认导入（${selectedCatalogItems.length}）` }}</button></div>
    </div></div></Teleport>

    <div v-if="showProjectModal" class="overlay" @click.self="closeProjectModal"><div class="modal"><h2 class="modal-title">{{ editingProject ? '编辑项目' : '新建项目' }}</h2><form @submit.prevent="saveProject">
      <div class="form-group"><label class="form-label">项目名称</label><input v-model="projectForm.name" class="form-input" required placeholder="commerce" :disabled="editingProject && applicationCount(editingProject.id) > 0" /></div>
      <div class="form-group"><label class="form-label">项目英文名</label><input v-model.trim="projectForm.slug" class="form-input" required pattern="[a-z0-9]([-a-z0-9]*[a-z0-9])?" maxlength="40" placeholder="commerce" title="仅支持小写英文字母、数字和连字符，最长 40 位" :disabled="editingProject && !!editingProject.slug && !!editingProject.environments?.length" /></div>
      <div class="form-group"><label class="form-label">说明</label><textarea v-model="projectForm.description" class="form-input form-textarea" placeholder="订单服务及其部署环境" /></div>
      <div v-if="editingProject" class="form-group"><label class="form-label">默认镜像仓库</label><SelectMenu v-model.number="projectForm.defaultImageRegistryID" class="form-select"><option :value="0">不设置默认仓库</option><option v-for="registry in projectRegistries(editingProject)" :key="registry.id" :value="registry.id">{{ registry.name }} · {{ registry.endpoint }}</option></SelectMenu></div>
      <div class="modal-actions"><button type="button" class="btn" @click="closeProjectModal">取消</button><button class="btn btn-primary" :disabled="submitting">{{ submitting ? '保存中...' : editingProject ? '保存' : '创建项目' }}</button></div>
    </form></div></div>

    <ConfirmDialog
      :open="!!projectDeleteTarget"
      title="删除项目"
      :message="projectDeleteTarget ? `确认删除项目“${projectDeleteTarget.name}”吗？该操作不可撤销。` : ''"
      confirm-text="删除"
      :busy="submitting"
      @cancel="projectDeleteTarget = null"
      @confirm="deleteProject"
    />

    <ConfirmDialog
      :open="!!deleteApplicationTarget"
      title="删除部署组件"
      :message="deleteApplicationTarget ? `确认删除“${deleteApplicationTarget.name}”吗？其上线模板和商城导入关联也会删除。` : ''"
      confirm-text="删除"
      :busy="submitting"
      @cancel="deleteApplicationTarget = null"
      @confirm="confirmDeleteApplication"
    />

    <Teleport to="body"><div v-if="releaseApp" class="overlay" @click.self="releaseApp = null"><div class="modal release-modal"><h2 class="modal-title">发布 {{ releaseApp.name }}</h2><form @submit.prevent="createRelease">
      <div v-if="releaseTemplates.length" class="form-group"><label class="form-label">上线模板</label><SelectMenu v-model.number="releaseForm.template_id" class="form-select" required><option v-for="template in releaseTemplates" :key="template.id" :value="template.id">{{ template.name }}{{ template.is_default ? '（默认）' : '' }}</option></SelectMenu></div>
      <div v-if="selectedReleaseTemplate" class="template-summary"><span>模板 v{{ selectedReleaseTemplate.revision }}</span><strong>{{ selectedReleaseTemplate.spec.image }}</strong><small>{{ selectedReleaseTemplate.spec.replicas }} 副本 · Service {{ selectedReleaseTemplate.spec.service?.port }}</small></div>
      <p v-else class="template-intro">当前应用尚未配置上线模板，请先在应用详情中创建模板。</p>
      <div class="form-group"><label class="form-label">版本号</label><input v-model.trim="releaseForm.version" class="form-input" :required="!selectedReleaseTemplate?.default_version" :placeholder="selectedReleaseTemplate?.default_version || '1.4.2'" pattern="[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}" :disabled="!releaseTemplates.length" /><p class="form-hint">{{ selectedReleaseTemplate?.default_version ? `留空则使用模板默认版本 ${selectedReleaseTemplate.default_version}。` : '当前模板未设置默认版本号，请填写镜像版本号。' }}</p></div>
      <div class="modal-actions"><button type="button" class="btn" @click="releaseApp = null">取消</button><button v-if="!releaseTemplates.length" type="button" class="btn btn-primary" @click="openTemplateManagement(releaseApp)">管理模板</button><button v-else class="btn btn-primary" :disabled="submitting">{{ submitting ? '提交中...' : '开始发布' }}</button></div>
    </form></div></div></Teleport>
    <ErrorNoticeModal :open="Boolean(mutationError)" :message="mutationError" teleport @close="mutationError = ''" />
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getClusters } from '../../api/clusters.js'
import {
  createApplication as createApplicationRequest,
  createProject,
  createRelease as createReleaseRequest,
  deleteApplication as deleteApplicationRequest,
  deleteProject as deleteProjectRequest,
  getApplicationCatalog,
  getApplications,
  getDeploymentTemplates,
  getImageRegistries,
  getProjects,
  getWorkspace,
  importApplicationCatalogItems,
  updateProject,
} from '../../api/applications.js'
import { useAsyncResource } from '../../composables/useAsyncResource.js'
import ConfirmDialog from '../../components/ConfirmDialog.vue'
import EmptyState from '../../components/EmptyState.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import FeedbackBanner from '../../components/FeedbackBanner.vue'
import PageHeader from '../../components/PageHeader.vue'
import SectionTabsHeader from '../../components/SectionTabsHeader.vue'
import SurfaceCard from '../../components/SurfaceCard.vue'

const props = defineProps({ section: { type: String, default: 'workspace' } })
const router = useRouter()
const route = useRoute()
const applications = ref([])
const catalogItems = ref([])
const selectedCatalogIDs = ref([])
const catalogLoading = ref(false)
const showCatalogImport = ref(false)
const projects = ref([])
const clusters = ref([])
const applicationsLoaded = ref(false)
const projectsLoaded = ref(false)
const readError = ref('')
const mutationError = ref('')
const submitting = ref(false)
const showCreate = ref(false)
const showProjectModal = ref(false)
const editingProject = ref(null)
const projectDeleteTarget = ref(null)
const deleteApplicationTarget = ref(null)
const releaseApp = ref(null)
const releaseTemplates = ref([])
const imageRegistries = ref([])
const createForm = ref(newApplicationForm())
const projectForm = ref({ name: '', slug: '', description: '', defaultImageRegistryID: 0 })
const releaseForm = ref(newReleaseForm())
const workspaceOverview = ref(null)
const workspaceTabs = [{ id: 'runtime', label: '运行概览' }, { id: 'releases', label: '发布记录' }, { id: 'config', label: '部署配置' }, { id: 'access', label: '访问入口' }]
const activeWorkspaceTab = ref('runtime')
const workspaceTemplates = ref({})
const workspaceTemplatesLoading = ref(false)
const workspaceTemplatesError = ref('')
let workspaceTemplateLoadID = 0
const applicationsResource = useAsyncResource(({ signal }, scope = {}) => getApplications(scope, { signal }), [])
const projectsResource = useAsyncResource(({ signal }) => getProjects({ signal }), [])
const clustersResource = useAsyncResource(({ signal }) => getClusters({ signal }), [])
const imageRegistriesResource = useAsyncResource(({ signal }, projectID) => getImageRegistries({ projectID }, { signal }), [])
const workspaceResource = useAsyncResource(({ signal }, projectID, environmentID) => getWorkspace(projectID, environmentID, { signal }), null)
const releaseTemplatesResource = useAsyncResource(({ signal }, applicationID) => getDeploymentTemplates(applicationID, { signal }), [])
let sectionLoadID = 0

const pageMeta = computed(() => ({
  workspace: { title: '工作台', subtitle: '当前项目与环境下的应用、发布和运行状态' },
  projects: { title: '项目与环境', subtitle: '定义应用归属、部署环境和默认命名空间' },
}[props.section] || { title: '工作台', subtitle: '当前项目与环境下的应用、发布和运行状态' }))

const workspaceProjectID = computed(() => Number(route.query.project_id) || 0)
const workspaceEnvironmentID = computed(() => Number(route.query.environment_id) || 0)
const workspaceProject = computed(() => projects.value.find(item => item.id === workspaceProjectID.value) || null)
const workspaceEnvironment = computed(() => workspaceProject.value?.environments?.find(item => item.id === workspaceEnvironmentID.value) || null)
const workspaceProjectOptions = computed(() => projects.value.map(project => ({ value: project.id, label: project.name, description: project.description || '未设置项目说明' })))
const workspaceEnvironmentOptions = computed(() => (workspaceProject.value?.environments || []).map(environment => ({ value: environment.id, label: environment.name, triggerLabel: `${environment.name} · ${clusterName(environment.cluster_id)}`, description: `${clusterName(environment.cluster_id)} · ${environment.namespace}` })))
const workspaceReady = computed(() => !!workspaceProject.value && !!workspaceEnvironment.value)
const enabledCatalogItems = computed(() => catalogItems.value.filter(item => item.enabled))
const selectedCatalogItems = computed(() => enabledCatalogItems.value.filter(item => selectedCatalogIDs.value.includes(item.id)))
const catalogConflicts = computed(() => {
  const names = selectedCatalogItems.value.flatMap(item => item.components.map(component => component.application_name))
  const existing = new Set(applications.value.map(app => app.name))
  return names.filter((name, index) => existing.has(name) || names.indexOf(name) !== index)
})
const selectedReleaseTemplate = computed(() => releaseTemplates.value.find(template => template.id === releaseForm.value.template_id) || null)
const workspaceReleases = computed(() => workspaceOverview.value?.releases || workspaceOverview.value?.recent_releases || [])
const workspaceEndpointRows = computed(() => applications.value.flatMap(app => (app.endpoints?.length ? app.endpoints : [null]).map((endpoint, index) => ({ key: `${app.id}-${index}`, app, endpoint }))))
const workspaceCount = computed(() => {
  if (activeWorkspaceTab.value === 'releases') return workspaceReleases.value.length
  if (activeWorkspaceTab.value === 'access') return workspaceEndpointRows.value.length
  return applications.value.length
})
function defaultWorkspaceTemplate(applicationID) { const templates = workspaceTemplates.value[applicationID] || []; return templates.find(item => item.is_default) || templates.find(item => item.enabled) || templates[0] }
function formatWorkspaceTime(value) { return value ? new Date(value).toLocaleString('zh-CN') : '-' }
function selectWorkspaceTab(tab) { activeWorkspaceTab.value = tab; if (tab === 'config') loadWorkspaceTemplates() }
async function loadWorkspaceTemplates() {
  const loadID = ++workspaceTemplateLoadID
  const apps = [...applications.value]
  if (!apps.length) { workspaceTemplates.value = {}; return }
  workspaceTemplatesLoading.value = true
  workspaceTemplatesError.value = ''
  const result = await Promise.allSettled(apps.map(app => getDeploymentTemplates(app.id)))
  if (loadID !== workspaceTemplateLoadID) return
  workspaceTemplates.value = Object.fromEntries(result.map((entry, index) => [apps[index].id, entry.status === 'fulfilled' ? entry.value || [] : []]))
  if (result.some(entry => entry.status === 'rejected')) workspaceTemplatesError.value = '部分组件的部署配置读取失败，请刷新后重试'
  workspaceTemplatesLoading.value = false
}

function newApplicationForm() { return { projectID: 0, environmentID: 0, applicationName: '' } }
function newReleaseForm() { return { template_id: 0, version: '' } }
function releaseBadge(status) { return status === 'succeeded' ? 'badge-online' : status === 'failed' ? 'badge-danger' : 'badge-offline' }
function releaseLabel(status) { return status === 'succeeded' ? '成功' : status === 'failed' ? '失败' : '发布中' }
function runtimeBadge(status) { return status === 'running' ? 'badge-online' : status === 'deploying' ? 'badge-deploying' : status === 'not_released' || status === 'unknown' ? 'badge-offline' : 'badge-danger' }
function runtimeLabel(status) { return ({ running: '运行中', deploying: '发布中', degraded: '异常', unavailable: '不可用', not_released: '未发布', unknown: '未知' })[status] || '未知' }
function applicationCount(projectID) { return applications.value.filter(app => app.project_id === projectID).length }
function clusterName(clusterID) { return clusters.value.find(cluster => cluster.id === Number(clusterID))?.name || (clusterID ? `集群 #${clusterID}` : '未绑定集群') }

async function fetchApplications(scoped = false) {
  const scope = scoped && workspaceReady.value ? { projectID: workspaceProjectID.value, environmentID: workspaceEnvironmentID.value } : {}
  const result = await applicationsResource.refresh(scope)
  if (result !== undefined) applications.value = result || []
  else if (applicationsResource.error.value) readError.value = applicationsResource.error.value.message || '加载应用失败'
  applicationsLoaded.value = true
}

async function fetchProjects() {
  const [result, clusterList] = await Promise.all([projectsResource.refresh(), clustersResource.refresh()])
  if (result !== undefined) projects.value = result || []
  else if (projectsResource.error.value) readError.value = projectsResource.error.value.message || '加载项目失败'
  if (clusterList !== undefined) clusters.value = clusterList || []
  projectsLoaded.value = true
}

async function fetchImageRegistries(projectID) {
  const result = await imageRegistriesResource.refresh(projectID)
  if (result !== undefined) imageRegistries.value = result || []
  else if (imageRegistriesResource.error.value) {
    readError.value = imageRegistriesResource.error.value.message || '加载镜像仓库失败'
  }
}

async function fetchWorkspace() {
  const result = await workspaceResource.refresh(workspaceProjectID.value, workspaceEnvironmentID.value)
  if (result !== undefined) {
    workspaceOverview.value = result
    applications.value = result?.applications || []
    if (activeWorkspaceTab.value === 'config') loadWorkspaceTemplates()
  } else if (workspaceResource.error.value) {
    readError.value = workspaceResource.error.value.message || '加载工作台失败'
  }
  applicationsLoaded.value = true
}

async function loadSection(section) {
  const loadID = ++sectionLoadID
  readError.value = ''
  if (section === 'projects') {
    await Promise.all([fetchProjects(), fetchApplications(), fetchImageRegistries()])
    return
  }
  if (workspaceProjectID.value && workspaceEnvironmentID.value) {
    await fetchProjects()
    if (loadID !== sectionLoadID) return
    if (workspaceEnvironment.value?.cluster_id === 0) {
      workspaceOverview.value = null
      applications.value = []
      applicationsLoaded.value = true
    } else if (workspaceReady.value) await fetchWorkspace()
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
    applications.value = []
    applicationsLoaded.value = true
  }
}

async function openCreateApplication() {
  mutationError.value = ''
  if (!workspaceReady.value || workspaceEnvironment.value?.cluster_id === 0) { mutationError.value = '请先为环境绑定集群'; return }
  createForm.value = { projectID: workspaceProjectID.value, environmentID: workspaceEnvironmentID.value, applicationName: '' }
  showCreate.value = true
}
async function openCatalogImport() {
  if (!workspaceReady.value || !workspaceEnvironment.value?.cluster_id) return
  mutationError.value = ''
  selectedCatalogIDs.value = []
  catalogItems.value = []
  catalogLoading.value = true
  showCatalogImport.value = true
  try { catalogItems.value = await getApplicationCatalog() || [] }
  catch (error) { mutationError.value = error.message || '读取商城应用失败'; showCatalogImport.value = false }
  finally { catalogLoading.value = false }
}
async function importCatalogItems() {
  if (!workspaceReady.value || !selectedCatalogItems.value.length || catalogConflicts.value.length) return
  submitting.value = true; mutationError.value = ''
  try {
    await importApplicationCatalogItems({ project_id: workspaceProjectID.value, environment_id: workspaceEnvironmentID.value, item_ids: selectedCatalogItems.value.map(item => item.id) })
    showCatalogImport.value = false
    selectedCatalogIDs.value = []
    await fetchWorkspace()
  } catch (error) { mutationError.value = error.message || '导入商城应用失败' }
  finally { submitting.value = false }
}
async function createApplication() { submitting.value = true; mutationError.value = ''; try { await createApplicationRequest({ projectID: createForm.value.projectID, environmentID: createForm.value.environmentID, name: createForm.value.applicationName }); showCreate.value = false; createForm.value = newApplicationForm(); await fetchWorkspace() } catch (e) { mutationError.value = e.message || '创建应用失败' } finally { submitting.value = false } }
async function confirmDeleteApplication() {
  if (!deleteApplicationTarget.value) return
  submitting.value = true; mutationError.value = ''
  try { await deleteApplicationRequest(deleteApplicationTarget.value.id); deleteApplicationTarget.value = null; await fetchWorkspace() }
  catch (error) { mutationError.value = error.message || '删除部署组件失败' }
  finally { submitting.value = false }
}
function projectRegistries(project) { return imageRegistries.value.filter(registry => registry.enabled && registry.projects?.some(allowedProject => allowedProject.id === project.id)) }
function openProjectEditor(project) { editingProject.value = project; projectForm.value = { name: project.name, slug: project.slug || '', description: project.description || '', defaultImageRegistryID: project.default_image_registry_id || 0 }; showProjectModal.value = true }
function closeProjectModal() { showProjectModal.value = false; editingProject.value = null; projectForm.value = { name: '', slug: '', description: '', defaultImageRegistryID: 0 } }
async function saveProject() { const isEditing = !!editingProject.value; submitting.value = true; mutationError.value = ''; try { const payload = { name: projectForm.value.name, slug: projectForm.value.slug, description: projectForm.value.description, ...(isEditing ? { default_image_registry_id: projectForm.value.defaultImageRegistryID } : {}) }; if (isEditing) await updateProject(editingProject.value.id, payload); else await createProject(payload); closeProjectModal(); await Promise.all([fetchProjects(), fetchApplications()]) } catch (e) { mutationError.value = e.message || (isEditing ? '更新项目失败' : '创建项目失败') } finally { submitting.value = false } }
function requestProjectDelete(project) { projectDeleteTarget.value = project }
async function deleteProject() { if (!projectDeleteTarget.value) return; submitting.value = true; mutationError.value = ''; try { await deleteProjectRequest(projectDeleteTarget.value.id); projectDeleteTarget.value = null; await Promise.all([fetchProjects(), fetchApplications()]) } catch (e) { mutationError.value = e.message || '删除项目失败' } finally { submitting.value = false } }
async function openProjectEnvironments(project) { await router.push(`/applications/projects/${project.id}`) }
async function openProjectManagement() { await router.push('/applications/projects') }
async function updateWorkspace(projectID, environmentID) { const query = {}; if (projectID) query.project_id = String(projectID); if (environmentID) query.environment_id = String(environmentID); if (projectID && environmentID) localStorage.setItem('cylism.application-workspace', JSON.stringify({ project_id: projectID, environment_id: environmentID })); await router.push({ path: '/applications', query }) }
async function selectWorkspaceProject(value) { const projectID = Number(value) || 0; const project = projects.value.find(item => item.id === projectID); const environmentID = project?.environments?.length === 1 ? project.environments[0].id : 0; await updateWorkspace(projectID, environmentID) }
async function selectWorkspaceEnvironment(value) { await updateWorkspace(workspaceProjectID.value, Number(value) || 0) }

async function openRelease(app) {
  readError.value = ''
  releaseForm.value = newReleaseForm()
  releaseTemplates.value = []
  const result = await releaseTemplatesResource.refresh(app.id)
  if (result !== undefined) {
    releaseTemplates.value = (result || []).filter(template => template.enabled)
    const defaultTemplate = releaseTemplates.value.find(template => template.is_default) || releaseTemplates.value[0]
    releaseForm.value.template_id = defaultTemplate?.id || 0
  } else if (releaseTemplatesResource.error.value) readError.value = releaseTemplatesResource.error.value.message || '加载上线模板失败'
  releaseApp.value = app
}
async function createRelease() { mutationError.value = ''; if (!releaseForm.value.template_id || (!releaseForm.value.version && !selectedReleaseTemplate.value?.default_version)) { mutationError.value = '请选择上线模板并填写版本号，或设置模板默认版本号'; return } submitting.value = true; try { const applicationID = releaseApp.value.id; const release = await createReleaseRequest(applicationID, releaseForm.value); releaseApp.value = null; await router.push(`/applications/${applicationID}/releases/${release.id}`); fetchWorkspace() } catch (e) { mutationError.value = e.message || '创建发布失败' } finally { submitting.value = false } }
async function openTemplateManagement(app) { releaseApp.value = null; await router.push(`/applications/${app.id}`) }
async function openDetails(app) { await router.push(`/applications/${app.id}`) }
async function openApplicationTab(app, tab) { await router.push({ path: `/applications/${app.id}`, query: { tab } }) }
async function openReleaseDetails(release) { await router.push(`/applications/${release.application_id}/releases/${release.id}`) }

watch(() => [props.section, route.query.project_id, route.query.environment_id], () => { showCatalogImport.value = false; workspaceTemplateLoadID++; workspaceTemplates.value = {}; workspaceTemplatesLoading.value = false; loadSection(props.section) }, { immediate: true })
</script>

<style scoped>
.workspace-applications-table-wrap{padding:0 20px 12px}.workspace-applications-table{min-width:760px}.workspace-applications-table th:first-child,.workspace-applications-table td:first-child{min-width:200px;overflow-wrap:anywhere}.workspace-applications-table th:nth-child(2),.workspace-applications-table td:nth-child(2){min-width:120px;white-space:nowrap}.workspace-applications-table th:last-child,.workspace-applications-table td:last-child{width:1%;text-align:right;white-space:nowrap}.workspace-applications-table .btn-group{justify-content:flex-end;flex-wrap:nowrap}
.page-header { display:flex; align-items:flex-start; justify-content:space-between; gap:var(--space-16); }
.page-subtitle { margin:var(--space-4) 0 0; color:var(--text-secondary); font-size:13px; }
.release-modal { width:min(680px, calc(100vw - 32px)); }.template-summary{display:grid;gap:4px;margin-bottom:var(--space-16);padding:12px;border:1px solid var(--border-muted);border-radius:var(--radius-control);background:var(--surface-subtle)}.template-summary span,.template-summary small,.template-intro{color:var(--text-secondary);font-size:12px}.template-summary strong{overflow-wrap:anywhere;font-size:13px}.template-intro{margin:0 0 var(--space-16)}
.compact-input { margin-top:6px; }.unit-input { display:grid; grid-template-columns:minmax(0, 1fr) 86px; gap:6px; }
.check-row { display:flex; gap:8px; align-items:center; margin:var(--space-12) 0; color:var(--text-secondary); font-size:13px; }
.workspace-import-modal { width: min(580px, 100%); }
.workspace-import-target { margin: 0 0 14px; color: var(--text-secondary); font-size: 12px; }
.workspace-import-options { display: grid; max-height: 180px; overflow-y: auto; border-top: 1px solid var(--border-muted); }
.workspace-import-option { display: flex; align-items: center; gap: 10px; min-height: 42px; border-bottom: 1px solid var(--border-muted); font-size: 12px; cursor: pointer; }
.workspace-import-option .cell-secondary { margin-left: auto; }
.workspace-import-selection { max-height: 35vh; overflow-y: auto; border-top: 1px solid var(--border-muted); }
.workspace-import-package { padding: 12px 0 5px; border-bottom: 1px solid var(--border-muted); font-size: 12px; }
.workspace-import-component { display: flex; align-items: center; justify-content: space-between; gap: 12px; min-height: 42px; padding-left: 12px; }
.workspace-import-component small { display: block; margin-top: 3px; color: var(--text-muted); }
.form-textarea { min-height:82px; resize:vertical; }
.project-table { min-width:720px; }.project-table th:last-child,.project-table td:last-child { width:1%; white-space:nowrap; }.project-table th:last-child { text-align:right; }.project-table .btn-group { justify-content:flex-end; flex-wrap:nowrap; }
.confirm-copy { margin:0; color:var(--text-secondary); font-size:13px; }
.overview-metrics{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:8px;margin-bottom:var(--space-16)}.metric{display:grid;gap:4px;padding:12px;border:1px solid var(--border-muted);border-radius:var(--radius-control);background:var(--surface-subtle)}.metric span{color:var(--text-secondary);font-size:12px}.metric strong{font-size:22px}
.page-actions{display:flex;align-items:center;justify-content:flex-end;flex-wrap:wrap;gap:8px}.workspace-context{display:flex;align-items:stretch;gap:8px}.workspace-picker{position:relative;display:grid;min-width:144px;gap:3px}.context-picker-label{padding-left:2px;color:var(--text-muted);font-size:10px;font-weight:700;line-height:1}.context-picker-trigger{display:flex;min-width:0;height:36px;align-items:center;gap:7px;padding:0 9px 0 10px;border:1px solid var(--border-muted);border-radius:var(--radius-control);background:var(--surface-raised);color:var(--text-primary);font:inherit;font-size:12px;text-align:left;cursor:pointer;transition:border-color .18s ease,background .18s ease,box-shadow .18s ease}.context-picker-trigger:hover:not(:disabled){border-color:var(--focus);background:var(--surface-hover)}.context-picker-trigger:focus-visible{outline:2px solid var(--focus);outline-offset:2px}.context-picker-trigger.is-open{border-color:var(--focus);box-shadow:0 0 0 2px color-mix(in srgb,var(--focus) 18%,transparent)}.context-picker-trigger:disabled{cursor:not-allowed;opacity:.55}.context-picker-value{min-width:0;overflow:hidden;flex:1;text-overflow:ellipsis;white-space:nowrap}.context-picker-trigger svg{flex:0 0 auto;color:var(--text-muted);transition:transform .18s ease}.context-picker-trigger.is-open svg{transform:rotate(180deg);color:var(--action-primary)}.context-picker-menu{position:absolute;z-index:25;top:calc(100% + 6px);left:0;display:grid;width:max-content;min-width:100%;max-width:min(310px,calc(100vw - 32px));gap:3px;padding:5px;border:1px solid var(--border);border-radius:var(--radius-control);background:var(--surface-raised);box-shadow:var(--shadow);backdrop-filter:blur(24px) saturate(140%)}.context-picker-menu--environment{min-width:230px}.context-picker-option{display:flex;min-width:0;align-items:center;justify-content:space-between;gap:18px;padding:8px;border:0;border-radius:6px;background:transparent;color:var(--text-primary);font:inherit;text-align:left;cursor:pointer}.context-picker-option:hover,.context-picker-option.is-selected{background:var(--surface-hover)}.context-picker-option span{display:grid;min-width:0;gap:2px}.context-picker-option strong,.context-picker-option small{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.context-picker-option strong{font-size:12px}.context-picker-option small{color:var(--text-muted);font-size:10px}.context-picker-option svg{flex:0 0 auto;color:var(--action-primary)}.context-picker-empty{padding:9px 8px;color:var(--text-muted);font-size:11px}.workspace-metrics{grid-template-columns:repeat(4,minmax(0,1fr))}.workspace-section{min-width:0;padding:var(--space-16) 0;border-top:1px solid var(--border-muted)}.section-heading{display:flex;align-items:flex-start;justify-content:space-between;gap:var(--space-12);margin-bottom:var(--space-12)}.section-heading h2{margin:0;font-size:15px}.section-heading p{margin:4px 0 0;color:var(--text-secondary);font-size:12px}.workspace-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:var(--space-16)}.compact-list{display:grid;gap:6px}.list-row{display:flex;align-items:center;justify-content:space-between;gap:8px;width:100%;padding:8px;border:1px solid var(--border-muted);border-radius:var(--radius-control);background:var(--surface-subtle);color:var(--text-primary);font:inherit;text-align:left;cursor:pointer}.list-row:hover{background:var(--surface-raised)}.list-row span:first-child{display:grid;min-width:0;gap:2px}.list-row strong,.list-row small{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.list-row small{color:var(--text-secondary);font-size:11px}.runtime-count,.release-state,.endpoint-more{display:block;margin-top:4px;font-size:10px}.endpoint-link{display:inline-block;max-width:260px;overflow:hidden;color:var(--action-primary);text-overflow:ellipsis;white-space:nowrap;text-decoration:none}.endpoint-link:hover{text-decoration:underline}.empty-inline{padding:14px 0;color:var(--text-muted);font-size:12px}
.context-picker-select{min-width:0}.context-picker-select--environment{min-width:230px}:deep(.context-picker-select .select-menu-options){width:max-content;min-width:100%;max-width:min(310px,calc(100vw - 32px))}:deep(.context-picker-select--environment .select-menu-options){min-width:230px}
.workspace-section{border-top:0}
.workspace-applications-table td{vertical-align:middle}
.workspace-applications-table td:nth-child(3){overflow-wrap:anywhere}
@media (max-width:760px){.workspace-grid{grid-template-columns:1fr}.workspace-metrics{grid-template-columns:repeat(2,minmax(0,1fr))}}@media (max-width:640px) { .page-header { align-items:stretch; flex-direction:column; }.page-header .btn { width:100%; }.page-actions{width:100%}.workspace-context{width:100%}.workspace-picker{flex:1;min-width:0}.context-picker-menu{max-width:calc(100vw - 32px)} }
@media (max-width:640px){.workspace-applications-table-wrap{padding-right:14px;padding-left:14px}}
.workspace-list { margin-top: var(--tabbed-page-content-gap); }
.workspace-toolbar { display: flex; min-height: 62px; align-items: center; justify-content: space-between; gap: var(--space-16); padding: 12px var(--space-16); }
.workspace-context { display: flex; flex: 1 1 auto; min-width: 0; flex-wrap: wrap; align-items: center; gap: var(--space-12); }
.workspace-picker { display: flex; flex: 0 1 auto; min-width: 0; align-items: center; gap: var(--space-8); }
.context-picker-label { flex: 0 0 auto; padding: 0; color: var(--text-secondary); font-size: 12px; font-weight: 400; line-height: 1; }
.context-picker-select { width: 190px; }
.context-picker-select--environment { width: 240px; min-width: 0; }
.workspace-toolbar-actions { display: flex; flex: 0 0 auto; align-items: center; justify-content: flex-end; gap: var(--space-8); }
.workspace-count { margin-right: 4px; color: var(--text-secondary); font-size: 12px; white-space: nowrap; }
.workspace-table-content { min-width: 0; }
@media (max-width: 860px) {
  .workspace-toolbar { flex-wrap: wrap; }
  .workspace-toolbar-actions { margin-left: auto; }
}
@media (max-width: 640px) {
  .workspace-context { width: 100%; gap: var(--space-8); }
  .workspace-picker { flex: 1 1 100%; }
  .context-picker-select, .context-picker-select--environment { flex: 1 1 auto; width: auto; }
  .workspace-toolbar-actions { flex-wrap: wrap; }
}
</style>
