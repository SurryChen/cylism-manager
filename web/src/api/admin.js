import { api } from './index.js'

export function getAdminTables(options) { return api.get('/admin/tables', options) }
export function getAdminTableRows(table, { page, size, sort, order } = {}, options) {
  const params = new URLSearchParams({ page: String(page), size: String(size), sort, order })
  return api.get(`/admin/tables/${encodeURIComponent(table)}?${params}`, options)
}

function post(path, body, options) {
  return options === undefined ? api.post(path, body) : api.post(path, body, options)
}

function put(path, body, options) {
  return options === undefined ? api.put(path, body) : api.put(path, body, options)
}

function remove(path, options) {
  return options === undefined ? api.delete(path) : api.delete(path, undefined, options)
}

export function createAdminTableRow(table, body, options) {
  return post(`/admin/tables/${encodeURIComponent(table)}`, body, options)
}

export function updateAdminTableRow(table, id, body, options) {
  return put(`/admin/tables/${encodeURIComponent(table)}/${encodeURIComponent(id)}`, body, options)
}

export function deleteAdminTableRow(table, id, options) {
  return remove(`/admin/tables/${encodeURIComponent(table)}/${encodeURIComponent(id)}`, options)
}
