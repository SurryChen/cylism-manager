import { api } from './index.js'

export function getClusterNodes(options) { return api.get('/nodes', options) }
export function getClusterServers(options) { return api.get('/servers', options) }
export function getClusterInventory(options) {
  return Promise.all([getClusterNodes(options), getClusterServers(options)])
}
