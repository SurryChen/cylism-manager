import { api } from './index.js'

const settings = '/settings/github'
const bindings = productId => `/products/${encodeURIComponent(productId)}/github/repositories`
const pageQuery = (page, extra) => new URLSearchParams({ page, ...extra })

export const getGitHubConnection = options => api.get(settings, options)
export const startGitHubRegistration = body => api.post(`${settings}/manifest`, body)
export const deleteGitHubConnection = () => api.delete(settings)
export const getGitHubInstallations = (page, options) => api.get(`${settings}/installations?${pageQuery(page)}`, options)
export const getInstallationRepositories = (id, page, options) =>
  api.get(`${settings}/installations/${encodeURIComponent(id)}/repositories?${pageQuery(page)}`, options)
export const getGitHubBindings = (productId, options) => api.get(bindings(productId), options)
export const bindGitHubRepository = (productId, body) => api.post(bindings(productId), body)
export const unbindGitHubRepository = (productId, id) => api.delete(`${bindings(productId)}/${encodeURIComponent(id)}`)
export const getGitHubRevisions = (productId, id, kind, page, options) =>
  api.get(`${bindings(productId)}/${encodeURIComponent(id)}/revisions?${pageQuery(page, { kind })}`, options)
