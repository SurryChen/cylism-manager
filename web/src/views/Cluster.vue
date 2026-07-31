<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">集群节点</h1>
    </div>

    <div class="card section-gap">
      <p class="section-copy">
        节点状态直接来自当前 K3s 集群。新增工作节点请先在“服务器”页面补全 SSH 信息并完成加入集群。
      </p>
    </div>

    <div v-if="error" class="k8s-banner k8s-banner-warn section-gap">⚠ {{ error }}</div>

    <div class="card">
      <div v-if="nodes.length === 0" class="empty-state">
        <span class="empty-icon">⬡</span><span class="empty-text">暂无 K8s 节点</span>
      </div>
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <th>节点名</th><th>状态</th><th>角色</th><th>K8s 版本</th><th>IP</th><th>映射服务器</th><th>CPU</th><th>内存</th><th></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="node in nodes" :key="node.name">
              <td class="cell-primary">{{ node.name }}</td>
              <td>
                <span class="badge" :class="node.ready ? 'badge-online' : 'badge-offline'">
                  {{ node.ready ? '就绪' : '未就绪' }}
                </span>
              </td>
              <td>{{ node.roles || '-' }}</td>
              <td>{{ node.version || '-' }}</td>
              <td>{{ node.internal_ip || '-' }}</td>
              <td>
                <span v-if="mappedServer(node)" class="badge badge-online">{{ mappedServer(node).name }}</span>
                <span v-else class="badge badge-offline">未映射</span>
              </td>
              <td>{{ node.cpu_cores || '-' }}核</td>
              <td>{{ node.memory_mb ? (node.memory_mb / 1024).toFixed(1) + 'G' : '-' }}</td>
              <td>
                <div class="btn-group action-cell">
                  <button class="btn btn-sm" :disabled="checkingNode === node.name" @click="openDrain(node)">{{ checkingNode === node.name ? '检查中...' : '驱逐' }}</button>
                  <button class="btn btn-sm btn-danger" :disabled="checkingNode === node.name" @click="openRemove(node)">移出</button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <div v-if="drainTarget" class="overlay" @click.self="drainTarget = null">
      <div class="modal">
        <h2 class="modal-title">驱逐节点</h2>
        <p class="modal-copy">节点会先停止接收新 Pod，再通过 Kubernetes Eviction API 迁移可安全中断的工作负载。</p>
        <div class="drain-summary"><span>将迁移 {{ drainTarget.plan?.evictable?.length || 0 }} 个 Pod</span><span>跳过 {{ drainTarget.plan?.skipped?.length || 0 }} 个 Pod</span></div>
        <div v-if="drainTarget.plan?.evictable?.length" class="drain-pods"><strong>将请求迁移</strong><small v-for="pod in drainTarget.plan.evictable" :key="pod.namespace + pod.name">{{ pod.namespace }}/{{ pod.name }}<template v-if="pod.owner_kind"> · {{ pod.owner_kind }}</template></small></div>
        <div v-if="drainTarget.plan?.skipped?.length" class="drain-pods"><strong>不会迁移</strong><small v-for="pod in drainTarget.plan.skipped" :key="pod.namespace + pod.name">{{ pod.namespace }}/{{ pod.name }}: {{ pod.reason }}</small></div>
        <div v-if="drainTarget.plan?.requires_empty_dir_confirmation?.length" class="drain-warning"><strong>本地临时数据</strong><small v-for="pod in drainTarget.plan.requires_empty_dir_confirmation" :key="pod.namespace + pod.name">{{ pod.namespace }}/{{ pod.name }}</small><label class="check-row"><input v-model="deleteEmptyDirData" type="checkbox" /> 允许删除 emptyDir 临时数据</label></div>
        <div v-if="drainTarget.plan?.blocked?.length" class="drain-blockers"><strong>当前不能自动驱逐</strong><small v-for="pod in drainTarget.plan.blocked" :key="pod.namespace + pod.name">{{ pod.namespace }}/{{ pod.name }}: {{ pod.reason }}</small></div>
        <div class="modal-actions">
          <button class="btn" @click="drainTarget = null">取消</button>
          <button class="btn btn-danger" :disabled="draining || hasDrainBlocker || needsEmptyDirConfirmation" @click="doDrain">{{ draining ? '驱逐中...' : '确认驱逐' }}</button>
        </div>
      </div>
    </div>

    <div v-if="removeTarget" class="overlay" @click.self="removeTarget = null">
      <div class="modal">
        <h2 class="modal-title">移出集群</h2>
        <p class="modal-copy">移出仅删除 Kubernetes Node 记录。请先在宿主机停止 k3s 或 k3s-agent，并完成驱逐。</p>
        <div v-if="removeTarget.check?.blockers?.length" class="drain-blockers"><strong>尚不能移出</strong><small v-for="(blocker, index) in removeTarget.check.blockers" :key="index">{{ blocker.namespace ? `${blocker.namespace}/${blocker.name}: ` : '' }}{{ blocker.reason }}</small></div>
        <div class="modal-actions">
          <button class="btn" @click="removeTarget = null">取消</button>
          <button class="btn btn-danger" :disabled="removing || !removeTarget.check?.can_remove" @click="doRemoveNode">{{ removing ? '移出中...' : '确认移出' }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { api } from '../api/index.js'

const nodes = ref([])
const servers = ref([])
const error = ref('')
const drainTarget = ref(null)
const removeTarget = ref(null)
const checkingNode = ref('')
const deleteEmptyDirData = ref(false)
const draining = ref(false)
const removing = ref(false)

const serverLookup = computed(() => servers.value)

onMounted(() => {
  fetchData()
})

async function fetchData() {
  error.value = ''
  try {
    const [nodeList, serverList] = await Promise.all([
      api.get('/nodes'),
      api.get('/servers'),
    ])
    nodes.value = nodeList || []
    servers.value = serverList || []
  } catch (e) {
    error.value = e.message || '加载节点失败'
  }
}

function mappedServer(node) {
  return serverLookup.value.find(server => {
    // 优先用 IP 匹配（server.host === node.internal_ip）
    if (server.host && server.host === node.internal_ip) return true
    // fallback 用 k8s_node_name（忽略大小写）
    if (server.k8s_node_name && server.k8s_node_name.toLowerCase() === node.name.toLowerCase()) return true
    return false
  })
}

const hasDrainBlocker = computed(() => (drainTarget.value?.plan?.blocked?.length || 0) > 0)
const needsEmptyDirConfirmation = computed(() => (drainTarget.value?.plan?.requires_empty_dir_confirmation?.length || 0) > 0 && !deleteEmptyDirData.value)

async function openDrain(node) {
  checkingNode.value = node.name
  error.value = ''
  try {
    const plan = await api.get(`/nodes/${node.name}/drain-plan`)
    deleteEmptyDirData.value = false
    drainTarget.value = { ...node, plan }
  } catch (e) { error.value = e.message || '检查驱逐条件失败' } finally { checkingNode.value = '' }
}

async function doDrain() {
  if (!drainTarget.value) return
  draining.value = true
  try {
    const result = await api.post(`/nodes/${drainTarget.value.name}/drain`, { delete_empty_dir_data: deleteEmptyDirData.value })
    drainTarget.value = null
    await fetchData()
    if (result.pending?.length) error.value = `有 ${result.pending.length} 个 Pod 受 PodDisruptionBudget 保护，稍后可再次执行驱逐`
  } catch (e) { error.value = e.message || '驱逐失败' } finally { draining.value = false }
}

async function openRemove(node) {
  checkingNode.value = node.name
  error.value = ''
  try {
    const check = await api.get(`/nodes/${node.name}/removal-check`)
    removeTarget.value = { ...node, check }
  } catch (e) { error.value = e.message || '检查移出条件失败' } finally { checkingNode.value = '' }
}

async function doRemoveNode() {
  if (!removeTarget.value) return
  removing.value = true
  try {
    await api.delete(`/nodes/${removeTarget.value.name}`)
    removeTarget.value = null
    await fetchData()
  } catch (e) { error.value = e.message || '移出失败' } finally { removing.value = false }
}
</script>

<style scoped>
.section-copy {
  margin: 0;
  color: var(--text-secondary);
  font-size: 13px;
  line-height: 1.6;
}
.modal-copy{margin:0;color:var(--text-secondary);font-size:13px;line-height:1.6}.drain-summary{display:flex;gap:8px;flex-wrap:wrap;margin-top:16px}.drain-summary span{padding:5px 7px;border-radius:var(--radius-control);background:var(--surface-subtle);color:var(--text-secondary);font-size:11px}.drain-pods,.drain-warning,.drain-blockers{display:grid;gap:6px;margin-top:16px;padding:10px;border-radius:var(--radius-control);font-size:12px}.drain-pods{max-height:132px;overflow:auto;background:var(--surface-subtle);color:var(--text-secondary)}.drain-warning{background:var(--warning-surface);color:var(--warning)}.drain-blockers{background:var(--danger-surface);color:var(--danger)}.drain-pods small,.drain-warning small,.drain-blockers small{overflow-wrap:anywhere}.check-row{display:flex;align-items:center;gap:8px;margin-top:4px;color:var(--text-primary);font-size:12px}
</style>
