<template>
  <div class="cluster-network-page">
    <PageBackLink to="/cluster?tab=list">返回集群列表</PageBackLink>
    <SectionTabsHeader :title="cluster?.name || '网络访问'" :tabs="tabs" :active-tab="activeTab" test-id-prefix="network-tab" @select="selectTab" />
    <FeedbackBanner v-if="error" tone="warning" :message="error" />
    <SurfaceCard v-if="loading && !cluster" class="network-content" padding="none"><EmptyState variant="loading" message="加载中..." /></SurfaceCard>
    <ClusterAgentGate v-else-if="cluster" :key="`${clusterID}-${activeTab}`" :cluster-id="clusterID"><component :is="activeComponent" :key="`${clusterID}-${activeTab}`" :cluster-id="clusterID" class="network-content" /></ClusterAgentGate>
  </div>
</template>

<script setup>
import { computed, defineAsyncComponent, ref, watch } from 'vue'
import PageBackLink from '../../components/PageBackLink.vue'
import { getCluster } from '../../api/clusters.js'
import { useAsyncResource } from '../../composables/useAsyncResource.js'
import { useRoutedTab } from '../../composables/useRoutedTab.js'
import EmptyState from '../../components/EmptyState.vue'
import ClusterAgentGate from '../../components/ClusterAgentGate.vue'
import FeedbackBanner from '../../components/FeedbackBanner.vue'
import SectionTabsHeader from '../../components/SectionTabsHeader.vue'
import SurfaceCard from '../../components/SurfaceCard.vue'

const props = defineProps({ clusterID: { type: String, required: true } })
const Sites = defineAsyncComponent(() => import('./Sites.vue'))
const Certificates = defineAsyncComponent(() => import('./Certificates.vue'))

const tabs = [
  { id: 'routes', label: '路由', component: Sites },
  { id: 'certificates', label: '证书', component: Certificates },
]
const { activeTab, selectTab } = useRoutedTab({ tabs, defaultTab: 'routes' })
const activeComponent = computed(() => tabs.find(item => item.id === activeTab.value)?.component || Sites)
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
.cluster-network-page { min-width: 0; }
.network-content { margin-top: var(--tabbed-page-content-gap); }
</style>
