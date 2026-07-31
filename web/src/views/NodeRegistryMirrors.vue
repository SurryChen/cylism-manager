<template>
  <div>
    <div class="page-header">
      <div>
        <h1 class="page-title">节点镜像源</h1>
        <p class="page-subtitle">统一下发 K3s 节点的 registries.yaml，应用后会重启对应 K3s 服务</p>
      </div>
      <button class="btn btn-primary" @click="openCreate">+ 新建镜像源</button>
    </div>

    <div v-if="error" class="k8s-banner k8s-banner-warn section-gap">{{ error }}</div>

    <div v-if="loaded && mirrors.length" class="card section-gap">
      <div class="table-wrap">
        <table class="data-table">
          <thead>
            <tr><th>名称</th><th>Registry</th><th>镜像地址</th><th>验证镜像</th><th>认证</th><th>状态</th><th>探测</th><th>节点结果</th><th>操作</th></tr>
          </thead>
          <tbody>
            <tr v-for="mirror in mirrors" :key="mirror.id">
              <td class="cell-primary">{{ mirror.name }}</td>
              <td>{{ mirror.registry }}</td>
              <td class="endpoints">{{ endpointText(mirror) }}</td>
              <td class="verification-image">{{ mirror.verification_image || '-' }}</td>
              <td>{{ mirror.credential_configured ? '凭据已配置' : '-' }}</td>
              <td><span class="badge" :class="mirror.enabled ? 'badge-online' : 'badge-offline'">{{ mirror.enabled ? '已启用' : '已停用' }}</span></td>
              <td>
                <span class="badge" :class="verificationClass(mirror.last_verify_status)">{{ verificationLabel(mirror.last_verify_status) }}</span>
                <small v-if="mirror.last_verify_error" class="verification-error">{{ mirror.last_verify_error }}</small>
              </td>
              <td>
                <div v-if="mirror.node_statuses?.length" class="node-statuses">
                  <small v-for="status in mirror.node_statuses" :key="status.server_id" :class="nodeStatusClass(status.status)">
                    {{ status.server?.name || status.server_id }}: {{ nodeStatusLabel(status.status) }}<span v-if="status.detail"> - {{ status.detail }}</span>
                  </small>
                </div>
                <span v-else>-</span>
              </td>
              <td class="action-cell">
                <div class="btn-group">
                  <button class="btn btn-sm" :data-testid="`verify-node-registry-mirror-${mirror.id}`" :disabled="verifyingID === mirror.id" @click="verifyMirror(mirror)">
                    {{ verifyingID === mirror.id ? '检测中...' : '检测' }}
                  </button>
                  <button class="btn btn-sm" :data-testid="`apply-node-registry-mirror-${mirror.id}`" :disabled="isApplying(mirror.id)" @click="openApply(mirror)">
                    {{ isApplying(mirror.id) ? '应用中...' : '选择节点应用' }}
                  </button>
                  <button class="btn btn-sm" @click="openEdit(mirror)">编辑</button>
                  <button class="btn btn-sm btn-danger" @click="deleteTarget = mirror">删除</button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <div v-if="applyTarget" class="overlay" @click.self="closeApply">
      <div class="modal apply-modal">
        <h2 class="modal-title">选择应用节点</h2>
        <p class="confirm-copy">只会修改选中的节点，并在每个节点上备份旧的 registries.yaml 后安排重启 K3s。</p>
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

    <div v-if="showModal" class="overlay" @click.self="closeModal">
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
    </div>

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
import { api } from '../api/index.js'

const mirrors = ref([])
const loaded = ref(false)
const error = ref('')
const showModal = ref(false)
const editing = ref(null)
const deleteTarget = ref(null)
const submitting = ref(false)
const applying = ref(false)
const activeApplyIDs = ref([])
const verifyingID = ref(null)
const servers = ref([])
const applyTarget = ref(null)
const selectedServerIDs = ref([])
const form = ref(blank())
let applyPollTimer = null

const clusterServers = computed(() => servers.value.filter(server => server.cluster_role))

function blank() {
  return { name: '', registry: '', verification_image: '', endpoints: '', username: '', credential: '', insecure_skip_verify: false, enabled: true }
}

function endpointText(mirror) {
  try { return JSON.parse(mirror.endpoints).join(', ') } catch { return mirror.endpoints }
}

function verificationLabel(status) {
  return { succeeded: '可用', failed: '失败' }[status] || '未检测'
}

function verificationClass(status) {
  return { succeeded: 'badge-online', failed: 'badge-danger' }[status] || 'badge-offline'
}

function nodeStatusLabel(status) {
  return { pending: '等待中', applying: '应用中', success: '成功', skipped: '跳过', failed: '失败' }[status] || status
}

function nodeStatusClass(status) {
  return status === 'success' ? 'success' : status === 'failed' ? 'failed' : 'pending'
}

function isApplying(mirrorID) {
  return activeApplyIDs.value.includes(mirrorID)
}

async function load() {
  error.value = ''
  try {
    mirrors.value = await api.get('/node-registry-mirrors') || []
    const pendingIDs = mirrors.value.filter(mirror => mirror.last_apply_status === 'applying').map(mirror => mirror.id)
    if (pendingIDs.length) startApplyPolling(pendingIDs)
  } catch (e) { error.value = e.message || '加载节点镜像源失败' } finally { loaded.value = true }
}

async function loadServers() {
  try { servers.value = await api.get('/servers') || [] } catch (e) { error.value = e.message || '加载集群节点失败' }
}

function openCreate() {
  editing.value = null
  form.value = blank()
  showModal.value = true
}

function openEdit(mirror) {
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
  error.value = ''
  try {
    const payload = { ...form.value, endpoints: form.value.endpoints.split('\n').map(value => value.trim()).filter(Boolean) }
    if (editing.value && !payload.credential) delete payload.credential
    if (editing.value) await api.put(`/node-registry-mirrors/${editing.value.id}`, payload)
    else await api.post('/node-registry-mirrors', payload)
    closeModal()
    await load()
  } catch (e) { error.value = e.message || '保存节点镜像源失败' } finally { submitting.value = false }
}

async function verifyMirror(mirror) {
  verifyingID.value = mirror.id
  error.value = ''
  try {
    await api.post(`/node-registry-mirrors/${mirror.id}/verify`)
    await load()
  } catch (e) { error.value = e.message || '检测节点镜像源失败' } finally { verifyingID.value = null }
}

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
  error.value = ''
  try {
    const mirrorID = applyTarget.value.id
    const updated = await api.post(`/node-registry-mirrors/${mirrorID}/apply`, { server_ids: selectedServerIDs.value })
    applyTarget.value = null
    selectedServerIDs.value = []
    const index = mirrors.value.findIndex(mirror => mirror.id === mirrorID)
    if (index >= 0 && updated?.id === mirrorID) mirrors.value[index] = updated
    startApplyPolling([mirrorID])
  } catch (e) { error.value = e.message || '下发节点镜像源失败' } finally { applying.value = false }
}

function startApplyPolling(mirrorIDs) {
  activeApplyIDs.value = [...new Set([...activeApplyIDs.value, ...mirrorIDs])]
  window.clearInterval(applyPollTimer)
  applyPollTimer = window.setInterval(async () => {
    try {
      const updates = await Promise.all(activeApplyIDs.value.map(mirrorID => api.get(`/node-registry-mirrors/${mirrorID}/apply-status`)))
      const completedIDs = []
      for (const mirror of updates) {
        const index = mirrors.value.findIndex(item => item.id === mirror.id)
        if (index >= 0) mirrors.value[index] = mirror
        if (mirror.last_apply_status !== 'applying') completedIDs.push(mirror.id)
      }
      activeApplyIDs.value = activeApplyIDs.value.filter(mirrorID => !completedIDs.includes(mirrorID))
      if (!activeApplyIDs.value.length) {
        window.clearInterval(applyPollTimer)
      }
    } catch (e) {
      error.value = e.message || '读取节点应用进度失败'
    }
  }, 2000)
}

async function remove() {
  try {
    await api.delete(`/node-registry-mirrors/${deleteTarget.value.id}`)
    deleteTarget.value = null
    await load()
  } catch (e) { error.value = e.message || '删除节点镜像源失败' }
}

onMounted(() => { load(); loadServers() })
onBeforeUnmount(() => window.clearInterval(applyPollTimer))
</script>

<style scoped>
.page-header { display:flex; justify-content:space-between; gap:var(--space-16); }
.endpoints, .verification-image { max-width:220px; overflow-wrap:anywhere; }
.node-statuses { display:grid; gap:3px; }
.node-statuses small, .verification-error { font-size:11px; }
.success { color:var(--success); }
.pending { color:var(--warning); }
.failed, .verification-error { color:var(--danger); }
.mirror-modal { width:min(580px,calc(100vw - 32px)); }
.apply-modal { width:min(520px,calc(100vw - 32px)); }
.node-selection { display:grid; gap:8px; max-height:300px; overflow:auto; margin-top:16px; }
.node-option { display:flex; align-items:flex-start; gap:10px; padding:10px; border:1px solid var(--border-muted); border-radius:var(--radius-control); background:var(--surface-subtle); cursor:pointer; }
.node-option span { display:grid; gap:3px; min-width:0; }.node-option small { color:var(--text-secondary); font-size:11px; overflow-wrap:anywhere; }
.form-hint, .confirm-copy { color:var(--text-muted); font-size:12px; }
.check-row { display:flex; gap:8px; margin:12px 0; color:var(--text-secondary); font-size:13px; }
@media (max-width:640px) { .page-header { flex-direction:column; } .page-header .btn { width:100%; } }
</style>
