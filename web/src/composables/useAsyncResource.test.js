import { effectScope } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { useAsyncResource } from './useAsyncResource.js'

function deferred() {
  let resolve
  let reject
  const promise = new Promise((onResolve, onReject) => {
    resolve = onResolve
    reject = onReject
  })
  return { promise, resolve, reject }
}

function createResource(loader, initialValue) {
  const scope = effectScope()
  const resource = scope.run(() => useAsyncResource(loader, initialValue))
  return { resource, scope }
}

describe('useAsyncResource', () => {
  it('cancels an earlier request and keeps the newest result', async () => {
    const first = deferred()
    const second = deferred()
    const loader = vi.fn()
      .mockImplementationOnce(() => first.promise)
      .mockImplementationOnce(() => second.promise)
    const { resource, scope } = createResource(loader, 'initial')

    const firstRequest = resource.refresh()
    const secondRequest = resource.refresh()
    expect(loader.mock.calls[0][0].signal.aborted).toBe(true)

    first.resolve('stale')
    second.resolve('current')
    const [firstResult, secondResult] = await Promise.all([firstRequest, secondRequest])

    expect(firstResult).toBeUndefined()
    expect(secondResult).toBe('current')
    expect(resource.data.value).toBe('current')
    expect(resource.loading.value).toBe(false)
    scope.stop()
  })

  it('exposes a request error without replacing the last value', async () => {
    const error = new Error('request failed')
    const { resource, scope } = createResource(() => Promise.reject(error), { ready: true })

    await resource.refresh()

    expect(resource.data.value).toEqual({ ready: true })
    expect(resource.error.value).toBe(error)
    expect(resource.loading.value).toBe(false)
    scope.stop()
  })
})
