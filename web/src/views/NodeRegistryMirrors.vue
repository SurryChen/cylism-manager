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

    <section class="proxy-panel section-gap">
      <div class="section-heading">
        <div><h2>自建 Registry Proxy</h2><p>每个实例只代理一个上游 Registry，临时缓存到期后通过重建 Pod 清空，不使用 PVC。</p></div>
        <button class="btn btn-sm" data-testid="create-registry-proxy" @click="openProxy()">部署代理</button>
      </div>
      <div v-if="proxies.length" class="proxy-list">
        <article v-for="item in proxies" :key="item.id" class="proxy-instance">
          <div class="proxy-instance-heading"><strong>{{ item.name }}</strong><span class="badge" :class="item.status === 'ready' ? 'badge-online' : item.status === 'failed' ? 'badge-danger' : 'badge-offline'">{{ proxyStatusLabel(item.status) }}</span></div>
          <div class="proxy-status"><span>Registry {{ item.registry }}</span><span>上游 {{ item.upstream_url }}</span><span>入口 {{ item.endpoint_host }}:{{ item.node_port }}</span><span>节点 {{ item.node_name }}</span><span>缓存 {{ item.cache_limit_gi }} Gi / {{ item.cleanup_interval_hours }} 小时</span><span>出网代理 {{ item.outbound_proxy_configured ? '已配置' : '未配置' }}</span><span v-if="item.last_diagnostic_status" class="egress-status" :class="`egress-${item.last_diagnostic_status}`">出网 {{ diagnosticStatusLabel(item.last_diagnostic_status) }}</span><small v-if="item.last_error">{{ item.last_error }}</small><small v-if="item.last_diagnostic_error">{{ item.last_diagnostic_error }}</small></div>
          <div class="btn-group proxy-actions"><button v-if="isLegacyDockerHubProxy(item)" class="btn btn-sm" :disabled="migratingID === item.id" :data-testid="`migrate-registry-proxy-${item.id}`" @click="migrationTarget = item">{{ migratingID === item.id ? '迁移中...' : '迁移资源命名' }}</button><button class="btn btn-sm" :disabled="proxyDiagnosingID === item.id" :data-testid="`diagnose-registry-proxy-${item.id}`" @click="diagnoseProxy(item)">{{ proxyDiagnosingID === item.id ? '诊断中...' : '诊断' }}</button><button class="btn btn-sm" @click="openProxy(item)">配置</button><button class="btn btn-sm btn-danger" :disabled="proxyCleaningID === item.id" @click="cleanupProxy(item)">{{ proxyCleaningID === item.id ? '清理中...' : '立即清理缓存' }}</button></div>
        </article>
      </div>
      <div v-else class="empty-inline">尚未部署。为每个需要加速的 Registry 单独部署代理，例如 docker.io 或 registry.k8s.io。</div>
    </section>

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

    <div v-if="showProxyModal" class="overlay" @click.self="closeProxy"><div class="modal proxy-modal"><h2 class="modal-title">{{ editingProxy ? '配置 Registry Proxy' : '部署 Registry Proxy' }}</h2><form @submit.prevent="deployProxy"><div class="form-row"><div class="form-group"><label class="form-label">代理名称</label><input v-model.trim="proxyForm.name" class="form-input" required placeholder="Kubernetes Registry 代理" /></div><div class="form-group"><label class="form-label">Registry</label><input v-model.trim="proxyForm.registry" class="form-input" required placeholder="registry.k8s.io" /></div></div><div class="form-group"><label class="form-label">上游地址</label><input v-model.trim="proxyForm.upstream_url" type="url" class="form-input" placeholder="留空时使用 Registry 对应的 HTTPS 地址" /><p class="form-hint">Docker Hub 留空会使用 registry-1.docker.io；其他 Registry 必须使用自身的 HTTPS 地址。</p></div><div class="form-group"><label class="form-label">部署节点</label><select v-model="proxyForm.node_name" class="form-select" required><option value="" disabled>选择可访问上游 Registry 的节点</option><option v-for="server in clusterServers" :key="server.id" :value="server.k8s_node_name">{{ server.name }} · {{ server.k8s_node_name }}</option></select></div><div class="form-group"><label class="form-label">节点可访问 IP</label><input v-model.trim="proxyForm.endpoint_host" class="form-input" required placeholder="100.81.x.x 或 10.x.x.x" /><p class="form-hint">仅支持私网或 Tailscale IP。此地址将作为节点镜像源端点。</p></div><div class="form-row"><div class="form-group"><label class="form-label">NodePort</label><input v-model.number="proxyForm.node_port" type="number" min="30000" max="32767" class="form-input" required /></div><div class="form-group"><label class="form-label">临时缓存上限 (Gi)</label><input v-model.number="proxyForm.cache_limit_gi" type="number" min="1" max="100" class="form-input" required /></div></div><div class="form-group"><label class="form-label">定期清理 (小时)</label><input v-model.number="proxyForm.cleanup_interval_hours" type="number" min="1" max="168" class="form-input" required /></div><div class="config-section-title">可选出网代理</div><div class="form-group"><label class="form-label">HTTP_PROXY</label><input v-model.trim="proxyForm.http_proxy" type="url" class="form-input" placeholder="留空保持现有配置" /></div><div class="form-group"><label class="form-label">HTTPS_PROXY</label><input v-model.trim="proxyForm.https_proxy" type="url" class="form-input" placeholder="留空保持现有配置" /></div><div class="form-group"><label class="form-label">NO_PROXY</label><input v-model.trim="proxyForm.no_proxy" class="form-input" placeholder="localhost,127.0.0.1,.cluster.local" /></div><p class="form-hint">代理地址加密保存，重新打开配置不会展示已有地址。保存后会滚动重建该 Proxy。</p><div class="modal-actions"><button type="button" class="btn" @click="closeProxy">取消</button><button class="btn btn-primary" :disabled="proxyDeploying">{{ proxyDeploying ? '提交中...' : editingProxy ? '保存配置' : '部署代理' }}</button></div></form></div></div>

    <div v-if="migrationTarget" class="overlay" @click.self="migrationTarget = null"><div class="modal"><h2 class="modal-title">迁移代理资源命名</h2><p class="confirm-copy">将删除旧的 Deployment 和 Service，并以新资源名重建。代理会短暂中断，但入口地址和 NodePort 保持不变。</p><div class="modal-actions"><button class="btn" :disabled="migratingID === migrationTarget.id" @click="migrationTarget = null">取消</button><button class="btn btn-danger" data-testid="confirm-registry-proxy-migration" :disabled="migratingID === migrationTarget.id" @click="migrateProxy">{{ migratingID === migrationTarget.id ? '迁移中...' : '确认迁移' }}</button></div></div></div>

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
const proxies = ref([])
const showProxyModal = ref(false)
const proxyDeploying = ref(false)
const proxyCleaningID = ref(null)
const editingProxy = ref(null)
const migrationTarget = ref(null)
const migratingID = ref(null)
const proxyDiagnosingID = ref(null)
const proxyForm = ref(proxyBlank())
const form = ref(blank())
let applyPollTimer = null

const clusterServers = computed(() => servers.value.filter(server => server.cluster_role))

function blank() {
  return { name: '', registry: '', verification_image: '', endpoints: '', username: '', credential: '', insecure_skip_verify: false, enabled: true }
}
function proxyBlank() { return { name: '', registry: 'docker.io', upstream_url: '', node_name: '', endpoint_host: '', node_port: 30500, cache_limit_gi: 10, cleanup_interval_hours: 24, http_proxy: '', https_proxy: '', no_proxy: '', clear_outbound_proxy: false } }

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

function proxyStatusLabel(status) { return { ready: '就绪', deploying: '部署中', failed: '失败', missing: '缺失' }[status] || '未知' }
function diagnosticStatusLabel(status) { return { healthy: '正常', proxy_not_ready: '未就绪', dns_resolution_failed: 'DNS 解析失败', upstream_connect_timeout: '上游连接超时', upstream_tls_failed: 'TLS 失败', upstream_http_error: '上游响应异常', diagnostic_failed: '诊断失败' }[status] || status }
function isLegacyDockerHubProxy(item) { return item.registry === 'docker.io' && item.resource_name === 'cylism-registry-proxy' }
async function loadProxies() { try { proxies.value = await api.get('/registry-proxies') || [] } catch (e) { error.value = e.message || '加载自建镜像代理失败' } }
function openProxy(item = null) { editingProxy.value = item; proxyForm.value = item ? { name: item.name, registry: item.registry, upstream_url: item.upstream_url, node_name: item.node_name, endpoint_host: item.endpoint_host, node_port: item.node_port, cache_limit_gi: item.cache_limit_gi, cleanup_interval_hours: item.cleanup_interval_hours, http_proxy: '', https_proxy: '', no_proxy: item.no_proxy || '', clear_outbound_proxy: false } : proxyBlank(); showProxyModal.value = true }
function closeProxy() { showProxyModal.value = false; editingProxy.value = null; proxyForm.value = proxyBlank() }
async function deployProxy() { proxyDeploying.value = true; error.value = ''; try { if (editingProxy.value) await api.put(`/registry-proxies/${editingProxy.value.id}`, proxyForm.value); else await api.post('/registry-proxies', proxyForm.value); closeProxy(); await loadProxies() } catch (e) { error.value = e.message || '部署自建镜像代理失败' } finally { proxyDeploying.value = false } }
async function cleanupProxy(item) { proxyCleaningID.value = item.id; error.value = ''; try { await api.post(`/registry-proxies/${item.id}/cleanup`); await loadProxies() } catch (e) { error.value = e.message || '清理代理缓存失败' } finally { proxyCleaningID.value = null } }
async function migrateProxy() { if (!migrationTarget.value) return; const item = migrationTarget.value; migratingID.value = item.id; error.value = ''; try { await api.post(`/registry-proxies/${item.id}/migrate-resource-name`); migrationTarget.value = null; await loadProxies() } catch (e) { error.value = e.message || '迁移代理资源命名失败' } finally { migratingID.value = null } }
async function diagnoseProxy(item) { proxyDiagnosingID.value = item.id; error.value = ''; try { await api.post(`/registry-proxies/${item.id}/diagnose`); await loadProxies() } catch (e) { error.value = e.message || '代理出网诊断失败' } finally { proxyDiagnosingID.value = null } }

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

onMounted(() => { load(); loadServers(); loadProxies() })
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
.proxy-panel { padding:var(--space-16) 0; border-bottom:1px solid var(--border-muted); }.section-heading { display:flex; justify-content:space-between; gap:var(--space-16); align-items:flex-start; }.section-heading h2 { margin:0; font-size:16px; }.section-heading p,.proxy-status small { margin:4px 0 0; color:var(--text-secondary); font-size:12px; }.proxy-list { display:grid; gap:10px; margin-top:12px; }.proxy-instance { padding:12px; border:1px solid var(--border-muted); border-radius:var(--radius-control); background:var(--surface-subtle); }.proxy-instance-heading { display:flex; align-items:center; gap:8px; }.proxy-status { display:flex; flex-wrap:wrap; gap:8px 14px; align-items:center; margin-top:8px; color:var(--text-secondary); font-size:13px; }.proxy-status small { width:100%; color:var(--danger); }.proxy-actions { margin-top:12px; }
.proxy-modal { width:min(520px,calc(100vw - 32px)); }
.egress-healthy { color:var(--success); }.egress-upstream_connect_timeout,.egress-dns_resolution_failed,.egress-diagnostic_failed { color:var(--danger); }
.apply-modal { width:min(520px,calc(100vw - 32px)); }
.node-selection { display:grid; gap:8px; max-height:300px; overflow:auto; margin-top:16px; }
.node-option { display:flex; align-items:flex-start; gap:10px; padding:10px; border:1px solid var(--border-muted); border-radius:var(--radius-control); background:var(--surface-subtle); cursor:pointer; }
.node-option span { display:grid; gap:3px; min-width:0; }.node-option small { color:var(--text-secondary); font-size:11px; overflow-wrap:anywhere; }
.form-hint, .confirm-copy { color:var(--text-muted); font-size:12px; }
.check-row { display:flex; gap:8px; margin:12px 0; color:var(--text-secondary); font-size:13px; }
@media (max-width:640px) { .page-header,.section-heading { flex-direction:column; } .page-header .btn { width:100%; } }
</style>
