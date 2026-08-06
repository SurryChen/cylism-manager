<template>
  <section class="disk-growth-workspace">
    <header class="disk-growth-heading">
      <div><h2>磁盘增长诊断</h2><p>按增长量定位节点目录、PVC 与容器可写层，帮助确认空间增长来源。</p></div>
      <div class="disk-growth-controls">
        <label>时间范围<select v-model="range" class="form-select"><option v-for="item in ranges" :key="item.value" :value="item.value">{{ item.label }}</option></select></label>
        <label>节点<select v-model="node" class="form-select"><option value="">全部节点</option><option v-for="item in readyNodes" :key="item.name" :value="item.name">{{ nodeLabel(item) }}</option></select></label>
        <button class="icon-button" type="button" title="刷新磁盘诊断" aria-label="刷新磁盘诊断" :disabled="loading" @click="load"><RefreshCw :size="16" :class="{ 'is-spinning': loading }" /></button>
      </div>
    </header>

    <div v-if="error" class="k8s-banner k8s-banner-warn section-gap">{{ error }}</div>
    <section class="disk-growth-grid section-gap" :aria-busy="loading">
      <article class="card"><div class="card-header"><div><h2 class="card-title">节点挂载点</h2><p>可用空间减少最多的挂载点</p></div><span class="badge badge-offline">{{ rows.mounts.length }} 项</span></div><GrowthTable :rows="rows.mounts" empty="所选时间内没有可识别的节点磁盘增长" /></article>
      <article class="card"><div class="card-header"><div><h2 class="card-title">存储卷</h2><p>PVC 增长量及当前挂载 Pod</p></div><span class="badge badge-offline">{{ rows.pvcs.length }} 项</span></div><GrowthTable :rows="rows.pvcs" kind="pvc" empty="所选时间内没有 PVC 使用量增长" /></article>
      <article class="card disk-growth-containers"><div class="card-header"><div><h2 class="card-title">容器可写层</h2><p>不包含已挂载 PVC 的数据目录</p></div><span class="badge badge-offline">{{ rows.containers.length }} 项</span></div><GrowthTable :rows="rows.containers" kind="container" empty="所选时间内没有容器可写层增长" /></article>
    </section>
  </section>
</template>

<script setup>
import { computed, defineComponent, h, onMounted, ref, watch } from 'vue'
import { RefreshCw } from 'lucide-vue-next'
import { api } from '../api/index.js'

const props = defineProps({ nodes: { type: Array, default: () => [] } })

const ranges = [
  { value: '1h', label: '最近 1 小时' },
  { value: '6h', label: '最近 6 小时' },
  { value: '24h', label: '最近 24 小时' },
  { value: '7d', label: '最近 7 天' },
]
const range = ref('6h')
const node = ref('')
const loading = ref(false)
const error = ref('')
const rows = ref({ mounts: [], pvcs: [], containers: [] })
const readyNodes = computed(() => props.nodes.filter(item => item.ready))

const GrowthTable = defineComponent({
  props: { rows: { type: Array, required: true }, kind: String, empty: { type: String, required: true } },
  setup(tableProps) {
    return () => tableProps.rows.length ? h('div', { class: 'table-wrap' }, [h('table', { class: 'data-table disk-growth-table' }, [
      h('thead', [h('tr', tableProps.kind === 'pvc'
        ? [h('th', '命名空间'), h('th', 'PVC / 当前使用者'), h('th', '节点'), h('th', '增加量')]
        : tableProps.kind === 'container'
          ? [h('th', '命名空间'), h('th', 'Pod / 容器'), h('th', '节点'), h('th', '增加量')]
          : [h('th', '节点'), h('th', '挂载点'), h('th', '增加量')])]),
      h('tbody', tableProps.rows.map(row => h('tr', { key: `${tableProps.kind || 'mount'}/${row.node || ''}/${row.namespace || ''}/${row.pvc || row.pod || row.mount_point || ''}/${row.container || ''}/${row.growth_bytes || 0}` }, tableProps.kind === 'pvc'
        ? [h('td', row.namespace || '-'), h('td', [h('strong', { class: 'cell-primary' }, row.pvc || '-'), row.consumers?.length ? h('small', { class: 'disk-growth-meta' }, `使用者: ${row.consumers.join('、')}`) : h('small', { class: 'disk-growth-meta' }, '当前未挂载')]), h('td', row.node || '-'), h('td', formatBytes(row.growth_bytes))]
        : tableProps.kind === 'container'
          ? [h('td', row.namespace || '-'), h('td', [h('strong', { class: 'cell-primary' }, row.pod || '-'), h('small', { class: 'disk-growth-meta' }, row.container || '-')]), h('td', row.node || '-'), h('td', formatBytes(row.growth_bytes))]
          : [h('td', row.node || '-'), h('td', { class: 'cell-primary' }, row.mount_point || '-'), h('td', formatBytes(row.growth_bytes))]
      )))])]) : h('div', { class: 'empty-inline' }, tableProps.empty)
  },
})

onMounted(load)
watch([range, node], load)

function nodeLabel(item) { return item.display_name || item.name }
function formatBytes(value) {
  const bytes = Number(value) || 0
  if (bytes < 1024) return `${bytes.toFixed(0)} B`
  const units = ['KiB', 'MiB', 'GiB', 'TiB']
  let amount = bytes
  let index = -1
  do { amount /= 1024; index += 1 } while (amount >= 1024 && index < units.length - 1)
  return `${amount >= 10 ? amount.toFixed(0) : amount.toFixed(1)} ${units[index]}`
}
async function load() {
  loading.value = true
  error.value = ''
  try {
    const params = new URLSearchParams({ range: range.value })
    if (node.value) params.set('node', node.value)
    const result = await api.get(`/monitoring/disk-growth?${params.toString()}`)
    rows.value = { mounts: result?.mounts || [], pvcs: result?.pvcs || [], containers: result?.containers || [] }
  } catch (cause) { error.value = cause.message || '读取磁盘增长诊断失败' } finally { loading.value = false }
}
</script>

<style scoped>
.disk-growth-heading{display:flex;align-items:center;justify-content:space-between;gap:var(--space-16)}.disk-growth-heading h2{margin:0;color:var(--text-primary);font-size:16px}.disk-growth-heading p,.card-header p{margin:5px 0 0;color:var(--text-secondary);font-size:12px}.disk-growth-controls{display:flex;align-items:end;flex-wrap:wrap;gap:8px}.disk-growth-controls label{display:grid;gap:4px;color:var(--text-secondary);font-size:11px;font-weight:700}.disk-growth-controls select{min-width:130px;min-height:34px;padding:6px 28px 6px 9px;font-size:11px}.disk-growth-grid{display:grid;margin-top:var(--space-20);grid-template-columns:repeat(2,minmax(0,1fr));gap:var(--space-16)}.disk-growth-containers{grid-column:1/-1}.disk-growth-table td{vertical-align:top}.disk-growth-meta{display:block;margin-top:3px;color:var(--text-secondary);font-size:11px;line-height:1.35}@media(max-width:760px){.disk-growth-heading{align-items:flex-start;flex-direction:column}.disk-growth-controls{width:100%}.disk-growth-controls label{flex:1}.disk-growth-controls select{width:100%;min-width:0}.disk-growth-grid{grid-template-columns:1fr}}
</style>
