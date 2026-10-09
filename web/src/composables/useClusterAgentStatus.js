import { computed, ref, watch } from 'vue'
import { getClusterAgentStatus } from '../api/clusters.js'

export function useClusterAgentStatus(clusterId) {
  const status = ref(null)
  const loading = ref(false)
  const error = ref('')
  let sequence = 0

  async function refresh() {
    const id = clusterId.value
    const request = ++sequence
    status.value = null
    error.value = ''
    if (!id) { loading.value = false; return }
    loading.value = true
    try {
      const result = await getClusterAgentStatus(id)
      if (request === sequence) status.value = result
    } catch (err) {
      if (request === sequence) error.value = err.message || '检测 Agent 状态失败'
    } finally {
      if (request === sequence) loading.value = false
    }
  }

  watch(clusterId, refresh, { immediate: true })
  return { status, loading, error, ready: computed(() => !clusterId.value || status.value?.state === 'ready'), refresh }
}
