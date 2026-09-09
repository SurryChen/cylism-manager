import { api } from './index.js'

export function getAuditLogs({ limit = 20, offset = 0, resourceType = '', action = '', keyword = '', sort = '', order = '' } = {}, options) {
  const params = new URLSearchParams({ limit: String(limit), offset: String(offset) })
  if (resourceType) params.set('resource_type', resourceType)
  if (action) params.set('action', action)
  if (keyword) params.set('keyword', keyword)
  if (sort) params.set('sort', sort)
  if (order) params.set('order', order)
  return api.get(`/audit-logs?${params}`, options)
}
