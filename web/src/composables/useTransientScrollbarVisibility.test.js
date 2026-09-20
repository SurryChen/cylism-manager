import { effectScope } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useTransientScrollbarVisibility } from './useTransientScrollbarVisibility.js'

afterEach(() => {
  vi.useRealTimers()
  document.documentElement.classList.remove('is-scrolling')
  document.body.innerHTML = ''
})

describe('useTransientScrollbarVisibility', () => {
  it('shows the document scrollbar while the document is scrolling and hides it after the delay', () => {
    vi.useFakeTimers()
    const scope = effectScope()
    scope.run(() => useTransientScrollbarVisibility())

    document.dispatchEvent(new Event('scroll'))
    expect(document.documentElement.classList).toContain('is-scrolling')

    vi.advanceTimersByTime(699)
    expect(document.documentElement.classList).toContain('is-scrolling')
    vi.advanceTimersByTime(1)
    expect(document.documentElement.classList).not.toContain('is-scrolling')

    scope.stop()
  })

  it('tracks nested scroll containers independently and resets the latest timer', () => {
    vi.useFakeTimers()
    const scope = effectScope()
    scope.run(() => useTransientScrollbarVisibility({ hideDelay: 700 }))
    const first = document.createElement('div')
    const second = document.createElement('div')
    document.body.append(first, second)

    first.dispatchEvent(new Event('scroll'))
    vi.advanceTimersByTime(500)
    second.dispatchEvent(new Event('scroll'))
    vi.advanceTimersByTime(200)

    expect(first.classList).not.toContain('is-scrolling')
    expect(second.classList).toContain('is-scrolling')

    first.dispatchEvent(new Event('scroll'))
    vi.advanceTimersByTime(699)
    expect(first.classList).toContain('is-scrolling')
    vi.advanceTimersByTime(1)
    expect(first.classList).not.toContain('is-scrolling')

    scope.stop()
  })

  it('clears active classes and listeners when the scope is disposed', () => {
    vi.useFakeTimers()
    const scope = effectScope()
    scope.run(() => useTransientScrollbarVisibility())
    const container = document.createElement('div')
    document.body.append(container)
    container.dispatchEvent(new Event('scroll'))

    scope.stop()
    expect(container.classList).not.toContain('is-scrolling')

    container.dispatchEvent(new Event('scroll'))
    expect(container.classList).not.toContain('is-scrolling')
  })
})
