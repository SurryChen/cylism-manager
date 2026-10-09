<template>
  <div class="cluster-resources-page">
    <RouterLink class="back-link" to="/cluster?tab=list"><ArrowLeft :size="16" />返回集群列表</RouterLink>
    <SectionTabsHeader :title="cluster?.name || 'Kubernetes 资源'" :tabs="tabs" :active-tab="activeTab" test-id-prefix="resource-tab" @select="selectTab" />
    <FeedbackBanner v-if="error" tone="warning" :message="error" />
    <SurfaceCard v-if="loading && !cluster" class="resources-content" padding="none"><EmptyState variant="loading" message="加载中..." /></SurfaceCard>
    <ClusterAgentGate v-else-if="cluster" :key="`${clusterID}-${activeTab}`" :cluster-id="clusterID"><component :is="activeComponent" :key="`${clusterID}-${activeTab}`" :cluster-id="clusterID" class="resources-content" /></ClusterAgentGate>
  </div>
</template>

<script setup>
import { computed, defineAsyncComponent, ref, watch } from 'vue'
import { ArrowLeft } from 'lucide-vue-next'
import { getCluster } from '../../api/clusters.js'
import { useAsyncResource } from '../../composables/useAsyncResource.js'
import { useRoutedTab } from '../../composables/useRoutedTab.js'
import EmptyState from '../../components/EmptyState.vue'
import ClusterAgentGate from '../../components/ClusterAgentGate.vue'
import FeedbackBanner from '../../components/FeedbackBanner.vue'
import SectionTabsHeader from '../../components/SectionTabsHeader.vue'
import SurfaceCard from '../../components/SurfaceCard.vue'

const props = defineProps({ clusterID: { type: String, required: true } })

const Workloads = defineAsyncComponent(() => import('./Workloads.vue'))
const Services = defineAsyncComponent(() => import('./Services.vue'))
const Configs = defineAsyncComponent(() => import('./Configs.vue'))

const tabs = [
  { id: 'workloads', label: '工作负载', component: Workloads },
  { id: 'services', label: '服务', component: Services },
  { id: 'configs', label: '配置', component: Configs },
]
const { activeTab, selectTab } = useRoutedTab({ tabs, defaultTab: 'workloads' })
const activeComponent = computed(() => tabs.find(item => item.id === activeTab.value)?.component || Workloads)
const cluster = ref(null)
const error = ref('')
const resource = useAsyncResource(({ signal }, id) => getCluster(id, { signal }), null)
const loading = resource.loading

watch(() => props.clusterID, async id => {
  cluster.value = null
  error.value = ''
  const result = await resource.refresh(id)
  if (id !== props.clusterID) return
  if (result) cluster.value = result
  else if (resource.error.value) error.value = resource.error.value.message || '加载集群失败'
}, { immediate: true })
</script>

<style scoped>
.cluster-resources-page { min-width: 0; }
.back-link { display: inline-flex; width: fit-content; align-items: center; gap: 6px; margin-bottom: var(--space-16); color: var(--text-secondary); font-size: 13px; font-weight: 600; text-decoration: none; }
.back-link:hover { color: var(--action-primary); }
.back-link:focus-visible { outline: 2px solid var(--focus); outline-offset: 3px; }
.resources-content { margin-top: var(--tabbed-page-content-gap); }
.resources-content :deep(.page-header) { margin-top: var(--space-20); margin-bottom: var(--space-12); }
.resources-content :deep(.page-title) { font-size: 18px; }
</style>
