<template>
  <div>
    <TabbedWorkspaceCard class="chart-repository-workspace">
      <template #actions>
        <button class="icon-button" type="button" title="刷新 Chart 仓库" aria-label="刷新 Chart 仓库" :disabled="loading" @click="load">
          <RefreshCw :size="16" :class="{ 'is-spinning': loading }" />
        </button>
        <button class="btn btn-primary" @click="openCreate">新建 Chart 仓库</button>
      </template>
      <div v-if="loaded && items.length" class="table-wrap">
        <table class="data-table">
          <thead><tr><th>名称</th><th>仓库地址</th><th>Chart</th><th>版本</th><th>状态</th><th>检测</th><th>操作</th></tr></thead>
          <tbody>
            <tr v-for="item in items" :key="item.id">
              <td class="cell-primary">{{ item.name }}</td>
              <td>{{ item.endpoint }}</td>
              <td>{{ item.chart_name }}</td>
              <td>{{ item.chart_version }}</td>
              <td><span class="badge" :class="item.enabled ? 'badge-online' : 'badge-offline'">{{ item.enabled ? '已启用' : '已停用' }}</span></td>
              <td><span class="badge" :class="item.last_verify_status === 'succeeded' ? 'badge-online' : item.last_verify_status === 'failed' ? 'badge-danger' : 'badge-offline'" :title="item.last_verify_error || ''">{{ item.last_verify_status === 'succeeded' ? '可用' : item.last_verify_status === 'failed' ? '失败' : '未检测' }}</span></td>
              <td><div class="btn-group"><button class="btn btn-sm" @click="verify(item)">检测</button><button class="btn btn-sm" @click="edit(item)">编辑</button><button class="btn btn-sm btn-danger" @click="target = item">删除</button></div></td>
            </tr>
          </tbody>
        </table>
      </div>
      <EmptyState v-else-if="loaded" message="还没有 Chart 仓库" />
    </TabbedWorkspaceCard>

    <ErrorNoticeModal :open="Boolean(error)" title="Chart 仓库操作失败" :message="error" @close="dismissError" />

    <div v-if="modal" class="overlay" @click.self="close">
      <div class="modal">
        <h2 class="modal-title">{{ editing ? '编辑 Chart 仓库' : '新建 Chart 仓库' }}</h2>
        <form @submit.prevent="save">
          <div class="form-group"><label class="form-label">名称</label><input v-model.trim="form.name" required class="form-input" placeholder="jetstack" /></div>
          <div class="form-group"><label class="form-label">仓库地址</label><input v-model.trim="form.endpoint" required type="url" class="form-input" placeholder="https://charts.jetstack.io" /></div>
          <div class="form-row"><div class="form-group"><label class="form-label">Chart 名称</label><input v-model.trim="form.chart_name" required class="form-input" placeholder="cert-manager" /></div><div class="form-group"><label class="form-label">固定版本</label><input v-model.trim="form.chart_version" required class="form-input" placeholder="v1.16.3" /></div></div>
          <label class="check-row"><input v-model="form.enabled" type="checkbox" /> 启用此仓库</label>
          <div class="modal-actions"><button type="button" class="btn" @click="close">取消</button><button class="btn btn-primary" :disabled="saving">保存</button></div>
        </form>
      </div>
    </div>

    <div v-if="target" class="overlay">
      <div class="modal">
        <h2 class="modal-title">删除 Chart 仓库</h2>
        <div class="modal-actions"><button class="btn" @click="target = null">取消</button><button class="btn btn-danger" @click="remove">删除</button></div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { RefreshCw } from 'lucide-vue-next'
import { createChartRepository, deleteChartRepository, getChartRepositories, updateChartRepository, verifyChartRepository } from '../../api/chart-repositories.js'
import EmptyState from '../../components/EmptyState.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import TabbedWorkspaceCard from '../../components/TabbedWorkspaceCard.vue'

const items = ref([])
const loaded = ref(false)
const loading = ref(false)
const error = ref('')
const modal = ref(false)
const editing = ref(null)
const target = ref(null)
const saving = ref(false)
const form = ref(blank())

function blank() {
  return { name: '', endpoint: '', chart_name: 'cert-manager', chart_version: 'v1.16.3', enabled: true }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    items.value = await getChartRepositories() || []
  } catch (err) {
    error.value = err.message || '加载 Chart 仓库失败'
  } finally {
    loading.value = false
    loaded.value = true
  }
}

function openCreate() {
  editing.value = null
  form.value = blank()
  modal.value = true
}

function edit(item) {
  editing.value = item
  form.value = { name: item.name, endpoint: item.endpoint, chart_name: item.chart_name, chart_version: item.chart_version, enabled: item.enabled }
  modal.value = true
}

function close() {
  modal.value = false
  editing.value = null
}

function dismissError() {
  error.value = ''
}

async function save() {
  saving.value = true
  error.value = ''
  try {
    if (editing.value) await updateChartRepository(editing.value.id, form.value)
    else await createChartRepository(form.value)
    close()
    await load()
  } catch (err) {
    error.value = err.message || '保存失败'
  } finally {
    saving.value = false
  }
}

async function verify(item) {
  error.value = ''
  try {
    await verifyChartRepository(item.id)
    await load()
  } catch (err) {
    error.value = err.message || '检测失败'
  }
}

async function remove() {
  if (!target.value) return
  saving.value = true
  error.value = ''
  try {
    await deleteChartRepository(target.value.id)
    target.value = null
    await load()
  } catch (err) {
    error.value = err.message || '删除失败'
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.check-row { display: flex; gap: 8px; margin: 12px 0; color: var(--text-secondary); font-size: 13px; }
</style>
