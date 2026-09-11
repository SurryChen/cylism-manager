import { ref } from 'vue'

export function useActionState(initialResult = null) {
  const running = ref(false)
  const error = ref(null)
  const result = ref(initialResult)
  let actionID = 0

  async function run(action) {
    if (typeof action !== 'function') throw new TypeError('useActionState action must be a function')

    const currentID = ++actionID
    running.value = true
    error.value = null

    try {
      const nextResult = await action()
      if (currentID === actionID) result.value = nextResult
      return nextResult
    } catch (actionError) {
      if (currentID === actionID) error.value = actionError
      throw actionError
    } finally {
      if (currentID === actionID) running.value = false
    }
  }

  function reset() {
    actionID += 1
    running.value = false
    error.value = null
    result.value = initialResult
  }

  return { running, error, result, run, reset }
}
