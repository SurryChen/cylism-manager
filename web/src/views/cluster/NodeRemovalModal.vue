<template>
  <BaseModal :open="Boolean(server)" title="移出集群" :show-close="!submitting" :close-on-overlay="!submitting" :close-on-escape="!submitting" @close="close">
    <div class="cluster-deployment-form">
      <p class="removal-description">排空 Pod 后，停止并禁用 K3s，删除集群节点并解除关联。保留服务器记录和 SSH 信息，默认保留本地 K3s 数据。</p>
      <p class="cell-secondary">请先切走指向此节点的 DNS 和外部入口流量。排空遵守 PodDisruptionBudget；检查后出现的新阻塞也会中止操作。</p>
      <p class="cell-secondary">若执行失败，已完成的排空或停用步骤不会自动恢复，可查看任务日志后重新检查。</p>
      <EmptyState v-if="loading" variant="loading" message="检查节点及 Pod 中..." />
      <template v-else-if="plan">
        <p v-if="plan.node_missing" class="cell-secondary">节点对象已不存在，将核对上次移出任务的主机身份，继续停用检查并完成解绑。</p>
        <div v-for="group in podGroups" :key="group.label" class="removal-pod-group">
          <strong>{{ group.label }} <span class="badge" :class="group.blocked ? 'badge-danger' : 'badge-offline'">{{ group.pods.length }}</span></strong>
          <ul v-if="group.pods.length"><li v-for="pod in group.pods" :key="pod.namespace + '/' + pod.name"><span>{{ pod.namespace }}/{{ pod.name }}</span><span class="cell-secondary">{{ pod.reason }}</span></li></ul>
        </div>
        <p v-if="!plan.can_start" class="cell-secondary">请先处理阻塞 Pod，再重新检查。</p>
        <label v-if="needsEmptyDir" class="removal-checkbox"><input v-model="deleteEmptyDirData" type="checkbox" />允许丢弃上述 Pod 的 emptyDir 临时数据</label>
        <label class="removal-checkbox"><input v-model="uninstallK3s" type="checkbox" :disabled="submitting || plan.node_uninstalled" />同时卸载 K3s 并清理本地数据（不可恢复）</label>
        <p v-if="!uninstallK3s" class="cell-secondary">保留的数据可稍后从“服务器”页面继续卸载清理。</p>
        <div class="deployment-form-field"><label for="removal-node-name">输入节点名 {{ plan.node_name }} 确认</label><input id="removal-node-name" v-model="confirmNodeName" class="form-input" autocomplete="off" :disabled="submitting" /></div>
      </template>
      <EmptyState v-else message="未能读取节点检查结果，请重新检查" />
    </div>
    <template #actions>
      <button class="btn" type="button" :disabled="submitting" @click="close">取消</button>
      <button class="btn" type="button" :disabled="loading || submitting" @click="loadPlan">重新检查</button>
      <button class="btn btn-danger" type="button" :disabled="!canSubmit" @click="submit">{{ submitting ? '启动中...' : '确认移出' }}</button>
    </template>
  </BaseModal>
  <ErrorNoticeModal :open="Boolean(error)" :message="error" teleport @close="error = ''" />
</template>

<script setup>
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import BaseModal from '../../components/BaseModal.vue'
import EmptyState from '../../components/EmptyState.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import { getNodeRemovalPlan, removeClusterNode } from '../../api/clusters.js'

const props = defineProps({ clusterId: { type: String, required: true }, server: { type: Object, default: null } })
const emit = defineEmits(['close', 'started'])
const plan = ref(null)
const loading = ref(false)
const submitting = ref(false)
const error = ref('')
const confirmNodeName = ref('')
const deleteEmptyDirData = ref(false)
const uninstallK3s = ref(false)
let requestID = 0
let controller
const needsEmptyDir = computed(() => Boolean(plan.value?.drain?.requires_empty_dir_confirmation?.length))
const canSubmit = computed(() => !loading.value && !submitting.value && Boolean(plan.value?.can_start) && confirmNodeName.value === plan.value?.node_name && (!needsEmptyDir.value || deleteEmptyDirData.value))
const podGroups = computed(() => plan.value?.drain ? [
  { label: '将迁移', pods: plan.value.drain.evictable || [] },
  { label: '需确认临时数据', pods: plan.value.drain.requires_empty_dir_confirmation || [] },
  { label: '阻塞项', pods: plan.value.drain.blocked || [], blocked: true },
  { label: '跳过驱逐（DaemonSet、静态或已完成 Pod）', pods: plan.value.drain.skipped || [] },
] : [])

watch(() => [props.clusterId, props.server?.id], () => {
  requestID += 1
  controller?.abort()
  plan.value = null
  error.value = ''
  confirmNodeName.value = ''
  deleteEmptyDirData.value = false
  uninstallK3s.value = false
  submitting.value = false
  loading.value = false
  if (props.server) void loadPlan()
}, { immediate: true })
onBeforeUnmount(() => { requestID += 1; controller?.abort() })

function close() { if (!submitting.value) emit('close') }
async function loadPlan() {
  if (!props.server || submitting.value) return
  const id = ++requestID
  controller?.abort()
  controller = new AbortController()
  loading.value = true
  plan.value = null
  confirmNodeName.value = ''
  deleteEmptyDirData.value = false
  uninstallK3s.value = false
  error.value = ''
  try {
    const result = await getNodeRemovalPlan(props.clusterId, props.server.id, { signal: controller.signal })
    if (id === requestID) { plan.value = result; uninstallK3s.value = Boolean(result.node_uninstalled) }
  } catch (err) {
    if (id === requestID) error.value = err.message || '检查节点失败'
  } finally { if (id === requestID) loading.value = false }
}
async function submit() {
  if (!canSubmit.value || !props.server) return
  const id = requestID
  submitting.value = true
  error.value = ''
  try {
    await removeClusterNode(props.clusterId, props.server.id, { confirm_node_name: confirmNodeName.value, node_uid: plan.value.node_uid, delete_empty_dir_data: deleteEmptyDirData.value, uninstall_k3s: uninstallK3s.value })
    if (id === requestID) { emit('started'); emit('close') }
  } catch (err) {
    if (id === requestID) error.value = err.message || '启动移出任务失败'
  } finally { if (id === requestID) submitting.value = false }
}
</script>

<style scoped>
.cluster-deployment-form { display: grid; gap: var(--space-12); }
.removal-description { margin: 0; font-size: 13px; line-height: 1.6; }
.cluster-deployment-form > p { margin: 0; }
.removal-pod-group { display: grid; gap: var(--space-8); font-size: 12px; }
.removal-pod-group strong { display: flex; align-items: center; gap: var(--space-8); }
.removal-pod-group ul { display: grid; gap: var(--space-8); margin: 0; padding-left: var(--space-20); max-height: 140px; overflow-y: auto; }
.removal-pod-group li { overflow-wrap: anywhere; }
.removal-pod-group .cell-secondary { display: block; }
.removal-checkbox { display: flex; align-items: center; gap: var(--space-8); font-size: 13px; }
.deployment-form-field { display: grid; gap: var(--space-8); font-size: 12px; }
</style>
