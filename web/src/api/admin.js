import { api } from './index.js'

export function getAdminTables(options) { return api.get('/admin/tables', options) }
export function getAdminTableRows(table, { page, size, sort, order } = {}, options) {
  const params = new URLSearchParams({ page: String(page), size: String(size), sort, order })
  return api.get(`/admin/tables/${encodeURIComponent(table)}?${params}`, options)
}
