<template>
  <div class="infrastructure-hub">
    <SectionTabsHeader title="集群" :tabs="tabs" :active-tab="activeTab" test-id-prefix="cluster-tab" @select="selectTab" />
    <component :is="activeComponent" :class="['hub-content', { 'hub-content--nodes': activeTab === 'nodes', 'hub-content--configuration': activeTab !== 'nodes' }]" />
  </div>
</template>

<script setup>
import { computed, defineAsyncComponent } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import SectionTabsHeader from '../../components/SectionTabsHeader.vue'

const Cluster = defineAsyncComponent(() => import('./Cluster.vue'))
const NodeRegistryMirrors = defineAsyncComponent(() => import('./NodeRegistryMirrors.vue'))
const ClusterDNS = defineAsyncComponent(() => import('./ClusterDNS.vue'))
const ChartRepositories = defineAsyncComponent(() => import('./ChartRepositories.vue'))
const SystemComponents = defineAsyncComponent(() => import('./SystemComponents.vue'))

const route = useRoute()
const router = useRouter()
const tabs = [
  { id: 'nodes', label: '节点', component: Cluster },
  { id: 'registry-mirrors', label: '节点镜像源', component: NodeRegistryMirrors },
  { id: 'dns', label: '集群 DNS', component: ClusterDNS },
  { id: 'chart-repositories', label: 'Chart 仓库', component: ChartRepositories },
  { id: 'system-components', label: '系统组件', component: SystemComponents },
]
const activeTab = computed(() => tabs.some(item => item.id === route.query.tab) ? route.query.tab : 'nodes')
const activeComponent = computed(() => tabs.find(item => item.id === activeTab.value)?.component || Cluster)

function selectTab(tab) {
  router.push({ path: '/cluster', query: { ...route.query, tab } })
}
</script>

<style scoped>
.hub-content :deep(.page-header) { margin-top: var(--space-20); margin-bottom: var(--space-12); }
.hub-content :deep(.page-title) { font-size: 18px; }
.hub-content--nodes { display: block; margin-top: var(--space-20); }
.hub-content--nodes :deep(.page-header) { display: none; }
.hub-content--configuration :deep(.page-header) { justify-content: flex-end; }
.hub-content--configuration :deep(.page-header > div:first-child) { display: none; }
</style>
