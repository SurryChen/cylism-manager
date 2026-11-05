import { api } from './index.js'

const path = (clusterId, component) => `/clusters/${encodeURIComponent(clusterId)}/monitoring${component === 'loki' ? '/logs' : ''}/node-migrations`
export const getComponentMigrations = (clusterId, component, options) => api.get(path(clusterId, component), options)
export const startComponentMigration = (clusterId, component, node, options) => api.post(path(clusterId, component), { target_node_name: node }, options)
export const controlComponentMigration = (clusterId, component, id, action, options) => api.post(`${path(clusterId, component)}/${encodeURIComponent(id)}/control`, { action }, options)
