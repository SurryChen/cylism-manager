<template>
  <div class="infrastructure-hub">
    <header class="hub-header">
      <h1 class="page-title">网络访问</h1>
      <nav class="hub-tabs" aria-label="网络访问">
      <button v-for="item in tabs" :key="item.id" :data-testid="`network-tab-${item.id}`" class="hub-tab" :class="{ 'is-active': activeTab === item.id }" :aria-current="activeTab === item.id ? 'page' : undefined" @click="selectTab(item.id)">
        {{ item.label }}
      </button>
      </nav>
    </header>
    <component :is="activeComponent" class="hub-content" />
  </div>
</template>

<script setup>
import { computed, defineAsyncComponent } from 'vue'
import { useRoutedTab } from '../../composables/useRoutedTab.js'

const Sites = defineAsyncComponent(() => import('./Sites.vue'))
const Certificates = defineAsyncComponent(() => import('./Certificates.vue'))

const tabs = [
  { id: 'routes', label: '路由', component: Sites },
  { id: 'certificates', label: '证书', component: Certificates },
]
const { activeTab, selectTab } = useRoutedTab({ tabs, defaultTab: 'routes', path: '/network' })
const activeComponent = computed(() => tabs.find(item => item.id === activeTab.value)?.component || Sites)
</script>

<style scoped>
.hub-header { display: flex; align-items: flex-end; gap: 34px; height: 52px; min-height: 0; border-bottom: 1px solid var(--border-muted); }
.hub-header .page-title { margin: 0 0 11px; font-size: 24px; }.hub-tabs { display: flex; align-items: flex-end; gap: 26px; }
.hub-tab { position: relative; padding: 0 0 12px; border: 0; color: var(--text-secondary); background: transparent; font: inherit; font-size: 14px; cursor: pointer; }
.hub-tab:hover, .hub-tab.is-active { color: var(--text-primary); }.hub-tab.is-active { font-weight: 650; }
.hub-tab.is-active::after { content: ''; position: absolute; right: 0; bottom: -1px; left: 0; height: 2px; background: var(--action-primary); }
.hub-content :deep(.page-header) { margin-top: var(--space-20); margin-bottom: var(--space-12); }.hub-content :deep(.page-title) { font-size: 18px; }
@media (max-width: 640px) { .hub-header { align-items: flex-start; flex-direction: column; gap: 0; }.hub-header .page-title { margin-bottom: var(--space-12); }.hub-tabs { width: 100%; gap: 22px; } }
</style>
