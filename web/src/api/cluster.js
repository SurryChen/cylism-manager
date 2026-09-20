import { api } from './index.js'

export function getClusterNodes(options) { return api.get('/nodes', options) }
export function getClusterServers(options) { return api.get('/servers', options) }
export function getClusterPlatform(options) { return api.get('/k8s/platform', options) }
export function getClusterInventory(options) {
  return Promise.all([getClusterNodes(options), getClusterServers(options)])
}

function nodePath(nodeName) {
  return `/nodes/${encodeURIComponent(nodeName)}`
}

function post(path, body, options) {
  if (options === undefined && body === undefined) return api.post(path)
  return options === undefined ? api.post(path, body) : api.post(path, body, options)
}

function patch(path, body, options) {
  return options === undefined ? api.patch(path, body) : api.patch(path, body, options)
}

function remove(path, options) {
  return options === undefined ? api.delete(path) : api.delete(path, undefined, options)
}

export function getNodeDrainPlan(nodeName, options) {
  return api.get(`${nodePath(nodeName)}/drain-plan`, options)
}

export function drainNode(nodeName, body, options) {
  return post(`${nodePath(nodeName)}/drain`, body, options)
}

export function forceDrainNode(nodeName, body, options) {
  return post(`${nodePath(nodeName)}/force-drain`, body, options)
}

export function rejoinNode(nodeName, options) {
  return post(`${nodePath(nodeName)}/rejoin`, undefined, options)
}

export function getNodeLabels(nodeName, options) {
  return api.get(`${nodePath(nodeName)}/labels`, options)
}

export function updateNodeLabels(nodeName, body, options) {
  return patch(`${nodePath(nodeName)}/labels`, body, options)
}

export function getNodeRemovalCheck(nodeName, options) {
  return api.get(`${nodePath(nodeName)}/removal-check`, options)
}

export function removeClusterNode(nodeName, options) {
  return remove(nodePath(nodeName), options)
}
