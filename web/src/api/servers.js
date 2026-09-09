import { api } from './index.js'

export function getServers(options) {
  return api.get('/servers', options)
}

export function getServerResourceStats(options) {
  return api.get('/servers/resource-stats', options)
}

export function getServerNetworkDiagnostics(options) {
  return api.get('/servers/network-diagnostics', options)
}

export function getServerStats(serverID, options) {
  return api.get(`/servers/${serverID}/stats`, options)
}
