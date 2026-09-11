import { effectScope } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { usePolling } from './usePolling.js'

afterEach(() => {
  vi.useRealTimers()
})

function createPolling(callback, options) {
  const scope = effectScope()
  const polling = scope.run(() => usePolling(callback, options))
  return { polling, scope }
}

describe('usePolling', () => {
  it('runs immediately and then at the configured interval', async () => {
    vi.useFakeTimers()
    const callback = vi.fn().mockResolvedValue(undefined)
    const { polling, scope } = createPolling(callback, { interval: 1000 })

    polling.start({ immediate: true })
    expect(callback).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1000)
    expect(callback).toHaveBeenCalledTimes(2)

    scope.stop()
  })

  it('does not overlap async callbacks', async () => {
    vi.useFakeTimers()
    let resolveCallback
    const callback = vi.fn(() => new Promise(resolve => { resolveCallback = resolve }))
    const { polling, scope } = createPolling(callback, { interval: 1000 })

    polling.start({ immediate: true })
    await vi.advanceTimersByTimeAsync(3000)
    expect(callback).toHaveBeenCalledTimes(1)

    resolveCallback()
    await vi.advanceTimersByTimeAsync(1000)
    expect(callback).toHaveBeenCalledTimes(2)

    scope.stop()
  })

  it('is idempotent and stops future invocations', async () => {
    vi.useFakeTimers()
    const callback = vi.fn().mockResolvedValue(undefined)
    const { polling, scope } = createPolling(callback, { interval: 1000 })

    polling.start()
    polling.start()
    expect(polling.isRunning.value).toBe(true)
    await vi.advanceTimersByTimeAsync(1000)
    expect(callback).toHaveBeenCalledTimes(1)
    polling.stop()
    expect(polling.isRunning.value).toBe(false)
    await vi.advanceTimersByTimeAsync(3000)
    expect(callback).toHaveBeenCalledTimes(1)

    scope.stop()
  })

  it('keeps polling after a callback failure', async () => {
    vi.useFakeTimers()
    const callback = vi.fn()
      .mockRejectedValueOnce(new Error('temporary failure'))
      .mockResolvedValue(undefined)
    const { polling, scope } = createPolling(callback, { interval: 1000 })

    polling.start()
    await vi.advanceTimersByTimeAsync(1000)
    await vi.advanceTimersByTimeAsync(1000)
    expect(callback).toHaveBeenCalledTimes(2)

    scope.stop()
  })

  it('stops automatically when the owning scope is disposed', async () => {
    vi.useFakeTimers()
    const callback = vi.fn().mockResolvedValue(undefined)
    const { polling, scope } = createPolling(callback, { interval: 1000 })

    polling.start()
    scope.stop()
    await vi.advanceTimersByTimeAsync(2000)

    expect(polling.isRunning.value).toBe(false)
    expect(callback).not.toHaveBeenCalled()
  })
})
