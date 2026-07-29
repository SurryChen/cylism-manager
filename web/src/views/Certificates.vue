<template>
  <div>
    <div class="page-header">
      <div><h1 class="page-title">证书</h1><p class="page-subtitle">查看 cert-manager 证书、签发者与 TLS Secret 状态</p></div>
      <button v-if="certManagerReady" class="btn btn-primary" @click="showAdd = true">+ 添加证书</button>
    </div>

    <section v-if="loaded && !certManagerReady" class="card cert-manager-status section-gap">
      <div class="card-header"><div><h2 class="card-title">cert-manager {{ statusTitle }}</h2><p class="status-copy">{{ certManagerStatus?.message }}</p></div><span class="badge" :class="statusBadgeClass">{{ statusTitle }}</span></div>
      <div class="status-grid">
        <span :class="{ 'status-ok': certManagerStatus?.certificate_crd }">Certificate CRD</span>
        <span :class="{ 'status-ok': certManagerStatus?.issuer_crd }">Issuer CRD</span>
        <span :class="{ 'status-ok': certManagerStatus?.cluster_issuer_crd }">ClusterIssuer CRD</span>
        <span :class="{ 'status-ok': certManagerStatus?.controller_ready }">Controller</span>
        <span :class="{ 'status-ok': certManagerStatus?.webhook_ready }">Webhook</span>
        <span :class="{ 'status-ok': certManagerStatus?.ca_injector_ready }">CA Injector</span>
      </div>
      <div class="modal-actions status-actions">
        <button v-if="certManagerStatus?.state === 'not_installed' && certManagerStatus?.installer_available" class="btn btn-primary" :disabled="installing" @click="installCertManager">{{ installing ? '正在创建安装任务...' : '安装 cert-manager' }}</button>
        <button class="btn" :disabled="installing" @click="refreshCertManager">重新检测</button>
      </div>
    </section>

    <div v-if="loaded && certManagerReady && (certs.length || issuers.length)" class="filter-bar section-gap">
      <div class="filter-control">
        <label class="form-label" for="certificate-namespace">命名空间</label>
        <select id="certificate-namespace" v-model="filterNs" class="form-select">
          <option value="">全部</option>
          <option v-for="ns in namespaces" :key="ns" :value="ns">{{ ns }}</option>
        </select>
      </div>
    </div>

    <div v-if="error" class="k8s-banner k8s-banner-warn section-gap">{{ error }}</div>

    <section v-if="loaded && certManagerReady && filteredCerts.length" class="card section-gap">
      <div class="card-header"><h2 class="card-title">证书</h2><span class="section-count">{{ filteredCerts.length }} 项</span></div>
      <div class="table-wrap">
        <table class="data-table">
          <thead>
            <tr><th>名称</th><th>命名空间</th><th>域名</th><th>签发者</th><th>TLS Secret</th><th>到期</th><th>状态</th><th></th></tr>
          </thead>
          <tbody>
            <tr v-for="cert in filteredCerts" :key="cert.namespace + '/' + cert.name">
              <td class="cell-primary">{{ cert.name }}</td>
              <td>{{ cert.namespace }}</td>
              <td>{{ cert.domains || '-' }}</td>
              <td><span>{{ cert.issuer || '-' }}</span><small v-if="cert.issuer_kind" class="cell-secondary">{{ cert.issuer_kind }}</small></td>
              <td>{{ cert.secret_name || '-' }}</td>
              <td>{{ formatDate(cert.expiry_date) }}</td>
              <td>
                <span class="badge" :class="certStatusClass(cert.status)">
                  {{ certStatusLabel(cert.status) }}
                </span>
                <small v-if="cert.reason" class="cert-reason">{{ cert.reason }}</small>
              </td>
              <td>
                <div class="btn-group action-cell">
                  <button class="btn btn-sm btn-danger" @click="confirmDelete(cert)">删除</button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <section v-if="loaded && certManagerReady && visibleIssuers.length" class="card section-gap">
      <div class="card-header"><h2 class="card-title">签发者</h2><span class="section-count">{{ visibleIssuers.length }} 项</span></div>
      <div class="table-wrap"><table class="data-table"><thead><tr><th>名称</th><th>类型</th><th>命名空间</th><th>状态</th><th>原因</th></tr></thead><tbody>
        <tr v-for="issuer in visibleIssuers" :key="issuerKey(issuer)"><td class="cell-primary">{{ issuer.name }}</td><td>{{ issuer.kind }}</td><td>{{ issuer.namespace || '集群级' }}</td><td><span class="badge" :class="issuer.ready ? 'badge-online' : 'badge-danger'">{{ issuer.ready ? '就绪' : '不可用' }}</span></td><td>{{ issuer.reason || '-' }}</td></tr>
      </tbody></table></div>
    </section>

    <div v-if="loaded && certManagerReady && !certs.length && !issuers.length && !error" class="empty-state certificate-empty">
      <span class="empty-icon">🔒</span>
      <span class="empty-text">尚未发现 cert-manager 证书或签发者</span>
    </div>

    <!-- Add Cert modal -->
    <div v-if="showAdd" class="overlay" @click.self="showAdd = false">
      <div class="modal">
        <h2 class="modal-title">添加证书</h2>
        <form @submit.prevent="createCert">
          <div class="form-row">
            <div class="form-group">
              <label class="form-label">名称</label>
              <input v-model="form.name" class="form-input" placeholder="my-cert" required />
            </div>
            <div class="form-group">
              <label class="form-label">命名空间</label>
              <input v-model="form.namespace" class="form-input" placeholder="default" required />
            </div>
          </div>
          <div class="form-group">
            <label class="form-label">域名（多个用逗号分隔）</label>
            <input v-model="form.domains" class="form-input" placeholder="example.com,*.example.com" required />
          </div>
          <div class="form-group">
            <label class="form-label">签发者</label>
            <select v-model="form.issuer" class="form-select" required :disabled="availableIssuers.length === 0"><option value="" disabled>{{ availableIssuers.length ? '选择可用 Issuer' : '当前命名空间没有可用签发者' }}</option><option v-for="issuer in availableIssuers" :key="issuerKey(issuer)" :value="issuerKey(issuer)">{{ issuer.kind }} · {{ issuer.name }}{{ issuer.namespace ? ` (${issuer.namespace})` : '' }}</option></select>
            <p class="form-hint">仅显示状态为就绪的签发者；命名空间级 Issuer 必须与证书位于同一命名空间。</p>
          </div>
          <div class="modal-actions">
            <button type="button" class="btn" @click="showAdd = false">取消</button>
            <button type="submit" class="btn btn-primary" :disabled="submitting || availableIssuers.length === 0">{{ submitting ? '创建中...' : '确认添加' }}</button>
          </div>
        </form>
      </div>
    </div>

    <!-- Delete confirm modal -->
    <div v-if="deleteTarget" class="overlay" @click.self="deleteTarget = null">
      <div class="modal">
        <h2 class="modal-title">删除证书</h2>
        <p class="modal-copy">
          确定删除 <strong>{{ deleteTarget.name }}</strong>（{{ deleteTarget.namespace }}）？
        </p>
        <div class="modal-actions">
          <button class="btn" @click="deleteTarget = null">取消</button>
          <button class="btn btn-danger" @click="removeCert">确认删除</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, computed, watch } from 'vue'
import { api } from '../api/index.js'

const certs = ref([])
const issuers = ref([])
const filterNs = ref('')
const loaded = ref(false)
const error = ref('')
const submitting = ref(false)
const installing = ref(false)
const showAdd = ref(false)
const deleteTarget = ref(null)
const form = ref(newCertificateForm())
const certManagerStatus = ref(null)

const certManagerReady = computed(() => certManagerStatus.value?.state === 'ready')
const statusTitle = computed(() => ({ ready: '已就绪', not_installed: '未安装', installing: '安装中', degraded: '异常', unauthorized: '未授权', unavailable: '不可用' }[certManagerStatus.value?.state] || '状态未知'))
const statusBadgeClass = computed(() => certManagerReady.value ? 'badge-online' : certManagerStatus.value?.state === 'installing' ? 'badge-deploying' : 'badge-danger')
const namespaces = computed(() => [...new Set([...certs.value.map(c => c.namespace), ...issuers.value.map(i => i.namespace).filter(Boolean)])].sort())
const filteredCerts = computed(() =>
  filterNs.value ? certs.value.filter(c => c.namespace === filterNs.value) : certs.value
)
const visibleIssuers = computed(() => filterNs.value ? issuers.value.filter(i => !i.namespace || i.namespace === filterNs.value) : issuers.value)
const availableIssuers = computed(() => issuers.value.filter(issuer => issuer.ready && (issuer.kind === 'ClusterIssuer' || issuer.namespace === form.value.namespace)))

onMounted(refreshCertManager)
watch(() => form.value.namespace, ensureSelectedIssuer)

async function refreshCertManager() {
  error.value = ''
  try {
    certManagerStatus.value = await api.get('/certs/status')
    if (!certManagerReady.value) {
      certs.value = []
      issuers.value = []
      return
    }
    await fetchCertResources()
  } catch (e) { error.value = e.message || '检测 cert-manager 状态失败' } finally { loaded.value = true }
}

async function fetchCertResources() {
  try {
    const [certResult, issuerResult] = await Promise.all([api.get('/certs'), api.get('/certs/issuers')])
    certs.value = certResult || []
    issuers.value = issuerResult || []
    ensureSelectedIssuer()
  } catch (e) { error.value = e.message || '加载 cert-manager 资源失败' }
}

async function installCertManager() {
  installing.value = true
  error.value = ''
  try {
    certManagerStatus.value = await api.post('/certs/install')
  } catch (e) { error.value = e.message || '创建 cert-manager 安装任务失败' } finally { installing.value = false }
}

async function createCert() {
  const selectedIssuer = availableIssuers.value.find(issuer => issuerKey(issuer) === form.value.issuer)
  if (!selectedIssuer) { error.value = '请选择与命名空间匹配的可用签发者'; return }
  submitting.value = true
  error.value = ''
  try {
    const body = { ...form.value }
    body.domains = body.domains.split(',').map(d => d.trim()).filter(Boolean)
    body.issuer_ref = selectedIssuer.name
    body.issuer_kind = selectedIssuer.kind
    delete body.issuer
    await api.post('/certs', body)
    showAdd.value = false
    form.value = newCertificateForm()
    await refreshCertManager()
  } catch (e) { error.value = e.message || '创建证书失败' } finally { submitting.value = false }
}

function confirmDelete(cert) { deleteTarget.value = cert }

async function removeCert() {
  error.value = ''
  try {
    await api.delete(`/certs/${deleteTarget.value.namespace}/${deleteTarget.value.name}`)
    deleteTarget.value = null
    await refreshCertManager()
  } catch (e) { error.value = e.message || '删除证书失败' }
}

function newCertificateForm() { return { name: '', namespace: 'default', domains: '', issuer: '' } }
function issuerKey(issuer) { return `${issuer.kind}/${issuer.namespace || '_'}/${issuer.name}` }
function ensureSelectedIssuer() {
  if (!availableIssuers.value.some(issuer => issuerKey(issuer) === form.value.issuer)) form.value.issuer = availableIssuers.value[0] ? issuerKey(availableIssuers.value[0]) : ''
}

function formatDate(d) {
  if (!d) return '-'
  return new Date(d).toLocaleDateString('zh-CN', { month: 'short', day: 'numeric', year: 'numeric' })
}

function certStatusLabel(s) {
  s = String(s || '').toLowerCase()
  const m = { ready: '就绪', pending: '签发中', failed: '失败', expired: '已过期' }
  return m[s] || s || '-'
}

function certStatusClass(s) {
  s = String(s || '').toLowerCase()
  const m = { ready: 'badge-online', pending: 'badge-deploying', failed: 'badge-danger', expired: 'badge-danger' }
  return m[s] || 'badge-offline'
}
</script>

<style scoped>
.section-count { color:var(--text-muted); font:10px/1 var(--font-mono); }.cell-secondary { display:block; margin-top:3px; color:var(--text-muted); font-size:10px; }.cert-reason { display:block; max-width:180px; margin-top:4px; color:var(--danger); font-size:10px; overflow-wrap:anywhere; }.form-hint { margin:6px 0 0; color:var(--text-muted); font-size:11px; line-height:1.5; }.certificate-empty { min-height:150px; }.status-copy { margin:4px 0 0; color:var(--text-secondary); font-size:12px; }.status-grid { display:grid; grid-template-columns:repeat(3, minmax(0, 1fr)); gap:8px; margin-top:16px; }.status-grid span { padding:7px 8px; border:1px solid var(--border-muted); color:var(--text-muted); font-size:11px; }.status-grid .status-ok { border-color:var(--success-border); color:var(--success); background:var(--success-subtle); }.status-actions { justify-content:flex-start; margin-top:16px; } @media (max-width:640px) { .status-grid { grid-template-columns:repeat(2, minmax(0, 1fr)); } }
</style>
