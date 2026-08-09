import { createRouter, createWebHashHistory } from 'vue-router'
import { getAccessToken } from '../api/index.js'
import Dashboard from '../views/Dashboard.vue'
import Servers from '../views/Servers.vue'
import CertificateOperations from '../views/CertificateOperations.vue'
import AuditLogs from '../views/AuditLogs.vue'
import Login from '../views/Login.vue'
import DBAdmin from '../views/DBAdmin.vue'
import SystemSettings from '../views/SystemSettings.vue'
import Applications from '../views/Applications.vue'
import ProjectEnvironments from '../views/ProjectEnvironments.vue'
import ApplicationDetails from '../views/ApplicationDetails.vue'
import ReleaseDetails from '../views/ReleaseDetails.vue'
import ImageRegistries from '../views/ImageRegistries.vue'
import Domains from '../views/Domains.vue'
import Monitoring from '../views/Monitoring.vue'
import PersistentVolumes from '../views/PersistentVolumes.vue'
import ClusterHub from '../views/ClusterHub.vue'
import ResourceHub from '../views/ResourceHub.vue'
import NetworkHub from '../views/NetworkHub.vue'
import RuntimeManagement from '../views/RuntimeManagement.vue'

const routes = [
  { path: '/login', component: Login, meta: { public: true } },
  { path: '/', component: Dashboard },
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
