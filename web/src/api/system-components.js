import { api } from './index.js'
export { getClusterNodes } from './cluster.js'

export function getSystemComponents(options) { return api.get('/system-components', options) }

function put(path, body, options) { return options ? api.put(path, body, options) : api.put(path, body) }
function post(path, options) { return options ? api.post(path, undefined, options) : api.post(path) }

export function updateSystemComponent(chartName, payload, options) {
  return put(`/system-components/${encodeURIComponent(chartName)}`, payload, options)
}

export function revertSystemComponent(chartName, options) {
  return post(`/system-components/${encodeURIComponent(chartName)}/revert`, options)
}
