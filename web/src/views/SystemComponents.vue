<template>
  <section class="page">
    <div class="page-header">
      <div>
        <h1 class="page-title">系统组件</h1>
        <p class="page-subtitle">通过 HelmChartConfig 持久化管理 K3s 内置组件，升级后配置依然生效</p>
      </div>
      <button class="btn" :disabled="loading" @click="load">刷新</button>
    </div>

    <p v-if="error" class="detail">{{ error }}</p>
    <p v-if="message" class="notice notice-success">{{ message }}</p>

    <div class="table-wrap">
      <table class="data-table">
        <thead>
          <tr><th>组件</th><th>命名空间</th><th>实际状态</th><th>配置状态</th><th>操作</th></tr>
        </thead>
        <tbody>
          <tr v-for="item in items" :key="item.chart_name">
            <td><strong>{{ item.chart_name }}</strong></td>
            <td>{{ item.namespace }}</td>
            <td>
              <template v-if="item.deployment">
                <span>{{ item.deployment.ready_replicas }}/{{ item.deployment.replicas }} 就绪</span>
                <small class="cell-secondary">{{ strategyText(item.deployment.strategy) }}</small>
                <small class="cell-secondary">{{ item.deployment.image }}</small>
              </template>
              <span v-else class="cell-secondary">未部署或读取失败</span>
            </td>
            <td>
              <span v-if="item.has_config" :class="item.apply_status === 'succeeded' ? 'status-success' : 'status-warning'">
                {{ item.apply_status === 'succeeded' ? '已应用' : item.apply_status || '待应用' }}
              </span>
              <span v-else class="cell-secondary">默认配置</span>
              <small v-if="item.apply_error" class="detail">{{ item.apply_error }}</small>
            </td>
            <td class="action-cell">
              <button class="btn" @click="edit(item)">编辑配置</button>
              <button v-if="item.has_config" class="btn btn-danger" @click="revert(item)">恢复默认</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div v-if="editing" class="overlay" @click.self="editing = null">
      <form class="modal" @submit.prevent="save">
        <h2 class="modal-title">配置 {{ editing.chart_name }}</h2>
        <p class="modal-copy">写入 HelmChartConfig CRD，由 helm-controller 渲染；K3s 升级后依然保留。</p>
        <label class="form-group">
          <span class="form-label">values YAML</span>
          <textarea v-model="valuesContent" class="textarea-input" rows="10" spellcheck="false" />
        </label>
        <div class="modal-actions">
          <button type="button" class="btn" @click="editing = null">取消</button>
          <button type="button" class="btn" @click="applyBaseline">安全滚动基线</button>
          <button type="submit" class="btn btn-primary" :disabled="saving">{{ saving ? '保存中...' : '保存并应用' }}</button>
        </div>
      </form>
    </div>
  </section>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { api } from '../api/index.js'

const items = ref([])
const editing = ref(null)
const valuesContent = ref('')
const saving = ref(false)
const loading = ref(false)
const error = ref('')
const message = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    items.value = (await api.get('/system-components')) || []
  } catch (err) {
    error.value = err.message || '加载系统组件失败'
  } finally {
    loading.value = false
  }
}

function strategyText(strategy) {
  if (!strategy) return ''
  if (strategy.type !== 'RollingUpdate' || !strategy.rollingUpdate) return strategy.type || ''
  return `RollingUpdate U:${strategy.rollingUpdate.maxUnavailable} S:${strategy.rollingUpdate.maxSurge}`
}

function edit(item) {
  editing.value = item
  valuesContent.value = item.values_content || ''
  error.value = ''
  message.value = ''
}

function applyBaseline() {
  const chart = editing.value?.chart_name
  const lines = chart === 'coredns'
    ? ['replicas: 2', 'maxUnavailable: 0', 'maxSurge: 1']
    : ['maxUnavailable: 0', 'maxSurge: 1']
  valuesContent.value = lines.join('\n') + '\n'
}

async function save() {
  if (!editing.value) return
  saving.value = true
  error.value = ''
  message.value = ''
  try {
    await api.put(`/system-components/${editing.value.chart_name}`, { values_content: valuesContent.value })
    message.value = '系统组件配置已应用'
    editing.value = null
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
  message.value = ''
  try {
    await api.post(`/system-components/${item.chart_name}/revert`)
    message.value = '已恢复默认配置'
    await load()
  } catch (err) {
    error.value = err.message || '恢复默认配置失败'
  }
}

onMounted(load)
</script>

<style scoped>
.page { padding-bottom: var(--space-24); }
.page-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; margin-bottom: var(--space-16); }
.data-table small { display: block; }
.notice-success { margin: 0 0 var(--space-16); padding: 10px 12px; border-radius: var(--radius-control); color: var(--success); background: var(--success-surface); }
</style>
