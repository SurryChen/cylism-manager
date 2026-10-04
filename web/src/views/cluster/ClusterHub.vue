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

const ClusterList = defineAsyncComponent(() => import('./ClusterList.vue'))
const AccessCertificates = defineAsyncComponent(() => import('./AccessCertificates.vue'))

const tabs = [
  { id: 'list', label: '列表', component: ClusterList },
  { id: 'access-certificates', label: '访问证书', component: AccessCertificates },
]
const { activeTab, selectTab } = useRoutedTab({ tabs, defaultTab: 'list', path: '/cluster' })
const activeComponent = computed(() => tabs.find(item => item.id === activeTab.value)?.component || ClusterList)
</script>

<style scoped>
.hub-content { display: block; margin-top: var(--tabbed-page-content-gap); }
</style>
