import { api } from './index.js'

export function getDashboardOverview(options) {
  return api.get('/dashboard', options)
}

export function getKubernetesDashboard(options) {
  return api.get('/k8s/dashboard', options)
}

export function getAlertOverview(options) {
  return api.get('/monitoring/alerts/overview', options)
}
