import { watch } from 'vue'
import { getListPage } from '../api/list-pages.js'
import { usePaginatedResource } from './usePaginatedResource.js'

// Adapt existing editable row refs to a committed server pagination snapshot.
export function useListPage(path, rows, error) {
  const resource = usePaginatedResource((query, options) => getListPage(typeof path === 'function' ? path() : path, query, options), 'items')
  watch(resource.items, items => { rows.value = items })
  watch(resource.error, cause => { if (cause) error.value = cause.message || '读取列表失败' })
  async function refresh(filters = resource.filters.value) {
    const previous = resource.filters.value
    const same = JSON.stringify(filters) === JSON.stringify(previous)
    if (!same) { resource.reset(); rows.value = [] }
    const result = await (same ? resource.refresh() : resource.load(filters))
    if (!result && resource.error.value) throw resource.error.value
    return result
  }
  return { ...resource, refresh }
}
