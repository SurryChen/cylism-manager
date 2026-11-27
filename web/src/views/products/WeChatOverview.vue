<template>
  <div v-if="connections.length" class="wechat-overview">
    <MetricTrendChart title="微信运营数据" date-only :span-gaps="false" :series="series" :unit="metric.unit" :loading="analytics.loading.value && !analytics.data.value">
      <template #actions>
        <button type="button" class="icon-button" title="刷新微信运营数据" aria-label="刷新微信运营数据" :disabled="analytics.loading.value" @click="load(true)"><RefreshCw :size="15" /></button>
      </template>
      <template #toolbar>
        <div class="wechat-overview-toolbar">
          <SelectMenu v-if="connections.length > 1" v-model="channelID" aria-label="运营数据渠道">
            <option v-for="item in connections" :key="item.channel_id" :value="item.channel_id">{{ channelLabel(item.channel_id) }}</option>
          </SelectMenu>
          <SelectMenu v-model="metricID" aria-label="微信运营指标">
            <option v-for="item in metrics" :key="item.id" :value="item.id">{{ item.label }}</option>
          </SelectMenu>
          <SelectMenu v-model="days" aria-label="微信数据时间范围">
            <option :value="7">近 7 天</option>
            <option :value="30">近 30 天</option>
          </SelectMenu>
          <span v-if="dataThrough" class="cell-secondary">数据截至 {{ dataThrough }}</span>
          <span v-if="analytics.data.value?.synced_at" class="cell-secondary">最近查询 {{ formatSyncTime(analytics.data.value.synced_at) }}</span>
          <span v-if="metricError" class="cell-secondary" role="status">{{ metricError }}</span>
        </div>
      </template>
    </MetricTrendChart>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { RefreshCw } from 'lucide-vue-next'
import SelectMenu from '../../components/SelectMenu.vue'
import MetricTrendChart from '../monitoring/MetricTrendChart.vue'
import { getWeChatAnalytics, getWeChatSettings, weChatChannels } from '../../api/wechat.js'
import { useAsyncResource } from '../../composables/useAsyncResource.js'

const props = defineProps({ product: { type: Object, required: true } })
const emit = defineEmits(['error'])
const channels = computed(() => weChatChannels(props.product))
const settings = useAsyncResource(({ signal }, id) => getWeChatSettings(id, { signal }))
const analytics = useAsyncResource(({ signal }, id, channel, range, refresh) => getWeChatAnalytics(id, channel, range, refresh, { signal }))
const connections = computed(() => (settings.data.value?.connections || []).filter(item => item.analytics_connected && channels.value.some(channel => channel.id === item.channel_id)))
const channelID = ref(0)
const days = ref(7)
const metricID = ref('active_users')
const currentChannel = computed(() => channels.value.find(item => item.id === channelID.value))
const metrics = computed(() => [
  { id: 'active_users', label: '活跃用户', unit: ' 人' },
  { id: 'new_users', label: '新增用户', unit: ' 人' },
  { id: 'visits', label: '访问次数', unit: ' 次' },
  ...(currentChannel.value?.platform === 'mini_game' ? [
    { id: 'next_day_retention', label: '新增用户次日留存', unit: '%' },
    { id: 'shares', label: '分享次数', unit: ' 次' },
    { id: 'ad_revenue', label: '广告分成后收入', unit: ' 元' },
  ] : []),
])
const metric = computed(() => metrics.value.find(item => item.id === metricID.value) || metrics.value[0])
const samples = computed(() => (analytics.data.value?.samples || []).filter(item => item.metric === metric.value.id))
const dataThrough = computed(() => samples.value.findLast(item => item.value != null)?.day)
const metricError = computed(() => analytics.data.value?.metric_errors?.[metric.value.id])
const dayTimestamp = day => Date.parse(`${day}T00:00:00+08:00`) / 1000
const points = computed(() => {
  const data = analytics.data.value
  if (!data) return []
  const values = new Map(samples.value.map(item => [dayTimestamp(item.day), item.value]))
  const points = []
  for (let timestamp = dayTimestamp(data.from); timestamp <= dayTimestamp(data.through); timestamp += 86400) {
    points.push({ timestamp, value: values.get(timestamp) ?? null })
  }
  return points
})
const series = computed(() => [{
  label: metric.value.label,
  values: points.value,
}])

const channelLabel = id => channels.value.find(channel => channel.id === id)?.name || '微信'
const formatSyncTime = value => new Intl.DateTimeFormat('zh-CN', { timeZone: 'Asia/Shanghai', dateStyle: 'short', timeStyle: 'short' }).format(new Date(value))

watch(() => props.product, async product => {
  if (!weChatChannels(product).length) {
    settings.cancel()
    settings.data.value = null
    return
  }
  await settings.refresh(product.id)
  if (settings.error.value) emit('error', settings.error.value.message)
}, { immediate: true })

watch(connections, rows => {
  if (!rows.some(item => item.channel_id === channelID.value)) channelID.value = rows[0]?.channel_id || 0
})
watch([channelID, days], () => {
  analytics.cancel()
  analytics.data.value = null
  if (metric.value.id !== metricID.value) metricID.value = metric.value.id
  if (channelID.value) load(false)
})
async function load(refresh) {
  const result = await analytics.refresh(props.product.id, channelID.value, days.value, refresh)
  const failure = analytics.error.value?.message || result?.error
  if (failure) emit('error', failure)
}
</script>

<style scoped>
.wechat-overview { margin-top: var(--space-16); }
.wechat-overview-toolbar { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-8); }
</style>
