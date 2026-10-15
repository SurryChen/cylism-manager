<template>
  <SurfaceCard as="div" padding="none" class="sing-box-card">
    <div class="sing-box-toolbar">
      <span class="cell-secondary">已检测 {{ detectedCount }} / {{ servers.length }} 台 · {{ runningCount }} 台运行中</span>
      <button class="icon-button" type="button" title="刷新代理状态" aria-label="刷新代理状态" :disabled="refreshing" @click="refreshAll"><RefreshCw :size="16" :class="{ 'is-spinning': refreshing }" /></button>
    </div>
    <EmptyState v-if="!servers.length" message="暂无服务器" />
    <div v-else class="table-wrap sing-box-table-wrap"><table class="data-table"><thead><tr><th>服务器</th><th>主机</th><th>sing-box</th><th>配置</th><th>运行状态</th><th class="action-cell">操作</th></tr></thead><tbody>
      <tr v-for="server in servers" :key="server.id">
        <td>{{ server.name }}</td><td>{{ server.host }}</td>
        <td :title="statusFor(server.id)?.error || undefined">{{ pendingIDs.has(server.id) ? '检测中' : statusFor(server.id)?.error ? '检测失败' : statusFor(server.id)?.installed ? statusFor(server.id)?.version || '已安装' : '未安装' }}</td>
        <td>{{ statusFor(server.id)?.configured ? '已导入' : '-' }}</td>
        <td><span class="badge" :class="statusFor(server.id)?.running ? 'badge-online' : 'badge-offline'">{{ pendingIDs.has(server.id) ? '检测中' : statusFor(server.id)?.error ? '无法检测' : statusFor(server.id)?.running ? '运行中' : '未运行' }}</span></td>
        <td class="action-cell"><button class="icon-button icon-button--table" type="button" :title="`管理 ${server.name} 的代理`" :aria-label="`管理 ${server.name} 的代理`" @click="openManage(server)"><Settings2 :size="15" /></button></td>
      </tr>
    </tbody></table></div>
  </SurfaceCard>

  <BaseModal :open="Boolean(selectedServer)" :title="`代理 · ${selectedServer?.name || ''}`" teleport :show-close="!working" :close-on-overlay="!working" :close-on-escape="!working" @close="closeManage">
    <div class="sing-box-details">
      <div><span>程序</span><strong>{{ pendingIDs.has(selectedServer?.id) ? '检测中' : selectedStatus?.error ? '无法检测' : selectedStatus?.installed ? `sing-box ${selectedStatus.version || ''}` : '未安装' }}</strong></div>
      <div><span>配置</span><strong>{{ pendingIDs.has(selectedServer?.id) || selectedStatus?.error ? '-' : selectedStatus?.configured ? '已导入' : '未导入' }}</strong></div>
      <div><span>服务</span><strong>{{ pendingIDs.has(selectedServer?.id) ? '检测中' : selectedStatus?.error ? '无法检测' : selectedStatus?.running ? '运行中' : '未运行' }}</strong></div>
    </div>
    <EmptyState v-if="pendingIDs.has(selectedServer?.id)" variant="loading" message="正在读取代理状态" />
    <EmptyState v-else-if="selectedStatus?.error" message="无法读取代理状态" />
    <div v-else class="sing-box-manage">
      <template v-if="!selectedStatus?.installed">
        <label class="form-label" for="sing-box-egress">安装下载出口</label>
        <SelectMenu id="sing-box-egress" v-model="egressServerID" :options="egressOptions" aria-label="安装下载出口" :disabled="working" />
        <button class="btn btn-primary" type="button" :disabled="working" @click="operate('install')">{{ working ? '安装中...' : '安装 sing-box' }}</button>
      </template>
      <form v-else class="sing-box-form" @submit.prevent="operate('import')">
        <label class="form-label" for="sing-box-url">配置 URL</label>
        <div class="sing-box-url-row"><input id="sing-box-url" v-model.trim="configURL" class="form-input" :type="showURL ? 'url' : 'password'" autocomplete="off" placeholder="https://.../config.json" required /><button class="icon-button" type="button" :title="showURL ? '隐藏配置地址' : '显示配置地址'" :aria-label="showURL ? '隐藏配置地址' : '显示配置地址'" @click="showURL = !showURL"><EyeOff v-if="showURL" :size="16" /><Eye v-else :size="16" /></button></div>
        <button class="btn" type="submit" :disabled="working || !configURL">{{ working ? '导入中...' : selectedStatus?.configured ? '重新导入' : '导入配置' }}</button>
      </form>
    </div>
    <template #actions>
      <button v-if="!selectedStatus?.error && selectedStatus?.installed && selectedStatus?.configured && !selectedStatus?.running" class="btn btn-primary" type="button" :disabled="working" @click="operate('start')">启动代理</button>
      <button v-if="!selectedStatus?.error && selectedStatus?.running" class="btn btn-danger" type="button" :disabled="working" @click="confirmStop = true">停止代理</button>
      <button class="icon-button" type="button" title="刷新当前代理状态" aria-label="刷新当前代理状态" :disabled="working || pendingIDs.has(selectedServer?.id)" @click="refreshOne(selectedServer)"><RefreshCw :size="16" /></button>
      <button class="btn" type="button" :disabled="working" @click="closeManage">关闭</button>
    </template>
  </BaseModal>
  <ConfirmDialog :open="confirmStop" title="停止 sing-box 代理" :message="`停止 ${selectedServer?.name || ''} 的代理服务？`" confirm-text="停止代理" :busy="working" @confirm="operate('stop')" @cancel="confirmStop = false" />
  <ErrorNoticeModal :open="Boolean(error)" title="代理操作失败" :message="error" teleport @close="error = ''" />
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { Eye, EyeOff, RefreshCw, Settings2 } from 'lucide-vue-next'
import { getSingBoxStatus, importSingBoxConfig, installSingBox, startSingBox, stopSingBox } from '../../api/servers.js'
import BaseModal from '../../components/BaseModal.vue'
import ConfirmDialog from '../../components/ConfirmDialog.vue'
import EmptyState from '../../components/EmptyState.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import SurfaceCard from '../../components/SurfaceCard.vue'

const props = defineProps({ servers: { type: Array, default: () => [] } })
const statuses = ref(new Map())
const pendingIDs = ref(new Set())
const refreshing = ref(false)
const selectedServer = ref(null)
const configURL = ref('')
const egressServerID = ref('')
const showURL = ref(false)
const working = ref(false)
const confirmStop = ref(false)
const error = ref('')
const runningCount = computed(() => props.servers.filter(server => statuses.value.get(server.id)?.running).length)
const detectedCount = computed(() => props.servers.filter(server => statuses.value.has(server.id) && !statuses.value.get(server.id)?.error).length)
const selectedStatus = computed(() => statusFor(selectedServer.value?.id))
const egressOptions = computed(() => [
  { value: '', label: '目标服务器直连' },
  ...props.servers.filter(server => server.id !== selectedServer.value?.id && !server.proxy_server_id)
    .map(server => ({ value: server.id, label: server.name || server.host, description: server.host })),
])
let generation = 0

function statusFor(id) { return statuses.value.get(id) }

async function refreshOne(server, currentGeneration = generation) {
  pendingIDs.value = new Set([...pendingIDs.value, server.id])
  try {
    const status = await getSingBoxStatus(server.id)
    if (currentGeneration !== generation) return
    statuses.value = new Map(statuses.value).set(server.id, status)
    if (status.error && selectedServer.value?.id === server.id) error.value = status.error
  } catch (cause) {
    if (currentGeneration !== generation) return
    statuses.value = new Map(statuses.value).set(server.id, { server_id: server.id, error: cause.message })
    if (selectedServer.value?.id === server.id) error.value = cause.message
  } finally {
    if (currentGeneration === generation) {
      const pending = new Set(pendingIDs.value)
      pending.delete(server.id)
      pendingIDs.value = pending
    }
  }
}

async function refreshAll() {
  const currentGeneration = ++generation
  refreshing.value = true
  pendingIDs.value = new Set(props.servers.map(server => server.id))
  await Promise.all(props.servers.map(server => refreshOne(server, currentGeneration)))
  if (currentGeneration === generation) refreshing.value = false
}

function openManage(server) {
  selectedServer.value = server
  configURL.value = ''
  egressServerID.value = ''
  showURL.value = false
  if (!statusFor(server.id) || statusFor(server.id).error) void refreshOne(server)
}

function closeManage() {
  if (working.value) return
  selectedServer.value = null
  configURL.value = ''
  confirmStop.value = false
}

async function operate(action) {
  const server = selectedServer.value
  if (!server || working.value) return
  working.value = true
  try {
    const operation = { install: () => installSingBox(server.id, egressServerID.value || null), import: () => importSingBoxConfig(server.id, configURL.value), start: () => startSingBox(server.id), stop: () => stopSingBox(server.id) }[action]
    const status = await operation()
    statuses.value = new Map(statuses.value).set(server.id, status)
    if (action === 'import') configURL.value = ''
    confirmStop.value = false
  } catch (cause) {
    error.value = cause.message
  } finally {
    working.value = false
  }
}

watch(() => props.servers.map(server => server.id).join(','), () => { void refreshAll() }, { immediate: true })
</script>

<style scoped>
.sing-box-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 12px; min-height: 62px; padding: 12px var(--space-20); }
.sing-box-table-wrap { padding: 0 var(--space-20) var(--space-20); }
.sing-box-details { display: grid; gap: 10px; margin-bottom: 20px; }
.sing-box-details > div { display: flex; justify-content: space-between; gap: 16px; font-size: 13px; }
.sing-box-details span { color: var(--text-secondary); }
.sing-box-manage, .sing-box-form { display: grid; gap: 10px; }
.sing-box-url-row { display: flex; gap: 8px; min-width: 0; }
.sing-box-url-row .form-input { min-width: 0; flex: 1; }
.sing-box-form > .btn { justify-self: start; }
@media (max-width: 640px) { .sing-box-toolbar { padding-right: 14px; padding-left: 14px; }.sing-box-table-wrap { padding-right: 14px; padding-left: 14px; } }
</style>
