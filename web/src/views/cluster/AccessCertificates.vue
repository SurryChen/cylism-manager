<template>
  <SurfaceCard class="access-cert-card" padding="none">
    <div class="access-cert-toolbar">
      <div class="access-cert-summary">
        <span class="cell-secondary">共 {{ certificates.length }} 张证书</span>
        <span v-if="ca" class="badge" :class="isExpired(ca.not_after) ? 'badge-danger' : 'badge-online'">{{ isExpired(ca.not_after) ? '访问 CA 已过期' : '访问 CA 有效' }}</span>
        <span v-else class="badge badge-offline">未创建访问 CA</span>
      </div>
      <div class="access-cert-actions">
        <span class="access-cert-tooltip" data-tooltip="刷新访问证书">
          <button class="icon-button" type="button" aria-label="刷新访问证书" :disabled="loading" @click="load"><RefreshCw :size="15" :class="{ 'is-spinning': loading }" /></button>
        </span>
        <span v-if="ca" class="access-cert-tooltip" data-tooltip="下载访问 CA 证书：供 Agent 建立信任">
          <button class="icon-button" type="button" aria-label="下载访问 CA 证书，供 Agent 建立信任" @click="download('cylism-access-ca.crt', ca.certificate_pem)"><Download :size="15" /></button>
        </span>
        <span v-if="ca" class="access-cert-tooltip" data-tooltip="下载证书吊销列表 CRL：供校验证书是否已吊销">
          <button class="icon-button" type="button" aria-label="下载证书吊销列表 CRL，供校验证书是否已吊销" @click="downloadCRL"><ShieldX :size="15" /></button>
        </span>
        <button v-if="!ca" class="btn btn-primary" type="button" @click="openCreateCA">创建访问 CA</button>
        <button v-else class="btn btn-primary" type="button" :disabled="isExpired(ca.not_after) || !clusters.length" @click="openIssue">签发证书</button>
      </div>
    </div>
    <FeedbackBanner v-if="error" class="access-cert-error" tone="warning" :message="error" />
    <EmptyState v-if="!loaded" variant="loading" message="加载中..." />
    <EmptyState v-else-if="!certificates.length && !error" message="暂无访问证书" />
    <div v-else-if="certificates.length" class="table-wrap">
      <table class="data-table">
        <thead><tr><th>集群</th><th>用途</th><th>身份 / 地址</th><th>状态</th><th>到期时间</th><th class="action-cell">操作</th></tr></thead>
        <tbody><tr v-for="item in certificates" :key="item.id">
          <td class="cell-primary">{{ clusterName(item.cluster_id) }}</td>
          <td>{{ item.kind === 'agent_server' ? 'Agent 服务端' : '平台客户端' }}</td>
          <td><span>{{ item.endpoint || item.subject }}</span><span class="cell-secondary">{{ item.fingerprint.slice(0, 16) }}...</span></td>
          <td><span class="badge" :class="statusClass(item)">{{ statusLabel(item) }}</span></td>
          <td>{{ formatTime(item.not_after) }}</td>
          <td class="action-cell"><div class="access-cert-row-actions">
            <button class="icon-button icon-button--table" type="button" :title="`下载证书 #${item.id}`" :aria-label="`下载证书 #${item.id}`" @click="downloadCertificate(item)"><Download :size="14" /></button>
            <button v-if="!item.revoked_at" class="icon-button icon-button--table" type="button" :disabled="item.in_use" :title="item.in_use ? '使用中的证书不能吊销' : `吊销证书 #${item.id}`" :aria-label="item.in_use ? '使用中的证书不能吊销' : `吊销证书 #${item.id}`" @click="openRevoke(item)"><ShieldX :size="14" /></button>
            <button v-if="!item.in_use" class="icon-button icon-button--table" type="button" :title="`删除未使用证书 #${item.id}`" :aria-label="`删除未使用证书 #${item.id}`" @click="openDelete(item)"><Trash2 :size="14" /></button>
          </div></td>
        </tr></tbody>
      </table>
    </div>
  </SurfaceCard>

  <BaseModal :open="createCAOpen" title="创建访问 CA" size="small" @close="createCAOpen = false">
    <p>将生成平台与集群 Agent 使用的信任根。CA 私钥加密保存在平台数据库中。</p>
    <FeedbackBanner v-if="actionError" tone="warning" :message="actionError" />
    <template #actions><button class="btn" type="button" :disabled="saving" @click="createCAOpen = false">取消</button><button class="btn btn-primary" type="button" :disabled="saving" @click="createCA">{{ saving ? '创建中...' : '创建 CA' }}</button></template>
  </BaseModal>

  <BaseModal :open="issueOpen" title="签发访问证书" @close="closeIssue">
    <form id="issue-access-certificate" class="access-cert-form" @submit.prevent="issue">
      <div class="form-group"><label class="form-label" for="access-cluster">集群</label><SelectMenu id="access-cluster" v-model="form.cluster_id" :options="clusterOptions" placeholder="请选择集群" aria-label="集群" /></div>
      <div class="form-group"><label class="form-label" for="access-kind">证书用途</label><SelectMenu id="access-kind" v-model="form.kind" :options="kindOptions" aria-label="证书用途" /></div>
      <div v-if="form.kind === 'agent_server'" class="form-group"><label class="form-label" for="access-csr">Agent 证书签名请求（PEM）</label><textarea id="access-csr" v-model.trim="form.csr" class="form-input access-csr-input" spellcheck="false" placeholder="-----BEGIN CERTIFICATE REQUEST-----" required /></div>
      <FeedbackBanner v-if="actionError" tone="warning" :message="actionError" />
    </form>
    <template #actions><button class="btn" type="button" :disabled="saving" @click="closeIssue">取消</button><button class="btn btn-primary" type="submit" form="issue-access-certificate" :disabled="saving || !form.cluster_id || (form.kind === 'agent_server' && !form.csr.trim())">{{ saving ? '签发中...' : '签发' }}</button></template>
  </BaseModal>

  <BaseModal :open="Boolean(revokeTarget)" title="吊销访问证书" size="small" @close="revokeTarget = null">
    <p>吊销 {{ clusterName(revokeTarget?.cluster_id) }} 的{{ revokeTarget?.kind === 'agent_server' ? 'Agent 服务端' : '平台客户端' }}证书？</p>
    <FeedbackBanner v-if="actionError" tone="warning" :message="actionError" />
    <template #actions><button class="btn" type="button" :disabled="saving" @click="revokeTarget = null">取消</button><button class="btn btn-danger" type="button" :disabled="saving" @click="revoke">{{ saving ? '吊销中...' : '确认吊销' }}</button></template>
  </BaseModal>

  <BaseModal :open="Boolean(deleteTarget)" title="删除未使用证书" size="small" @close="deleteTarget = null">
    <p>确定删除 {{ clusterName(deleteTarget?.cluster_id) }} 的{{ deleteTarget?.kind === 'agent_server' ? 'Agent 服务端' : '平台客户端' }}证书？删除后无法恢复。</p>
    <FeedbackBanner v-if="actionError" tone="warning" :message="actionError" />
    <template #actions><button class="btn" type="button" :disabled="saving" @click="deleteTarget = null">取消</button><button class="btn btn-danger" type="button" :disabled="saving" @click="removeCertificate">{{ saving ? '删除中...' : '确认删除' }}</button></template>
  </BaseModal>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { Download, RefreshCw, ShieldX, Trash2 } from 'lucide-vue-next'
import { deleteAccessCertificate, getAccessCA, getAccessCertificatePEM, getAccessCertificates, getAccessCRL, initializeAccessCA, issueAccessCertificate, revokeAccessCertificate } from '../../api/access-certificates.js'
import { getClusters } from '../../api/clusters.js'
import BaseModal from '../../components/BaseModal.vue'
import EmptyState from '../../components/EmptyState.vue'
import FeedbackBanner from '../../components/FeedbackBanner.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import SurfaceCard from '../../components/SurfaceCard.vue'
import { formatShortDateTime as formatTime } from '../../utils/formatters.js'

const ca = ref(null)
const certificates = ref([])
const clusters = ref([])
const loaded = ref(false)
const loading = ref(false)
const error = ref('')
const actionError = ref('')
const saving = ref(false)
const createCAOpen = ref(false)
const issueOpen = ref(false)
const revokeTarget = ref(null)
const deleteTarget = ref(null)
const form = ref({ cluster_id: '', kind: 'agent_server', csr: '' })
const clusterOptions = computed(() => clusters.value.map(item => ({ value: String(item.id), label: item.name })))
const kindOptions = [{ value: 'agent_server', label: 'Agent 服务端' }, { value: 'platform_client', label: '平台客户端' }]

onMounted(() => { void load() })

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [currentCA, items, clusterList] = await Promise.all([getAccessCA(), getAccessCertificates(), getClusters()])
    ca.value = currentCA
    certificates.value = items || []
    clusters.value = clusterList || []
  } catch (err) { error.value = err.message || '加载访问证书失败' }
  finally { loaded.value = true; loading.value = false }
}

function clusterName(id) { return clusters.value.find(item => item.id === id)?.name || `集群 #${id}` }
function isExpired(value) { return new Date(value).getTime() <= Date.now() }
function statusLabel(item) { return item.revoked_at ? '已吊销' : item.in_use ? '使用中' : isExpired(item.not_after) ? '已过期' : '未使用' }
function statusClass(item) { return item.revoked_at || isExpired(item.not_after) ? 'badge-danger' : item.in_use ? 'badge-online' : 'badge-offline' }
function openCreateCA() { actionError.value = ''; createCAOpen.value = true }
function openIssue() { form.value = { cluster_id: '', kind: 'agent_server', csr: '' }; actionError.value = ''; issueOpen.value = true }
function openRevoke(item) { actionError.value = ''; revokeTarget.value = item }
function openDelete(item) { actionError.value = ''; deleteTarget.value = item }
function closeIssue() { if (!saving.value) issueOpen.value = false }

async function createCA() {
  saving.value = true
  actionError.value = ''
  try { ca.value = await initializeAccessCA(); createCAOpen.value = false }
  catch (err) { actionError.value = err.message || '创建访问 CA 失败' }
  finally { saving.value = false }
}

async function issue() {
  if (!form.value.cluster_id || saving.value) return
  saving.value = true
  actionError.value = ''
  try {
    const item = await issueAccessCertificate({ cluster_id: Number(form.value.cluster_id), kind: form.value.kind, csr: form.value.kind === 'agent_server' ? form.value.csr : '' })
    certificates.value.unshift(item)
    issueOpen.value = false
  } catch (err) { actionError.value = err.message || '签发证书失败' }
  finally { saving.value = false }
}

async function revoke() {
  if (!revokeTarget.value || saving.value) return
  saving.value = true
  actionError.value = ''
  try { await revokeAccessCertificate(revokeTarget.value.id); revokeTarget.value = null; await load() }
  catch (err) { actionError.value = err.message || '吊销证书失败' }
  finally { saving.value = false }
}

async function removeCertificate() {
  if (!deleteTarget.value || saving.value) return
  saving.value = true
  actionError.value = ''
  try { await deleteAccessCertificate(deleteTarget.value.id); deleteTarget.value = null; await load() }
  catch (err) { actionError.value = err.message || '删除访问证书失败' }
  finally { saving.value = false }
}

function download(name, contents) {
  const url = URL.createObjectURL(new Blob([contents], { type: 'application/x-pem-file' }))
  const link = document.createElement('a')
  link.href = url
  link.download = name
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}

async function downloadCertificate(item) {
  try { const result = await getAccessCertificatePEM(item.id); download(`cluster-${item.cluster_id}-${item.kind}.crt`, result.certificate_pem) }
  catch (err) { error.value = err.message || '下载访问证书失败' }
}

async function downloadCRL() {
  try { const result = await getAccessCRL(); download('cylism-access.crl', result.crl_pem) }
  catch (err) { error.value = err.message || '下载吊销列表失败' }
}
</script>

<style scoped>
.access-cert-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 12px; min-height: 62px; padding: 12px var(--space-20); }
.access-cert-summary, .access-cert-actions, .access-cert-row-actions { display: flex; align-items: center; gap: 8px; }
.access-cert-tooltip { position: relative; display: inline-flex; }
.access-cert-tooltip::after { position: absolute; z-index: 20; top: calc(100% + 8px); right: 0; width: max-content; max-width: 280px; padding: 7px 9px; border: 1px solid var(--border); border-radius: var(--radius-control); background: var(--surface-raised); box-shadow: var(--shadow-soft); color: var(--text-primary); content: attr(data-tooltip); font-size: 12px; line-height: 1.4; opacity: 0; pointer-events: none; transform: translateY(-3px); transition: opacity .15s ease, transform .15s ease; white-space: normal; }
.access-cert-tooltip:hover::after, .access-cert-tooltip:focus-within::after { opacity: 1; transform: translateY(0); }
.access-cert-row-actions { justify-content: flex-end; gap: 4px; }
.access-cert-card > .table-wrap { padding: 0 var(--space-20) var(--space-12); }
.access-cert-card > .empty-state { padding: var(--space-24); }
.access-cert-error { margin: 0 var(--space-20) var(--space-12); }
.access-cert-form { display: grid; gap: 12px; }
.access-csr-input { min-height: 140px; font-family: monospace; font-size: 12px; resize: vertical; }
@media (max-width: 760px) {
  .access-cert-toolbar { align-items: stretch; flex-direction: column; padding: 12px 14px; }
  .access-cert-actions { flex-wrap: wrap; }
  .access-cert-card > .table-wrap { padding-right: 14px; padding-left: 14px; }
  .access-cert-error { margin-right: 14px; margin-left: 14px; }
}
</style>
