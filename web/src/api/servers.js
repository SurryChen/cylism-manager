import { api } from './index.js'

export function getServers(options) {
  return api.get('/servers', options)
}

export function getServerResourceStats(options) {
  return api.get('/servers/resource-stats', options)
}

export function getServerStats(serverID, options) {
  return api.get(`/servers/${serverID}/stats`, options)
}

export function runServerSpeedTest(body, options) {
  return post('/servers/speed-test', body, options)
}

export function getServerSpeedDependencies(serverID, options) {
  return api.get(`${serverPath(serverID)}/speed-test/dependencies`, options)
}

export function installServerSpeedDependencies(serverID, options) {
  return post(`${serverPath(serverID)}/speed-test/dependencies/install`, undefined, options)
}

export function getTailscaleStatuses(options) {
  return api.get('/servers/tailscale', options)
}

export function getTailscaleStatus(serverID, options) {
  return api.get(`${serverPath(serverID)}/tailscale`, options)
}

export function getSingBoxStatus(serverID, options) {
  return api.get(`${serverPath(serverID)}/sing-box`, options)
}

export function installSingBox(serverID, egressServerID = null) {
  return post(`${serverPath(serverID)}/sing-box/install`, egressServerID ? { egress_server_id: egressServerID } : {})
}

export function importSingBoxConfig(serverID, url) {
  return post(`${serverPath(serverID)}/sing-box/import`, { url })
}

export function startSingBox(serverID) {
  return post(`${serverPath(serverID)}/sing-box/start`)
}

export function stopSingBox(serverID) {
  return post(`${serverPath(serverID)}/sing-box/stop`)
}

export function installTailscale(serverID) {
  return post(`${serverPath(serverID)}/tailscale/install`)
}

export function joinTailscale(serverID, authKey) {
  return post(`${serverPath(serverID)}/tailscale/join`, { auth_key: authKey })
}

export function leaveTailscale(serverID) {
  return post(`${serverPath(serverID)}/tailscale/leave`)
}

export function checkTailscaleConnections() {
  return post('/servers/tailscale/connections')
}

export function diagnoseTailscaleConnection(fromID, toID) {
  return post('/servers/tailscale/connections/diagnose', { from_id: fromID, to_id: toID })
}

function serverPath(serverID) {
  return `/servers/${encodeURIComponent(serverID)}`
}

function nodePath(serverID) {
  return `/nodes/${encodeURIComponent(serverID)}`
}

function post(path, body, options) {
  if (options === undefined && body === undefined) return api.post(path)
  return options === undefined ? api.post(path, body) : api.post(path, body, options)
}

function put(path, body, options) {
  return options === undefined ? api.put(path, body) : api.put(path, body, options)
}

function remove(path, options) {
  return options === undefined ? api.delete(path) : api.delete(path, undefined, options)
}

export function createServer(body, options) {
  return post('/servers', body, options)
}

export function updateServer(serverID, body, options) {
  return put(serverPath(serverID), body, options)
}

export function deleteServer(serverID, options) {
  return remove(serverPath(serverID), options)
}

export function probeServer(serverID, body, options) {
  return post(`${serverPath(serverID)}/probe`, body, options)
}

export function resolveServerHostname(serverID, body, options) {
  return post(serverID == null ? '/servers/hostname' : `${serverPath(serverID)}/hostname`, body, options)
}

export function preimportServer(serverID, options) {
  return post(`${nodePath(serverID)}/preimport`, undefined, options)
}

export function importServerToCluster(serverID, body, options) {
  return post(`${nodePath(serverID)}/import`, body, options)
}

export function unbindServer(serverID, options) {
  return post(`${serverPath(serverID)}/unbind`, undefined, options)
}
