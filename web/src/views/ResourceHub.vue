<template>
  <div class="infrastructure-hub">
    <header class="hub-header">
      <h1 class="page-title">Kubernetes 资源</h1>
      <nav class="hub-tabs" aria-label="Kubernetes 资源">
      <button v-for="item in tabs" :key="item.id" :data-testid="`resource-tab-${item.id}`" class="hub-tab" :class="{ 'is-active': activeTab === item.id }" :aria-current="activeTab === item.id ? 'page' : undefined" @click="selectTab(item.id)">
        {{ item.label }}
      </button>
      </nav>
    </header>
    <component :is="activeComponent" class="hub-content" />
  </div>
</template>

<script setup>
import { computed, defineAsyncComponent } from 'vue'
import { useRoute, useRouter } from 'vue-router'

const Workloads = defineAsyncComponent(() => import('./Workloads.vue'))
const Services = defineAsyncComponent(() => import('./Services.vue'))
const Configs = defineAsyncComponent(() => import('./Configs.vue'))

const route = useRoute()
const router = useRouter()
const tabs = [
  { id: 'workloads', label: '工作负载', component: Workloads },
  { id: 'services', label: '服务', component: Services },
  { id: 'configs', label: '配置', component: Configs },
]
const activeTab = computed(() => tabs.some(item => item.id === route.query.tab) ? route.query.tab : 'workloads')
const activeComponent = computed(() => tabs.find(item => item.id === activeTab.value)?.component || Workloads)

function selectTab(tab) {
  router.push({ path: '/resources', query: { ...route.query, tab } })
}
</script>

<style scoped>
.hub-header { display: flex; align-items: flex-end; gap: 34px; height: 52px; min-height: 0; border-bottom: 1px solid var(--border-muted); }
.hub-header .page-title { margin: 0 0 11px; font-size: 24px; }
.hub-tabs { display: flex; align-items: flex-end; gap: 26px; }
.hub-tab { position: relative; padding: 0 0 12px; border: 0; color: var(--text-secondary); background: transparent; font: inherit; font-size: 14px; cursor: pointer; }
.hub-tab:hover, .hub-tab.is-active { color: var(--text-primary); }.hub-tab.is-active { font-weight: 650; }
.hub-tab.is-active::after { content: ''; position: absolute; right: 0; bottom: -1px; left: 0; height: 2px; background: var(--action-primary); }
.hub-content :deep(.page-header) { margin-top: var(--space-20); margin-bottom: var(--space-12); }.hub-content :deep(.page-title) { font-size: 18px; }
@media (max-width: 640px) { .hub-header { align-items: flex-start; flex-direction: column; gap: 0; }.hub-header .page-title { margin-bottom: var(--space-12); }.hub-tabs { width: 100%; gap: 22px; } }
</style>
