<template>
  <section class="instance-monitor">
    <div class="usage-toolbar"><span class="cell-secondary">{{ usage ? `读取于 ${new Date(usage.checked_at).toLocaleTimeString('zh-CN', { hour12: false })} · 每 15 秒刷新` : '实时资源使用量' }}</span><button class="icon-button" title="刷新资源使用量" aria-label="刷新资源使用量" :disabled="loading" @click="load"><RefreshCw :size="15" /></button></div>
    <EmptyState v-if="loading && !usage" variant="loading" message="正在读取资源使用量..." />
    <EmptyState v-else-if="!usage" message="资源使用量读取失败" description="请刷新重试" />
    <template v-else>
      <div v-if="usage.metrics_available" class="usage-stats">
        <div class="usage-stat"><span class="cell-secondary">CPU</span><strong>{{ (usage.cpu_millicores / 1000).toFixed(2) }} <small>核</small></strong><span class="cell-secondary">上限 {{ (usage.cpu_limit_millicores / 1000).toFixed(2) }} 核 · {{ percent(usage.cpu_millicores, usage.cpu_limit_millicores) }}</span><div class="usage-track"><span :style="{ width: barWidth(usage.cpu_millicores, usage.cpu_limit_millicores) }" /></div></div>
        <div class="usage-stat"><span class="cell-secondary">内存</span><strong>{{ (usage.memory_bytes / 1048576).toFixed(0) }} <small>MiB</small></strong><span class="cell-secondary">上限 {{ (usage.memory_limit_bytes / 1048576).toFixed(0) }} MiB · {{ percent(usage.memory_bytes, usage.memory_limit_bytes) }}</span><div class="usage-track"><span :style="{ width: barWidth(usage.memory_bytes, usage.memory_limit_bytes) }" /></div></div>
      </div>
      <EmptyState v-else message="暂时没有资源指标" :description="usage.metrics_message" />
      <dl class="usage-facts"><div><dt>Pod 状态</dt><dd><span class="badge" :class="usage.ready ? 'badge-online' : 'badge-offline'">{{ usage.ready ? '就绪' : usage.phase }}</span></dd></div><div><dt>所在节点</dt><dd>{{ usage.node_name || '待调度' }}</dd></div><div><dt>容器重启</dt><dd>{{ usage.restarts }} 次</dd></div><div><dt>最近退出原因</dt><dd>{{ usage.last_termination || '无记录' }}</dd></div></dl>
      <p class="form-hint">使用量来自 MySQL 容器的实时采样，上限以当前 Pod 为准。</p>
    </template>
    <ErrorNoticeModal :open="Boolean(error)" :message="error" teleport @close="error = ''" />
  </section>
</template>
<script setup>
import { onBeforeUnmount, ref, watch } from 'vue'
import { RefreshCw } from 'lucide-vue-next'
import { getDatabaseMetrics } from '../../api/databases.js'
import { usePolling } from '../../composables/usePolling.js'
import EmptyState from '../../components/EmptyState.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
const props = defineProps({ instanceId: { type: Number, required: true } })
const usage = ref(null), loading = ref(false), error = ref('')
let generation = 0
const percent = (value, limit) => limit > 0 ? `${(value / limit * 100).toFixed(1)}%` : '未设置上限'
const barWidth = (value, limit) => `${limit > 0 ? Math.min(100, value / limit * 100) : 0}%`
async function load() {
  if (loading.value) return
  const current = generation; loading.value = true
  try { const result = await getDatabaseMetrics(props.instanceId); if (current === generation) usage.value = result }
  catch (cause) { if (current === generation) { usage.value = null; error.value = cause.message || '资源使用量读取失败' } }
  finally { if (current === generation) loading.value = false }
}
const polling = usePolling(load, { interval: 15000 })
watch(() => props.instanceId, () => { generation++; usage.value = null; loading.value = false; error.value = ''; polling.stop(); load(); polling.start() }, { immediate: true })
onBeforeUnmount(() => { generation++ })
</script>
<style scoped>
.usage-toolbar { display: flex; justify-content: space-between; align-items: center; gap: 12px; margin-bottom: 20px; }.usage-stats { display: grid; grid-template-columns: 1fr 1fr; gap: 24px; padding-bottom: 24px; border-bottom: 1px solid var(--border-muted); }.usage-stat { display: grid; gap: 10px; }.usage-stat strong { font-size: 28px; font-weight: 600; font-variant-numeric: tabular-nums; }.usage-stat small { font-size: 12px; color: var(--text-secondary); font-weight: 400; }.usage-track { height: 5px; border-radius: 3px; background: var(--border-muted); overflow: hidden; }.usage-track span { display: block; height: 100%; background: var(--action-primary); transition: width .3s; }.usage-facts { display: grid; grid-template-columns: repeat(2,minmax(0,1fr)); gap: 20px; margin: 24px 0 16px; }.usage-facts dt { font-size: 12px; color: var(--text-secondary); margin-bottom: 8px; }.usage-facts dd { margin: 0; font-size: 14px; overflow-wrap: anywhere; }
@media (max-width:600px) { .usage-stats { grid-template-columns: 1fr; gap: 24px; } }
</style>
