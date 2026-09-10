<template>
  <section class="runtime-page">
    <SectionTabsHeader title="Agent 助手" :tabs="tabs" active-tab="instances">
      <template #actions>
        <div class="btn-group">
          <button class="icon-button" title="刷新助手列表" aria-label="刷新助手列表" :disabled="loading" @click="load">
            <RefreshCw :size="16" :class="{ 'is-spinning': loading }" />
          </button>
          <button class="btn btn-primary" data-testid="runtime-create" @click="openCreate"><Plus :size="15" />新建助手</button>
        </div>
      </template>
    </SectionTabsHeader>

    <main class="runtime-content">
      <div class="runtime-layout">
      <article class="card runtime-list-card">
        <div class="card-header"><div><h2 class="card-title">助手实例</h2><p>默认部署到 cylism-assistant 命名空间</p></div><span class="badge badge-offline">{{ runtimes.length }} 个</span></div>
        <div v-if="loading" class="empty-state">正在读取助手实例...</div>
        <div v-else-if="!runtimes.length" class="empty-state"><Bot :size="26" class="empty-icon" /><span class="empty-text">还没有 Agent 助手</span></div>
        <button v-for="item in runtimes" :key="item.id" class="runtime-item" :class="{ 'is-selected': selected?.id === item.id }" @click="select(item)">
          <span class="runtime-dot" :class="`status-${item.status}`"></span>
          <span class="runtime-item-main"><strong>{{ item.name }}</strong><small>{{ item.runtime_type }} · {{ item.namespace }}</small></span>
          <span class="runtime-item-status">{{ statusLabel(item.status) }}</span>
        </button>
      </article>

      <article class="card runtime-detail-card">
        <div class="card-header"><div><h2 class="card-title">助手详情</h2><p v-if="selected">{{ selected.image }}</p></div></div>
        <div v-if="selected" class="runtime-detail">
          <div class="runtime-status-banner"><span class="runtime-dot" :class="`status-${selected.status}`"></span><strong>{{ statusLabel(selected.status) }}</strong><span>{{ selected.health_detail || '尚未执行健康检查' }}</span></div>
          <dl class="runtime-facts"><div><dt>类型</dt><dd>{{ selected.runtime_type }}</dd></div><div><dt>部署方式</dt><dd>{{ selected.deployment_mode === 'external' ? '外部连接' : '平台托管' }}</dd></div><div><dt>命名空间</dt><dd>{{ selected.namespace }}</dd></div><div><dt>版本</dt><dd>{{ displayVersion(selected) }}</dd></div><div><dt>连接地址</dt><dd>{{ selected.endpoint_url || '-' }}</dd></div><div v-if="selected.deployment_mode !== 'external'"><dt>PVC</dt><dd>{{ selected.pvc_name }} · {{ selected.storage }}</dd></div><div><dt>模型</dt><dd>{{ selected.model_name || '-' }} · {{ selected.api_style }}</dd></div></dl>
          <div class="form-actions"><button class="btn" @click="editSelected">编辑配置</button><button class="btn" :disabled="working" @click="deploy(selected)">部署或更新</button><button class="btn" :disabled="working" @click="health(selected)">健康检查</button><button class="btn" @click="chatOpen = true" :disabled="!selected">聊天</button><button class="btn btn-danger" :disabled="working" @click="openUninstall(selected)">卸载</button></div>
          <section v-if="selected.deployment_mode === 'managed' && selected.runtime_type === 'nanobot'" class="agent-tools-panel" aria-label="Agent 平台能力">
            <div class="agent-tools-header"><div><h3>Agent 平台能力</h3><p>{{ selected.agent_tool_enabled ? '已安装受控 Cylism CLI' : '尚未安装受控 Cylism CLI' }}</p></div><div class="agent-tools-actions"><template v-if="selected.agent_tool_enabled"><button class="btn" :disabled="working" @click="updateAgentTools">更新工具</button><button class="btn btn-danger" :disabled="working" @click="uninstallAgentTools">卸载工具</button></template><button v-else class="btn btn-primary" :disabled="working" @click="installAgentTools">安装工具</button></div></div>
            <p class="agent-tools-notice">安装、更新或卸载会滚动重建 Runtime，进行中的聊天连接将断开。</p>
            <div class="agent-permission-summary"><span>{{ enabledGrantCount }} 项能力已授权</span><button class="btn" :disabled="working || !selected.agent_tool_enabled" @click="permissionModal = true">管理权限</button></div>
          </section>
        </div>
        <div v-else class="empty-state"><Bot :size="26" class="empty-icon" /><span class="empty-text">选择一个助手实例查看详情</span></div>
      </article>
    </div>
    </main>

    <Teleport to="body">
      <div v-if="editing" class="overlay runtime-editor-overlay" @click.self="cancelEdit">
        <section class="modal runtime-create-modal" role="dialog" aria-modal="true" :aria-label="form.id ? '助手配置' : '新建助手'">
          <div class="runtime-modal-header">
            <h2 class="modal-title">{{ form.id ? '助手配置' : '新建助手' }}</h2>
            <button type="button" class="icon-button" title="关闭" aria-label="关闭" @click="cancelEdit"><X :size="18" /></button>
          </div>
          <form class="runtime-form" @submit.prevent="save">
          <label>名称<input v-model.trim="form.name" required pattern="[a-z0-9]([-a-z0-9]*[a-z0-9])?" placeholder="nanobot-main" :readonly="!!form.id" /></label>
          <label>Runtime 类型<select v-model="form.runtime_type" :disabled="!!form.id" @change="onRuntimeTypeChange"><option v-for="definition in catalog" :key="definition.runtime_type" :value="definition.runtime_type">{{ definition.display_name || definition.runtime_type }}</option></select></label>
          <label>部署方式<select v-model="form.deployment_mode" :disabled="!!form.id" @change="onDeploymentModeChange"><option value="managed">平台托管（当前集群）</option><option value="external">外部连接（其他机器）</option></select></label>
          <label>镜像<input v-model.trim="form.image" :required="form.deployment_mode === 'managed'" placeholder="托管模式填写镜像地址" /></label>
          <label v-if="form.deployment_mode === 'external'">Runtime 连接地址<input v-model.trim="form.endpoint_url" required placeholder="https://agent.example.com" /></label>
          <div v-if="form.deployment_mode === 'external'" class="form-grid">
            <label>端口<input v-model.number="form.port" type="number" min="1" max="65535" /></label>
            <label>健康路径<input v-model.trim="form.health_path" placeholder="/health" /></label>
          </div>
          <div class="form-grid">
            <label>模型名称<input v-model.trim="form.model_name" placeholder="模型名称" /></label>
            <label>模型协议<select v-model="form.api_style"><option v-for="protocol in supportedProtocols" :key="protocol" :value="protocol">{{ protocolLabel(protocol) }}</option></select></label>
          </div>
          <label>模型 API 地址<input v-model.trim="form.model_base_url" placeholder="https://provider.example.com/v1" /></label>
          <label>模型 API Key <input v-model="form.api_key" type="password" :placeholder="form.api_key_configured ? '已配置，留空保持不变' : '填写后保存到 Kubernetes Secret'" autocomplete="new-password" /></label>
          <div class="form-grid"><label v-if="form.id">PVC 名称<input :value="form.pvc_name" readonly /></label><label>PVC 容量（Gi）<input v-model.number="form.storage" type="number" min="1" step="1" placeholder="10" :readonly="!!form.id" /></label></div>
          <label v-if="form.deployment_mode === 'managed'">部署节点<select v-model="form.node_name" :disabled="!!form.id"><option value="">不限制（由调度器选择）</option><option v-for="node in nodes" :key="node" :value="node">{{ node }}</option></select></label>
            <div class="form-actions"><button type="button" class="btn" @click="cancelEdit">取消</button><button type="submit" class="btn btn-primary" :disabled="saving">{{ saving ? '保存中...' : '保存' }}</button></div>
          </form>
        </section>
      </div>
    </Teleport>

    <Teleport to="body">
      <div v-if="permissionModal" class="overlay" @click.self="permissionModal = false">
        <section class="modal agent-permission-modal" role="dialog" aria-modal="true" aria-label="Agent 能力授权">
          <div class="runtime-modal-header"><h2 class="modal-title">Agent 能力授权</h2><button type="button" class="icon-button" title="关闭" aria-label="关闭" @click="permissionModal = false"><X :size="18" /></button></div>
          <p class="modal-copy">授权决定 Nanobot 可以查询哪些平台资源；扩缩容等变更操作仍需单独审批。</p>
          <div class="agent-grants-modal-list">
            <div v-for="capability in agentCapabilities" :key="capability.id" class="agent-grant-modal-row">
              <label class="agent-grant-toggle"><input v-model="agentGrantState[capability.id].enabled" type="checkbox" :disabled="!selected.agent_tool_enabled" /><span><strong>{{ capability.label }}</strong><small>{{ capability.description }}</small></span></label>
              <template v-if="capability.clusterScoped"><span class="agent-grant-scope">所有命名空间</span></template>
              <template v-else><label class="agent-grant-all"><input type="checkbox" :checked="agentGrantState[capability.id].namespaces.includes('*')" :disabled="!agentGrantState[capability.id].enabled" @change="toggleAllNamespaces(capability.id, $event.target.checked)" />所有命名空间</label><div v-if="!agentGrantState[capability.id].namespaces.includes('*')" class="namespace-options"><label v-for="namespace in namespaces" :key="namespace"><input type="checkbox" :value="namespace" v-model="agentGrantState[capability.id].namespaces" :disabled="!agentGrantState[capability.id].enabled" />{{ namespace }}</label></div></template>
            </div>
          </div>
          <div class="modal-actions"><button type="button" class="btn" @click="permissionModal = false">取消</button><button type="button" class="btn btn-primary" :disabled="working || !selected.agent_tool_enabled" @click="saveAgentGrants">保存授权</button></div>
        </section>
      </div>
    </Teleport>

    <Teleport to="body">
      <div v-if="uninstallTarget" class="overlay" @click.self="uninstallTarget = null">
        <section class="modal runtime-uninstall-modal" role="dialog" aria-modal="true" aria-label="卸载助手">
          <h2 class="modal-title">卸载助手</h2>
          <p class="confirm-copy">将卸载 Runtime <strong>{{ uninstallTarget.name }}</strong>。默认保留 PVC 与已有数据。</p>
          <label v-if="uninstallTarget.deployment_mode !== 'external'" class="checkbox-label uninstall-delete-option">
            <input v-model="deleteData" type="checkbox" data-testid="runtime-delete-data" />
            <span>同时删除 PVC 和记忆数据</span>
          </label>
          <p v-if="deleteData" class="uninstall-warning">将永久删除 Runtime 的会话、长期记忆、工作区和 PVC，无法恢复。</p>
          <div class="modal-actions">
            <button class="btn" :disabled="working" @click="uninstallTarget = null">取消</button>
            <button class="btn btn-danger" :disabled="working" @click="confirmUninstall">{{ working ? '卸载中...' : '确认卸载' }}</button>
          </div>
        </section>
      </div>
    </Teleport>

    <Teleport to="body">
      <div v-if="notice" class="overlay" @click.self="notice = null">
        <section class="modal runtime-notice-modal" role="alertdialog" aria-modal="true" :aria-label="notice.type === 'error' ? '操作失败' : '操作成功'">
          <div class="runtime-notice-icon" :class="`is-${notice.type}`">
            <CheckCircle2 v-if="notice.type === 'success'" :size="22" />
            <AlertCircle v-else :size="22" />
          </div>
          <h2 class="modal-title">{{ notice.type === 'error' ? '操作失败' : '操作成功' }}</h2>
          <p class="runtime-notice-text">{{ notice.text }}</p>
          <div class="modal-actions">
            <button class="btn" :class="notice.type === 'error' ? 'btn-danger' : 'btn-primary'" @click="notice = null">确定</button>
          </div>
        </section>
      </div>
    </Teleport>

    <ChatDrawer v-model="chatOpen" :runtime="selected" @manage-permissions="permissionModal = true" />
  </section>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { AlertCircle, Bot, CheckCircle2, Plus, RefreshCw, X } from 'lucide-vue-next'
import {
  createRuntime,
  deployRuntime,
  getRuntimeAgentState,
  getRuntimeCatalog,
  getRuntimeNamespaces,
  getRuntimeNodes,
  getRuntimes,
  healthCheckRuntime,
  installRuntimeAgentTools,
  resolveAgentOperation as resolveAgentOperationRequest,
  saveAgentCapabilityGrants,
  uninstallRuntime,
  uninstallRuntimeAgentTools,
  updateRuntime,
  updateRuntimeAgentTools,
} from '../api/runtimes.js'
import { useAsyncResource } from '../composables/useAsyncResource.js'
import ChatDrawer from '../components/ChatDrawer.vue'
import SectionTabsHeader from '../components/SectionTabsHeader.vue'

const runtimes = ref([])
const catalog = ref([])
const nodes = ref([])
const selected = ref(null)
const editing = ref(false)
const loading = ref(false)
const saving = ref(false)
const working = ref(false)
const deleteData = ref(false)
const uninstallTarget = ref(null)
const notice = ref(null)
const chatOpen = ref(false)
const permissionModal = ref(false)
const agentLoading = ref(false)
const agentOperations = ref([])
const namespaces = ref([])
const agentCapabilities = [
  { id: 'cluster.read', label: '集群摘要', description: '读取节点数量和集群摘要', clusterScoped: true },
  { id: 'workload.read', label: '工作负载查询', description: '读取 Deployment、StatefulSet 与 DaemonSet 摘要' },
  { id: 'workload.logs', label: '工作负载日志', description: '读取受限行数和大小的容器日志' },
  { id: 'events.read', label: '关联事件查询', description: '读取 Pod、PVC 的受限关联事件，用于定位调度、拉取和挂载失败' },
  { id: 'storage.read', label: '存储状态查询', description: '读取 PVC 绑定与请求容量摘要，不返回存储凭据' },
  { id: 'deployment.scale', label: 'Deployment 扩缩容', description: '始终需要管理员审批' },
  { id: 'registry.read', label: '镜像源诊断', description: '读取脱敏后的镜像源状态，并关联分析 Pod 镜像拉取失败原因', clusterScoped: true },
  { id: 'registry.verify', label: '节点镜像源连通性', description: '在已纳管节点验证已配置镜像源的 DNS 与 /v2/ 连通性', clusterScoped: true },
  { id: 'dns.read', label: '集群 DNS 状态', description: '查看平台管理的 DNS 转发策略、CoreDNS 就绪状态和允许域名的最近解析结果', clusterScoped: true },
  { id: 'registry.proxy_diagnose', label: '镜像代理出网诊断', description: '查看管理员从受管 Registry Proxy Pod 发起的最近出网诊断结果', clusterScoped: true },
  { id: 'registry.pull_check', label: '节点镜像拉取检测', description: '拉取平台配置的验证镜像，始终需要管理员审批', clusterScoped: true },
  { id: 'alert.read', label: '告警事件读取', description: '读取已持久化的告警上下文，不包含通知凭据', clusterScoped: true },
  { id: 'monitoring.read', label: '磁盘增长诊断', description: '只读取 Manager 固定的节点磁盘增长指标，不能提交 PromQL', clusterScoped: true },
  { id: 'maintenance.inspect', label: '节点磁盘巡检', description: '只读取固定系统目录、Journal 与文件系统的占用，不能指定命令或路径', clusterScoped: true },
  { id: 'maintenance.cleanup', label: '固定清理配方', description: '仅能请求 Journal、containerd 或 Docker 未使用镜像清理，始终需要管理员审批', clusterScoped: true },
]
const emptyAgentGrantState = () => Object.fromEntries(agentCapabilities.map(capability => [capability.id, { enabled: false, namespaces: [] }]))
const agentGrantState = ref(emptyAgentGrantState())
const enabledGrantCount = computed(() => Object.values(agentGrantState.value).filter(grant => grant.enabled).length)
const tabs = [{ id: 'instances', label: '实例' }]

const emptyForm = () => ({ name: '', runtime_type: 'nanobot', deployment_mode: 'managed', runtime_version: '', image: '', namespace: 'cylism-assistant', port: 8900, health_path: '/health', endpoint_url: '', pvc_name: '', storage: 10, storage_class_name: '', node_name: '', model_name: '', model_base_url: '', api_style: 'responses', api_key: '', api_key_configured: false, config: {} })
const form = ref(emptyForm())
const supportedProtocols = computed(() => catalog.value.find(item => item.runtime_type === form.value.runtime_type)?.supported_model_protocols || ['responses'])
const catalogResource = useAsyncResource(({ signal }) => getRuntimeCatalog({ signal }), [])
const runtimesResource = useAsyncResource(({ signal }) => getRuntimes({ signal }), [])
const agentStateResource = useAsyncResource(({ signal }, runtimeID) => getRuntimeAgentState(runtimeID, { signal }), null)

function statusLabel(status) { return ({ draft: '未部署', deploying: '部署中', ready: '就绪', degraded: '异常', failed: '失败', uninstalled: '已卸载' })[status] || status || '未知' }
function protocolLabel(protocol) { return ({ responses: 'Responses API', anthropic: 'Anthropic API' })[protocol] || protocol }
function storageGi(value) { const parsed = Number.parseInt(String(value || ''), 10); return Number.isFinite(parsed) && parsed > 0 ? parsed : 10 }
function tagVersion(image) { const match = String(image || '').match(/:([^/@]+)$/); return match && match[1] && match[1] !== 'latest' ? match[1] : '' }
function displayVersion(item) { return item?.runtime_version || tagVersion(item?.image) || '-' }
function applyManagedEndpointDefaults() { if (form.value.deployment_mode !== 'managed') return; const definition = catalog.value.find(item => item.runtime_type === form.value.runtime_type); form.value.port = definition?.default_port || 8900; form.value.health_path = definition?.default_health_path || '/health' }
function onRuntimeTypeChange() { if (!supportedProtocols.value.includes(form.value.api_style)) form.value.api_style = supportedProtocols.value[0] || 'responses'; applyManagedEndpointDefaults() }
function onDeploymentModeChange() { applyManagedEndpointDefaults() }
function select(item) { selected.value = item; editing.value = false; deleteData.value = false; permissionModal.value = false; clearNotice(); loadAgentState() }
function openCreate() { selected.value = null; form.value = emptyForm(); if (catalog.value[0]) { form.value.runtime_type = catalog.value[0].runtime_type; form.value.api_style = catalog.value[0].supported_model_protocols?.[0] || 'responses' }; onRuntimeTypeChange(); editing.value = true; deleteData.value = false; clearNotice() }
function editSelected() { form.value = { ...emptyForm(), ...selected.value, api_key: '' }; form.value.storage = storageGi(selected.value.storage); editing.value = true; deleteData.value = false; clearNotice() }
function cancelEdit() { editing.value = false; if (!selected.value && runtimes.value.length) selected.value = runtimes.value[0] }
function clearNotice() { notice.value = null }
function showNotice(type, text) { notice.value = { type, text } }
function formBody() { const body = { ...form.value }; delete body.id; delete body.status; delete body.api_key_configured; delete body.config; if (!body.api_key) delete body.api_key; body.storage = `${storageGi(body.storage)}Gi`; return body }
async function loadNodes() { try { const list = await getRuntimeNodes(); nodes.value = Array.isArray(list) ? list.filter(node => node && node.name).map(node => node.name) : [] } catch { nodes.value = [] } }
async function loadNamespaces() { try { const list = await getRuntimeNamespaces(); namespaces.value = Array.isArray(list) ? list.map(item => item?.name).filter(Boolean).sort() : [] } catch { namespaces.value = [] } }
async function loadCatalog() { const definitions = await catalogResource.refresh(); if (Array.isArray(definitions) && definitions.every(item => item.runtime_type && Array.isArray(item.supported_model_protocols))) catalog.value = definitions; else if (catalogResource.error.value) showNotice('error', catalogResource.error.value.message) }
async function load() { loading.value = true; const result = await runtimesResource.refresh(); if (result) { runtimes.value = result; if (selected.value) { selected.value = runtimes.value.find(item => item.id === selected.value.id) || null; if (selected.value) loadAgentState() } } else if (runtimesResource.error.value) showNotice('error', runtimesResource.error.value.message); loading.value = false }
async function save() { saving.value = true; clearNotice(); try { const result = form.value.id ? await updateRuntime(form.value.id, formBody()) : await createRuntime(formBody()); showNotice('success', result.message || 'Runtime 已保存'); await load(); selected.value = runtimes.value.find(item => item.id === result.id) || runtimes.value[0] || null; editing.value = false } catch (err) { showNotice('error', err.message) } finally { saving.value = false } }
async function deploy(item) { await runAction(() => deployRuntime(item.id), 'Runtime 已部署或更新') }
async function health(item) { await runAction(() => healthCheckRuntime(item.id), '健康检查已完成') }
function openUninstall(item) { uninstallTarget.value = item; deleteData.value = false; clearNotice() }
async function confirmUninstall() {
  const item = uninstallTarget.value
  if (!item) return
  await runAction(() => uninstallRuntime(item.id, { deleteData: deleteData.value }), 'Runtime 已卸载')
  uninstallTarget.value = null
  deleteData.value = false
}
async function runAction(action, success) { working.value = true; clearNotice(); try { const result = await action(); showNotice('success', result.message || success); await load() } catch (err) { showNotice('error', err.message) } finally { working.value = false } }
function resetAgentGrants(grants = []) {
  const next = emptyAgentGrantState()
  for (const grant of grants) {
    if (!next[grant.capability] || !grant.enabled) continue
    next[grant.capability].enabled = true
    if (grant.namespace === '*') next[grant.capability].namespaces = ['*']
    else if (!next[grant.capability].namespaces.includes('*')) next[grant.capability].namespaces.push(grant.namespace)
  }
  agentGrantState.value = next
}
async function loadAgentState() {
  if (!selected.value || selected.value.deployment_mode !== 'managed' || selected.value.runtime_type !== 'nanobot') return
  agentLoading.value = true
  try {
    const result = await agentStateResource.refresh(selected.value.id)
    await loadNamespaces()
    const [grants, operations] = result || [[], []]
    resetAgentGrants(Array.isArray(grants) ? grants : [])
    agentOperations.value = Array.isArray(operations) ? operations : []
  } catch (err) { showNotice('error', err.message) } finally { agentLoading.value = false }
}
async function installAgentTools() { await runAction(() => installRuntimeAgentTools(selected.value.id), 'Agent 工具已开始安装，Runtime 正在滚动重建') }
async function updateAgentTools() { await runAction(() => updateRuntimeAgentTools(selected.value.id), 'Agent 工具更新已提交，Runtime 正在滚动重建') }
async function uninstallAgentTools() { await runAction(() => uninstallRuntimeAgentTools(selected.value.id), 'Agent 工具已卸载，Runtime 正在滚动重建') }
async function saveAgentGrants() {
  const grants = agentCapabilities.flatMap(capability => {
    if (!agentGrantState.value[capability.id].enabled) return [{ capability: capability.id, namespace: capability.clusterScoped ? '*' : selected.value.namespace, enabled: false }]
    const scopes = capability.clusterScoped ? ['*'] : (agentGrantState.value[capability.id].namespaces.length ? agentGrantState.value[capability.id].namespaces : [selected.value.namespace])
    return scopes.map(namespace => ({ capability: capability.id, namespace, enabled: true }))
  })
  working.value = true
  clearNotice()
  try { const result = await saveAgentCapabilityGrants(selected.value.id, grants); showNotice('success', result.message || 'Agent 能力授权已更新'); permissionModal.value = false; await loadAgentState() } catch (err) { showNotice('error', err.message) } finally { working.value = false }
}
function toggleAllNamespaces(capabilityID, enabled) { agentGrantState.value[capabilityID].namespaces = enabled ? ['*'] : [] }
async function resolveAgentOperation(operation, approve) {
  working.value = true
  clearNotice()
  try { const result = await resolveAgentOperationRequest(operation.operation_id, approve); showNotice('success', result.message || (approve ? 'Agent 操作已批准' : 'Agent 操作已拒绝')); await loadAgentState() } catch (err) { showNotice('error', err.message) } finally { working.value = false }
}
onMounted(async () => { await loadCatalog(); if (!catalog.value.length) catalog.value = [{ runtime_type: 'nanobot', display_name: 'nanobot', supported_model_protocols: ['responses', 'anthropic'] }]; await load(); loadNodes() })
</script>

<style scoped>
.runtime-content { margin-top: var(--space-20); }
.runtime-layout { display: grid; grid-template-columns: minmax(280px, 0.8fr) minmax(420px, 1.4fr); gap: var(--space-16); }
.card-header { align-items: flex-start; }.card-header p { margin: 5px 0 0; color: var(--text-secondary); font-size: 12px; }
.runtime-editor-overlay { z-index: 1300; }.runtime-create-modal { width: min(720px, calc(100vw - 32px)); }.runtime-modal-header { display: flex; align-items: flex-start; justify-content: space-between; gap: var(--space-16); }.runtime-modal-header .modal-title { margin-bottom: 5px; }
.runtime-item { width: 100%; border: 0; border-bottom: 1px solid var(--border-muted); background: transparent; color: var(--text-primary); padding: 14px 12px; display: flex; align-items: center; gap: 10px; text-align: left; cursor: pointer; }
.runtime-item:hover, .runtime-item.is-selected { background: var(--surface-subtle); }
.runtime-item-main { display: flex; flex: 1; flex-direction: column; gap: 3px; min-width: 0; }.runtime-item-main small { color: var(--text-muted); }.runtime-item-status { font-size: 12px; color: var(--text-muted); }.runtime-dot { width: 9px; height: 9px; border-radius: 50%; background: var(--text-muted); flex: 0 0 auto; }.status-ready { background: var(--success); }.status-deploying { background: var(--warning); }.status-failed, .status-degraded { background: var(--danger); }
.runtime-form, .runtime-detail { padding: 16px 0; }.runtime-form label { display: flex; flex-direction: column; gap: 7px; margin-bottom: 14px; color: var(--text-muted); font-size: 13px; }.runtime-form input, .runtime-form select { width: 100%; border: 1px solid var(--border-muted); border-radius: var(--radius-control); padding: 9px 10px; background: var(--surface-input); color: var(--text-primary); }.form-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }.form-actions { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 8px; margin-top: 18px; }.runtime-status-banner { display: flex; align-items: center; gap: 9px; padding: 12px; background: var(--surface-subtle); border-left: 3px solid var(--success); }.runtime-status-banner span:last-child { color: var(--text-muted); font-size: 13px; }.runtime-facts { display: grid; grid-template-columns: 1fr 1fr; gap: 14px; margin: 22px 0; }.runtime-facts div { min-width: 0; }.runtime-facts dt { color: var(--text-muted); font-size: 12px; margin-bottom: 4px; }.runtime-facts dd { margin: 0; overflow-wrap: anywhere; }.runtime-notice-modal { width: min(420px, calc(100vw - 32px)); }.runtime-notice-modal .modal-title { margin-bottom: 0; }.runtime-notice-icon { display: grid; width: 46px; height: 46px; margin-bottom: 14px; place-items: center; border-radius: 50%; }.runtime-notice-icon.is-success { background: var(--success-surface); color: var(--success); }.runtime-notice-icon.is-error { background: var(--danger-surface); color: var(--danger); }.runtime-notice-text { margin: 10px 0 0; color: var(--text-secondary); font-size: 13px; line-height: 1.7; overflow-wrap: anywhere; }
.agent-tools-panel { margin-top: 28px; padding-top: 20px; border-top: 1px solid var(--border-muted); }.agent-tools-header, .agent-grants-header { display: flex; align-items: center; justify-content: space-between; gap: 12px; }.agent-tools-header h3, .agent-grants-header h4 { margin: 0; font-size: 15px; }.agent-tools-header p { margin: 5px 0 0; color: var(--text-secondary); font-size: 12px; }.agent-tools-actions { display: flex; flex: 0 0 auto; flex-wrap: wrap; gap: 8px; }.agent-tools-notice { margin: 12px 0; color: var(--warning); font-size: 12px; line-height: 1.6; }.agent-grants, .agent-operations { margin-top: 18px; }.agent-grant-row { display: grid; grid-template-columns: auto minmax(150px, 1fr) minmax(120px, .7fr); align-items: center; gap: 10px; padding: 10px 0; border-bottom: 1px solid var(--border-muted); }.agent-grant-row input[type="checkbox"] { width: 16px; height: 16px; accent-color: var(--accent); }.agent-grant-row span { display: flex; min-width: 0; flex-direction: column; gap: 3px; }.agent-grant-row small, .agent-operation-row small { color: var(--text-muted); font-size: 12px; }.agent-grant-row input:not([type="checkbox"]) { min-width: 0; border: 1px solid var(--border-muted); border-radius: var(--radius-control); padding: 7px 8px; background: var(--surface-input); color: var(--text-primary); }.agent-grant-scope { color: var(--text-secondary); font-size: 12px; }.agent-empty { padding: 14px 0; color: var(--text-muted); font-size: 13px; }.agent-operation-row { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 11px 0; border-bottom: 1px solid var(--border-muted); }.agent-operation-row > span:first-child { display: flex; min-width: 0; flex-direction: column; gap: 4px; }.operation-actions { display: flex; gap: 7px; flex: 0 0 auto; }
.agent-permission-summary { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-top: 16px; padding: 12px; border: 1px solid var(--border-muted); border-radius: var(--radius-control); background: var(--surface-subtle); color: var(--text-secondary); font-size: 12px; }
.agent-permission-modal { width: min(680px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); overflow: auto; }
.agent-grants-modal-list { display: grid; gap: 0; margin-top: 16px; }
.agent-grant-modal-row { display: grid; grid-template-columns: minmax(180px, 1fr) minmax(180px, 1fr); gap: 12px; padding: 13px 0; border-bottom: 1px solid var(--border-muted); }
.agent-grant-toggle { display: flex; align-items: flex-start; gap: 9px; margin: 0; color: var(--text-primary); }
.agent-grant-toggle input, .agent-grant-all input, .namespace-options input { margin-top: 3px; accent-color: var(--accent); }
.agent-grant-toggle span { display: grid; gap: 3px; }
.agent-grant-toggle small { color: var(--text-secondary); font-size: 11px; line-height: 1.4; }
.agent-grant-scope { align-self: center; color: var(--text-secondary); font-size: 12px; }
.agent-grant-all { display: flex; align-items: flex-start; gap: 7px; margin: 0; color: var(--text-secondary); font-size: 12px; }
.namespace-options { display: flex; max-height: 130px; flex-wrap: wrap; gap: 7px 14px; grid-column: 2; overflow: auto; padding: 8px 10px; border: 1px solid var(--border-muted); border-radius: var(--radius-control); background: var(--surface-input); }
.namespace-options label { display: flex; align-items: flex-start; gap: 5px; margin: 0; color: var(--text-secondary); font-size: 11px; }
.runtime-uninstall-modal { width: min(460px, calc(100vw - 32px)); }.uninstall-delete-option { margin-top: 14px; padding: 12px; border: 1px solid var(--border-muted); border-radius: var(--radius-control); background: var(--surface-subtle); cursor: pointer; transition: border-color .18s ease, background .18s ease; }.uninstall-delete-option:has(input:checked) { border-color: var(--danger); background: var(--danger-surface); }.uninstall-delete-option input { accent-color: var(--danger); }.uninstall-warning { margin: 8px 0 0; padding: 10px 12px; border-radius: var(--radius-control); background: var(--danger-surface); color: var(--danger); font-size: 12px; line-height: 1.6; }
.form-grid > :only-child { grid-column: 1 / -1; }.runtime-form input[readonly] { color: var(--text-muted); cursor: not-allowed; }
 @media (max-width: 850px) { .runtime-layout { grid-template-columns: 1fr; }.runtime-facts, .form-grid, .agent-grant-row, .agent-grant-modal-row { grid-template-columns: 1fr; }.agent-grant-row input[type="checkbox"] { justify-self: start; }.agent-tools-header, .agent-operation-row { align-items: flex-start; flex-direction: column; }.namespace-options { grid-column: auto; } }
</style>
