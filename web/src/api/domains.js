import { api } from './index.js'

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
