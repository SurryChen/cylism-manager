import { api } from './index.js'

const base = '/access-certificates'

export function getAccessCA(options) { return api.get(`${base}/ca`, options) }
export function initializeAccessCA() { return api.post(`${base}/ca`) }
export function getAccessCertificates(options) { return api.get(base, options) }
export function issueAccessCertificate(payload) { return api.post(base, payload) }
export function getAccessCertificatePEM(id) { return api.get(`${base}/${encodeURIComponent(id)}`) }
export function revokeAccessCertificate(id) { return api.post(`${base}/${encodeURIComponent(id)}/revoke`) }
export function deleteAccessCertificate(id) { return api.delete(`${base}/${encodeURIComponent(id)}`) }
export function getAccessCRL() { return api.get(`${base}/crl`) }
