import { getCurrentScope, onScopeDispose, ref } from 'vue'

// Owns only timer mechanics; callers keep polling conditions and error state.
export function usePolling(callback, { interval = 5000 } = {}) {
  if (typeof callback !== 'function') throw new TypeError('usePolling callback must be a function')
  if (!Number.isFinite(interval) || interval <= 0) throw new TypeError('usePolling interval must be positive')

  const isRunning = ref(false)
  const defaultInterval = interval
  let timer = null
  let inFlight = false

  function invoke() {
    if (!isRunning.value || inFlight) return
    inFlight = true
    try {
      const result = callback()
      if (result && typeof result.then === 'function') {
        result.catch(() => undefined).finally(() => { inFlight = false })
      } else {
        inFlight = false
      }
    } catch {
      inFlight = false
    }
  }

  function stop() {
    if (timer !== null) window.clearInterval(timer)
    timer = null
    isRunning.value = false
  }

  function start({ immediate = false, interval: nextInterval = defaultInterval } = {}) {
    if (isRunning.value) return
    if (!Number.isFinite(nextInterval) || nextInterval <= 0) throw new TypeError('usePolling interval must be positive')
    isRunning.value = true
    timer = window.setInterval(invoke, nextInterval)
    if (immediate) invoke()
  }

  if (getCurrentScope()) onScopeDispose(stop)

  return { isRunning, start, stop }
}
