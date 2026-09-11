import { effectScope } from 'vue'
import { describe, expect, it } from 'vitest'
import { useActionState } from './useActionState.js'

function deferred() {
  let resolve
  let reject
  const promise = new Promise((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

function createActionState() {
  const scope = effectScope()
  const state = scope.run(() => useActionState())
  return { state, scope }
}

describe('useActionState', () => {
  it('exposes a successful result and clears the running state', async () => {
    const { state, scope } = createActionState()

    await expect(state.run(() => Promise.resolve({ id: 1 }))).resolves.toEqual({ id: 1 })

    expect(state.result.value).toEqual({ id: 1 })
    expect(state.error.value).toBe(null)
    expect(state.running.value).toBe(false)
    scope.stop()
  })

  it('stores a failed action error and releases the running state', async () => {
    const { state, scope } = createActionState()
    const error = new Error('保存失败')

    await expect(state.run(() => Promise.reject(error))).rejects.toBe(error)

    expect(state.error.value).toBe(error)
    expect(state.result.value).toBe(null)
    expect(state.running.value).toBe(false)
    scope.stop()
  })

  it('clears the previous error when a new action starts', async () => {
    const { state, scope } = createActionState()
    const error = new Error('第一次失败')
    await expect(state.run(() => Promise.reject(error))).rejects.toBe(error)

    const pending = deferred()
    const nextRun = state.run(() => pending.promise)
    expect(state.running.value).toBe(true)
    expect(state.error.value).toBe(null)

    pending.resolve('完成')
    await expect(nextRun).resolves.toBe('完成')
    expect(state.result.value).toBe('完成')
    expect(state.running.value).toBe(false)
    scope.stop()
  })

  it('resets result and error state without affecting the action API', async () => {
    const { state, scope } = createActionState()
    await state.run(() => Promise.resolve('完成'))

    state.reset()

    expect(state.result.value).toBe(null)
    expect(state.error.value).toBe(null)
    expect(state.running.value).toBe(false)
    scope.stop()
  })
})
