import { api } from './index.js'

function get(path, options) { return options === undefined ? api.get(path) : api.get(path, options) }
function post(path, body, options) {
  if (options !== undefined) return api.post(path, body, options)
  return body === undefined ? api.post(path) : api.post(path, body)
}
function put(path, body, options) { return options === undefined ? api.put(path, body) : api.put(path, body, options) }

export function getRegistryProxies(options) { return get('/registry-proxies', options) }
export function createRegistryProxy(payload, options) { return post('/registry-proxies', payload, options) }
export function updateRegistryProxy(id, payload, options) { return put(`/registry-proxies/${encodeURIComponent(id)}`, payload, options) }
export function diagnoseRegistryProxy(id, options) { return post(`/registry-proxies/${encodeURIComponent(id)}/diagnose`, undefined, options) }
export function cleanupRegistryProxy(id, options) { return post(`/registry-proxies/${encodeURIComponent(id)}/cleanup`, undefined, options) }
export function migrateRegistryProxy(id, options) { return post(`/registry-proxies/${encodeURIComponent(id)}/migrate-resource-name`, undefined, options) }
