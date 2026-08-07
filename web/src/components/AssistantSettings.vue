<template>
  <section class="card assistant-settings">
    <div class="card-header">
      <div>
        <h2 class="card-title">智能助手</h2>
        <p class="settings-copy">管理 PydanticAI Runtime、模型密钥和专用审计存储。</p>
      </div>
      <span class="badge" :class="{ 'badge-online': status.configured, 'badge-offline': !status.configured }">{{ status.configured ? "模型已配置" : "模型待配置" }}</span>
    </div>

    <section class="assistant-runtime" aria-live="polite">
      <div class="assistant-runtime-heading"><div><h3>Runtime 部署状态</h3><p>{{ runtimeStatus.message || '正在读取 Kubernetes Runtime 状态' }}</p></div><div class="assistant-runtime-actions"><span class="badge" :class="runtimeBadgeClass">{{ runtimeLabel }}</span><button v-if="canUninstall" class="icon-button danger-action" type="button" title="卸载 Runtime 和 PVC" aria-label="卸载 Runtime 和 PVC" :disabled="uninstalling" @click="uninstall"><Trash2 :size="16" /></button><button class="icon-button" type="button" title="刷新 Runtime 状态" aria-label="刷新 Runtime 状态" :disabled="refreshing" @click="refresh"><RefreshCw :size="16" :class="{ 'is-spinning': refreshing }" /></button></div></div>
      <div class="assistant-runtime-grid"><div><span>就绪副本</span><strong>{{ runtimeStatus.ready_replicas || 0 }} / {{ runtimeStatus.desired_replicas || 1 }}</strong></div><div><span>目标节点</span><strong>{{ runtimeStatus.node_name || '-' }}</strong></div><div><span>审计存储</span><strong>{{ runtimeStatus.storage || '-' }}</strong></div><div><span>运行模型</span><strong>{{ runtimeStatus.model || '-' }}</strong></div></div>
      <p v-if="migration" class="assistant-migration">{{ migrationLabel }}</p>
      <p v-if="hasLegacyRuntime" class="assistant-legacy-runtime"><strong>发现待迁移的旧 Runtime 审计存储</strong><span>{{ legacyRuntimeStatus.namespace || 'default' }} / {{ legacyRuntimeStatus.pvc_name }}</span><span v-if="legacyRuntimeStatus.node_name || legacyRuntimeStatus.storage">原节点 {{ legacyRuntimeStatus.node_name || '-' }} · {{ legacyRuntimeStatus.storage || '-' }}</span></p>
    </section>

    <div class="assistant-settings-grid">
      <form class="assistant-provider" @submit.prevent="saveProvider">
        <div class="assistant-provider-heading">
          <h3>{{ editingProviderID ? "编辑 Responses API 模型" : "Responses API 模型提供商" }}</h3>
          <button v-if="editingProviderID" class="icon-button" type="button" title="取消编辑" aria-label="取消编辑" @click="resetProvider"><X :size="16" /></button>
        </div>
        <label class="form-group"><span class="form-label">名称</span><input v-model.trim="provider.name" class="form-input" required placeholder="OpenAI" /></label>
        <label class="form-group"><span class="form-label">模型</span><input v-model.trim="provider.model" class="form-input" required placeholder="gpt-4o-mini" /></label>
        <label class="form-group"><span class="form-label">Responses API Base URL</span><input v-model.trim="provider.base_url" class="form-input" placeholder="留空使用 OpenAI 官方端点" /></label>
        <label class="form-group"><span class="form-label">API Key</span><input v-model.trim="provider.api_key" class="form-input" type="password" :required="!editingProviderID" :placeholder="editingProviderID ? '留空保留现有密钥' : '仅保存密文'" /></label>
        <label class="check-row"><input v-model="provider.enabled" type="checkbox" />启用此模型提供商</label>
        <label class="check-row"><input v-model="provider.is_default" type="checkbox" />设为当前 Runtime 模型</label>
        <div class="assistant-provider-commands"><button class="btn btn-secondary assistant-provider-probe" type="button" :disabled="probingProvider" @click="probeProvider">{{ probingProvider ? "探测中..." : "测试模型连接" }}</button><button class="btn btn-primary" :disabled="savingProvider">{{ savingProvider ? "保存中..." : editingProviderID ? "保存并同步 Runtime" : "保存模型提供商" }}</button></div>
      </form>

      <form class="assistant-install" @submit.prevent="install">
        <h3>{{ hasLegacyRuntime ? '迁移并部署 Runtime' : 'Runtime 部署' }}</h3>
        <label class="form-group"><span class="form-label">模型提供商</span><select v-model.number="installForm.provider_id" class="form-select" required><option :value="0" disabled>选择已保存的模型</option><option v-for="item in providers" :key="item.id" :value="item.id">{{ item.name }} · {{ item.model }}</option></select></label>
        <label class="form-group"><span class="form-label">目标节点</span><select v-model="installForm.node_name" class="form-select" required><option value="" disabled>选择就绪节点</option><option v-for="node in readyNodes" :key="node.name" :value="node.name">{{ node.display_name || node.name }}</option></select></label>
        <label class="form-group"><span class="form-label">审计 PVC 容量</span><input v-model.trim="installForm.storage" class="form-input" required placeholder="1Gi" /></label>
        <button class="btn btn-primary" :disabled="installing || !providers.length">{{ installing ? "部署中..." : hasLegacyRuntime ? "迁移并部署 Runtime" : "部署或更新 Runtime" }}</button>
      </form>

      <form class="assistant-runtime-migration" @submit.prevent="migrateRuntime">
        <h3>Runtime 存储迁移</h3>
        <p class="settings-copy">迁移审计数据并将 Runtime 切换到另一台就绪节点。</p>
        <label class="form-group"><span class="form-label">目标节点</span><select v-model="migrationForm.target_node_name" class="form-select" required :disabled="runtimeStatus.state !== 'ready' || !!migration"><option value="" disabled>选择不同的就绪节点</option><option v-for="node in migrationNodes" :key="node.name" :value="node.name">{{ node.display_name || node.name }}</option></select></label>
        <button class="btn btn-primary" :disabled="migrating || runtimeStatus.state !== 'ready' || !!migration || !migrationForm.target_node_name">{{ migrating ? "迁移提交中..." : "迁移 Runtime 存储" }}</button>
      </form>
    </div>

    <div v-if="providers.length" class="assistant-provider-list">
      <div v-for="item in providers" :key="item.id" class="assistant-provider-row">
        <div class="assistant-provider-copy"><strong>{{ item.name }}</strong><small>{{ item.model }} · {{ item.base_url || "OpenAI 官方端点" }}</small></div>
        <div class="assistant-provider-actions"><span v-if="isDefault(item.id)" class="badge badge-online">当前 Runtime</span><button class="icon-button" type="button" title="编辑模型提供商" aria-label="编辑模型提供商" @click="editProvider(item)"><Pencil :size="16" /></button><button class="icon-button danger-action" type="button" title="删除模型提供商" aria-label="删除模型提供商" :disabled="isDefault(item.id) || deletingProviderID === item.id" @click="deleteProvider(item)"><Trash2 :size="16" /></button></div>
      </div>
    </div>

    <p v-if="message" class="settings-copy assistant-result">{{ message }}</p>
  </section>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from "vue"
import { Pencil, RefreshCw, Trash2, X } from "lucide-vue-next"
import { api } from "../api/index.js"

const status = ref({ configured: false, runtime_status: {} })
const providers = ref([])
const nodes = ref([])
const defaultProviderID = ref(0)
const editingProviderID = ref(null)
const deletingProviderID = ref(null)
const message = ref("")
const savingProvider = ref(false)
const probingProvider = ref(false)
const installing = ref(false)
const uninstalling = ref(false)
const migrating = ref(false)
const refreshing = ref(false)
const provider = ref(newProvider())
const installForm = ref({ provider_id: 0, node_name: "", storage: "1Gi" })
const migrationForm = ref({ target_node_name: "" })
const readyNodes = computed(() => nodes.value.filter(node => node.ready !== false))
const runtimeStatus = computed(() => status.value.runtime_status || {})
const legacyRuntimeStatus = computed(() => status.value.legacy_runtime_status || null)
const hasLegacyRuntime = computed(() => runtimeStatus.value.state === 'not_installed' && !!legacyRuntimeStatus.value?.pvc_name)
const canUninstall = computed(() => ['ready', 'installing', 'degraded'].includes(runtimeStatus.value.state) || !!legacyRuntimeStatus.value?.state && legacyRuntimeStatus.value.state !== 'not_installed')
const installDefaults = computed(() => {
  if (hasLegacyRuntime.value) return legacyRuntimeStatus.value
  return runtimeStatus.value.state === 'ready' ? runtimeStatus.value : null
})
const migration = computed(() => status.value.migration || null)
const migrationNodes = computed(() => readyNodes.value.filter(node => node.name !== runtimeStatus.value.node_name))
const runtimeLabel = computed(() => ({ ready: 'Runtime 已就绪', installing: 'Runtime 部署中', not_installed: 'Runtime 未部署', degraded: 'Runtime 状态异常', unavailable: 'Runtime 状态不可用' }[runtimeStatus.value.state] || 'Runtime 状态未知'))
const runtimeBadgeClass = computed(() => ({ ready: 'badge-online', installing: 'badge-deploying', not_installed: 'badge-offline', degraded: 'badge-danger', unavailable: 'badge-offline' }[runtimeStatus.value.state] || 'badge-offline'))
const migrationLabel = computed(() => ({ pending: '正在准备审计存储迁移', provisioning_target: '正在创建目标审计存储', stopping_source: '正在停止源 Runtime', copying: '正在复制审计数据', verifying: '正在校验审计 SQLite 文件', starting_target: '正在启动新的 Runtime', cleaning_legacy: '正在清理源 Runtime 资源' }[migration.value?.status] || migration.value?.detail || 'Runtime 存储迁移中'))
let refreshTimer

function newProvider() {
  return { name: "", provider_type: "openai_responses", base_url: "", model: "", api_key: "", enabled: true, is_default: true }
}

function isDefault(id) {
  return Number(defaultProviderID.value) === Number(id)
}

function resetProvider() {
  editingProviderID.value = null
  provider.value = newProvider()
}

function editProvider(item) {
  editingProviderID.value = item.id
  provider.value = { name: item.name, provider_type: "openai_responses", base_url: item.base_url || "", model: item.model, api_key: "", enabled: item.enabled, is_default: isDefault(item.id) }
  message.value = ""
}

async function refresh() {
  refreshing.value = true
  const [nextStatus, providerResult, nodeResult] = await Promise.allSettled([api.get("/assistant/status"), api.get("/assistant/providers"), api.get("/nodes")])
  if (nextStatus.status === "fulfilled") status.value = nextStatus.value
  if (providerResult.status === "fulfilled") {
    providers.value = providerResult.value.providers || []
    defaultProviderID.value = Number(providerResult.value.default_provider_id) || 0
    installForm.value.provider_id = defaultProviderID.value || providers.value[0]?.id || 0
  }
  if (nodeResult.status === "fulfilled") nodes.value = nodeResult.value || []
  if (!installForm.value.node_name && installDefaults.value?.node_name && readyNodes.value.some(node => node.name === installDefaults.value.node_name)) {
    installForm.value.node_name = installDefaults.value.node_name
  }
  if (installDefaults.value && installForm.value.storage === '1Gi' && installDefaults.value.storage) {
    installForm.value.storage = installDefaults.value.storage
  }
  refreshing.value = false
}

async function saveProvider() {
  savingProvider.value = true
  message.value = ""
  try {
    const isEditing = editingProviderID.value !== null
    const result = isEditing ? await api.put(`/assistant/providers/${editingProviderID.value}`, provider.value) : await api.post("/assistant/providers", provider.value)
    message.value = isEditing ? "模型提供商已更新" : "模型提供商已保存"
    resetProvider()
    await refresh()
    return result
  } catch (error) {
    message.value = error.message || "保存失败"
  } finally {
    savingProvider.value = false
  }
}

async function probeProvider() {
  probingProvider.value = true
  message.value = ""
  try {
    const path = editingProviderID.value === null ? "/assistant/providers/test" : `/assistant/providers/${editingProviderID.value}/test`
    await api.post(path, provider.value)
    message.value = "模型 API 连接成功"
  } catch (error) {
    message.value = error.message || "模型 API 连接失败"
  } finally {
    probingProvider.value = false
  }
}

async function deleteProvider(item) {
  if (!window.confirm(`确认删除模型提供商“${item.name}”吗？`)) return
  deletingProviderID.value = item.id
  message.value = ""
  try {
    await api.delete(`/assistant/providers/${item.id}`)
    if (editingProviderID.value === item.id) resetProvider()
    message.value = "模型提供商已删除"
    await refresh()
  } catch (error) {
    message.value = error.message || "删除失败"
  } finally {
    deletingProviderID.value = null
  }
}

async function install() {
  installing.value = true
  message.value = ""
  try {
    const result = await api.post("/assistant/install", installForm.value)
    message.value = result.message || "Runtime 已提交部署"
    await refresh()
  } catch (error) {
    message.value = error.message || "部署失败"
  } finally {
    installing.value = false
  }
}

async function uninstall() {
  if (!window.confirm('确认卸载智能助手 Runtime 吗？将删除运行资源和审计 PVC，但保留命名空间。')) return
  uninstalling.value = true
  message.value = ''
  try {
    const result = await api.delete('/assistant')
    message.value = result.message || '智能助手 Runtime 已卸载'
    await refresh()
  } catch (error) {
    message.value = error.message || '卸载失败'
  } finally {
    uninstalling.value = false
  }
}

async function migrateRuntime() {
  migrating.value = true
  message.value = ""
  try {
    const result = await api.post("/assistant/runtime/migrations", migrationForm.value)
    message.value = result.message || "Runtime 存储迁移已开始"
    migrationForm.value.target_node_name = ""
    await refresh()
  } catch (error) {
    message.value = error.message || "迁移失败"
  } finally {
    migrating.value = false
  }
}

onMounted(() => {
  refresh()
  refreshTimer = window.setInterval(refresh, 10000)
})
onBeforeUnmount(() => window.clearInterval(refreshTimer))
</script>

<style scoped>
.assistant-settings{grid-column:1/-1}.assistant-runtime{margin-top:var(--space-16);padding:14px;border:1px solid var(--border-muted);border-radius:var(--radius-control);background:var(--surface-subtle)}.assistant-runtime-heading{display:flex;align-items:flex-start;justify-content:space-between;gap:12px}.assistant-runtime-heading h3{margin:0;font-size:14px}.assistant-runtime-heading p{margin:4px 0 0;color:var(--text-secondary);font-size:12px;line-height:1.5}.assistant-runtime-actions{display:flex;align-items:center;gap:6px;flex:0 0 auto}.assistant-runtime-actions .icon-button{width:30px;height:30px}.assistant-runtime-grid{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:10px;margin-top:var(--space-16)}.assistant-runtime-grid div{display:grid;min-width:0;gap:4px}.assistant-runtime-grid span{color:var(--text-secondary);font-size:11px}.assistant-runtime-grid strong{overflow:hidden;font-size:13px;text-overflow:ellipsis;white-space:nowrap}.assistant-migration{margin:var(--space-12) 0 0;color:var(--text-secondary);font-size:12px}.assistant-legacy-runtime{display:grid;gap:3px;margin:var(--space-12) 0 0;padding:10px;border-left:3px solid var(--warning);background:var(--surface-raised);color:var(--text-secondary);font-size:12px}.assistant-legacy-runtime strong{color:var(--text-primary)}.assistant-settings-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:var(--space-20);margin-top:var(--space-16)}.assistant-settings-grid form{display:grid;align-content:start;gap:10px;padding:14px;border:1px solid var(--border-muted);border-radius:var(--radius-control);background:var(--surface-subtle)}.assistant-runtime-migration .settings-copy{margin:0;font-size:12px}.assistant-provider-heading{display:flex;align-items:center;justify-content:space-between;gap:10px}.assistant-provider-heading .icon-button{width:30px;height:30px}.assistant-provider-commands{display:flex;gap:8px}.assistant-provider-commands .btn{flex:1}h3{margin:0 0 2px;font-size:14px}.check-row{display:flex;align-items:center;gap:8px;color:var(--text-secondary);font-size:12px}.assistant-provider-list{display:grid;gap:6px;margin-top:var(--space-16);border-top:1px solid var(--border-muted);padding-top:var(--space-12)}.assistant-provider-row{display:flex;align-items:center;justify-content:space-between;gap:12px;padding:8px 0}.assistant-provider-copy{display:grid;min-width:0;gap:3px}.assistant-provider-copy small{overflow-wrap:anywhere;color:var(--text-muted);font:10px/1.3 var(--font-mono)}.assistant-provider-actions{display:flex;align-items:center;gap:6px;flex:0 0 auto}.assistant-provider-actions .icon-button{width:30px;height:30px}.assistant-result{margin-top:var(--space-16)}.is-spinning{animation:spin .8s linear infinite}@keyframes spin{to{transform:rotate(360deg)}}@media(max-width:700px){.assistant-runtime-heading,.assistant-provider-row{align-items:flex-start;flex-direction:column}.assistant-runtime-actions{width:100%;justify-content:flex-end}.assistant-runtime-grid,.assistant-settings-grid{grid-template-columns:repeat(2,minmax(0,1fr))}}@media(max-width:460px){.assistant-runtime-grid,.assistant-settings-grid{grid-template-columns:1fr}.assistant-provider-commands{flex-direction:column}}
</style>
