<template>
  <section class="card assistant-settings">
    <div class="card-header">
      <div>
        <h2 class="card-title">智能助手</h2>
        <p class="settings-copy">平台管理 PydanticAI Runtime、模型密钥和专用审计存储。</p>
      </div>
      <span class="badge" :class="{ 'badge-online': status.configured, 'badge-offline': !status.configured }">{{ status.configured ? "模型已配置" : "待配置" }}</span>
    </div>

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
        <button class="btn btn-primary" :disabled="savingProvider">{{ savingProvider ? "保存中..." : editingProviderID ? "保存并同步 Runtime" : "保存模型提供商" }}</button>
      </form>

      <form class="assistant-install" @submit.prevent="install">
        <h3>Runtime 部署</h3>
        <label class="form-group"><span class="form-label">模型提供商</span><select v-model.number="installForm.provider_id" class="form-select" required><option :value="0" disabled>选择已保存的模型</option><option v-for="item in providers" :key="item.id" :value="item.id">{{ item.name }} · {{ item.model }}</option></select></label>
        <label class="form-group"><span class="form-label">目标节点</span><select v-model="installForm.node_name" class="form-select" required><option value="" disabled>选择就绪节点</option><option v-for="node in readyNodes" :key="node.name" :value="node.name">{{ node.display_name || node.name }}</option></select></label>
        <label class="form-group"><span class="form-label">审计 PVC 容量</span><input v-model.trim="installForm.storage" class="form-input" required placeholder="1Gi" /></label>
        <button class="btn btn-primary" :disabled="installing || !providers.length">{{ installing ? "部署中..." : "部署或更新 Runtime" }}</button>
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
import { computed, onMounted, ref } from "vue"
import { Pencil, Trash2, X } from "lucide-vue-next"
import { api } from "../api/index.js"

const status = ref({ configured: false })
const providers = ref([])
const nodes = ref([])
const defaultProviderID = ref(0)
const editingProviderID = ref(null)
const deletingProviderID = ref(null)
const message = ref("")
const savingProvider = ref(false)
const installing = ref(false)
const provider = ref(newProvider())
const installForm = ref({ provider_id: 0, node_name: "", storage: "1Gi" })
const readyNodes = computed(() => nodes.value.filter(node => node.ready !== false))

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
  const [nextStatus, providerResult, nodeResult] = await Promise.allSettled([api.get("/assistant/status"), api.get("/assistant/providers"), api.get("/nodes")])
  if (nextStatus.status === "fulfilled") status.value = nextStatus.value
  if (providerResult.status === "fulfilled") {
    providers.value = providerResult.value.providers || []
    defaultProviderID.value = Number(providerResult.value.default_provider_id) || 0
    installForm.value.provider_id = defaultProviderID.value || providers.value[0]?.id || 0
  }
  if (nodeResult.status === "fulfilled") nodes.value = nodeResult.value || []
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

onMounted(refresh)
</script>

<style scoped>
.assistant-settings{grid-column:1/-1}.assistant-settings-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:var(--space-20);margin-top:var(--space-16)}.assistant-settings-grid form{display:grid;align-content:start;gap:10px;padding:14px;border:1px solid var(--border-muted);border-radius:var(--radius-control);background:var(--surface-subtle)}.assistant-provider-heading{display:flex;align-items:center;justify-content:space-between;gap:10px}.assistant-provider-heading .icon-button{width:30px;height:30px}h3{margin:0 0 2px;font-size:14px}.check-row{display:flex;align-items:center;gap:8px;color:var(--text-secondary);font-size:12px}.assistant-provider-list{display:grid;gap:6px;margin-top:var(--space-16);border-top:1px solid var(--border-muted);padding-top:var(--space-12)}.assistant-provider-row{display:flex;align-items:center;justify-content:space-between;gap:12px;padding:8px 0}.assistant-provider-copy{display:grid;min-width:0;gap:3px}.assistant-provider-copy small{overflow-wrap:anywhere;color:var(--text-muted);font:10px/1.3 var(--font-mono)}.assistant-provider-actions{display:flex;align-items:center;gap:6px;flex:0 0 auto}.assistant-provider-actions .icon-button{width:30px;height:30px}.assistant-result{margin-top:var(--space-16)}@media(max-width:700px){.assistant-settings-grid{grid-template-columns:1fr}.assistant-provider-row{align-items:flex-start;flex-direction:column}.assistant-provider-actions{width:100%;justify-content:flex-end}}
</style>
