<template>
  <div class="database-monitor">
    <div class="monitor-toolbar"><span class="cell-secondary">{{ item.cloud_instance_id || '阿里云 RDS 原生监控' }}</span><div class="monitor-actions"><SelectMenu v-if="item.cloud_connection_id" v-model="hours" aria-label="监控时间范围" :options="[{ value: 1, label: '最近 1 小时' }, { value: 6, label: '最近 6 小时' }, { value: 24, label: '最近 24 小时' }]" @change="load" /><button v-if="item.cloud_connection_id" class="icon-button" title="刷新监控" aria-label="刷新监控" :disabled="loading" @click="load"><RefreshCw :size="15" /></button><button class="btn btn-sm" @click="openConfig">{{ item.cloud_connection_id ? '监控设置' : '关联云监控' }}</button></div></div>
    <EmptyState v-if="!item.cloud_connection_id" message="关联阿里云账号后查看 CPU、内存、空间和连接数" />
    <EmptyState v-else-if="loading" variant="loading" message="正在读取 RDS 监控..." />
    <EmptyState v-else-if="!series.length" :message="failed ? '监控读取失败，可刷新重试' : '该时间范围内暂无指标'" />
    <div v-else class="monitor-charts"><MetricTrendChart v-for="metric in series" :key="metric.key" :title="metric.label" :unit="metric.unit" :series="[metric]" /></div>
    <BaseModal :open="configOpen" title="关联 RDS 云监控" size="medium" teleport @close="configOpen = false"><div class="monitor-form"><label class="form-group"><span class="form-label">阿里云账号</span><SelectMenu v-model="form.connection_id" aria-label="阿里云账号" :options="connectionOptions" /></label><label class="form-group"><span class="form-label">地域</span><input v-model.trim="form.region" class="form-input" placeholder="cn-guangzhou"></label><label class="form-group"><span class="form-label">RDS 实例 ID</span><input v-model.trim="form.instance_id" class="form-input" placeholder="rm-..."></label><p class="form-hint">复用平台云账号，需要 rds:DescribeDBInstancePerformance 只读权限。</p></div><template #actions><button class="btn" :disabled="saving" @click="configOpen = false">取消</button><button class="btn btn-primary" :disabled="saving || !form.connection_id || !form.region || !form.instance_id" @click="save">{{ saving ? '保存中...' : '保存' }}</button></template></BaseModal>
    <ErrorNoticeModal :open="Boolean(error)" title="云监控操作失败" :message="error" teleport @close="error = ''" />
  </div>
</template>
<script setup>
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { RefreshCw } from 'lucide-vue-next'
import { configureDatabaseMonitor, getDatabaseMetrics } from '../../api/databases.js'
import { getCloudConnections } from '../../api/cloud-resources.js'
import BaseModal from '../../components/BaseModal.vue'
import EmptyState from '../../components/EmptyState.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import MetricTrendChart from '../monitoring/MetricTrendChart.vue'
const props = defineProps({ item: { type: Object, required: true } })
const emit = defineEmits(['updated'])
const hours = ref(1), loading = ref(false), saving = ref(false), configOpen = ref(false), error = ref(''), failed = ref(false), series = ref([]), connections = ref([])
const form = reactive({ connection_id: 0, region: '', instance_id: '' })
const connectionOptions = computed(() => [{ value: 0, label: '选择阿里云账号' }, ...connections.value.filter(item => item.provider === 'aliyun').map(item => ({ value: item.id, label: item.name }))])
let generation = 0
onBeforeUnmount(() => { generation++ })
async function load() {
  const request = ++generation
  series.value = []; failed.value = false
  if (!props.item.cloud_connection_id) { loading.value = false; return }
  loading.value = true
  try { const result = await getDatabaseMetrics(props.item.id, hours.value); if (request === generation) series.value = result.series || [] }
  catch (err) { if (request === generation) { failed.value = true; error.value = err.message || '读取监控失败' } }
  finally { if (request === generation) loading.value = false }
}
async function openConfig() {
  Object.assign(form, { connection_id: props.item.cloud_connection_id || 0, region: props.item.cloud_region || '', instance_id: props.item.cloud_instance_id || '' })
  try { connections.value = await getCloudConnections(); configOpen.value = true } catch (err) { error.value = err.message || '读取云账号失败' }
}
async function save() {
  if (saving.value) return
  saving.value = true
  try { const item = await configureDatabaseMonitor(props.item.id, { ...form }); emit('updated', item); configOpen.value = false }
  catch (err) { error.value = err.message || '保存监控配置失败' }
  finally { saving.value = false }
}
watch(() => [props.item.id, props.item.cloud_connection_id, props.item.cloud_region, props.item.cloud_instance_id], load, { immediate: true })
</script>
<style scoped>
.monitor-toolbar, .monitor-actions { display: flex; align-items: center; justify-content: space-between; gap: 8px; }.monitor-toolbar { margin-bottom: 16px; }.monitor-charts { display: grid; grid-template-columns: repeat(2,minmax(0,1fr)); gap: 12px; }.monitor-charts :deep(.metric-trend-canvas) { height: 180px; }.monitor-form { display: grid; gap: 12px; }.monitor-form .form-group { margin: 0; }
@media (max-width:760px) { .monitor-charts { grid-template-columns: 1fr; }.monitor-toolbar { flex-wrap: wrap; } }
</style>
