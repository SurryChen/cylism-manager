import { effectScope, nextTick, ref } from 'vue'
import { afterEach, describe, expect, it } from 'vitest'
import { useBodyScrollLock } from './useBodyScrollLock.js'

afterEach(() => {
  document.body.style.overflow = ''
})

describe('useBodyScrollLock', () => {
  it('locks scrolling and restores the previous overflow value', () => {
    document.body.style.overflow = 'auto'
    const scope = effectScope()
    scope.run(() => useBodyScrollLock(true))

    expect(document.body.style.overflow).toBe('hidden')

    scope.stop()
    expect(document.body.style.overflow).toBe('auto')
  })

  it('keeps scrolling locked until the final scope is released', () => {
    const firstScope = effectScope()
    const secondScope = effectScope()
    firstScope.run(() => useBodyScrollLock(true))
    secondScope.run(() => useBodyScrollLock(true))

    firstScope.stop()
    expect(document.body.style.overflow).toBe('hidden')

    secondScope.stop()
    expect(document.body.style.overflow).toBe('')
  })

  it('reacts to a lock ref changing over time', async () => {
    const locked = ref(false)
    const scope = effectScope()
    scope.run(() => useBodyScrollLock(locked))

    expect(document.body.style.overflow).toBe('')
    locked.value = true
    await nextTick()
    expect(document.body.style.overflow).toBe('hidden')
    locked.value = false
    await nextTick()
    expect(document.body.style.overflow).toBe('')

    scope.stop()
  })
})
