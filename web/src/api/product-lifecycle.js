import { api } from './index.js'
const segment = value => encodeURIComponent(String(value))
const base = id => `/products/${segment(id)}`
export const getProductWorkbench = options => api.get('/products/workbench', options)
export const getLifecycleOptions = (id, options) => api.get(`${base(id)}/lifecycle-options`, options)
export const getLifecyclePage = (id, kind, query, options) => api.get(`${base(id)}/lifecycle/${segment(kind)}?${new URLSearchParams(query)}`, options)
export const saveLifecycleRecord = (id, kind, record) => record.id
  ? api.put(`${base(id)}/lifecycle/${segment(kind)}/${segment(record.id)}`, record)
  : api.post(`${base(id)}/lifecycle/${segment(kind)}`, record)
export const deleteProductTask = (id, taskID) => api.delete(`${base(id)}/lifecycle/tasks/${segment(taskID)}`)
export const deleteProductTaskGroup = (id, groupID) => api.delete(`${base(id)}/lifecycle/task-groups/${segment(groupID)}`)
export const deleteReleaseUnit = (id, unitID) => api.delete(`${base(id)}/lifecycle/release-units/${segment(unitID)}`)
export const getDistributionEvents = (id, recordID, query, options) => api.get(`${base(id)}/distributions/${segment(recordID)}/events?${new URLSearchParams(query)}`, options)
export const importProductNextAction = id => api.post(`${base(id)}/import-next-action`, {})
