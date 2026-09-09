<template>
  <div class="settings-page">
    <SectionTabsHeader title="系统设置" :tabs="tabs" :active-tab="activeTab" test-id-prefix="system-settings-tab" @select="selectTab" />

    <main class="settings-content">
      <KeepAlive>
        <component :is="activeComponent" ref="activeView" />
      </KeepAlive>
    </main>
  </div>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import SectionTabsHeader from '../components/SectionTabsHeader.vue'
import SystemSettingsSecurity from './SystemSettingsSecurity.vue'
import SystemSettingsEntry from './SystemSettingsEntry.vue'
import SystemSettingsRelease from './SystemSettingsRelease.vue'
import { usePolling } from '../composables/usePolling.js'

const route = useRoute()
const router = useRouter()
const activeView = ref(null)
const tabs = [
  { id: 'security', label: '安全与访问', component: SystemSettingsSecurity },
  { id: 'entry', label: '平台入口', component: SystemSettingsEntry },
  { id: 'release', label: '发布与更新', component: SystemSettingsRelease },
]
const activeTab = computed(() => tabs.some(tab => tab.id === route.query.tab) ? route.query.tab : 'security')
const activeComponent = computed(() => tabs.find(tab => tab.id === activeTab.value)?.component || SystemSettingsSecurity)
const refreshPolling = usePolling(() => activeView.value?.refresh?.(), { interval: 15000 })

onMounted(() => {
  refreshPolling.start()
})

onBeforeUnmount(refreshPolling.stop)

watch(activeTab, async () => {
  await nextTick()
  activeView.value?.refresh?.()
})

function selectTab(tab) {
  router.push({ path: '/settings/system', query: { ...route.query, tab } })
}
</script>

<style scoped src="./SystemSettings.shared.css"></style>
