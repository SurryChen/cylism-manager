import { computed } from 'vue'
import { useAsyncResource } from './useAsyncResource.js'
import { PAGE_SIZE_OPTIONS } from './usePagination.js'

// Tokens and rows are committed together; a failed request preserves navigation.
export function useCursorResource(loader) {
  const resource = useAsyncResource(async ({ signal }, filters, page, pageSize, tokens) => {
    const result = await loader({ ...filters, limit: pageSize, token: tokens[page - 1] || '' }, { signal })
    return { items: result.items || [], nextToken: result.nextToken || '', filters, page, pageSize, tokens }
  }, { items: [], nextToken: '', filters: {}, page: 1, pageSize: 10, tokens: [''] })
  const state = resource.data
  function reset() {
    resource.cancel()
    state.value = { items: [], nextToken: '', filters: {}, page: 1, pageSize: state.value.pageSize, tokens: [''] }
    resource.error.value = null
  }
  function load(filters = state.value.filters) { return resource.refresh(filters, 1, state.value.pageSize, ['']) }
  function refresh() { const s = state.value; return resource.refresh(s.filters, s.page, s.pageSize, s.tokens) }
  function changePage(page) {
    const s = state.value
    if (resource.loading.value || !Number.isInteger(page) || page < 1 || page === s.page || page > s.page + 1) return
    const tokens = s.tokens.slice(0, s.page)
    if (page > s.page) { if (!s.nextToken) return; tokens[page - 1] = s.nextToken }
    return resource.refresh(s.filters, page, s.pageSize, tokens)
  }
  function changePageSize(size) { if (!resource.loading.value && PAGE_SIZE_OPTIONS.includes(size)) return resource.refresh(state.value.filters, 1, size, ['']) }
  return { items: computed(() => state.value.items), page: computed(() => state.value.page), pageSize: computed(() => state.value.pageSize), hasNext: computed(() => Boolean(state.value.nextToken)), loading: resource.loading, error: resource.error, load, refresh, reset, changePage, changePageSize, cancel: resource.cancel }
}
