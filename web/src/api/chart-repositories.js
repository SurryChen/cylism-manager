import { api } from './index.js'

function segment(value) { return encodeURIComponent(String(value)) }
function post(path, body, options) {
  return options ? api.post(path, body, options) : body === undefined ? api.post(path) : api.post(path, body)
}
function put(path, body, options) { return options ? api.put(path, body, options) : api.put(path, body) }
function remove(path, options) { return options ? api.delete(path, undefined, options) : api.delete(path) }

export function getChartRepositories(options) { return api.get('/chart-repositories', options) }
export function createChartRepository(body, options) { return post('/chart-repositories', body, options) }
export function updateChartRepository(id, body, options) { return put(`/chart-repositories/${segment(id)}`, body, options) }
export function verifyChartRepository(id, options) { return post(`/chart-repositories/${segment(id)}/verify`, undefined, options) }
export function deleteChartRepository(id, options) { return remove(`/chart-repositories/${segment(id)}`, options) }
