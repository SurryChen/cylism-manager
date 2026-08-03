<template>
  <div class="infrastructure-hub">
    <SectionTabsHeader title="集群" :tabs="tabs" :active-tab="activeTab" test-id-prefix="cluster-tab" @select="selectTab" />
    <component :is="activeComponent" :class="['hub-content', { 'hub-content--nodes': activeTab === 'nodes', 'hub-content--configuration': activeTab !== 'nodes' }]" />
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Cluster from './Cluster.vue'
import NodeRegistryMirrors from './NodeRegistryMirrors.vue'
import ChartRepositories from './ChartRepositories.vue'
import SectionTabsHeader from '../components/SectionTabsHeader.vue'

const route = useRoute()
const router = useRouter()
const tabs = [
  { id: 'nodes', label: '节点', component: Cluster },
  { id: 'registry-mirrors', label: '节点镜像源', component: NodeRegistryMirrors },
  { id: 'chart-repositories', label: 'Chart 仓库', component: ChartRepositories },
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
