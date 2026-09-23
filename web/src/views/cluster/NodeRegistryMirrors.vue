<template>
  <div>
    <TabbedWorkspaceCard class="rule-workspace" data-testid="node-registry-mirror-workspace">
      <template #meta><span>{{ enabledMirrorCount }} 条已启用规则</span></template>
      <template #actions><div class="btn-group workspace-actions"><button class="btn" data-testid="open-node-config" @click="openNodeConfig">查看节点配置</button><a class="btn proxy-management-link" href="#/delivery/registry?tab=registry-proxy">管理 Registry Proxy</a><button class="btn btn-primary" @click="openCreate">+ 新建镜像源</button></div></template>
      <div v-if="loaded && mirrors.length" class="table-wrap"><table class="data-table mirror-rule-list"><colgroup><col class="mirror-name-column" /><col class="mirror-registry-column" /><col class="mirror-endpoint-column" /><col class="mirror-verify-column" /><col class="mirror-status-column" /><col class="mirror-apply-column" /><col class="mirror-actions-column" /></colgroup><thead><tr><th>名称</th><th>Registry</th><th>镜像地址</th><th>验证状态</th><th>状态</th><th>最近应用</th><th>操作</th></tr></thead><tbody><tr v-for="mirror in mirrors" :key="mirror.id"><td class="cell-primary"><span class="mirror-cell-truncate mirror-name" tabindex="0" :aria-label="mirrorNameTooltip(mirror)" @mouseenter="showMirrorTooltip(mirrorNameTooltip(mirror), $event)" @mousemove="moveMirrorTooltip" @mouseleave="hideMirrorTooltip" @focus="showMirrorTooltip(mirrorNameTooltip(mirror), $event)" @blur="hideMirrorTooltip">{{ mirror.name }}</span></td><td><span class="mirror-cell-truncate mirror-registry" tabindex="0" :aria-label="mirror.registry" @mouseenter="showMirrorTooltip(mirror.registry, $event)" @mousemove="moveMirrorTooltip" @mouseleave="hideMirrorTooltip" @focus="showMirrorTooltip(mirror.registry, $event)" @blur="hideMirrorTooltip">{{ mirror.registry }}</span></td><td><span class="mirror-cell-truncate mirror-endpoints" tabindex="0" :aria-label="endpointText(mirror)" @mouseenter="showMirrorTooltip(endpointText(mirror), $event)" @mousemove="moveMirrorTooltip" @mouseleave="hideMirrorTooltip" @focus="showMirrorTooltip(endpointText(mirror), $event)" @blur="hideMirrorTooltip">{{ endpointText(mirror) }}</span></td><td><span class="badge" :class="verificationClass(mirror.last_verify_status)">{{ verificationLabel(mirror.last_verify_status) }}</span></td><td><span class="badge" :class="mirror.enabled ? 'badge-online' : 'badge-offline'">{{ mirror.enabled ? '已启用' : '已停用' }}</span></td><td><button v-if="mirror.node_statuses?.length" class="btn btn-sm" @click="openApplyRecords(mirror)">查看记录</button><span v-else>-</span></td><td><div class="row-actions"><button class="btn btn-sm" :data-testid="`verify-node-registry-mirror-${mirror.id}`" :disabled="verifyingID === mirror.id" @click="verifyMirror(mirror)">{{ verifyingID === mirror.id ? '检测中...' : '检测' }}</button><button class="btn btn-sm btn-primary" :data-testid="`apply-node-registry-mirror-${mirror.id}`" :disabled="isApplying(mirror.id)" @click="openApply(mirror)">{{ isApplying(mirror.id) ? '应用中...' : '选择节点应用' }}</button><button class="btn btn-sm" :data-testid="`edit-node-registry-mirror-${mirror.id}`" @click="openEdit(mirror)">编辑</button><button class="btn btn-sm btn-danger" @click="deleteTarget = mirror">删除</button></div></td></tr></tbody></table></div>
      <p v-else-if="loaded" class="empty-inline">还没有镜像规则。新建规则后，可选择节点下发。</p>
    </TabbedWorkspaceCard>

    <ErrorNoticeModal :open="Boolean(pageError)" :title="pageErrorTitle" :message="pageError" @close="dismissPageError" />

    <Teleport to="body"><div v-if="mirrorTooltip" class="mirror-value-tooltip" role="tooltip" :style="{ left: `${mirrorTooltip.x}px`, top: `${mirrorTooltip.y}px` }">{{ mirrorTooltip.text }}</div></Teleport>

    <Teleport to="body"><div v-if="nodeConfigOpen" class="overlay" @click.self="closeNodeConfig"><section class="modal node-config-modal" role="dialog" aria-modal="true" aria-label="查看节点配置"><header class="drawer-header"><div><h2 class="modal-title">节点配置</h2><p>仅展示脱敏后的 Registry 配置摘要。</p></div><button class="icon-button" title="关闭" aria-label="关闭" @click="closeNodeConfig">×</button></header><div class="node-config-controls"><label class="form-group"><span class="form-label">集群节点</span><select v-model.number="selectedConfigServerID" class="form-input"><option :value="0">选择节点</option><option v-for="server in clusterServers" :key="server.id" :value="server.id">{{ server.name }} · {{ server.k8s_node_name || server.host }}</option></select></label><div class="btn-group"><button class="btn btn-primary" data-testid="inspect-selected-node-config" :disabled="inspecting || !selectedConfigServerID" @click="inspectActualConfiguration(selectedConfigServerID)">{{ inspecting ? '检查中...' : '检查配置' }}</button><button class="btn" :disabled="inspecting" @click="inspectActualConfiguration()">检查全部</button></div></div><div v-if="inspectionError" class="k8s-banner k8s-banner-warn">{{ inspectionError }}</div><p v-if="inspection" class="inspection-time">最近检查：{{ inspectionTime }}</p><div v-if="inspection?.nodes?.length" class="node-observation-list"><button v-for="node in inspection.nodes" :key="node.server_id" class="node-observation-row" :class="{ selected: detailNode?.server_id === node.server_id }" @click="detailNode = node"><span><strong>{{ node.name }}</strong><small>{{ node.detail || nodeStateDescription(node.state) }}</small></span><span class="badge" :class="nodeStateClass(node.state)">{{ nodeStateLabel(node.state) }}</span></button></div><section v-if="detailNode" class="config-detail"><div class="config-detail-heading"><div><h3>{{ detailNode.name }}</h3><p>{{ nodeDifference(detailNode) }}</p></div><button class="btn btn-sm btn-danger" :disabled="restartingNode" @click="restartTarget = detailNode">重启 K3s 服务</button></div><div class="config-diff"><div><h3>期望规则</h3><p v-if="!detailNode.expected?.length" class="empty-inline">没有启用规则</p><dl v-else><div v-for="config in detailNode.expected" :key="`expected-${config.registry}`"><dt>{{ config.registry }}</dt><dd>{{ config.endpoints.join('、') || '未配置镜像地址' }}<small>{{ config.auth_configured ? '已配置认证' : '未配置认证' }} · {{ config.insecure_skip_verify ? '跳过 TLS 校验' : '校验证书' }}</small></dd></div></dl></div><div><h3>节点当前配置</h3><p v-if="!detailNode.actual?.length" class="empty-inline">未发现有效镜像规则</p><dl v-else><div v-for="config in detailNode.actual" :key="`actual-${config.registry}`"><dt>{{ config.registry }}</dt><dd>{{ config.endpoints.join('、') || '未配置镜像地址' }}<small>{{ config.auth_configured ? '已配置认证' : '未配置认证' }} · {{ config.insecure_skip_verify ? '跳过 TLS 校验' : '校验证书' }}</small></dd></div></dl></div></div></section></section></div></Teleport>

    <Teleport to="body"><div v-if="restartTarget" class="overlay" @click.self="restartTarget = null"><section class="modal restart-confirm-modal" role="alertdialog" aria-modal="true"><h2 class="modal-title">重启 K3s 服务</h2><p class="confirm-copy">将重启 {{ restartTarget.name }} 上的 K3s 服务，节点工作负载可能短暂不可用。不会重启整台主机，也不会修改 Registry 配置。</p><div class="modal-actions"><button class="btn" @click="restartTarget = null">取消</button><button class="btn btn-danger" data-testid="confirm-restart-k3s" :disabled="restartingNode" @click="restartK3s">{{ restartingNode ? '重启中...' : '确认重启' }}</button></div></section></div></Teleport>

    <Teleport to="body"><div v-if="recordsTarget" class="overlay" @click.self="recordsTarget = null"><section class="modal records-modal" role="dialog" aria-modal="true"><h2 class="modal-title">{{ recordsTarget.name }} 应用记录</h2><div class="apply-record-list"><div v-for="status in recordsTarget.node_statuses" :key="status.server_id"><strong>{{ status.server?.name || status.server_id }}</strong><span class="badge" :class="status.status === 'success' ? 'badge-online' : status.status === 'failed' ? 'badge-danger' : 'badge-offline'">{{ applyStatusLabel(status.status) }}</span><small>{{ status.detail || '-' }}</small></div></div><div class="modal-actions"><button class="btn btn-primary" @click="recordsTarget = null">关闭</button></div></section></div></Teleport>

    <div v-if="applyTarget" class="overlay" @click.self="closeApply">
      <div class="modal apply-modal">
        <h2 class="modal-title">选择应用节点</h2>
        <p class="confirm-copy">只会修改选中的节点。平台会将全部启用的镜像源规则渲染为完整的 registries.yaml，先备份旧配置，再重启对应 K3s 服务。</p>
        <div class="node-selection">
          <label v-for="server in clusterServers" :key="server.id" class="node-option">
            <input v-model="selectedServerIDs" type="checkbox" :value="server.id" />
            <span><strong>{{ server.name }}</strong><small>{{ server.k8s_node_name || server.host }} · {{ server.cluster_role }}</small></span>
          </label>
        </div>
        <p v-if="!clusterServers.length" class="empty-inline">没有找到已加入集群的服务器</p>
        <div class="modal-actions"><button class="btn" @click="closeApply">取消</button><button class="btn btn-primary" data-testid="submit-node-registry-apply" :disabled="applying || !selectedServerIDs.length" @click="applyMirror">{{ applying ? '提交中...' : `应用到 ${selectedServerIDs.length} 个节点` }}</button></div>
      </div>
    </div>

    <Teleport to="body"><div v-if="showModal" class="overlay mirror-overlay" @click.self="closeModal">
      <div class="modal mirror-modal">
        <h2 class="modal-title">{{ editing ? '编辑节点镜像源' : '新建节点镜像源' }}</h2>
        <form @submit.prevent="save">
          <div class="form-group">
            <label class="form-label">名称</label>
            <input v-model.trim="form.name" class="form-input" required placeholder="docker-hub-mirror" />
          </div>
          <div class="form-group">
            <label class="form-label">Registry</label>
            <input v-model.trim="form.registry" class="form-input" required placeholder="docker.io" />
          </div>
          <div class="form-group">
            <label class="form-label">验证镜像</label>
            <input v-model.trim="form.verification_image" class="form-input" required placeholder="docker.io/library/busybox:1.36" />
            <p class="form-hint">使用该镜像的 Manifest 检测每个代理端点；镜像 Registry 必须与上方 Registry 一致。</p>
          </div>
          <div class="form-group">
            <label class="form-label">镜像地址</label>
            <textarea v-model="form.endpoints" class="form-input" required placeholder="https://mirror.example.com" />
            <p class="form-hint">每行一个 HTTP 或 HTTPS 地址。</p>
          </div>
          <div class="form-row">
            <div class="form-group"><label class="form-label">账号</label><input v-model.trim="form.username" class="form-input" /></div>
            <div class="form-group"><label class="form-label">密码或 Token</label><input v-model="form.credential" type="password" class="form-input" :placeholder="editing ? '留空则不修改' : ''" /></div>
          </div>
          <label class="check-row"><input v-model="form.insecure_skip_verify" type="checkbox" /> 跳过 TLS 证书校验</label>
          <label class="check-row"><input v-model="form.enabled" type="checkbox" /> 启用此规则</label>
          <div class="modal-actions">
            <button type="button" class="btn" @click="closeModal">取消</button>
            <button class="btn btn-primary" :disabled="submitting">{{ submitting ? '保存中...' : '保存' }}</button>
          </div>
        </form>
      </div>
    </div></Teleport>

    <Teleport to="body"><div v-if="actionNotice" class="overlay mirror-notice-overlay" @click.self="actionNotice = null">
      <section class="modal mirror-notice-modal" role="alertdialog" aria-modal="true" :aria-label="actionNotice.title">
        <h2 class="modal-title">{{ actionNotice.title }}</h2>
        <p class="confirm-copy">{{ actionNotice.message }}</p>
        <div class="modal-actions"><button class="btn" :class="actionNotice.type === 'error' ? 'btn-danger' : 'btn-primary'" @click="actionNotice = null">知道了</button></div>
      </section>
    </div></Teleport>

    <div v-if="deleteTarget" class="overlay" @click.self="deleteTarget = null">
      <div class="modal">
        <h2 class="modal-title">删除镜像源</h2>
        <p class="confirm-copy">删除后不会自动恢复节点上的现有配置。</p>
        <div class="modal-actions"><button class="btn" @click="deleteTarget = null">取消</button><button class="btn btn-danger" @click="remove">删除</button></div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { getServers } from '../../api/servers.js'
import { createNodeRegistryMirror, applyNodeRegistryMirror, deleteNodeRegistryMirror, getNodeRegistryMirrorApplyStatus, getNodeRegistryMirrors, inspectActualNodeRegistryConfiguration, restartNodeK3sService, updateNodeRegistryMirror, verifyNodeRegistryMirror } from '../../api/node-registry-mirrors.js'
import { useAsyncResource } from '../../composables/useAsyncResource.js'
import { usePolling } from '../../composables/usePolling.js'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import TabbedWorkspaceCard from '../../components/TabbedWorkspaceCard.vue'

const mirrors = ref([])
const loaded = ref(false)
const mirrorError = ref('')
const serverError = ref('')
const pollingError = ref('')
const mutationError = ref('')
const showModal = ref(false)
const editing = ref(null)
const actionNotice = ref(null)
const deleteTarget = ref(null)
const submitting = ref(false)
const applying = ref(false)
const activeApplyIDs = ref([])
const verifyingID = ref(null)
const servers = ref([])
const applyTarget = ref(null)
const selectedServerIDs = ref([])
const inspection = ref(null)
const inspectionError = ref('')
const inspecting = ref(false)
const detailNode = ref(null)
const nodeConfigOpen = ref(false)
const selectedConfigServerID = ref(0)
const restartTarget = ref(null)
const pageError = computed(() => mutationError.value || pollingError.value || serverError.value || mirrorError.value)
const pageErrorTitle = computed(() => {
  if (mutationError.value) return '镜像源操作失败'
  if (pollingError.value) return '读取应用进度失败'
  if (serverError.value) return '加载集群节点失败'
  return '加载节点镜像源失败'
})
const restartingNode = ref(false)
const recordsTarget = ref(null)
const mirrorTooltip = ref(null)
const form = ref(blank())
const mirrorResource = useAsyncResource(({ signal }) => getNodeRegistryMirrors({ signal }), [])
const serverResource = useAsyncResource(({ signal }) => getServers({ signal }), [])
let pollingController = null
const applyPolling = usePolling(async () => {
  if (!activeApplyIDs.value.length) return
  try {
    pollingError.value = ''
    pollingController = new AbortController()
    const updates = await Promise.all(activeApplyIDs.value.map(mirrorID => getNodeRegistryMirrorApplyStatus(mirrorID, { signal: pollingController.signal })))
    const completedIDs = []
    for (const mirror of updates) {
      const index = mirrors.value.findIndex(item => item.id === mirror.id)
      if (index >= 0) mirrors.value[index] = mirror
      if (mirror.last_apply_status !== 'applying') completedIDs.push(mirror.id)
    }
    activeApplyIDs.value = activeApplyIDs.value.filter(mirrorID => !completedIDs.includes(mirrorID))
    if (!activeApplyIDs.value.length) applyPolling.stop()
  } catch (e) { if (e?.name !== 'AbortError') pollingError.value = e.message || '读取节点应用进度失败' }
  finally { pollingController = null }
}, { interval: 2000 })

const clusterServers = computed(() => servers.value.filter(server => server.cluster_role))
const enabledMirrorCount = computed(() => mirrors.value.filter(mirror => mirror.enabled).length)
const inspectionTime = computed(() => inspection.value?.inspected_at ? new Date(inspection.value.inspected_at).toLocaleString('zh-CN', { hour12: false }) : '')

function blank() {
  return { name: '', registry: '', verification_image: '', endpoints: '', username: '', credential: '', insecure_skip_verify: false, enabled: true }
}

function endpointText(mirror) {
  try { return JSON.parse(mirror.endpoints).join(', ') } catch { return mirror.endpoints }
}

function mirrorNameTooltip(mirror) {
  return mirror.verification_image ? `${mirror.name}\n验证镜像：${mirror.verification_image}` : mirror.name
}

function showMirrorTooltip(text, event) {
  if (!text) return
  mirrorTooltip.value = { text, x: 12, y: 12 }
  moveMirrorTooltip(event)
}

function moveMirrorTooltip(event) {
  if (!mirrorTooltip.value || typeof window === 'undefined') return
  const x = Number.isFinite(event?.clientX) ? event.clientX : 12
  const y = Number.isFinite(event?.clientY) ? event.clientY : 12
  mirrorTooltip.value = {
    ...mirrorTooltip.value,
    x: Math.min(x + 14, Math.max(12, window.innerWidth - 432)),
    y: Math.min(y + 16, Math.max(12, window.innerHeight - 252)),
  }
}

function hideMirrorTooltip() {
  mirrorTooltip.value = null
}

function verificationLabel(status) {
  return { succeeded: '可用', failed: '失败' }[status] || '未检测'
}

function verificationClass(status) {
  return { succeeded: 'badge-online', failed: 'badge-danger' }[status] || 'badge-offline'
}

function nodeStateLabel(state) { return { matching: '一致', missing: '缺失', drifted: '有漂移', unsupported: '无法检查', unreachable: '不可达', invalid: '配置无效' }[state] || state }
function nodeStateClass(state) { return { matching: 'badge-online', missing: 'badge-danger', drifted: 'badge-danger', unsupported: 'badge-offline', unreachable: 'badge-offline', invalid: 'badge-danger' }[state] || 'badge-offline' }
function nodeStateDescription(state) { return { matching: '当前配置与所有已启用规则一致', missing: '未发现期望的 Registry 配置', drifted: '当前配置与平台规则不一致', unsupported: '当前节点未配置 SSH 密钥认证', unreachable: '无法通过 SSH 读取节点配置', invalid: '节点 Registry 配置格式无效' }[state] || '检查状态未知' }
function nodeDifference(node) {
  const parts = []
  if (node.missing?.length) parts.push(`缺少 ${node.missing.join('、')}`)
  if (node.changed?.length) parts.push(`变更 ${node.changed.join('、')}`)
  if (node.extra?.length) parts.push(`额外 ${node.extra.join('、')}`)
  return parts.join('；') || (node.state === 'matching' ? '配置一致' : '无可展示的差异')
}
function applyStatusLabel(status) { return { pending: '等待中', applying: '应用中', success: '成功', skipped: '跳过', failed: '失败' }[status] || status }

function isApplying(mirrorID) {
  return activeApplyIDs.value.includes(mirrorID)
}

function dismissPageError() {
  if (mutationError.value) mutationError.value = ''
  else if (pollingError.value) pollingError.value = ''
  else if (serverError.value) serverError.value = ''
  else mirrorError.value = ''
}

async function load() {
  mirrorError.value = ''
  const result = await mirrorResource.refresh()
  if (!result) { mirrorError.value = mirrorResource.error.value?.message || '加载节点镜像源失败'; loaded.value = true; return }
  mirrors.value = result || []
  const pendingIDs = mirrors.value.filter(mirror => mirror.last_apply_status === 'applying').map(mirror => mirror.id)
  if (pendingIDs.length) startApplyPolling(pendingIDs)
  loaded.value = true
}

async function loadServers() {
  serverError.value = ''
  const result = await serverResource.refresh()
  if (result) servers.value = result || []
  else serverError.value = serverResource.error.value?.message || '加载集群节点失败'
}

function openCreate() {
  actionNotice.value = null
  editing.value = null
  form.value = blank()
  showModal.value = true
}

function openEdit(mirror) {
  actionNotice.value = null
  editing.value = mirror
  form.value = {
    name: mirror.name,
    registry: mirror.registry,
    verification_image: mirror.verification_image || '',
    endpoints: JSON.parse(mirror.endpoints).join('\n'),
    username: mirror.username || '',
    credential: '',
    insecure_skip_verify: mirror.insecure_skip_verify,
    enabled: mirror.enabled,
  }
  showModal.value = true
}

function closeModal() {
  showModal.value = false
  editing.value = null
  form.value = blank()
}

async function save() {
  submitting.value = true
  mutationError.value = ''
  const wasEditing = Boolean(editing.value)
  try {
    const payload = { ...form.value, endpoints: form.value.endpoints.split('\n').map(value => value.trim()).filter(Boolean) }
    if (editing.value && !payload.credential) delete payload.credential
    if (editing.value) await updateNodeRegistryMirror(editing.value.id, payload)
    else await createNodeRegistryMirror(payload)
    closeModal()
    await load()
    actionNotice.value = { type: 'success', title: wasEditing ? '节点镜像源已保存' : '节点镜像源已创建', message: '配置已保存。需要下发到节点时，请在列表中点击“选择节点应用”。' }
  } catch (e) {
    actionNotice.value = { type: 'error', title: wasEditing ? '保存节点镜像源失败' : '创建节点镜像源失败', message: e.message || '保存节点镜像源失败' }
  } finally { submitting.value = false }
}

async function verifyMirror(mirror) {
  verifyingID.value = mirror.id
  mutationError.value = ''
  try {
    await verifyNodeRegistryMirror(mirror.id)
    await load()
  } catch (e) { mutationError.value = e.message || '检测节点镜像源失败' } finally { verifyingID.value = null }
}

function openNodeConfig() {
  nodeConfigOpen.value = true
  inspectionError.value = ''
  detailNode.value = null
}

function closeNodeConfig() {
  if (inspecting.value || restartingNode.value) return
  nodeConfigOpen.value = false
  detailNode.value = null
}

async function inspectActualConfiguration(serverID = 0) {
  inspecting.value = true
  inspectionError.value = ''
  try {
    inspection.value = await inspectActualNodeRegistryConfiguration(serverID ? { server_id: serverID } : undefined)
    detailNode.value = inspection.value?.nodes?.[0] || null
  } catch (e) { inspectionError.value = e.message || '检查节点实际配置失败' } finally { inspecting.value = false }
}

async function restartK3s() {
  if (!restartTarget.value) return
  restartingNode.value = true
  try {
    const result = await restartNodeK3sService(restartTarget.value.server_id)
    actionNotice.value = { type: result.status === 'succeeded' ? 'success' : 'error', title: result.status === 'succeeded' ? 'K3s 服务已重启' : 'K3s 服务重启失败', message: result.detail || '操作已完成' }
    restartTarget.value = null
  } catch (e) { actionNotice.value = { type: 'error', title: 'K3s 服务重启失败', message: e.message || '重启节点 K3s 服务失败' } } finally { restartingNode.value = false }
}

function openApplyRecords(mirror) { recordsTarget.value = mirror }

function openApply(mirror) {
  applyTarget.value = mirror
  selectedServerIDs.value = []
}

function closeApply() {
  if (applying.value) return
  applyTarget.value = null
  selectedServerIDs.value = []
}

async function applyMirror() {
  if (!applyTarget.value || !selectedServerIDs.value.length) return
  applying.value = true
  mutationError.value = ''
  try {
    const mirrorID = applyTarget.value.id
    const updated = await applyNodeRegistryMirror(mirrorID, { server_ids: selectedServerIDs.value })
    applyTarget.value = null
    selectedServerIDs.value = []
    const index = mirrors.value.findIndex(mirror => mirror.id === mirrorID)
    if (index >= 0 && updated?.id === mirrorID) mirrors.value[index] = updated
    startApplyPolling([mirrorID])
  } catch (e) { mutationError.value = e.message || '下发节点镜像源失败' } finally { applying.value = false }
}

function startApplyPolling(mirrorIDs) {
  activeApplyIDs.value = [...new Set([...activeApplyIDs.value, ...mirrorIDs])]
  applyPolling.start()
}

function stopApplyPolling() {
  pollingController?.abort()
  pollingController = null
  applyPolling.stop()
}

async function remove() {
  try {
    await deleteNodeRegistryMirror(deleteTarget.value.id)
    deleteTarget.value = null
    await load()
  } catch (e) { mutationError.value = e.message || '删除节点镜像源失败' }
}

onMounted(() => { load(); loadServers() })
onBeforeUnmount(stopApplyPolling)
</script>

<style scoped>
.proxy-management-link { text-decoration:none; }
.inspection-time { margin:5px 0 0; color:var(--text-muted); font-size:12px; }
.mirror-rule-list { min-width:1190px; table-layout:fixed; }.mirror-name-column { width:220px; }.mirror-registry-column { width:170px; }.mirror-endpoint-column { width:260px; }.mirror-verify-column { width:88px; }.mirror-status-column { width:78px; }.mirror-apply-column { width:96px; }.mirror-actions-column { width:278px; }.mirror-rule-list td { height:54px; padding-top:10px; padding-bottom:10px; vertical-align:middle; white-space:nowrap; }.mirror-cell-truncate { display:block; min-width:0; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; cursor:default; }.mirror-cell-truncate:focus-visible { outline:2px solid var(--focus); outline-offset:2px; }.mirror-value-tooltip { position:fixed; z-index:1500; max-width:min(420px,calc(100vw - 24px)); max-height:min(240px,calc(100vh - 24px)); overflow:auto; padding:9px 11px; border:1px solid var(--border); border-radius:var(--radius-control); background:var(--surface-raised); box-shadow:var(--shadow-soft); color:var(--text-primary); font-size:12px; line-height:1.5; pointer-events:none; white-space:pre-wrap; overflow-wrap:anywhere; }.row-actions { display:flex; flex-wrap:nowrap; align-items:center; gap:6px; white-space:nowrap; }
.node-config-modal { width:min(800px,calc(100vw - 32px)); }.drawer-header { display:flex; align-items:flex-start; justify-content:space-between; gap:12px; }.drawer-header .modal-title { margin-bottom:4px; }.drawer-header p { margin:0; color:var(--text-secondary); font-size:12px; }.node-config-controls { display:flex; align-items:flex-end; gap:12px; margin:var(--space-16) 0; }.node-config-controls .form-group { flex:1; margin:0; }.node-observation-list { display:grid; gap:6px; margin-top:var(--space-12); }.node-observation-row { display:flex; width:100%; align-items:center; justify-content:space-between; gap:12px; padding:11px 12px; border:1px solid var(--border-muted); border-radius:var(--radius-control); background:var(--surface-subtle); color:var(--text-primary); font:inherit; text-align:left; cursor:pointer; }.node-observation-row:hover, .node-observation-row.selected { border-color:var(--focus); background:var(--surface-raised); }.node-observation-row > span:first-child { display:grid; min-width:0; gap:3px; }.node-observation-row small { overflow:hidden; color:var(--text-secondary); font-size:11px; text-overflow:ellipsis; white-space:nowrap; }.config-detail { margin-top:var(--space-16); padding-top:var(--space-16); border-top:1px solid var(--border-muted); }.config-detail-heading { display:flex; align-items:flex-start; justify-content:space-between; gap:12px; }.config-detail-heading h3 { margin:0; font-size:15px; }.config-detail-heading p { margin:4px 0 0; color:var(--text-secondary); font-size:12px; }.config-diff { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:16px; margin-top:var(--space-16); }.config-diff h3 { margin:0 0 8px; font-size:13px; }.config-diff dl { margin:0; border-top:1px solid var(--border-muted); }.config-diff dl > div { padding:10px 0; border-bottom:1px solid var(--border-muted); }.config-diff dt { font-weight:600; overflow-wrap:anywhere; }.config-diff dd { display:grid; gap:4px; margin:5px 0 0; color:var(--text-secondary); font-size:12px; overflow-wrap:anywhere; }.config-diff small { color:var(--text-muted); }.empty-inline { color:var(--text-muted); font-size:13px; }.records-modal { width:min(560px,calc(100vw - 32px)); }.apply-record-list { display:grid; gap:0; border-top:1px solid var(--border-muted); }.apply-record-list > div { display:grid; grid-template-columns:minmax(120px,1fr) auto; gap:8px 12px; padding:12px 0; border-bottom:1px solid var(--border-muted); }.apply-record-list small { grid-column:1 / -1; color:var(--text-secondary); font-size:12px; overflow-wrap:anywhere; }
.mirror-overlay { align-items: flex-start; overflow-y: auto; padding: 72px 16px 24px; }
.mirror-modal { width:min(580px,calc(100vw - 32px)); max-height: calc(100dvh - 96px); margin: 0 auto; }
.mirror-notice-overlay { z-index: 1300; }
.mirror-notice-modal { width: min(420px, calc(100vw - 32px)); }
.apply-modal { width:min(520px,calc(100vw - 32px)); }
.node-selection { display:grid; gap:8px; max-height:300px; overflow:auto; margin-top:16px; }
.node-option { display:flex; align-items:flex-start; gap:10px; padding:10px; border:1px solid var(--border-muted); border-radius:var(--radius-control); background:var(--surface-subtle); cursor:pointer; }
.node-option span { display:grid; gap:3px; min-width:0; }.node-option small { color:var(--text-secondary); font-size:11px; overflow-wrap:anywhere; }
.form-hint, .confirm-copy { color:var(--text-muted); font-size:12px; }
.check-row { display:flex; gap:8px; margin:12px 0; color:var(--text-secondary); font-size:13px; }
@media (max-width:640px) { .section-heading, .node-config-controls { flex-direction:column; align-items:stretch; }.workspace-actions { width:100%; }.workspace-actions .btn { flex:1; }.node-config-controls .btn { width:100%; }.config-diff { grid-template-columns:1fr; }.config-detail-heading { flex-direction:column; }.config-detail-heading .btn { width:100%; } }
</style>
