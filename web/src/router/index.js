import { createRouter, createWebHashHistory } from 'vue-router'
import { getAccessToken } from '../api/index.js'
import Dashboard from '../views/Dashboard.vue'
import Servers from '../views/Servers.vue'
import Cluster from '../views/Cluster.vue'
import Sites from '../views/Sites.vue'
import Certificates from '../views/Certificates.vue'
import CertificateOperations from '../views/CertificateOperations.vue'
import AuditLogs from '../views/AuditLogs.vue'
import Login from '../views/Login.vue'
import DBAdmin from '../views/DBAdmin.vue'
import Workloads from '../views/Workloads.vue'
import Services from '../views/Services.vue'
import Configs from '../views/Configs.vue'
import SystemSettings from '../views/SystemSettings.vue'
import Applications from '../views/Applications.vue'
import ProjectEnvironments from '../views/ProjectEnvironments.vue'
import ApplicationDetails from '../views/ApplicationDetails.vue'
import ReleaseDetails from '../views/ReleaseDetails.vue'
import ImageRegistries from '../views/ImageRegistries.vue'
import Domains from '../views/Domains.vue'
import NodeRegistryMirrors from '../views/NodeRegistryMirrors.vue'
import ChartRepositories from '../views/ChartRepositories.vue'

const routes = [
  { path: '/login', component: Login, meta: { public: true } },
  { path: '/', component: Dashboard },
  { path: '/applications', component: Applications, props: { section: 'applications' } },
  { path: '/applications/projects', component: Applications, props: { section: 'projects' } },
  { path: '/applications/projects/:projectID', component: ProjectEnvironments, props: true },
  { path: '/applications/releases', component: Applications, props: { section: 'releases' } },
  { path: '/applications/registries', component: ImageRegistries },
  { path: '/applications/domains', component: Domains },
  { path: '/applications/:applicationID/releases/:releaseID', component: ReleaseDetails, props: true },
  { path: '/applications/:applicationID', component: ApplicationDetails, props: true },
  { path: '/servers', component: Servers },
  { path: '/cluster', component: Cluster },
  { path: '/cluster/registry-mirrors', component: NodeRegistryMirrors },
  { path: '/cluster/chart-repositories', component: ChartRepositories },
  { path: '/workloads', component: Workloads },
  { path: '/services', component: Services },
  { path: '/routes', component: Sites },
  { path: '/certs', component: Certificates },
  { path: '/certs/:namespace/:name', component: CertificateOperations, props: true },
  { path: '/configs', component: Configs },
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
