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

export function probeServer(serverID, options) {
  return post(`${serverPath(serverID)}/probe`, undefined, options)
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
