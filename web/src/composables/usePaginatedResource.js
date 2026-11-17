import { computed } from 'vue'
import { useAsyncResource } from './useAsyncResource.js'
import { PAGE_SIZE_OPTIONS } from './usePagination.js'

// Keep rows, applied filters and pagination in one successful snapshot. Failed
// or superseded requests must never move the pager ahead of the visible rows.
export function usePaginatedResource(loader, itemsKey) {
  const resource = useAsyncResource(async ({ signal }, filters, page, pageSize) => {
    const fetchPage = value => loader({ ...filters, limit: pageSize, offset: (value - 1) * pageSize }, { signal })
    let result = await fetchPage(page)
    let total = result.total || 0
    while (page > Math.max(1, Math.ceil(total / pageSize))) {
      signal.throwIfAborted()
      page = Math.max(1, Math.ceil(total / pageSize))
      result = await fetchPage(page)
      total = result.total || 0
    }
    return { items: result[itemsKey] || [], total, page, pageSize, filters, response: result }
  }, { items: [], total: 0, page: 1, pageSize: 10, filters: {} })
  const items = computed(() => resource.data.value.items)
  const total = computed(() => resource.data.value.total)
  const page = computed(() => resource.data.value.page)
  const pageSize = computed(() => resource.data.value.pageSize)
  const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)))
  const response = computed(() => resource.data.value.response)
  function reset() {
    resource.cancel()
    resource.data.value = { items: [], total: 0, page: 1, pageSize: pageSize.value, filters: {} }
    resource.error.value = null
  }
  function load(filters = resource.data.value.filters) {
    return resource.refresh({ ...filters }, 1, pageSize.value)
  }
  function changePage(value) {
    if (!resource.loading.value && Number.isInteger(value) && value >= 1 && value <= totalPages.value && value !== page.value) {
      return resource.refresh(resource.data.value.filters, value, pageSize.value)
    }
  }
  function changePageSize(value) {
    if (!resource.loading.value && PAGE_SIZE_OPTIONS.includes(value) && value !== pageSize.value) {
      return resource.refresh(resource.data.value.filters, 1, value)
    }
  }
  function refresh() {
    return resource.refresh(resource.data.value.filters, page.value, pageSize.value)
  }
  return { items, total, page, pageSize, totalPages, filters: computed(() => resource.data.value.filters), response, refresh, reset, cancel: resource.cancel, loading: resource.loading, error: resource.error, load, changePage, changePageSize }
}
