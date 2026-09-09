import { api } from './index.js'

export function getCertificateStatus(options) { return api.get('/certs/status', options) }
export function getCertificates(options) { return api.get('/certs', options) }
export function getCertificateIssuers(options) { return api.get('/certs/issuers', options) }
export function getDNSCredentials(options) { return api.get('/certs/dns-credentials', options) }
export function getDNSProviders(options) { return api.get('/certs/dns-providers', options) }

export function getCertificateResources(options) {
  return Promise.all([
    getCertificates(options),
    getCertificateIssuers(options),
    getDNSCredentials(options),
    getDNSProviders(options),
  ])
}
