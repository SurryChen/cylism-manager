<template>
  <BaseModal :open="Boolean(server)" title="卸载保留的 K3s" :show-close="!submitting" :close-on-overlay="!submitting" :close-on-escape="!submitting" @close="close">
    <div class="cleanup-form">
      <p>此服务器已安全移出集群。继续卸载将删除 K3s 服务及本地集群数据，保留服务器记录和 SSH 信息。数据清理不可恢复。</p>
      <EmptyState v-if="loading" variant="loading" message="读取移出记录中..." />
      <template v-else-if="source">
        <p class="cell-secondary">原节点：{{ source.node_name }} · 移出任务 #{{ source.id }}。执行前将再次核对主机身份与服务停用状态。</p>
        <label v-if="!running && task?.status !== 'success'" class="cleanup-field" for="cleanup-node-name">输入节点名 {{ source.node_name }} 确认<input id="cleanup-node-name" v-model="confirmation" class="form-input" autocomplete="off" :disabled="submitting" /></label>
      </template>
      <EmptyState v-else-if="!task" message="没有可清理的移出记录，请刷新服务器列表" />
      <p v-if="task"><span class="badge" :class="task.status === 'failed' ? 'badge-danger' : task.status === 'success' ? 'badge-online' : 'badge-warn'">{{ task.status === 'running' ? '清理中' : task.status === 'success' ? '清理完成' : '清理失败，可核对后重试' }}</span> · 任务 #{{ task.id }}</p>
      <div v-if="logs.length" class="cleanup-logs" role="log" aria-live="polite"><div v-for="entry in logs" :key="entry.id">{{ entry.message }}</div></div>
    </div>
    <template #actions>
      <button class="btn" type="button" :disabled="submitting" @click="close">{{ task ? '关闭' : '取消' }}</button>
      <button class="btn" type="button" :disabled="loading || submitting" @click="refresh">刷新状态</button>
      <button v-if="task?.status !== 'success'" class="btn btn-danger" type="button" :disabled="!canSubmit" @click="submit">{{ submitting ? '启动中...' : '确认卸载并清理' }}</button>
    </template>
  </BaseModal>
  <ErrorNoticeModal :open="Boolean(error)" :message="error" teleport @close="error = ''" />
</template>

<script setup>
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import BaseModal from '../../components/BaseModal.vue'
import EmptyState from '../../components/EmptyState.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import { cleanupServerK3s, getServerK3sCleanup } from '../../api/servers.js'

const props = defineProps({ server: { type: Object, default: null } })
const emit = defineEmits(['close', 'completed'])
const source = ref(null)
const task = ref(null)
const logs = ref([])
const confirmation = ref('')
const loading = ref(false)
const statusReady = ref(false)
const submitting = ref(false)
const error = ref('')
const running = computed(() => task.value?.status === 'running')
const canSubmit = computed(() => statusReady.value && Boolean(source.value?.node_name) && confirmation.value === source.value.node_name && !loading.value && !submitting.value && !running.value)
let generation = 0
let timer
let controller
let completedTaskID
watch(() => props.server?.id, () => {
  generation += 1
  clearTimeout(timer)
  controller?.abort()
  source.value = null
  task.value = null
  logs.value = []
  confirmation.value = ''
  error.value = ''
  loading.value = false
  statusReady.value = false
  submitting.value = false
  completedTaskID = null
  if (props.server) void refresh()
}, { immediate: true })
onBeforeUnmount(() => { generation += 1; clearTimeout(timer); controller?.abort() })
function close() { if (!submitting.value) emit('close') }
async function refresh() {
  if (!props.server) return
  const current = generation
  clearTimeout(timer)
  controller?.abort()
  const requestController = new AbortController()
  controller = requestController
  loading.value = true
  statusReady.value = false
  try {
    const result = await getServerK3sCleanup(props.server.id, { signal: requestController.signal })
    if (current !== generation || requestController.signal.aborted) return
    if (source.value?.id !== result.source?.id) confirmation.value = ''
    source.value = result.source
    task.value = result.task
    logs.value = result.logs || []
    statusReady.value = true
    if (task.value?.status === 'success' && completedTaskID !== task.value.id) { completedTaskID = task.value.id; emit('completed') }
    if (running.value) timer = setTimeout(refresh, 2000)
  } catch (err) {
    if (current === generation && !requestController.signal.aborted) error.value = err.message || '读取清理状态失败，请刷新重试'
  } finally { if (current === generation && !requestController.signal.aborted) loading.value = false }
}
async function submit() {
  if (!canSubmit.value) return
  const current = generation
  submitting.value = true
  error.value = ''
  try {
    const result = await cleanupServerK3s(props.server.id, { removal_task_id: source.value.id, confirm_node_name: confirmation.value })
    if (current !== generation) return
    task.value = result
    confirmation.value = ''
    await refresh()
  } catch (err) { if (current === generation) error.value = err.message || '启动清理失败' }
  finally { if (current === generation) submitting.value = false }
}
</script>

<style scoped>
.cleanup-form, .cleanup-field { display: grid; gap: var(--space-12); }
.cleanup-form p { margin: 0; font-size: 13px; line-height: 1.6; }
.cleanup-field { font-size: 12px; }
.cleanup-logs { max-height: 240px; overflow: auto; padding: var(--space-12); background: var(--surface-raised); font: 12px/1.7 var(--font-mono); white-space: pre-wrap; overflow-wrap: anywhere; }
</style>
