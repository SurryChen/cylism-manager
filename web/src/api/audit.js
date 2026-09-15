import { api } from './index.js'

export function getAuditLogs({ limit = 20, offset = 0, resourceType = '', action = '', outcome = '', source = '', actorType = '', targetName = '', keyword = '', createdFrom = '', createdTo = '', sort = '', order = '' } = {}, options) {
  const params = new URLSearchParams({ limit: String(limit), offset: String(offset) })
  if (resourceType) params.set('resource_type', resourceType)
  if (action) params.set('action', action)
  if (outcome) params.set('outcome', outcome)
  if (source) params.set('source', source)
  if (actorType) params.set('actor_type', actorType)
  if (targetName) params.set('target_name', targetName)
  if (keyword) params.set('keyword', keyword)
  if (createdFrom) params.set('created_from', createdFrom)
  if (createdTo) params.set('created_to', createdTo)
  if (sort) params.set('sort', sort)
  if (order) params.set('order', order)
  return api.get(`/audit-logs?${params}`, options)
}
