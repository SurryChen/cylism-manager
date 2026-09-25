import { createRouter, createWebHashHistory } from 'vue-router'
import { getAccessToken } from '../api/index.js'

const Dashboard = () => import('../views/Dashboard.vue')
const Servers = () => import('../views/cluster/Servers.vue')
const CertificateOperations = () => import('../views/network/CertificateOperations.vue')
const AuditLogs = () => import('../views/AuditLogs.vue')
const OperationHistory = () => import('../views/OperationHistory.vue')
const Login = () => import('../views/Login.vue')
const DBAdmin = () => import('../views/DBAdmin.vue')
const SystemSettings = () => import('../views/settings/SystemSettings.vue')
const Applications = () => import('../views/applications/Applications.vue')
const ProjectEnvironments = () => import('../views/applications/ProjectEnvironments.vue')
const ApplicationDetails = () => import('../views/applications/ApplicationDetails.vue')
const ReleaseDetails = () => import('../views/applications/ReleaseDetails.vue')
const ImageRegistries = () => import('../views/applications/ImageRegistries.vue')
const Domains = () => import('../views/applications/Domains.vue')
const Monitoring = () => import('../views/Monitoring.vue')
const PersistentVolumes = () => import('../views/cluster/PersistentVolumes.vue')
const ClusterHub = () => import('../views/cluster/ClusterHub.vue')
const ResourceHub = () => import('../views/resources/ResourceHub.vue')
const NetworkHub = () => import('../views/network/NetworkHub.vue')
const RuntimeManagement = () => import('../views/RuntimeManagement.vue')
const ManagedOCIRegistries = () => import('../views/ManagedOCIRegistries.vue')

const routes = [
  { path: '/login', component: Login, meta: { public: true } },
  { path: '/', component: Dashboard },
  { path: '/cloud-services', redirect: '/cloud-services/registry' },
  { path: '/cloud-services/registry', component: ManagedOCIRegistries },
  { path: '/applications', component: Applications, props: { section: 'workspace' } },
  { path: '/applications/projects', component: Applications, props: { section: 'projects' } },
  { path: '/applications/projects/:projectID', component: ProjectEnvironments, props: true },
  { path: '/applications/overview', component: Applications, props: { section: 'overview' } },
  { path: '/applications/releases', redirect: '/applications/overview' },
  { path: '/applications/registries', component: ImageRegistries },
  { path: '/applications/domains', component: Domains },
  { path: '/applications/:applicationID/releases/:releaseID', component: ReleaseDetails, props: true },
  { path: '/applications/:applicationID', component: ApplicationDetails, props: true },
  { path: '/servers', component: Servers },
  { path: '/cluster', component: ClusterHub },
  { path: '/cluster/registry-mirrors', redirect: { path: '/cluster', query: { tab: 'registry-mirrors' } } },
  { path: '/cluster/chart-repositories', redirect: { path: '/cluster', query: { tab: 'chart-repositories' } } },
  { path: '/cluster/system-components', redirect: { path: '/cluster', query: { tab: 'system-components' } } },
  { path: '/cluster/storage', redirect: '/storage' },
  { path: '/storage', component: PersistentVolumes },
  { path: '/monitoring', component: Monitoring },
  { path: '/runtimes', component: RuntimeManagement },
  { path: '/delivery/registry', redirect: to => ({ path: '/cloud-services/registry', query: to.query }) },
  { path: '/resources', component: ResourceHub },
  { path: '/workloads', redirect: { path: '/resources', query: { tab: 'workloads' } } },
  { path: '/services', redirect: { path: '/resources', query: { tab: 'services' } } },
  { path: '/configs', redirect: { path: '/resources', query: { tab: 'configs' } } },
  { path: '/network', component: NetworkHub },
  { path: '/routes', redirect: { path: '/network', query: { tab: 'routes' } } },
  { path: '/certs', redirect: { path: '/network', query: { tab: 'certificates' } } },
  { path: '/certs/:namespace/:name', redirect: to => `/network/certificates/${to.params.namespace}/${to.params.name}` },
  { path: '/network/certificates/:namespace/:name', component: CertificateOperations, props: true },
  { path: '/settings/system', component: SystemSettings },
  { path: '/audit', component: AuditLogs },
  { path: '/operations', component: OperationHistory },
  { path: '/db-admin', component: DBAdmin },
]

const router = createRouter({
  history: createWebHashHistory(),
  routes,
})

router.beforeEach((to, from, next) => {
  const token = getAccessToken()
  if (!token && !to.meta.public) {
    next('/login')
  } else if (token && to.path === '/login') {
    next('/')
  } else {
    next()
  }
})

export default router
