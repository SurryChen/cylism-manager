<template>
  <div>
    <div class="page-header">
      <div>
        <h1 class="page-title">系统组件</h1>
        <p class="page-subtitle">通过 HelmChartConfig 持久化管理 K3s 内置组件，升级后配置依然生效</p>
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
                <span class="badge" :class="configBadgeClass(item)">{{ configBadgeText(item) }}</span>
                <small v-if="item.apply_error" class="detail">{{ item.apply_error }}</small>
                <small v-if="item.effective === false && item.effective_detail" class="detail">{{ item.effective_detail }}</small>
                <small v-if="item.last_applied_at" class="cell-secondary">应用于 {{ formatTime(item.last_applied_at) }}</small>
              </td>
              <td>
                <div class="btn-group">
                  <button v-if="!isEmbedded(item)" class="btn btn-sm" @click="edit(item)">编辑配置</button>
                  <button v-if="item.has_config" class="btn btn-sm btn-danger" @click="revert(item)">恢复默认</button>
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
        <p class="modal-copy">写入 HelmChartConfig CRD，由 helm-controller 渲染；K3s 升级后依然保留。</p>
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
            </select>
          </div>
          <p class="baseline-hint">
            安全滚动基线：推荐 <strong>maxUnavailable 0 + maxSurge 1</strong>，保证新 Pod Ready 后才下线旧 Pod，
            滚动更新期间服务不空窗；CoreDNS 还建议 2 副本。点“安全滚动基线”自动填入推荐值，点“保存并应用”才真正写入生效。
          </p>
          <div class="modal-actions">
            <button type="button" class="btn" @click="close">取消</button>
            <button type="button" class="btn" @click="applyBaseline">安全滚动基线</button>
            <button type="submit" class="btn btn-primary" :disabled="saving">{{ saving ? '保存中...' : '保存并应用' }}</button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { api } from '../api/index.js'

const items = ref([])
const loaded = ref(false)
const loading = ref(false)
const modal = ref(false)
const editing = ref(null)
const saving = ref(false)
const error = ref('')
const form = ref(blankForm())

function blankForm() {
  return { replicas: null, maxUnavailable: '0', maxSurge: '1' }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    items.value = (await api.get('/system-components')) || []
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
  if (!item.has_config) return '默认配置'
  if (item.apply_status === 'failed') return '失败'
  if (item.apply_status === 'succeeded' && item.effective === false) return '已保存未生效'
  return '已应用'
}

function isMissing(item) {
  return /not found/i.test(item.deployment_error || '')
}

function isEmbedded(item) {
  return item.chart_name === 'servicelb' && item.lb_active
}

function formatTime(value) {
  if (!value) return ''
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? String(value) : date.toLocaleString()
}

function parseValues(content) {
  const parsed = blankForm()
  for (const line of String(content || '').split('\n')) {
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
  const lines = []
  if (form.replicas && form.replicas > 0) lines.push(`replicas: ${form.replicas}`)
  if (form.maxUnavailable !== '') lines.push(`maxUnavailable: ${form.maxUnavailable}`)
  if (form.maxSurge !== '') lines.push(`maxSurge: ${form.maxSurge}`)
  return lines.join('\n') + '\n'
}

function edit(item) {
  editing.value = item
  form.value = parseValues(item.values_content)
  modal.value = true
}

function applyBaseline() {
  form.value = blankForm()
  form.value.maxUnavailable = '0'
  form.value.maxSurge = '1'
  if (editing.value?.chart_name === 'coredns') form.value.replicas = 2
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
  if (!window.confirm(`恢复 ${item.chart_name} 为默认配置？将删除 HelmChartConfig。`)) return
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
