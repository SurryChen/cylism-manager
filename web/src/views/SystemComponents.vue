<template>
  <div>
    <div class="page-header">
      <div>
        <h1 class="page-title">系统组件</h1>
        <p class="page-subtitle">依据集群实际控制源管理 K3s 内置组件，避免 Helm 与静态清单相互覆盖</p>
      </div>
      <button class="btn btn-primary" :disabled="loading" @click="load">刷新</button>
    </div>

    <div v-if="error" class="k8s-banner k8s-banner-warn section-gap">{{ error }}</div>

    <div v-if="loaded" class="card section-gap">
      <div class="table-wrap">
        <table class="data-table system-component-table">
          <thead>
            <tr><th>组件</th><th>控制方式</th><th>运行状态</th><th>配置</th><th>操作</th></tr>
          </thead>
          <tbody>
            <tr v-for="item in items" :key="item.chart_name">
              <td>
                <strong class="cell-primary">{{ item.chart_name }}</strong>
                <small class="cell-secondary">{{ item.namespace }}</small>
              </td>
              <td>
                <span class="mode-label" :class="`mode-${item.controller_mode || 'unknown'}`">{{ controllerModeText(item.controller_mode) }}</span>
                <small v-if="isEmbedded(item)" class="cell-secondary">由 K3s 内置控制器提供</small>
              </td>
              <td>
                <template v-if="item.deployment">
                  <span class="badge" :class="deploymentReady(item) ? 'badge-online' : 'badge-offline'">
                    {{ item.deployment.ready_replicas }}/{{ item.deployment.replicas }} 就绪
                  </span>
                  <small v-if="item.deployment.fixed_node" class="cell-secondary">固定节点：{{ item.deployment.fixed_node }}</small>
                  <small v-else-if="isStatic(item)" class="cell-secondary">自动调度</small>
                </template>
                <template v-else-if="item.workload">
                  <span class="badge" :class="workloadReady(item) ? 'badge-online' : 'badge-offline'">
                    {{ item.workload.ready }}/{{ item.workload.desired }} 就绪
                  </span>
                  <small class="cell-secondary">{{ item.workload.kind }}：{{ item.workload.name }}</small>
                </template>
                <template v-else-if="item.controller_mode === 'helm_chart'">
                  <span class="badge" :class="item.chart_failed ? 'badge-danger' : item.chart_ready ? 'badge-online' : 'badge-warn'">
                    {{ item.chart_failed ? '安装失败' : item.chart_ready ? '已安装' : '安装中' }}
                  </span>
                  <small class="cell-secondary">由 Helm 控制器管理</small>
                </template>
                <template v-else>
                  <span class="badge" :class="isEmbedded(item) ? 'badge-online' : isMissing(item) ? 'badge-offline' : 'badge-danger'">
                    {{ isEmbedded(item) ? '运行中' : isMissing(item) ? '未安装' : '待确认' }}
                  </span>
                </template>
              </td>
              <td>
                <span class="badge" :class="configBadgeClass(item)">{{ configBadgeText(item) }}</span>
                <small v-if="configNeedsAttention(item)" class="detail">{{ configIssueText(item) }}</small>
                <small v-if="item.last_applied_at" class="cell-secondary">应用于 {{ formatTime(item.last_applied_at) }}</small>
              </td>
              <td>
                <div class="btn-group">
                  <button v-if="canConfigure(item)" class="btn btn-sm" @click="edit(item)">配置</button>
                  <button v-if="item.has_config && canRestore(item)" class="btn btn-sm btn-danger" @click="revert(item)">恢复默认</button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <div v-if="modal" class="overlay" @click.self="close">
      <div class="modal">
        <h2 class="modal-title">配置 {{ editing?.chart_name }}</h2>
        <p class="modal-copy">{{ modalDescription(editing) }}</p>
        <form @submit.prevent="save">
          <div class="config-section-title">运行容量</div>
          <div class="form-row">
            <div class="form-group">
              <label class="form-label">副本数</label>
              <input v-model.number="form.replicas" type="number" min="1" class="form-input" placeholder="默认 1" />
            </div>
            <div class="form-group">
              <label class="form-label">更新时最大不可用</label>
              <select v-model="form.maxUnavailable" class="form-select">
                <option value="0">0（保持服务）</option>
                <option value="1">1（允许短暂减少）</option>
              </select>
            </div>
          </div>
          <div class="form-group">
            <label class="form-label">更新时最大额外副本</label>
            <select v-model="form.maxSurge" class="form-select">
              <option value="1">1（先启动新副本）</option>
              <option value="0">0（不额外扩容）</option>
              <option value="25%">25%（Kubernetes 默认）</option>
            </select>
          </div>
          <div v-if="canPlace(editing)" class="form-group">
            <div class="config-section-title">节点调度</div>
            <label class="form-label">部署节点</label>
            <select v-model="form.nodeName" class="form-select" data-testid="coredns-node-selector">
              <option value="">不固定，由 Kubernetes 调度</option>
              <option v-for="node in schedulableNodes" :key="node.name" :value="node.name">{{ node.display_name || node.name }}</option>
            </select>
            <span class="form-hint">固定后所有副本都会调度到该节点，节点故障时可能影响服务。</span>
          </div>
          <p v-if="isStatic(editing)" class="baseline-hint">
            推荐基线：2 副本、更新时保持可用（最大不可用 0、最大额外副本 1）。
          </p>
          <div class="modal-actions">
            <button type="button" class="btn" @click="close">取消</button>
            <button v-if="isStatic(editing)" type="button" class="btn" @click="applyBaseline">安全滚动基线</button>
            <button type="submit" class="btn btn-primary" :disabled="saving">{{ saving ? '保存中...' : '保存配置' }}</button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { api } from '../api/index.js'

const items = ref([])
const loaded = ref(false)
const loading = ref(false)
const modal = ref(false)
const editing = ref(null)
const saving = ref(false)
const error = ref('')
const form = ref(blankForm())
const nodes = ref([])
const schedulableNodes = computed(() => nodes.value.filter(node => node.ready && !node.evicted))

function blankForm() {
  return { replicas: null, maxUnavailable: '0', maxSurge: '1', nodeName: '' }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    items.value = (await api.get('/system-components')) || []
    try {
      nodes.value = (await api.get('/nodes')) || []
    } catch (_) {
      nodes.value = []
    }
  } catch (err) {
    error.value = err.message || '加载系统组件失败'
  } finally {
    loading.value = false
    loaded.value = true
  }
}

function configBadgeClass(item) {
  if (!item.has_config) return 'badge-offline'
  if (item.apply_status === 'failed') return 'badge-danger'
  if (item.apply_status === 'succeeded' && item.effective === false) return 'badge-danger'
  return 'badge-online'
}

function configBadgeText(item) {
  if (item.controller_mode === 'unknown') return '控制源未识别'
  if (item.controller_mode === 'embedded') return '内置管理'
  if (!item.has_config) return '默认配置'
  if (item.apply_status === 'failed') return '失败'
  if (item.apply_status === 'succeeded' && item.effective === false) return '已保存未生效'
  return '已应用'
}

function configNeedsAttention(item) {
  return item.apply_status === 'failed' || item.effective === false
}

function configIssueText(item) {
  if (item.effective === false) return '配置与实际状态不一致，请重新保存'
  return '配置未应用，请打开配置重试'
}

function deploymentReady(item) {
  return Number(item?.deployment?.ready_replicas) >= Number(item?.deployment?.replicas) && Number(item?.deployment?.replicas) > 0
}

function workloadReady(item) {
  return Number(item?.workload?.ready) >= Number(item?.workload?.desired) && Number(item?.workload?.desired) > 0
}

function isMissing(item) {
  return /not found/i.test(item.deployment_error || '')
}

function isEmbedded(item) {
  return item?.controller_mode === 'embedded'
}

function isStatic(item) {
  return item?.controller_mode === 'static_deployment'
}

function canConfigure(item) {
  return Boolean(item?.capabilities?.configure)
}

function canPlace(item) {
  return Boolean(item?.capabilities?.node_placement)
}

function canRestore(item) {
  return Boolean(item?.capabilities?.restore)
}

function controllerModeText(mode) {
  const labels = {
    helm_chart: 'Helm 管理',
    static_deployment: 'K3s 静态组件',
    embedded: 'K3s 内置',
    unknown: '待确认',
  }
  return labels[mode] || labels.unknown
}

function modalDescription(item) {
  if (isStatic(item)) return '设置副本、滚动更新和节点调度。平台会在 K3s 重启后自动恢复这些设置。'
  return '设置会写入 HelmChartConfig，并由 Helm 控制器负责生效。'
}

function formatTime(value) {
  if (!value) return ''
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? String(value) : date.toLocaleString()
}

function parseValues(content, item) {
  const rollingUpdate = item?.deployment?.strategy?.rollingUpdate
  const parsed = {
    ...blankForm(),
    replicas: Number(item?.deployment?.replicas) || null,
    maxUnavailable: String(rollingUpdate?.maxUnavailable ?? '0'),
    maxSurge: String(rollingUpdate?.maxSurge ?? '1'),
    nodeName: item?.deployment?.fixed_node || '',
  }
  for (const line of String(content || '').split('\n')) {
    const hostname = line.match(/^\s*kubernetes\.io\/hostname\s*:\s*(.+?)\s*$/)
    if (hostname) {
      parsed.nodeName = hostname[1]
      continue
    }
    const match = line.match(/^\s*([A-Za-z][A-Za-z0-9]*)\s*:\s*(.+?)\s*$/)
    if (!match) continue
    const key = match[1]
    const value = match[2]
    if (key === 'replicas') parsed.replicas = Number(value)
    else if (key === 'maxUnavailable') parsed.maxUnavailable = String(value)
    else if (key === 'maxSurge') parsed.maxSurge = String(value)
  }
  return parsed
}

function renderValues(form) {
  if (isStatic(editing.value)) {
    const lines = []
    if (form.replicas && form.replicas > 0) lines.push(`replicas: ${form.replicas}`)
    lines.push('deploymentStrategy:')
    lines.push('  type: RollingUpdate')
    lines.push('  rollingUpdate:')
    lines.push(`    maxUnavailable: ${form.maxUnavailable}`)
    lines.push(`    maxSurge: ${form.maxSurge}`)
    if (form.nodeName) {
      lines.push('nodeSelector:')
      lines.push(`  kubernetes.io/hostname: ${form.nodeName}`)
    } else {
      lines.push('nodeSelector: {}')
    }
    return lines.join('\n') + '\n'
  }
  const lines = []
  if (form.replicas && form.replicas > 0) lines.push(`replicas: ${form.replicas}`)
  if (form.maxUnavailable !== '') lines.push(`maxUnavailable: ${form.maxUnavailable}`)
  if (form.maxSurge !== '') lines.push(`maxSurge: ${form.maxSurge}`)
  return lines.join('\n') + '\n'
}

function edit(item) {
  editing.value = item
  form.value = parseValues(item.values_content, item)
  modal.value = true
}

function applyBaseline() {
  form.value.maxUnavailable = '0'
  form.value.maxSurge = '1'
  if (isStatic(editing.value)) form.value.replicas = 2
}

function close() {
  modal.value = false
  editing.value = null
}

async function save() {
  if (!editing.value) return
  saving.value = true
  error.value = ''
  try {
    await api.put(`/system-components/${editing.value.chart_name}`, { values_content: renderValues(form.value) })
    close()
    await load()
  } catch (err) {
    error.value = err.message || '保存系统组件配置失败'
  } finally {
    saving.value = false
  }
}

async function revert(item) {
  if (!window.confirm(`恢复 ${item.chart_name} 为默认配置？${isStatic(item) ? '将重置平台受控的 Deployment 字段。' : '将删除 HelmChartConfig。'}`)) return
  error.value = ''
  try {
    await api.post(`/system-components/${item.chart_name}/revert`)
    await load()
  } catch (err) {
    error.value = err.message || '恢复默认配置失败'
  }
}

onMounted(load)
</script>

<style scoped>
.page-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; }
.system-component-table th, .system-component-table td { vertical-align: middle; }
.system-component-table th:nth-child(1) { width: 20%; }
.system-component-table th:nth-child(2) { width: 18%; }
.system-component-table th:nth-child(3) { width: 20%; }
.system-component-table th:nth-child(4) { width: 25%; }
.system-component-table th:nth-child(5) { width: 17%; }
.mode-label { display: inline-block; padding: 4px 8px; border-radius: 999px; background: var(--surface-subtle); color: var(--text-secondary); font-size: 12px; line-height: 1.2; white-space: nowrap; }
.mode-static_deployment { color: var(--text-primary); background: color-mix(in srgb, var(--action-primary) 12%, var(--surface-subtle)); }
.mode-helm_chart { color: var(--text-primary); background: color-mix(in srgb, var(--focus) 12%, var(--surface-subtle)); }
.mode-embedded { color: var(--text-secondary); background: var(--surface-subtle); }
.mode-unknown { color: var(--warning); background: color-mix(in srgb, var(--warning) 14%, var(--surface-subtle)); }
.detail { display: block; max-width: 220px; color: var(--danger); overflow-wrap: anywhere; }
.cell-secondary { display: block; }
.config-section-title { margin: 20px 0 10px; color: var(--text-primary); font-size: 13px; font-weight: 600; }
.config-section-title:first-child { margin-top: 0; }
.baseline-hint { margin: 12px 0 0; padding: 9px 11px; border-radius: var(--radius-control); background: var(--surface-subtle); color: var(--text-secondary); font-size: 12px; line-height: 1.5; }
@media (max-width: 640px) {
  .page-header { flex-direction: column; }
  .page-header .btn { width: 100%; }
  .system-component-table th:nth-child(2), .system-component-table td:nth-child(2) { display: none; }
}
</style>
