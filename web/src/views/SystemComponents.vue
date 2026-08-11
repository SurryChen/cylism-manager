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
        <table class="data-table">
          <thead>
            <tr><th>组件</th><th>命名空间</th><th>实际状态</th><th>配置状态</th><th>操作</th></tr>
          </thead>
          <tbody>
            <tr v-for="item in items" :key="item.chart_name">
              <td class="cell-primary">{{ item.chart_name }}</td>
              <td>{{ item.namespace }}</td>
              <td>
                <template v-if="item.deployment">
                  <span class="badge" :class="Number(item.deployment.ready_replicas) >= Number(item.deployment.replicas) && Number(item.deployment.replicas) > 0 ? 'badge-online' : 'badge-offline'">
                    {{ item.deployment.ready_replicas }}/{{ item.deployment.replicas }} 就绪
                  </span>
                  <small class="cell-secondary">{{ strategyText(item.deployment.strategy) }}</small>
                  <small class="cell-secondary">{{ item.deployment.image }}</small>
                  <small v-if="item.deployment.fixed_node" class="cell-secondary">固定节点：{{ item.deployment.fixed_node }}</small>
                  <small v-else-if="isStatic(item)" class="cell-secondary">由 Kubernetes 调度</small>
                </template>
                <template v-else>
                  <span class="badge" :class="isEmbedded(item) ? 'badge-online' : isMissing(item) ? 'badge-offline' : 'badge-danger'">
                    {{ isEmbedded(item) ? '内置运行中' : isMissing(item) ? '未安装' : '读取失败' }}
                  </span>
                  <small v-if="isEmbedded(item)" class="cell-secondary">由 K3s 进程内提供（无独立组件）</small>
                  <small v-else-if="item.deployment_error && !isMissing(item)" class="detail">{{ item.deployment_error }}</small>
                  <small v-else class="cell-secondary">{{ isMissing(item) ? '集群中无同名 Deployment' : '未部署' }}</small>
                </template>
              </td>
              <td>
                <small class="cell-secondary">{{ controllerModeText(item.controller_mode) }}</small>
                <small v-if="item.detection_evidence?.length" class="cell-secondary">{{ item.detection_evidence.join('；') }}</small>
                <span class="badge" :class="configBadgeClass(item)">{{ configBadgeText(item) }}</span>
                <small v-if="item.apply_error" class="detail">{{ item.apply_error }}</small>
                <small v-if="item.effective === false && item.effective_detail" class="detail">{{ item.effective_detail }}</small>
                <small v-if="item.last_applied_at" class="cell-secondary">应用于 {{ formatTime(item.last_applied_at) }}</small>
              </td>
              <td>
                <div class="btn-group">
                  <button v-if="canConfigure(item)" class="btn btn-sm" @click="edit(item)">编辑配置</button>
                  <button v-if="canPlace(item)" class="btn btn-sm" @click="migrate(item)">迁移</button>
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
        <h2 class="modal-title">{{ modalMode === 'migration' ? `迁移 ${editing?.chart_name}` : `配置 ${editing?.chart_name}` }}</h2>
        <p class="modal-copy">{{ modalDescription(editing) }}</p>
        <form @submit.prevent="save">
          <div class="form-row">
            <div class="form-group">
              <label class="form-label">副本数</label>
              <input v-model.number="form.replicas" type="number" min="1" class="form-input" placeholder="默认 1" />
            </div>
            <div class="form-group">
              <label class="form-label">最大不可用（maxUnavailable）</label>
              <select v-model="form.maxUnavailable" class="form-select">
                <option value="0">0（滚动更新期间服务不掉线）</option>
                <option value="1">1（允许一个旧副本先下线）</option>
              </select>
            </div>
          </div>
          <div class="form-group">
            <label class="form-label">最大额外副本（maxSurge）</label>
            <select v-model="form.maxSurge" class="form-select">
              <option value="1">1（先起一个新副本再缩旧副本）</option>
              <option value="0">0（必须先下线旧副本）</option>
              <option value="25%">25%（Kubernetes 默认值）</option>
            </select>
          </div>
          <div v-if="canPlace(editing)" class="form-group">
            <label class="form-label">固定部署节点</label>
            <select v-model="form.nodeName" class="form-select" data-testid="coredns-node-selector">
              <option value="">不固定，由 Kubernetes 调度</option>
              <option v-for="node in schedulableNodes" :key="node.name" :value="node.name">{{ node.display_name || node.name }}</option>
            </select>
            <span class="form-hint">选择其他节点并保存后，CoreDNS 会按滚动策略迁移；固定后所有副本只会在该节点调度。</span>
          </div>
          <p v-if="isStatic(editing)" class="baseline-hint">
            安全滚动基线：推荐 <strong>maxUnavailable 0 + maxSurge 1</strong>，保证新 Pod Ready 后才下线旧 Pod，
            滚动更新期间服务不空窗；CoreDNS 还建议 2 副本。点“安全滚动基线”自动填入推荐值，点“保存并应用”才真正写入生效。
          </p>
          <div class="modal-actions">
            <button type="button" class="btn" @click="close">取消</button>
            <button v-if="isStatic(editing)" type="button" class="btn" @click="applyBaseline">安全滚动基线</button>
            <button type="submit" class="btn btn-primary" :disabled="saving">{{ saving ? '保存中...' : (modalMode === 'migration' ? '保存并迁移' : '保存并应用') }}</button>
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
const modalMode = ref('config')
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

function strategyText(strategy) {
  if (!strategy) return ''
  if (strategy.type !== 'RollingUpdate' || !strategy.rollingUpdate) return strategy.type || ''
  return `RollingUpdate U:${strategy.rollingUpdate.maxUnavailable} S:${strategy.rollingUpdate.maxSurge}`
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
    helm_chart: '控制方式：Helm Chart',
    static_deployment: '控制方式：K3s 静态 Deployment',
    embedded: '控制方式：K3s 内置控制器',
    unknown: '控制方式：未识别',
  }
  return labels[mode] || labels.unknown
}

function modalDescription(item) {
  if (isStatic(item)) return '平台仅保存并更新副本、滚动策略和节点选择器；K3s 重应用静态清单后，平台会按期望配置重放这些受控字段。'
  return '写入 HelmChartConfig CRD，由 helm-controller 渲染；实际生效字段取决于该 Chart 支持的 values。'
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
  modalMode.value = 'config'
  modal.value = true
}

function migrate(item) {
  editing.value = item
  form.value = parseValues(item.values_content, item)
  applyBaseline()
  modalMode.value = 'migration'
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
  modalMode.value = 'config'
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
.detail { display: block; max-width: 220px; color: var(--danger); overflow-wrap: anywhere; }
.cell-secondary { display: block; }
.baseline-hint { margin: 12px 0 0; padding: 10px 12px; border-radius: var(--radius-control); background: var(--surface-subtle); color: var(--text-secondary); font-size: 12px; line-height: 1.7; }
.baseline-hint strong { color: var(--text-primary); }
@media (max-width: 640px) {
  .page-header { flex-direction: column; }
  .page-header .btn { width: 100%; }
}
</style>
