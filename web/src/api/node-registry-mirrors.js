import { api } from './index.js'

function get(path, options) { return options ? api.get(path, options) : api.get(path) }
function post(path, body, options) {
  if (options) return api.post(path, body, options)
  return body === undefined ? api.post(path) : api.post(path, body)
}
function put(path, body, options) { return options ? api.put(path, body, options) : api.put(path, body) }
function remove(path, options) { return options ? api.delete(path, undefined, options) : api.delete(path) }

export function getNodeRegistryMirrors(options) { return get('/node-registry-mirrors', options) }
export function createNodeRegistryMirror(payload, options) { return post('/node-registry-mirrors', payload, options) }
export function updateNodeRegistryMirror(id, payload, options) { return put(`/node-registry-mirrors/${encodeURIComponent(id)}`, payload, options) }
export function deleteNodeRegistryMirror(id, options) { return remove(`/node-registry-mirrors/${encodeURIComponent(id)}`, options) }
export function verifyNodeRegistryMirror(id, options) { return post(`/node-registry-mirrors/${encodeURIComponent(id)}/verify`, undefined, options) }
export function applyNodeRegistryMirror(id, payload, options) { return post(`/node-registry-mirrors/${encodeURIComponent(id)}/apply`, payload, options) }
export function getNodeRegistryMirrorApplyStatus(id, options) { return get(`/node-registry-mirrors/${encodeURIComponent(id)}/apply-status`, options) }
