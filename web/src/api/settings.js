import { api } from './index.js'

export function getPlatformEndpoint(options) { return api.get('/platform/endpoint', options) }
export function getPlatformCertificates(options) { return api.get('/certs', options) }
export function getPlatformStatus(options) { return api.get('/platform/status', options) }
export function getTemporaryTokens(options) { return api.get('/auth/temporary-tokens', options) }
