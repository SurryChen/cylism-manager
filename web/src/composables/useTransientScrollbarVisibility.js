import { onScopeDispose } from 'vue'

const SCROLLING_CLASS = 'is-scrolling'

export function useTransientScrollbarVisibility({ hideDelay = 700 } = {}) {
  if (typeof document === 'undefined') return

  const timers = new Map()

  function clearTarget(target) {
    const timer = timers.get(target)
    if (timer !== undefined) {
      clearTimeout(timer)
      timers.delete(target)
    }
    target.classList.remove(SCROLLING_CLASS)
  }

  function markScrolling(event) {
    const target = event.target === document ? document.documentElement : event.target
    if (!target?.classList) return

    const existingTimer = timers.get(target)
    if (existingTimer !== undefined) clearTimeout(existingTimer)

    target.classList.add(SCROLLING_CLASS)
    timers.set(target, setTimeout(() => clearTarget(target), hideDelay))
  }

  document.addEventListener('scroll', markScrolling, true)

  onScopeDispose(() => {
    document.removeEventListener('scroll', markScrolling, true)
    for (const target of timers.keys()) clearTarget(target)
  })
}
