import { api } from './index.js'

function get(path, options) { return options === undefined ? api.get(path) : api.get(path, options) }
function post(path, body, options) {
  if (options !== undefined) return api.post(path, body, options)
  return body === undefined ? api.post(path) : api.post(path, body)
}
function put(path, body, options) { return options === undefined ? api.put(path, body) : api.put(path, body, options) }

export function getRegistryProxies(options) { return get('/registry-proxies', options) }
export function createRegistryProxy(payload, options) { return post('/registry-proxies', payload, options) }
export function importRegistryProxy(payload, options) { return post('/registry-proxies/import', payload, options) }
export function takeOverRegistryProxy(id, payload, options) { return post(`/registry-proxies/${encodeURIComponent(id)}/take-over`, payload, options) }
export function removeRegistryProxyObservation(id) { return api.delete(`/registry-proxies/${encodeURIComponent(id)}/observation`) }
export function updateRegistryProxy(id, payload, options) { return put(`/registry-proxies/${encodeURIComponent(id)}`, payload, options) }
export function diagnoseRegistryProxy(id, options) { return post(`/registry-proxies/${encodeURIComponent(id)}/diagnose`, undefined, options) }
export function cleanupRegistryProxy(id, options) { return post(`/registry-proxies/${encodeURIComponent(id)}/cleanup`, undefined, options) }
export function migrateRegistryProxy(id, options) { return post(`/registry-proxies/${encodeURIComponent(id)}/migrate-resource-name`, undefined, options) }
export function getRegistryProxyRelays(id, options) { return get(`/registry-proxies/${encodeURIComponent(id)}/relays`, options) }
export function createRegistryProxyRelay(id, payload) { return post(`/registry-proxies/${encodeURIComponent(id)}/relays`, payload) }
export function checkRegistryProxyRelay(id, relayID) { return post(`/registry-proxies/${encodeURIComponent(id)}/relays/${encodeURIComponent(relayID)}/check`) }
export function retryRegistryProxyRelay(id, relayID) { return post(`/registry-proxies/${encodeURIComponent(id)}/relays/${encodeURIComponent(relayID)}/retry`) }
export function deleteRegistryProxyRelay(id, relayID) { return api.delete(`/registry-proxies/${encodeURIComponent(id)}/relays/${encodeURIComponent(relayID)}`) }
export function getRegistryProxyNodeRoutes(id, options) { return get(`/registry-proxies/${encodeURIComponent(id)}/node-routes`, options) }
export function switchRegistryProxyNodeRoute(id, serverID, payload) { return put(`/registry-proxies/${encodeURIComponent(id)}/node-routes/${encodeURIComponent(serverID)}`, payload) }
export function checkRegistryProxyNodeRoute(id, serverID) { return post(`/registry-proxies/${encodeURIComponent(id)}/node-routes/${encodeURIComponent(serverID)}/check`) }
export function forgetRegistryProxyNodeRoute(id, serverID) { return api.delete(`/registry-proxies/${encodeURIComponent(id)}/node-routes/${encodeURIComponent(serverID)}`) }
