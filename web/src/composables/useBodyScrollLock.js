import { computed, getCurrentScope, onScopeDispose, ref, unref, watch } from 'vue'

let lockCount = 0
let previousOverflow = ''

function acquireLock() {
  if (typeof document === 'undefined' || !document.body) return
  if (lockCount === 0) previousOverflow = document.body.style.overflow
  lockCount += 1
  document.body.style.overflow = 'hidden'
}

function releaseLock() {
  if (typeof document === 'undefined' || !document.body || lockCount === 0) return
  lockCount -= 1
  if (lockCount === 0) {
    document.body.style.overflow = previousOverflow
    previousOverflow = ''
  }
}

export function useBodyScrollLock(locked = true) {
  const active = ref(false)

  function sync(nextLocked) {
    if (nextLocked && !active.value) {
      acquireLock()
      active.value = true
    } else if (!nextLocked && active.value) {
      releaseLock()
      active.value = false
    }
  }

  watch(() => unref(locked), sync, { immediate: true })

  if (getCurrentScope()) onScopeDispose(() => sync(false))

  return {
    locked: computed(() => active.value),
    lock: () => sync(true),
    unlock: () => sync(false),
  }
}
