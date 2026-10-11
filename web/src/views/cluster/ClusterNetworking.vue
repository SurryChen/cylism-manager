<template>
  <div class="cluster-networking">
    <ServerTailscale :servers="servers" :cluster="cluster" :node-infos="nodes" :nodes-loading="nodesLoading" :nodes-error="nodesError" @refresh-nodes="loadNodes">
      <template v-if="cluster.network_mode !== 'tailscale'" #actions>
        <button class="btn btn-primary" type="button" :disabled="checking || starting || taskRunning || !servers.length" @click="precheck">{{ checking ? '检查中...' : '迁移至 Tailscale' }}</button>
      </template>
      <template v-if="check" #footer>
        <div class="network-migration-results">
          <div v-if="check.nodes?.length" class="table-wrap"><table class="data-table"><thead><tr><th>服务器</th><th>角色</th><th>Tailscale IP</th><th>预检</th><th class="action-cell">操作</th></tr></thead><tbody><tr v-for="item in check.nodes" :key="item.server_id"><td>{{ item.name }}</td><td>{{ item.role === 'control-plane' ? '控制平面' : '工作节点' }}</td><td>{{ item.tailscale_ip || '-' }}</td><td><span class="badge" :class="item.ready ? 'badge-online' : 'badge-warn'">{{ item.ready ? '可迁移' : item.reason || '待处理' }}</span></td><td class="action-cell"><RouterLink v-if="!item.ready && item.reason?.includes('Tailscale')" class="btn btn-sm" :to="serverNetworkLink">配置组网</RouterLink></td></tr></tbody></table></div>
          <div v-if="check.ready" class="network-migration-footer"><button class="btn btn-primary" type="button" :disabled="starting || taskRunning" @click="confirmOpen = true">确认迁移</button></div>
        </div>
      </template>
    </ServerTailscale>
    <BaseModal :open="confirmOpen" title="迁移 K3s 集群组网" size="small" teleport :show-close="!starting" :close-on-overlay="!starting" :close-on-escape="!starting" @close="confirmOpen = false">
      <p>将依次重启控制平面与工作节点上的 K3s 服务。迁移期间业务访问可能短暂中断；任务会记录每个节点的结果，失败时尝试恢复原配置。</p>
      <p>迁移不会自动关闭服务器公网端口，请在验证完成后单独调整防火墙。</p>
      <template #actions><button class="btn" type="button" :disabled="starting" @click="confirmOpen = false">取消</button><button class="btn btn-primary" type="button" :disabled="starting" @click="startMigration">{{ starting ? '启动中...' : '开始迁移' }}</button></template>
    </BaseModal>
    <ErrorNoticeModal :open="Boolean(operationError)" :title="errorTitle" :message="operationError" teleport @close="operationError = ''" />
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { getClusterNodes } from '../../api/cluster.js'
import { checkClusterNetworkMigration, startClusterNetworkMigration } from '../../api/clusters.js'
import BaseModal from '../../components/BaseModal.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import ServerTailscale from './ServerTailscale.vue'

const emit = defineEmits(['operation-started'])
const props = defineProps({
  clusterId: { type: [String, Number], required: true },
  cluster: { type: Object, required: true },
  servers: { type: Array, default: () => [] },
  taskRunning: { type: Boolean, default: false },
})
const nodes = ref([])
const nodesLoading = ref(false)
const nodesError = ref('')
const check = ref(null)
const checking = ref(false)
const starting = ref(false)
const confirmOpen = ref(false)
const operationError = ref('')
const errorTitle = ref('操作失败')
const serverNetworkLink = computed(() => ({ path: '/servers', query: { tab: 'tailscale', return: `/cluster/${props.clusterId}?tab=networking` } }))
let controller

async function precheck() {
  if (checking.value) return
  checking.value = true
  check.value = null
  try {
    check.value = await checkClusterNetworkMigration(props.clusterId)
    if (!check.value?.ready) {
      errorTitle.value = '迁移预检未通过'
      operationError.value = check.value?.message || '请检查未通过的节点'
    }
  } catch (error) {
    errorTitle.value = '迁移预检失败'
    operationError.value = error.message || '组网迁移预检失败'
  }
  finally { checking.value = false }
}

async function startMigration() {
  if (!check.value?.ready || starting.value) return
  starting.value = true
  try {
    await startClusterNetworkMigration(props.clusterId)
    confirmOpen.value = false
    check.value = null
    emit('operation-started')
  } catch (error) { errorTitle.value = '启动迁移失败'; operationError.value = error.message || '启动组网迁移失败' }
  finally { starting.value = false }
}

async function loadNodes() {
  controller?.abort()
  const requestController = new AbortController()
  controller = requestController
  nodes.value = []
  nodesError.value = ''
  nodesLoading.value = Boolean(props.servers.length)
  if (!props.servers.length) return
  try {
    const result = await getClusterNodes({ signal: requestController.signal }, props.clusterId)
    if (!requestController.signal.aborted) nodes.value = result || []
  } catch (error) {
    if (!requestController.signal.aborted) {
      nodesError.value = error.message || '无法读取 K3s 节点 IP'
      errorTitle.value = '读取节点信息失败'
      operationError.value = nodesError.value
    }
  } finally {
    if (!requestController.signal.aborted) nodesLoading.value = false
  }
}

watch(() => [props.clusterId, props.servers.length], loadNodes, { immediate: true })

watch(() => props.clusterId, () => { check.value = null; confirmOpen.value = false })

onBeforeUnmount(() => controller?.abort())
</script>

<style scoped>
.cluster-networking{display:grid;gap:16px}
.network-migration-results{padding:0 20px 16px}
.network-migration-results .table-wrap{margin-top:12px}
.network-migration-results th:last-child,.network-migration-results td:last-child{text-align:right}
.network-migration-footer{display:flex;justify-content:flex-end;padding-top:14px}
@media(max-width:640px){.network-migration-results{padding:0 14px 14px}}
</style>
