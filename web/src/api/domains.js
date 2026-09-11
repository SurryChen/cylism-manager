import { api } from './index.js'

function post(path, body, options) {
  return options ? api.post(path, body, options) : body === undefined ? api.post(path) : api.post(path, body)
}
function put(path, body, options) { return options ? api.put(path, body, options) : api.put(path, body) }
function remove(path, options) { return options ? api.delete(path, undefined, options) : api.delete(path) }
function segment(value) { return encodeURIComponent(String(value)) }

function queryString(params) {
  const query = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) if (value !== undefined && value !== null && value !== '') query.set(key, String(value))
  const encoded = query.toString()
  return encoded ? `?${encoded}` : ''
}

export function getManagedDomains({ environmentID, unassigned = false } = {}, options) {
  return api.get(`/domains${queryString({ environment_id: environmentID, unassigned: unassigned || undefined })}`, options)
}

export function getDomainOptions(options) { return api.get('/certs/issuers', options) }
export function getImportableCertificates(environmentID, options) { return api.get(`/domains/importable-certificates${queryString({ environment_id: environmentID })}`, options) }
export function getClaimableDomains(environmentID, options) { return api.get(`/domains/claimable${queryString({ environment_id: environmentID })}`, options) }
export function createManagedDomain(body, options) { return post('/domains', body, options) }
export function updateManagedDomain(id, body, options) { return put(`/domains/${segment(id)}`, body, options) }
export function importManagedDomainCertificate(body, options) { return post('/domains/import', body, options) }
export function claimManagedDomain(id, body, options) { return post(`/domains/${segment(id)}/claim`, body, options) }
export function retryManagedDomainCertificate(id, options) { return post(`/domains/${segment(id)}/certificate`, undefined, options) }
export function deleteManagedDomain(id, options) { return remove(`/domains/${segment(id)}`, options) }
