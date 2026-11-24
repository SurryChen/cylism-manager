import { defineComponent, nextTick } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import DocumentScrollbar from './DocumentScrollbar.vue'
import { useTransientScrollbarVisibility } from '../composables/useTransientScrollbarVisibility.js'

let wrapper
let resizeCallback
let disconnect
let scrollTo
let rootHeight
let rootTop
let viewportHeight
const originalProperties = new Map()

function setMetric(name, get) {
  originalProperties.set(name, Object.getOwnPropertyDescriptor(document.documentElement, name))
  Object.defineProperty(document.documentElement, name, { configurable: true, get })
}

async function mountScrollbar() {
  wrapper = mount(defineComponent({
    components: { DocumentScrollbar },
    setup() { useTransientScrollbarVisibility(); return {} },
    template: '<DocumentScrollbar />',
  }), { attachTo: document.body })
  await nextTick()
}

function bar() { return document.querySelector('[role="scrollbar"]') }
function thumb() { return document.querySelector('.document-scrollbar-thumb') }

async function pointer(type, target, clientY) {
  const event = new Event(type, { bubbles: true, cancelable: true })
  Object.assign(event, { button: 0, pointerId: 1, clientY })
  target.dispatchEvent(event)
  await nextTick()
}

beforeEach(() => {
  vi.useFakeTimers()
  rootHeight = 2000
  rootTop = 0
  viewportHeight = 800
  setMetric('scrollHeight', () => rootHeight)
  setMetric('scrollTop', () => rootTop)
  setMetric('clientHeight', () => viewportHeight)
  disconnect = vi.fn()
  vi.stubGlobal('ResizeObserver', class {
    constructor(callback) { resizeCallback = callback }
    observe() {}
    disconnect = disconnect
  })
  vi.stubGlobal('requestAnimationFrame', callback => setTimeout(callback, 16))
  vi.stubGlobal('cancelAnimationFrame', id => clearTimeout(id))
  scrollTo = vi.spyOn(window, 'scrollTo').mockImplementation(({ top }) => {
    rootTop = top
    document.dispatchEvent(new Event('scroll'))
    window.dispatchEvent(new Event('scroll'))
  })
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  for (const [key, descriptor] of originalProperties) {
    if (descriptor) Object.defineProperty(document.documentElement, key, descriptor)
    else delete document.documentElement[key]
  }
  originalProperties.clear()
  document.body.innerHTML = ''
  document.body.style.overflow = ''
  document.documentElement.classList.remove('is-scrolling')
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

describe('DocumentScrollbar', () => {
  it('tracks the document position and hides again after scrolling stops', async () => {
    await mountScrollbar()
    expect(bar().getAttribute('aria-valuemax')).toBe('1200')
    expect(parseFloat(thumb().style.height)).toBeCloseTo(316.8)
    expect(document.documentElement.classList.contains('is-scrolling')).toBe(false)

    window.scrollTo({ top: 600 })
    await vi.advanceTimersByTimeAsync(16)
    expect(bar().getAttribute('aria-valuenow')).toBe('600')
    expect(parseFloat(thumb().style.transform.slice(11))).toBeCloseTo(237.6)
    expect(document.documentElement.classList.contains('is-scrolling')).toBe(true)
    await vi.advanceTimersByTimeAsync(684)
    expect(document.documentElement.classList.contains('is-scrolling')).toBe(false)
  })

  it('does not show a page scrollbar when only a nested region scrolls', async () => {
    await mountScrollbar()
    const nested = document.createElement('div')
    document.body.append(nested)
    nested.dispatchEvent(new Event('scroll'))
    await nextTick()
    expect(nested.classList.contains('is-scrolling')).toBe(true)
    expect(document.documentElement.classList.contains('is-scrolling')).toBe(false)
  })

  it('responds to content and viewport size changes and disappears when content fits', async () => {
    await mountScrollbar()
    rootHeight = 800
    resizeCallback()
    await vi.advanceTimersByTimeAsync(16)
    expect(bar()).toBeNull()
    rootHeight = 2400
    resizeCallback()
    await vi.advanceTimersByTimeAsync(16)
    expect(bar().getAttribute('aria-valuemax')).toBe('1600')
    viewportHeight = 600
    window.dispatchEvent(new Event('resize'))
    await vi.advanceTimersByTimeAsync(16)
    expect(bar().getAttribute('aria-valuemax')).toBe('1800')
  })

  it('supports keyboard scrolling with boundary clamping', async () => {
    await mountScrollbar()
    bar().dispatchEvent(new KeyboardEvent('keydown', { key: 'End', bubbles: true, cancelable: true }))
    expect(scrollTo).toHaveBeenLastCalledWith({ top: 1200, behavior: 'instant' })
    await vi.advanceTimersByTimeAsync(16)
    bar().dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true, cancelable: true }))
    expect(scrollTo).toHaveBeenLastCalledWith({ top: 1200, behavior: 'instant' })
    bar().dispatchEvent(new KeyboardEvent('keydown', { key: 'Home', bubbles: true, cancelable: true }))
    expect(scrollTo).toHaveBeenLastCalledWith({ top: 0, behavior: 'instant' })
  })

  it('supports dragging the thumb without losing its grabbed position', async () => {
    await mountScrollbar()
    const track = bar()
    track.setPointerCapture = vi.fn()
    track.hasPointerCapture = vi.fn(() => true)
    track.releasePointerCapture = vi.fn()
    vi.spyOn(track, 'getBoundingClientRect').mockReturnValue({ top: 4 })
    vi.spyOn(thumb(), 'getBoundingClientRect').mockReturnValue({ top: 4 })
    await pointer('pointerdown', thumb(), 14)
    expect(scrollTo).toHaveBeenLastCalledWith({ top: 0, behavior: 'instant' })
    await pointer('pointermove', track, 251.6)
    expect(scrollTo.mock.lastCall[0].top).toBeCloseTo(600)
    expect(track.classList.contains('is-dragging')).toBe(true)
    await pointer('pointerup', track, 251.6)
    expect(track.classList.contains('is-dragging')).toBe(false)
    expect(track.releasePointerCapture).toHaveBeenCalledWith(1)
    expect(document.activeElement).not.toBe(track)
  })

  it('hides while body scrolling is locked and restores after unlocking', async () => {
    await mountScrollbar()
    document.body.style.overflow = 'hidden'
    await Promise.resolve()
    await vi.advanceTimersByTimeAsync(32)
    expect(bar()).toBeNull()
    document.body.style.overflow = ''
    await Promise.resolve()
    await vi.advanceTimersByTimeAsync(32)
    expect(bar()).not.toBeNull()
  })

  it('disconnects observers and cancels scheduled work when unmounted', async () => {
    await mountScrollbar()
    resizeCallback()
    wrapper.unmount()
    wrapper = null
    expect(disconnect).toHaveBeenCalledOnce()
    expect(bar()).toBeNull()
    expect(vi.getTimerCount()).toBe(0)
  })
})
