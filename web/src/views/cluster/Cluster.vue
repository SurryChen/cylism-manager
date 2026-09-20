<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">集群节点</h1>
    </div>

    <SurfaceCard as="div" class="section-gap">
      <p class="section-copy">
        节点状态直接来自已连接的 Kubernetes 集群。当前平台：<strong>{{ platformLabel }}</strong><template v-if="platform.version"> · {{ platform.version }}</template>。
        <template v-if="platform.distribution === 'k3s'">K3s 集群支持工作节点加入流程；</template>服务器绑定和节点维护适用于 Kubernetes 兼容集群。
      </p>
    </SurfaceCard>

    <div v-if="error" class="k8s-banner k8s-banner-warn section-gap">⚠ {{ error }}</div>

    <SurfaceCard as="div">
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
                <span class="badge" :class="nodeHealthClass(node)">
                  {{ nodeHealthLabel(node) }}
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
                  <button class="btn btn-sm" :data-testid="`manage-labels-${node.name}`" :disabled="checkingNode === node.name" @click="openLabels(node)">标签</button>
                  <button v-if="node.evicted" class="btn btn-sm btn-primary" :data-testid="`rejoin-${node.name}`" :disabled="rejoiningNode === node.name" @click="rejoinNode(node)">{{ rejoiningNode === node.name ? '加入中...' : '重新加入' }}</button>
                  <template v-else>
                    <button class="btn btn-sm" :disabled="checkingNode === node.name" @click="openDrain(node)">{{ checkingNode === node.name ? '检查中...' : '驱逐' }}</button>
                    <button v-if="canForceDrain(node)" class="btn btn-sm btn-danger" :data-testid="`force-drain-${node.name}`" :disabled="checkingNode === node.name" @click="openForceDrain(node)">强制驱逐</button>
                  </template>
                  <button class="btn btn-sm btn-danger" :disabled="checkingNode === node.name" @click="openRemove(node)">移出</button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </SurfaceCard>
    <div v-if="rejoinError" class="k8s-banner k8s-banner-warn section-gap">{{ rejoinError }}</div>

    <div v-if="drainTarget" class="overlay" @click.self="closeDrain">
      <div class="modal">
        <h2 class="modal-title">驱逐节点</h2>
        <p class="modal-copy">节点会先停止接收新 Pod，再通过 Kubernetes Eviction API 迁移可安全中断的工作负载。</p>
        <div class="drain-summary"><span>将迁移 {{ drainTarget.plan?.evictable?.length || 0 }} 个 Pod</span><span>跳过 {{ drainTarget.plan?.skipped?.length || 0 }} 个 Pod</span></div>
        <div v-if="drainTarget.plan?.evictable?.length" class="drain-pods"><strong>将请求迁移</strong><small v-for="pod in drainTarget.plan.evictable" :key="pod.namespace + pod.name">{{ pod.namespace }}/{{ pod.name }}<template v-if="pod.owner_kind"> · {{ pod.owner_kind }}</template></small></div>
        <div v-if="drainTarget.plan?.skipped?.length" class="drain-pods"><strong>不会迁移</strong><small v-for="pod in drainTarget.plan.skipped" :key="pod.namespace + pod.name">{{ pod.namespace }}/{{ pod.name }}: {{ pod.reason }}</small></div>
        <div v-if="drainTarget.plan?.requires_empty_dir_confirmation?.length" class="drain-warning"><strong>本地临时数据</strong><small v-for="pod in drainTarget.plan.requires_empty_dir_confirmation" :key="pod.namespace + pod.name">{{ pod.namespace }}/{{ pod.name }}</small><label class="check-row"><input v-model="deleteEmptyDirData" type="checkbox" /> 允许删除 emptyDir 临时数据</label></div>
        <div v-if="drainTarget.plan?.blocked?.length" class="drain-blockers"><strong>当前不能自动驱逐</strong><small v-for="pod in drainTarget.plan.blocked" :key="pod.namespace + pod.name">{{ pod.namespace }}/{{ pod.name }}: {{ pod.reason }}</small></div>
        <p v-if="drainError" class="form-error">{{ drainError }}</p>
        <div class="modal-actions">
          <button class="btn" @click="closeDrain">取消</button>
          <button class="btn btn-danger" :disabled="draining || hasDrainBlocker || needsEmptyDirConfirmation" @click="doDrain">{{ draining ? '驱逐中...' : '确认驱逐' }}</button>
        </div>
      </div>
    </div>

    <div v-if="forceDrainTarget" class="overlay" @click.self="closeForceDrain">
      <div class="modal force-drain-modal">
        <h2 class="modal-title">故障节点强制驱逐</h2>
        <p class="modal-copy">此节点已超过 5 分钟未上报状态。强制驱逐会绕过 PodDisruptionBudget，直接删除可由控制器重建的 Pod。</p>
        <div v-if="forceDrainTarget.plan?.evictable?.length" class="drain-pods"><strong>将强制删除并重建</strong><small v-for="pod in forceDrainTarget.plan.evictable" :key="pod.namespace + pod.name">{{ pod.namespace }}/{{ pod.name }}<template v-if="pod.owner_kind"> · {{ pod.owner_kind }}</template></small></div>
        <div v-if="forceDrainTarget.plan?.skipped?.length" class="drain-pods"><strong>不会处理</strong><small v-for="pod in forceDrainTarget.plan.skipped" :key="pod.namespace + pod.name">{{ pod.namespace }}/{{ pod.name }}: {{ pod.reason }}</small></div>
        <div v-if="forceDrainTarget.plan?.blocked?.length" class="drain-blockers"><strong>不会删除无控制器 Pod</strong><small v-for="pod in forceDrainTarget.plan.blocked" :key="pod.namespace + pod.name">{{ pod.namespace }}/{{ pod.name }}: {{ pod.reason }}</small></div>
        <div v-if="forceDrainTarget.plan?.requires_empty_dir_confirmation?.length" class="drain-warning"><strong>本地临时数据</strong><small v-for="pod in forceDrainTarget.plan.requires_empty_dir_confirmation" :key="pod.namespace + pod.name">{{ pod.namespace }}/{{ pod.name }}</small><label class="check-row"><input v-model="forceDeleteEmptyDirData" type="checkbox" /> 允许删除 emptyDir 临时数据</label></div>
        <label class="check-row force-ack"><input v-model="forceAcknowledged" data-testid="force-drain-acknowledge" type="checkbox" /> 我理解此操作会绕过 PodDisruptionBudget</label>
        <div class="form-group compact-field"><label class="form-label">输入节点名确认</label><input v-model.trim="forceConfirmNodeName" class="form-input" data-testid="force-drain-node-name" :placeholder="forceDrainTarget.name" /></div>
        <p v-if="forceDrainError" class="form-error">{{ forceDrainError }}</p>
        <div class="modal-actions"><button class="btn" :disabled="forceDraining" @click="closeForceDrain">取消</button><button class="btn btn-danger" data-testid="submit-force-drain" :disabled="!canSubmitForceDrain" @click="doForceDrain">{{ forceDraining ? '强制驱逐中...' : '确认强制驱逐' }}</button></div>
      </div>
    </div>

    <div v-if="drainResult" class="overlay" @click.self="drainResult = null">
      <div class="modal result-modal">
        <h2 class="modal-title">{{ drainResult.forced ? '强制驱逐结果' : '驱逐结果' }}</h2>
        <p class="modal-copy">{{ drainResult.forced ? '已提交的删除请求会由控制器在健康节点重建 Pod。' : '已提交的迁移请求会由 Kubernetes 按工作负载策略处理。' }}</p>
        <div v-if="drainResult.evicted?.length" class="drain-pods"><strong>已提交迁移</strong><small v-for="pod in drainResult.evicted" :key="pod.namespace + pod.name">{{ pod.namespace }}/{{ pod.name }}</small></div>
        <div v-if="drainResult.deleted?.length" class="drain-pods"><strong>已提交强制删除</strong><small v-for="pod in drainResult.deleted" :key="pod.namespace + pod.name">{{ pod.namespace }}/{{ pod.name }}: {{ pod.reason }}</small></div>
        <div v-if="drainResult.pending?.length" class="drain-blockers"><strong>尚未迁移</strong><small v-for="pod in drainResult.pending" :key="pod.namespace + pod.name">{{ pod.namespace }}/{{ pod.name }}: {{ pod.reason }}</small></div>
        <div v-if="drainResult.failed?.length" class="drain-blockers"><strong>删除失败</strong><small v-for="pod in drainResult.failed" :key="pod.namespace + pod.name">{{ pod.namespace }}/{{ pod.name }}: {{ pod.reason }}</small></div>
        <div v-if="drainResult.blocked?.length" class="drain-blockers"><strong>未删除</strong><small v-for="pod in drainResult.blocked" :key="pod.namespace + pod.name">{{ pod.namespace }}/{{ pod.name }}: {{ pod.reason }}</small></div>
        <div v-if="drainResult.skipped?.length" class="drain-pods"><strong>已跳过</strong><small v-for="pod in drainResult.skipped" :key="pod.namespace + pod.name">{{ pod.namespace }}/{{ pod.name }}: {{ pod.reason }}</small></div>
        <div class="modal-actions"><button class="btn btn-primary" @click="drainResult = null">完成</button></div>
      </div>
    </div>

    <div v-if="removeTarget" class="overlay" @click.self="closeRemove">
      <div class="modal">
        <h2 class="modal-title">移出集群</h2>
        <p class="modal-copy">移出仅删除 Kubernetes Node 记录。请先在宿主机停止 k3s 或 k3s-agent，并完成驱逐。</p>
        <div v-if="removeTarget.check?.blockers?.length" class="drain-blockers"><strong>尚不能移出</strong><small v-for="(blocker, index) in removeTarget.check.blockers" :key="index">{{ blocker.namespace ? `${blocker.namespace}/${blocker.name}: ` : '' }}{{ blocker.reason }}</small></div>
        <p v-if="removeError" class="form-error">{{ removeError }}</p>
        <div class="modal-actions">
          <button class="btn" @click="closeRemove">取消</button>
          <button class="btn btn-danger" :disabled="removing || !removeTarget.check?.can_remove" @click="doRemoveNode">{{ removing ? '移出中...' : '确认移出' }}</button>
        </div>
      </div>
    </div>

    <div v-if="labelsTarget" class="overlay" @click.self="closeLabels">
      <div class="modal labels-modal">
        <h2 class="modal-title">管理节点标签</h2>
        <p class="modal-copy">{{ labelsTarget.displayName }} · {{ labelsTarget.name }}。工作负载选择节点时使用 <code>kubernetes.io/hostname</code> 标签；Kubernetes 和 K3s 系统标签只读。</p>
        <div class="labels-section">
          <strong>系统标签</strong>
          <div v-if="systemLabelEntries.length" class="labels-readonly"><div v-for="([key, value]) in systemLabelEntries" :key="key"><code>{{ key }}</code><span>{{ value || '(空)' }}</span></div></div>
          <p v-else class="empty-inline">无系统标签</p>
        </div>
        <div class="labels-section">
          <div class="labels-heading"><strong>自定义标签</strong><button class="btn btn-sm" type="button" @click="addLabel">添加标签</button></div>
          <div v-if="labelDraft.length" class="label-editor-list"><div v-for="(label, index) in labelDraft" :key="label.id" class="label-editor-row"><input v-model.trim="label.key" class="form-input" placeholder="team" /><input v-model.trim="label.value" class="form-input" placeholder="platform" /><button class="btn btn-sm btn-danger" type="button" title="删除标签" @click="removeLabel(index)">删除</button></div></div>
          <p v-else class="empty-inline">尚未设置自定义标签</p>
        </div>
        <p v-if="labelsError" class="form-error">{{ labelsError }}</p>
        <div class="modal-actions"><button class="btn" :disabled="savingLabels" @click="closeLabels">取消</button><button class="btn btn-primary" :disabled="savingLabels" @click="saveLabels">{{ savingLabels ? '保存中...' : '保存标签' }}</button></div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import {
  drainNode,
  forceDrainNode,
  getClusterInventory,
  getClusterPlatform,
  getNodeDrainPlan,
  getNodeLabels,
  getNodeRemovalCheck,
  rejoinNode as rejoinClusterNode,
  removeClusterNode,
  updateNodeLabels,
} from '../../api/cluster.js'
import { useAsyncResource } from '../../composables/useAsyncResource.js'
import SurfaceCard from '../../components/SurfaceCard.vue'

const nodes = ref([])
const servers = ref([])
const error = ref('')
const drainTarget = ref(null)
const removeTarget = ref(null)
const checkingNode = ref('')
const deleteEmptyDirData = ref(false)
const draining = ref(false)
const removing = ref(false)
const forceDrainTarget = ref(null)
const forceAcknowledged = ref(false)
const forceConfirmNodeName = ref('')
const forceDeleteEmptyDirData = ref(false)
const forceDraining = ref(false)
const drainResult = ref(null)
const rejoiningNode = ref('')
const labelsTarget = ref(null)
const labelDraft = ref([])
const savingLabels = ref(false)
const clusterResource = useAsyncResource(({ signal }) => getClusterInventory({ signal }), [[], []])
const platformResource = useAsyncResource(({ signal }) => getClusterPlatform({ signal }), { distribution: 'unknown', version: '' })
const drainPlanResource = useAsyncResource(({ signal }, nodeName) => getNodeDrainPlan(nodeName, { signal }), null)
const labelsResource = useAsyncResource(({ signal }, nodeName) => getNodeLabels(nodeName, { signal }), null)
const removalCheckResource = useAsyncResource(({ signal }, nodeName) => getNodeRemovalCheck(nodeName, { signal }), null)
const drainError = ref('')
const forceDrainError = ref('')
const labelsError = ref('')
const removeError = ref('')
const rejoinError = ref('')

const serverLookup = computed(() => servers.value)
const platform = computed(() => platformResource.data.value || { distribution: 'unknown', version: '' })
const platformLabel = computed(() => ({ k3s: 'K3s', kubernetes: 'Kubernetes', unknown: '未识别' })[platform.value.distribution] || '未识别')
const systemLabelEntries = computed(() => {
  if (!labelsTarget.value) return []
  const protectedKeys = new Set(labelsTarget.value.protectedKeys || [])
  return Object.entries(labelsTarget.value.labels || {}).filter(([key]) => protectedKeys.has(key)).sort(([left], [right]) => left.localeCompare(right))
})

onMounted(() => {
  fetchData()
})

async function fetchData() {
  error.value = ''
  const [result] = await Promise.all([clusterResource.refresh(), platformResource.refresh()])
  if (result) {
    const [nodeList, serverList] = result
    nodes.value = nodeList || []
    servers.value = serverList || []
  } else if (clusterResource.error.value) error.value = clusterResource.error.value.message || '加载节点失败'
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

function displayNode(node) {
  return mappedServer(node)?.name || node.name
}

function nodeHealthLabel(node) {
  if (node.evicted) return '已驱逐'
  return { ready: '就绪', not_ready: '未就绪', failed: '故障' }[node.health_state] || (node.ready ? '就绪' : '未就绪')
}

function nodeHealthClass(node) {
  if (node.evicted) return 'badge-warn'
  if (node.health_state === 'failed') return 'badge-danger'
  return node.ready ? 'badge-online' : 'badge-offline'
}

function canForceDrain(node) {
  return node.health_state === 'failed' && node.roles === 'worker'
}

const hasDrainBlocker = computed(() => (drainTarget.value?.plan?.blocked?.length || 0) > 0)
const needsEmptyDirConfirmation = computed(() => (drainTarget.value?.plan?.requires_empty_dir_confirmation?.length || 0) > 0 && !deleteEmptyDirData.value)
const needsForceEmptyDirConfirmation = computed(() => (forceDrainTarget.value?.plan?.requires_empty_dir_confirmation?.length || 0) > 0 && !forceDeleteEmptyDirData.value)
const canSubmitForceDrain = computed(() => Boolean(forceDrainTarget.value) && forceAcknowledged.value && forceConfirmNodeName.value === forceDrainTarget.value.name && !needsForceEmptyDirConfirmation.value && !forceDraining.value)

async function openDrain(node) {
  checkingNode.value = node.name
  drainError.value = ''
  try {
    const plan = await drainPlanResource.refresh(node.name)
    if (!plan) {
      drainError.value = drainPlanResource.error.value?.message || ''
      return
    }
    deleteEmptyDirData.value = false
    drainTarget.value = { ...node, plan }
  } catch (e) { drainError.value = e.message || '检查驱逐条件失败' } finally { if (checkingNode.value === node.name) checkingNode.value = '' }
}

function closeDrain() {
  if (draining.value) return
  drainPlanResource.cancel()
  drainTarget.value = null
  drainError.value = ''
}

async function doDrain() {
  if (!drainTarget.value) return
  draining.value = true
  drainError.value = ''
  try {
    const result = await drainNode(drainTarget.value.name, { delete_empty_dir_data: deleteEmptyDirData.value })
    drainTarget.value = null
    drainResult.value = result
    await fetchData()
  } catch (e) { drainError.value = e.message || '驱逐失败' } finally { draining.value = false }
}

async function openForceDrain(node) {
  checkingNode.value = node.name
  forceDrainError.value = ''
  try {
    const plan = await drainPlanResource.refresh(node.name)
    if (!plan) {
      forceDrainError.value = drainPlanResource.error.value?.message || ''
      return
    }
    forceAcknowledged.value = false
    forceConfirmNodeName.value = ''
    forceDeleteEmptyDirData.value = false
    forceDrainTarget.value = { ...node, plan }
  } catch (e) { forceDrainError.value = e.message || '检查故障节点驱逐条件失败' } finally { if (checkingNode.value === node.name) checkingNode.value = '' }
}

function closeForceDrain() {
  if (forceDraining.value) return
  drainPlanResource.cancel()
  forceDrainTarget.value = null
  forceDrainError.value = ''
}

async function doForceDrain() {
  if (!canSubmitForceDrain.value || !forceDrainTarget.value) return
  forceDraining.value = true
  forceDrainError.value = ''
  try {
    const result = await forceDrainNode(forceDrainTarget.value.name, {
      acknowledge_risk: true,
      confirm_node_name: forceConfirmNodeName.value,
      delete_empty_dir_data: forceDeleteEmptyDirData.value,
    })
    forceDrainTarget.value = null
    drainResult.value = result
    await fetchData()
  } catch (e) { forceDrainError.value = e.message || '故障节点强制驱逐失败' } finally { forceDraining.value = false }
}

async function rejoinNode(node) {
  rejoiningNode.value = node.name
  rejoinError.value = ''
  try {
    await rejoinClusterNode(node.name)
    await fetchData()
  } catch (e) { rejoinError.value = e.message || '节点重新加入失败' } finally { rejoiningNode.value = '' }
}

async function openLabels(node) {
  checkingNode.value = node.name
  labelsError.value = ''
  try {
    const result = await labelsResource.refresh(node.name)
    if (!result) {
      labelsError.value = labelsResource.error.value?.message || ''
      return
    }
    const protectedKeys = new Set(result.protected_keys || [])
    labelsTarget.value = { name: node.name, displayName: displayNode(node), labels: result.labels || {}, protectedKeys: result.protected_keys || [] }
    labelDraft.value = Object.entries(result.labels || {}).filter(([key]) => !protectedKeys.has(key)).sort(([left], [right]) => left.localeCompare(right)).map(([key, value], index) => ({ id: `${key}-${index}`, key, value }))
  } catch (e) { labelsError.value = e.message || '读取节点标签失败' } finally { if (checkingNode.value === node.name) checkingNode.value = '' }
}

function closeLabels(force = false) {
  if (savingLabels.value && !force) return
  labelsResource.cancel()
  labelsTarget.value = null
  labelDraft.value = []
  labelsError.value = ''
}

function addLabel() {
  labelDraft.value.push({ id: `new-${Date.now()}-${labelDraft.value.length}`, key: '', value: '' })
}

function removeLabel(index) {
  labelDraft.value.splice(index, 1)
}

async function saveLabels() {
  if (!labelsTarget.value) return
  const original = Object.entries(labelsTarget.value.labels || {}).filter(([key]) => !(labelsTarget.value.protectedKeys || []).includes(key)).reduce((result, [key, value]) => ({ ...result, [key]: value }), {})
  const next = {}
  for (const label of labelDraft.value) {
    if (!label.key || Object.prototype.hasOwnProperty.call(next, label.key)) {
      labelsError.value = '自定义标签键不能为空且不能重复'
      return
    }
    next[label.key] = label.value
  }
  const set = Object.fromEntries(Object.entries(next).filter(([key, value]) => original[key] !== value))
  const remove = Object.keys(original).filter(key => !Object.prototype.hasOwnProperty.call(next, key))
  savingLabels.value = true
  labelsError.value = ''
  try {
    await updateNodeLabels(labelsTarget.value.name, { set, remove })
    closeLabels(true)
    await fetchData()
  } catch (e) { labelsError.value = e.message || '保存节点标签失败' } finally { savingLabels.value = false }
}

async function openRemove(node) {
  checkingNode.value = node.name
  removeError.value = ''
  try {
    const check = await removalCheckResource.refresh(node.name)
    if (!check) {
      removeError.value = removalCheckResource.error.value?.message || ''
      return
    }
    removeTarget.value = { ...node, check }
  } catch (e) { removeError.value = e.message || '检查移出条件失败' } finally { if (checkingNode.value === node.name) checkingNode.value = '' }
}

async function doRemoveNode() {
  if (!removeTarget.value) return
  removing.value = true
  removeError.value = ''
  try {
    await removeClusterNode(removeTarget.value.name)
    removeTarget.value = null
    await fetchData()
  } catch (e) { removeError.value = e.message || '移出失败' } finally { removing.value = false }
}

function closeRemove() {
  if (removing.value) return
  removalCheckResource.cancel()
  removeTarget.value = null
  removeError.value = ''
}
</script>

<style scoped>
.section-copy {
  margin: 0;
  color: var(--text-secondary);
  font-size: 13px;
  line-height: 1.6;
}
.modal-copy{margin:0;color:var(--text-secondary);font-size:13px;line-height:1.6}.drain-summary{display:flex;gap:8px;flex-wrap:wrap;margin-top:16px}.drain-summary span{padding:5px 7px;border-radius:var(--radius-control);background:var(--surface-subtle);color:var(--text-secondary);font-size:11px}.drain-pods,.drain-warning,.drain-blockers{display:grid;gap:6px;margin-top:16px;padding:10px;border-radius:var(--radius-control);font-size:12px}.drain-pods{max-height:132px;overflow:auto;background:var(--surface-subtle);color:var(--text-secondary)}.drain-warning{background:var(--warning-surface);color:var(--warning)}.drain-blockers{background:var(--danger-surface);color:var(--danger)}.drain-pods small,.drain-warning small,.drain-blockers small{overflow-wrap:anywhere}.check-row{display:flex;align-items:center;gap:8px;margin-top:4px;color:var(--text-primary);font-size:12px}.force-drain-modal,.result-modal,.labels-modal{width:min(580px,calc(100vw - 32px))}.force-ack{margin-top:16px}.compact-field{margin-top:16px}.labels-section{display:grid;gap:8px;margin-top:18px}.labels-heading{display:flex;align-items:center;justify-content:space-between;gap:12px}.labels-readonly,.label-editor-list{display:grid;gap:6px}.labels-readonly{max-height:180px;overflow:auto;padding:10px;border:1px solid var(--border-muted);border-radius:var(--radius-control);background:var(--surface-subtle)}.labels-readonly>div{display:grid;grid-template-columns:minmax(0,1fr) minmax(100px,1fr);gap:12px;font-size:12px}.labels-readonly code,.labels-readonly span{overflow-wrap:anywhere}.labels-readonly span{color:var(--text-secondary)}.label-editor-row{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr) auto;gap:8px}.empty-inline{margin:0;color:var(--text-muted);font-size:12px}@media(max-width:600px){.label-editor-row,.labels-readonly>div{grid-template-columns:1fr}.label-editor-row .btn{width:100%}}
</style>
