import { api } from './index.js'

export function getOperations({ limit = 20, offset = 0, resourceType = '', status = '', keyword = '' } = {}, options) {
  const params = new URLSearchParams({ limit: String(limit), offset: String(offset) })
  if (resourceType) params.set('resource_type', resourceType)
  if (status) params.set('status', status)
  if (keyword) params.set('keyword', keyword)
  return api.get(`/operations?${params}`, options)
}
