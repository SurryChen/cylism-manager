<template>
  <div class="infrastructure-hub">
    <SectionTabsHeader title="集群" :tabs="tabs" :active-tab="activeTab" test-id-prefix="cluster-tab" @select="selectTab" />
    <component :is="activeComponent" class="hub-content" />
  </div>
</template>

<script setup>
import { computed, defineAsyncComponent } from 'vue'
import SectionTabsHeader from '../../components/SectionTabsHeader.vue'
import { useRoutedTab } from '../../composables/useRoutedTab.js'

const Cluster = defineAsyncComponent(() => import('./Cluster.vue'))
const NodeRegistryMirrors = defineAsyncComponent(() => import('./NodeRegistryMirrors.vue'))
const ClusterDNS = defineAsyncComponent(() => import('./ClusterDNS.vue'))
const ChartRepositories = defineAsyncComponent(() => import('./ChartRepositories.vue'))
const SystemComponents = defineAsyncComponent(() => import('./SystemComponents.vue'))

const tabs = [
  { id: 'nodes', label: '节点', component: Cluster },
  { id: 'registry-mirrors', label: '节点镜像源', component: NodeRegistryMirrors },
  { id: 'dns', label: '集群 DNS', component: ClusterDNS },
  { id: 'chart-repositories', label: 'Chart 仓库', component: ChartRepositories },
  { id: 'system-components', label: '系统组件', component: SystemComponents },
]
const { activeTab, selectTab } = useRoutedTab({ tabs, defaultTab: 'nodes', path: '/cluster' })
const activeComponent = computed(() => tabs.find(item => item.id === activeTab.value)?.component || Cluster)
</script>

<style scoped>
.hub-content { display: block; margin-top: var(--tabbed-page-content-gap); }
</style>
