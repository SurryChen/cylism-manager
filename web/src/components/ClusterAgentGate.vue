<template>
  <slot v-if="ready" />
  <SurfaceCard v-else class="cluster-agent-state" padding="none">
    <EmptyState v-if="loading" variant="loading" message="正在检测 Agent 连接..." />
    <EmptyState v-else variant="actionable" :message="message">
      <template #action>
        <div class="cluster-agent-actions">
          <RouterLink class="btn btn-primary" :to="destination">{{ actionLabel }}</RouterLink>
          <button class="btn" type="button" @click="refresh">重新检测</button>
        </div>
      </template>
    </EmptyState>
  </SurfaceCard>
</template>

<script setup>
import { computed, toRef, watch } from 'vue'
import { useClusterAgentStatus } from '../composables/useClusterAgentStatus.js'
import EmptyState from './EmptyState.vue'
import SurfaceCard from './SurfaceCard.vue'

const props = defineProps({ clusterId: { type: [String, Number], default: '' } })
const emit = defineEmits(['ready'])
const { status, loading, error, ready, refresh } = useClusterAgentStatus(toRef(props, 'clusterId'))
watch(ready, value => { if (value) emit('ready') }, { immediate: true })
const state = computed(() => status.value?.state)
const message = computed(() => {
  if (error.value) return `无法检测 Agent 状态：${error.value}`
  if (state.value === 'no_control_plane') return '当前集群还没有控制平面节点，请先部署或纳管集群。'
  if (state.value === 'certificate_error') return `Agent 访问证书不可用：${status.value?.message || '请检查访问证书。'}`
  if (state.value === 'unreachable') return 'Agent 尚未安装或当前无法连接。请到 Agent 管理检查部署和运行状态。'
  if (state.value === 'kubernetes_unavailable') return 'Agent 已响应，但 Kubernetes API 不可用。请检查 Agent 和控制平面运行状态。'
  return `Agent 暂不可用：${status.value?.message || '请检查 Agent 配置和运行状态。'}`
})
const destination = computed(() => state.value === 'no_control_plane' ? `/cluster/${props.clusterId}?tab=topology` : state.value === 'certificate_error' ? '/cluster?tab=access-certificates' : `/cluster/${props.clusterId}?tab=agent`)
const actionLabel = computed(() => state.value === 'no_control_plane' ? '前往集群拓扑' : state.value === 'certificate_error' ? '前往访问证书' : '前往 Agent 管理')
</script>

<style scoped>
.cluster-agent-state { margin-top: var(--tabbed-page-content-gap); }
.cluster-agent-actions { display: flex; flex-wrap: wrap; justify-content: center; gap: 8px; }
</style>
