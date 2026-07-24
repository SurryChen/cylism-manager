import { createRouter, createWebHashHistory } from 'vue-router'
import { getAccessToken } from '../api/index.js'
import Dashboard from '../views/Dashboard.vue'
import Servers from '../views/Servers.vue'
import Sites from '../views/Sites.vue'
import Certificates from '../views/Certificates.vue'
import Resources from '../views/Resources.vue'
import AuditLogs from '../views/AuditLogs.vue'
import Login from '../views/Login.vue'
import DBAdmin from '../views/DBAdmin.vue'

const routes = [
  { path: '/login', component: Login, meta: { public: true } },
  { path: '/', component: Dashboard },
  { path: '/servers', component: Servers },
  { path: '/routes', component: Sites },
  { path: '/certs', component: Certificates },
  { path: '/resources', component: Resources },
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
