<template>
  <article class="card metric-trend-chart">
    <div class="card-header">
      <div>
        <h2 class="card-title">{{ title }}</h2>
        <p v-if="subtitle" class="metric-trend-subtitle">{{ subtitle }}</p>
      </div>
      <div class="metric-trend-header-actions">
        <slot name="actions" />
        <span v-if="Number.isFinite(threshold)" class="metric-trend-threshold">阈值 {{ threshold }}{{ unit }}</span>
      </div>
    </div>
    <div v-if="$slots.toolbar" class="metric-trend-toolbar"><slot name="toolbar" /></div>
    <div v-if="loading" class="metric-trend-empty">正在读取历史指标...</div>
    <div v-else-if="!hasData" class="metric-trend-empty">该时间范围内暂无指标</div>
    <div v-else class="metric-trend-canvas"><Line :data="chartData" :options="chartOptions" /></div>
  </article>
</template>

<script setup>
import { computed } from 'vue'
import { Line } from 'vue-chartjs'
import { Chart as ChartJS, CategoryScale, Filler, Legend, LineElement, LinearScale, PointElement, Tooltip } from 'chart.js'

ChartJS.register(CategoryScale, Filler, Legend, LineElement, LinearScale, PointElement, Tooltip)

const props = defineProps({
  title: { type: String, required: true },
  subtitle: { type: String, default: '' },
  unit: { type: String, default: '' },
  threshold: { type: Number, default: Number.NaN },
  loading: { type: Boolean, default: false },
  series: { type: Array, default: () => [] },
})

const colors = ['#22736b', '#b86412', '#285f9a', '#9d4d22', '#5d6e3a', '#876235']
const pointMap = computed(() => new Map(props.series.flatMap(item => item.values || []).map(point => [Number(point.timestamp), true])))
const timestamps = computed(() => [...pointMap.value.keys()].sort((left, right) => left - right))
const hasData = computed(() => timestamps.value.length > 0)
const labels = computed(() => timestamps.value.map(formatTimestamp))
const chartData = computed(() => {
  const dataSets = props.series.map((item, index) => {
    const values = new Map((item.values || []).map(point => [Number(point.timestamp), Number(point.value)]))
    const color = item.color || colors[index % colors.length]
    return {
      label: item.label,
      data: timestamps.value.map(timestamp => values.get(timestamp) ?? null),
      borderColor: color,
      backgroundColor: color,
      borderWidth: 2,
      pointRadius: 0,
      pointHitRadius: 7,
      tension: 0.28,
      spanGaps: true,
    }
  })
  if (Number.isFinite(props.threshold)) {
    dataSets.push({
      label: `阈值 ${props.threshold}${props.unit}`,
      data: timestamps.value.map(() => props.threshold),
      borderColor: '#d84a3e',
      borderWidth: 1,
      borderDash: [5, 5],
      pointRadius: 0,
      fill: false,
    })
  }
  return { labels: labels.value, datasets: dataSets }
})
const chartOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  interaction: { intersect: false, mode: 'index' },
  plugins: {
    legend: { display: props.series.length > 1, position: 'bottom', labels: { boxWidth: 8, boxHeight: 8, padding: 12, color: '#596a75', font: { size: 11 } } },
    tooltip: { callbacks: { label: context => `${context.dataset.label}: ${formatValue(context.raw)}${props.unit}` } },
  },
  scales: {
    x: { grid: { display: false }, ticks: { color: '#77858d', font: { size: 10 }, maxTicksLimit: 6 } },
    y: { beginAtZero: true, grid: { display: false }, ticks: { color: '#77858d', font: { size: 10 }, callback: value => `${value}${props.unit}` } },
  },
}))

function formatTimestamp(timestamp) {
  const date = new Date(timestamp * 1000)
  return date.toLocaleString('zh-CN', { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' })
}

function formatValue(value) {
  const number = Number(value)
  if (!Number.isFinite(number)) return '-'
  return number >= 100 ? number.toFixed(0) : number.toFixed(1)
}
</script>

<style scoped>
.metric-trend-chart { min-width: 0; }
.metric-trend-subtitle { margin: 4px 0 0; color: var(--text-secondary); font-size: 11px; }
.metric-trend-header-actions { display: flex; align-items: center; gap: 10px; }
.metric-trend-threshold { color: var(--danger); font: 10px/1 var(--font-mono); }
.metric-trend-toolbar { margin: -2px 0 12px; }
.metric-trend-canvas { height: 220px; }
.metric-trend-empty { display: flex; height: 220px; align-items: center; justify-content: center; color: var(--text-muted); font-size: 12px; }
</style>
