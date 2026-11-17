import { api } from './index.js'

export function getPlatformEndpoint(options) { return api.get('/platform/endpoint', options) }
export function getPlatformCapabilities(options) { return api.get('/platform/capabilities', options) }
export function getPlatformCertificates(options) { return api.get('/certs', options) }
export function getPlatformStatus(options) { return api.get('/platform/status', options) }
export async function getPlatformReleasePage({ limit = 10, offset = 0 } = {}, options) {
  const result = await api.get('/platform/status?' + new URLSearchParams({ limit, offset }), options)
  // Older backends return only a recent window and ignore pagination parameters.
  // Paginate that window locally and let the view clearly identify the limitation.
  const legacy = result.releases_total == null
  const releases = result.releases || []
  return { ...result, total: legacy ? releases.length : result.releases_total, releases: legacy ? releases.slice(offset, offset + limit) : releases }
}
export function getTemporaryTokens(options) { return api.get('/auth/temporary-tokens', options) }

function post(path, body, options) {
  return options ? api.post(path, body, options) : body === undefined ? api.post(path) : api.post(path, body)
}
function put(path, body, options) { return options ? api.put(path, body, options) : api.put(path, body) }
function remove(path, options) { return options ? api.delete(path, undefined, options) : api.delete(path) }

export function updatePlatformEndpoint(body, options) { return put('/platform/endpoint', body, options) }
export function adoptPlatformIngress(options) { return post('/platform/endpoint/adopt-ingress', undefined, options) }
export function reconcilePlatformEndpoint(options) { return post('/platform/endpoint/reconcile', undefined, options) }
export function createTemporaryToken(body, options) { return post('/auth/temporary-tokens', body, options) }
export function deleteTemporaryToken(tokenID, options) {
  return remove(`/auth/temporary-tokens/${encodeURIComponent(tokenID)}`, options)
}
export function generatePlatformWebhookSecret(options) { return post('/platform/webhook-secret', undefined, options) }
export function updatePlatformImagePrefix(body, options) { return put('/platform/image-prefix', body, options) }
export function createPlatformRelease(body, options) { return post('/platform/releases', body, options) }
export function rollbackPlatformRelease(releaseID, options) {
  return post(`/platform/releases/${encodeURIComponent(releaseID)}/rollback`, undefined, options)
}

export function getTemporaryTokenPage(query, options) { return api.get('/auth/temporary-tokens?' + new URLSearchParams(query), options) }
