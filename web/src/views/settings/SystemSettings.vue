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
import { computed, nextTick, ref, watch } from 'vue'
import SectionTabsHeader from '../../components/SectionTabsHeader.vue'
import SystemSettingsSecurity from './SystemSettingsSecurity.vue'
import SystemSettingsEntry from './SystemSettingsEntry.vue'
import SystemSettingsRelease from './SystemSettingsRelease.vue'
import SystemSettingsCloudProviders from './SystemSettingsCloudProviders.vue'
import { useRoutedTab } from '../../composables/useRoutedTab.js'

const activeView = ref(null)
const tabs = [
  { id: 'security', label: '安全与访问', component: SystemSettingsSecurity },
  { id: 'entry', label: '平台入口', component: SystemSettingsEntry },
  { id: 'release', label: '发布与更新', component: SystemSettingsRelease },
  { id: 'cloud', label: '云提供商', component: SystemSettingsCloudProviders },
]
const { activeTab, selectTab } = useRoutedTab({ tabs, defaultTab: 'security', path: '/settings/system' })
const activeComponent = computed(() => tabs.find(tab => tab.id === activeTab.value)?.component || SystemSettingsSecurity)
watch(activeTab, async () => {
  await nextTick()
  activeView.value?.refresh?.()
})

</script>

<style scoped src="./SystemSettings.shared.css"></style>
