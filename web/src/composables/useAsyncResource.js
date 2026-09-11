import { getCurrentScope, onScopeDispose, ref } from 'vue'

// Keeps an independently loaded page resource current without introducing global state.
export function useAsyncResource(loader, initialValue = null) {
  const data = ref(initialValue)
  const loading = ref(false)
  const error = ref(null)
  let controller = null
  let requestID = 0

  function cancel() {
    requestID += 1
    controller?.abort()
    controller = null
    loading.value = false
  }

  async function refresh(...args) {
    const id = requestID + 1
    requestID = id
    controller?.abort()
    const activeController = new AbortController()
    controller = activeController
    loading.value = true
    error.value = null

    try {
      const result = await loader({ signal: activeController.signal }, ...args)
      if (id !== requestID) return undefined
      data.value = result
      return result
    } catch (requestError) {
      if (id === requestID && requestError?.name !== 'AbortError') error.value = requestError
      return undefined
    } finally {
      if (id === requestID) {
        loading.value = false
        controller = null
      }
    }
  }

  if (getCurrentScope()) onScopeDispose(cancel)

  return { data, loading, error, refresh, cancel }
}
