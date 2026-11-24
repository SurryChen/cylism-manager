import { api } from './index.js'
function clusterPath(clusterID, path) { return clusterID ? `/clusters/${encodeURIComponent(clusterID)}/k8s${path}` : path }

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

export function getCertificateStatus(options, clusterID) { return api.get(clusterPath(clusterID, '/certs/status'), options) }
export function getCertificates(options, clusterID) { return api.get(clusterPath(clusterID, '/certs'), options) }
export function getCertificateIssuers(options, clusterID) { return api.get(clusterPath(clusterID, '/certs/issuers'), options) }
export function getDNSCredentials(options, clusterID) { return api.get(clusterPath(clusterID, '/certs/dns-credentials'), options) }
export function getDNSProviders(options, clusterID) { return api.get(clusterPath(clusterID, '/certs/dns-providers'), options) }
export function getCertificateOperations(namespace, name, options, clusterID) {
  return api.get(clusterPath(clusterID, `/certs/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/operations`), options)
}

export function installCertificateManager(options, clusterID) { return post(clusterPath(clusterID, '/certs/install'), undefined, options) }
export function installDNSProvider(id, options, clusterID) { return post(clusterPath(clusterID, `/certs/dns-providers/${encodeURIComponent(id)}/install`), undefined, options) }
export function createCertificate(body, options, clusterID) { return post(clusterPath(clusterID, '/certs'), body, options) }
export function createIssuer(body, options, clusterID) { return post(clusterPath(clusterID, '/certs/issuers'), body, options) }
export function updateIssuer(kind, namespace, name, body, options, clusterID) { return put(clusterPath(clusterID, `/certs/issuers/${encodeURIComponent(kind)}/${encodeURIComponent(namespace || '_')}/${encodeURIComponent(name)}`), body, options) }
export function deleteIssuer(kind, namespace, name, options, clusterID) { return remove(clusterPath(clusterID, `/certs/issuers/${encodeURIComponent(kind)}/${encodeURIComponent(namespace || '_')}/${encodeURIComponent(name)}`), undefined, options) }
export function createDNSCredential(body, options, clusterID) { return post(clusterPath(clusterID, '/certs/dns-credentials'), body, options) }
export function updateDNSCredential(id, body, options, clusterID) { return put(clusterPath(clusterID, `/certs/dns-credentials/${encodeURIComponent(id)}`), body, options) }
export function deleteDNSCredential(id, options, clusterID) { return remove(clusterPath(clusterID, `/certs/dns-credentials/${encodeURIComponent(id)}`), undefined, options) }
export function deleteCertificate(namespace, name, options, clusterID) { return remove(clusterPath(clusterID, `/certs/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`), undefined, options) }

export function getCertificateResources(options) {
  return Promise.all([
    getCertificates(options),
    getCertificateIssuers(options),
    getDNSCredentials(options),
    getDNSProviders(options),
  ])
}
