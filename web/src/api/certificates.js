import { api } from './index.js'

function post(path, body, options) {
  if (body === undefined && options === undefined) return api.post(path)
  if (options === undefined) return api.post(path, body)
  return api.post(path, body, options)
}

function put(path, body, options) {
  return options === undefined ? api.put(path, body) : api.put(path, body, options)
}

function remove(path, body, options) {
  if (body === undefined && options === undefined) return api.delete(path)
  if (options === undefined) return api.delete(path, body)
  return api.delete(path, body, options)
}

export function getCertificateStatus(options) { return api.get('/certs/status', options) }
export function getCertificates(options) { return api.get('/certs', options) }
export function getCertificateIssuers(options) { return api.get('/certs/issuers', options) }
export function getDNSCredentials(options) { return api.get('/certs/dns-credentials', options) }
export function getDNSProviders(options) { return api.get('/certs/dns-providers', options) }
export function getCertificateOperations(namespace, name, options) {
  return api.get(`/certs/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/operations`, options)
}

export function installCertificateManager(options) { return post('/certs/install', undefined, options) }
export function installDNSProvider(id, options) { return post(`/certs/dns-providers/${encodeURIComponent(id)}/install`, undefined, options) }
export function createCertificate(body, options) { return post('/certs', body, options) }
export function createIssuer(body, options) { return post('/certs/issuers', body, options) }
export function updateIssuer(kind, namespace, name, body, options) { return put(`/certs/issuers/${encodeURIComponent(kind)}/${encodeURIComponent(namespace || '_')}/${encodeURIComponent(name)}`, body, options) }
export function deleteIssuer(kind, namespace, name, options) { return remove(`/certs/issuers/${encodeURIComponent(kind)}/${encodeURIComponent(namespace || '_')}/${encodeURIComponent(name)}`, undefined, options) }
export function createDNSCredential(body, options) { return post('/certs/dns-credentials', body, options) }
export function updateDNSCredential(id, body, options) { return put(`/certs/dns-credentials/${encodeURIComponent(id)}`, body, options) }
export function deleteDNSCredential(id, options) { return remove(`/certs/dns-credentials/${encodeURIComponent(id)}`, undefined, options) }
export function deleteCertificate(namespace, name, options) { return remove(`/certs/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`, undefined, options) }

export function getCertificateResources(options) {
  return Promise.all([
    getCertificates(options),
    getCertificateIssuers(options),
    getDNSCredentials(options),
    getDNSProviders(options),
  ])
}
