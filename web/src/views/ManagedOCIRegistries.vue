<template>
  <div class="page-shell registry-page">
    <header class="page-header">
      <div>
        <h1 class="page-title">自托管制品库</h1>
        <p class="page-subtitle">由 Cylism 在集群中部署和管理的 OCI 制品库，为平台应用提供私有镜像存储与分发能力。</p>
      </div>
      <button data-testid="deploy-registry" class="btn btn-primary" :disabled="!loaded || Boolean(registry)" @click="openCreate"><Plus :size="16" /> 部署制品库</button>
    </header>

    <p v-if="pageError" class="form-error" role="alert">{{ pageError }}</p>
    <div v-if="!loaded" class="empty-state">正在读取制品库配置...</div>
    <section v-else-if="!registry" class="registry-empty" data-testid="registry-empty">
      <Database :size="26" />
      <div><h2>尚未部署自托管制品库</h2><p>部署后，平台会创建受认证保护、使用持久化存储并通过集群入口提供服务的 OCI Registry。</p></div>
    </section>
    <template v-else>
      <section class="metric-grid registry-overview" data-testid="registry-summary">
        <article class="metric registry-metric"><span>访问地址</span><strong class="registry-endpoint">{{ endpointURL(registry) }}</strong><small>{{ registry.insecure_http ? 'HTTP，凭据和镜像层以明文传输' : `HTTPS · ${registry.certificate_name || '未关联证书'}` }}</small></article>
        <article class="metric registry-metric"><span>运行状态</span><strong><span class="badge" :class="statusBadgeClass(registry.status)"><i class="badge-dot"></i>{{ statusLabel(registry.status) }}</span></strong><small>{{ registry.last_error || 'Registry 工作负载状态已同步' }}</small></article>
        <article class="metric registry-metric"><span>持久化存储</span><strong>{{ registry.pvc_name }}</strong><small>{{ registry.storage_class_name }} · {{ registry.storage_size }} · {{ registry.pvc_phase || '等待状态同步' }}</small></article>
      </section>

      <section class="card registry-panel" data-testid="registry-configuration">
        <header class="card-header registry-panel-header"><div><h2 class="card-title">Registry 配置</h2><p>凭据仅以加密形式保存，不会再次展示。</p></div><div class="section-actions"><button v-if="needsRepair" data-testid="repair-registry" class="btn" :disabled="repairing" @click="repairRegistry"><RefreshCw :size="16" :class="{ 'is-spinning': repairing }" /> {{ repairing ? '修复中...' : '修复' }}</button><button class="icon-button" title="刷新状态" aria-label="刷新状态" :disabled="refreshing" @click="load"><RefreshCw :size="17" :class="{ 'is-spinning': refreshing }" /></button><button class="btn" @click="openEdit">编辑</button><button class="btn btn-danger" @click="deleteOpen = true">删除</button></div></header>
        <dl class="registry-properties"><div><dt>Registry 镜像</dt><dd>{{ registry.registry_image }}</dd></div><div><dt>命名空间</dt><dd>{{ registry.namespace }}</dd></div><div><dt>数据节点</dt><dd>{{ registry.data_node }}</dd></div><div><dt>运行资源</dt><dd>CPU {{ registry.cpu_request }} - {{ registry.cpu_limit }} · 内存 {{ registry.memory_request }} - {{ registry.memory_limit }}</dd></div><div><dt>拉取账号</dt><dd>{{ registry.pull_username }}</dd></div><div><dt>TLS 证书</dt><dd>{{ registry.insecure_http ? '不使用（HTTP）' : registry.certificate_name || '未关联平台证书' }}</dd></div></dl>
      </section>

    </template>

    <Teleport to="body"><div v-if="showForm" class="overlay registry-overlay" @click.self="closeForm"><div class="modal registry-modal"><h2 class="modal-title">{{ registry ? '编辑制品库' : '部署制品库' }}</h2><form @submit.prevent="save">
      <div class="form-group"><label class="form-label">名称</label><input v-model.trim="form.name" class="form-input" required placeholder="平台制品库" /></div>
      <div class="form-row"><div class="form-group"><label class="form-label">命名空间</label><input :value="form.namespace" class="form-input" readonly /><p class="form-hint">平台受管制品库固定部署在 <code>cylism-system</code>。</p></div><div class="form-group"><label class="form-label">Registry 镜像</label><input v-model.trim="form.registry_image" class="form-input" required placeholder="registry:2" /></div></div>
      <div class="form-group"><div class="volume-heading"><label class="form-label">持久化存储卷</label><a class="btn btn-sm" :href="pvcCreateLink()">创建 PVC</a></div><select v-model="form.pvc_name" class="form-select registry-pvc-select" required :disabled="Boolean(registry) || pvcOptionsLoading"><option value="" disabled>{{ pvcOptionsLoading ? '正在读取可用 PVC...' : '选择 cylism-system 中已有的 local-path PVC' }}</option><option v-for="pvc in pvcOptions" :key="pvc.name" :value="pvc.name">{{ pvc.name }} · {{ pvc.storage }} · {{ pvc.phase }}<template v-if="pvc.bound_node"> · {{ pvc.bound_node }}</template></option></select><p class="form-hint">仅显示 <code>local-path</code>、<code>ReadWriteOnce</code> 且未被工作负载引用的 PVC。平台只挂载所选 PVC，不会创建、修改或删除它。</p><p v-if="!pvcOptionsLoading && !pvcOptions.length" class="form-hint">没有可选 PVC，请先在存储卷管理中创建。</p></div>
      <div class="form-row"><div class="form-group"><label class="form-label">数据节点</label><select v-model="form.data_node" class="form-select registry-data-node-select" required :disabled="Boolean(registry) || Boolean(selectedPVC?.bound_node) || registryDataNodes.length === 0"><option value="" disabled>选择承载数据的 Kubernetes 节点</option><option v-for="node in registryDataNodes" :key="node" :value="node">{{ node }}</option></select><p class="form-hint">待绑定 PVC 会在此节点首次挂载时创建本地卷；已绑定 PVC 自动使用其绑定节点。</p></div><div class="form-group"><label class="form-label">所选存储</label><div class="form-input registry-storage-summary">{{ selectedPVC ? `${selectedPVC.storage_class_name} · ${selectedPVC.storage} · ${selectedPVC.phase}` : '选择 PVC 后显示' }}</div><p class="form-hint">StorageClass 和容量由已选 PVC 决定。</p></div></div>
      <section class="resource-config"><div class="section-heading"><div><h3>运行资源</h3><p>为 Registry Pod 预留并限制 CPU 与内存用量。</p></div></div><div class="resource-grid"><label class="form-group"><span class="form-label">CPU Request</span><input v-model.trim="form.cpu_request" class="form-input" required placeholder="100m" /></label><label class="form-group"><span class="form-label">CPU Limit</span><input v-model.trim="form.cpu_limit" class="form-input" required placeholder="500m" /></label><label class="form-group"><span class="form-label">内存 Request</span><input v-model.trim="form.memory_request" class="form-input" required placeholder="256Mi" /></label><label class="form-group"><span class="form-label">内存 Limit</span><input v-model.trim="form.memory_limit" class="form-input" required placeholder="1Gi" /></label></div></section>
      <p v-if="storagePreflight && !storagePreflight.ready" class="form-error">{{ storagePreflight.message }}</p>
      <div class="form-group"><label class="form-label">传输方式</label><div class="transport-options"><label><input v-model="form.insecure_http" :value="false" type="radio" /> HTTPS</label><label><input v-model="form.insecure_http" :value="true" type="radio" /> HTTP</label></div></div>
      <div v-if="!form.insecure_http" class="form-group"><div class="volume-heading"><label class="form-label">TLS 证书</label><a class="btn btn-sm" :href="certificateCreateLink()">创建证书</a></div><select v-model="form.certificate_name" class="form-select registry-certificate-select" required :disabled="certificatesLoading" @change="selectCertificate"><option value="" disabled>{{ certificatesLoading ? '正在读取可用证书...' : '选择 cylism-system 中已就绪的证书' }}</option><option v-for="certificate in certificateOptions" :key="certificate.namespace + '/' + certificate.name" :value="certificate.name">{{ certificate.name }} · {{ certificate.domains.join(', ') }}</option></select><p class="form-hint">选择证书后会自动填入其第一个精确域名；证书续期会由 cert-manager 自动写入 Ingress 使用的 Secret。</p><p v-if="certificateError" class="form-error">{{ certificateError }}</p><p v-else-if="!certificatesLoading && !certificateOptions.length" class="form-hint">没有可用证书。请先在 <code>cylism-system</code> 创建已就绪的证书。</p></div>
      <label v-else class="risk-confirm"><input v-model="form.confirm_insecure_http" type="checkbox" required /> 我确认 HTTP 会以明文传输镜像层与拉取凭据。</label>
      <div class="form-group"><label class="form-label">访问地址</label><input v-model.trim="form.endpoint" class="form-input" required :disabled="Boolean(registry)" placeholder="registry.example.com:31813" /><p class="form-hint">平台会创建对应 Host 的 Ingress，将该地址转发到 Registry；后续节点用它推送和拉取镜像，创建后不可直接修改。</p><p v-if="selectedCertificate && !firstExactCertificateDomain(selectedCertificate)" class="form-hint">所选证书仅包含通配符域名，请填写其覆盖的具体子域名。</p></div>
      <div class="form-row"><div class="form-group"><label class="form-label">拉取账号</label><input v-model.trim="form.pull_username" class="form-input" required placeholder="cylism-pull" /></div><div class="form-group"><label class="form-label">{{ registry ? '新拉取密码（留空保持不变）' : '拉取密码' }}</label><input v-model="form.pull_password" class="form-input" type="password" :required="!registry" minlength="12" autocomplete="new-password" /></div></div>
      <div class="modal-actions"><button class="btn" type="button" @click="closeForm">取消</button><button class="btn btn-primary" :disabled="submitting || !storagePreflight?.ready || (!form.insecure_http && (!form.certificate_name || certificatesLoading))">{{ submitting ? '保存中...' : registry ? '保存配置' : '开始部署' }}</button></div>
    </form></div></div></Teleport>

    <Teleport to="body"><div v-if="deleteOpen" class="overlay registry-overlay" @click.self="deleteOpen = false"><div class="modal"><h2 class="modal-title">删除制品库</h2><p class="confirm-copy">会删除 Deployment、Service、Ingress 和认证 Secret，但会保留 PVC <code>{{ registry.pvc_name }}</code> 中的镜像数据。</p><div class="modal-actions"><button class="btn" @click="deleteOpen = false">取消</button><button class="btn btn-danger" :disabled="submitting" @click="deleteRegistry">确认删除</button></div></div></div></Teleport>
    <Teleport to="body"><div v-if="actionError" class="overlay registry-notice-overlay" @click.self="actionError = null"><section class="modal registry-notice-modal" role="alertdialog" aria-modal="true" :aria-label="actionError.title"><div class="registry-notice-icon"><AlertCircle :size="22" /></div><h2 class="modal-title">{{ actionError.title }}</h2><p class="registry-notice-text">{{ actionError.message }}</p><div class="modal-actions"><button class="btn btn-danger" @click="actionError = null">确定</button></div></section></div></Teleport>
  </div>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { AlertCircle, Database, Plus, RefreshCw } from 'lucide-vue-next'
import { api } from '../api/index.js'

const registry = ref(null); const storagePreflight = ref(null); const pvcOptions = ref([]); const pvcOptionsLoading = ref(false); const certificateOptions = ref([]); const certificateError = ref(''); const certificatesLoading = ref(false); const loaded = ref(false); const refreshing = ref(false); const repairing = ref(false); const pageError = ref(''); const actionError = ref(null); const showForm = ref(false); const deleteOpen = ref(false); const submitting = ref(false); const form = ref(newForm())
const registryDataNodes = computed(() => storagePreflight.value?.data_nodes || [])
const selectedPVC = computed(() => pvcOptions.value.find(item => item.name === form.value.pvc_name) || null)
const selectedCertificate = computed(() => certificateOptions.value.find(item => item.name === form.value.certificate_name) || null)
const needsRepair = computed(() => ['degraded', 'failed', 'pending'].includes(registry.value?.status))
function newForm() { return { name: '', namespace: 'cylism-system', endpoint: '', registry_image: 'registry:2', data_node: '', pvc_name: '', cpu_request: '100m', cpu_limit: '500m', memory_request: '256Mi', memory_limit: '1Gi', insecure_http: false, confirm_insecure_http: false, certificate_name: '', pull_username: 'cylism-pull', pull_password: '', project_ids: [] } }
function endpointURL(item) { return `${item.insecure_http ? 'http' : 'https'}://${item.endpoint}` }
function statusLabel(status) { return ({ ready: '就绪', deploying: '部署中', failed: '失败', degraded: '异常', pending: '待部署', migration_required: '需迁移' })[status] || '未知' }
function statusBadgeClass(status) { return status === 'ready' ? 'badge-online' : status === 'failed' || status === 'degraded' || status === 'migration_required' ? 'badge-danger' : 'badge-deploying' }
function showActionError(title, err, fallback) { actionError.value = { title, message: err.message || fallback } }
async function load() { refreshing.value = true; pageError.value = ''; pvcOptionsLoading.value = true; try { const [registries, preflight, pvcs] = await Promise.all([api.get('/managed-oci-registries'), api.get('/managed-oci-registries/storage-preflight'), api.get('/managed-oci-registries/pvcs')]); registry.value = registries[0] || null; storagePreflight.value = preflight; pvcOptions.value = pvcs || [] } catch (err) { pageError.value = err.message || '加载制品库失败' } finally { loaded.value = true; refreshing.value = false; pvcOptionsLoading.value = false } }
function openCreate() { actionError.value = null; form.value = newForm(); showForm.value = true }
function openEdit() { actionError.value = null; const item = registry.value; form.value = { ...newForm(), name: item.name, endpoint: item.endpoint, registry_image: item.registry_image, data_node: item.data_node, pvc_name: item.pvc_name, cpu_request: item.cpu_request, cpu_limit: item.cpu_limit, memory_request: item.memory_request, memory_limit: item.memory_limit, insecure_http: item.insecure_http, confirm_insecure_http: item.insecure_http, certificate_name: item.certificate_name, pull_username: item.pull_username }; showForm.value = true }
function closeForm() { showForm.value = false; form.value = newForm(); certificateOptions.value = []; certificateError.value = '' }
function firstExactCertificateDomain(certificate) { return certificate?.domains?.find(domain => !domain.startsWith('*.')) || '' }
function selectCertificate() { if (registry.value) return; form.value.endpoint = firstExactCertificateDomain(selectedCertificate.value) }
async function loadCertificates() { certificateOptions.value = []; certificateError.value = ''; if (!showForm.value || form.value.insecure_http) return; certificatesLoading.value = true; try { const options = await api.get('/managed-oci-registries/certificates?namespace=cylism-system'); certificateOptions.value = options || []; if (!certificateOptions.value.some(item => item.name === form.value.certificate_name)) form.value.certificate_name = '' } catch (err) { certificateError.value = err.message || '读取可用证书失败' } finally { certificatesLoading.value = false } }
function endpointHostname() { try { return new URL(`https://${form.value.endpoint}`).hostname } catch { return '' } }
function certificateCreateLink() { const params = new URLSearchParams({ create: '1', namespace: 'cylism-system' }); const hostname = endpointHostname(); if (hostname) params.set('domains', hostname); return `#/network?tab=certificates&${params}` }
function pvcCreateLink() { const params = new URLSearchParams({ create: '1', namespace: 'cylism-system', name: 'cylism-oci-registry-data', storage_class_name: 'local-path', storage: '100Gi' }); if (form.value.data_node) params.set('node_name', form.value.data_node); return `#/storage?${params}` }
async function save() { submitting.value = true; actionError.value = null; try { const payload = { ...form.value }; if (registry.value && !payload.pull_password) delete payload.pull_password; if (registry.value) await api.put(`/managed-oci-registries/${registry.value.id}`, payload); else await api.post('/managed-oci-registries', payload); closeForm(); await load() } catch (err) { showActionError(registry.value ? '保存配置失败' : '部署失败', err, '保存制品库失败') } finally { submitting.value = false } }
async function repairRegistry() { if (!registry.value) return; repairing.value = true; actionError.value = null; try { await api.post(`/managed-oci-registries/${registry.value.id}/repair`); await load() } catch (err) { showActionError('修复制品库失败', err, '重新同步制品库资源失败') } finally { repairing.value = false } }
async function deleteRegistry() { submitting.value = true; actionError.value = null; try { await api.delete(`/managed-oci-registries/${registry.value.id}`, { confirm: true }); deleteOpen.value = false; await load() } catch (err) { showActionError('删除制品库失败', err, '删除制品库失败') } finally { submitting.value = false } }
onMounted(load)
watch([showForm, () => form.value.insecure_http], loadCertificates)
watch(() => selectedPVC.value?.bound_node, boundNode => { if (boundNode) form.value.data_node = boundNode })
</script>

<style scoped>
.page-header, .section-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: var(--space-16); }
.page-subtitle, .section-heading p, .form-hint, .registry-empty p, .confirm-copy { margin: 4px 0 0; color: var(--text-secondary); font-size: 13px; }
.registry-empty { display: flex; align-items: center; gap: 16px; margin-top: 28px; padding: 24px 0; border-top: 1px solid var(--border-muted); border-bottom: 1px solid var(--border-muted); }
.registry-empty h2 { margin: 0; font-size: 16px; }

.registry-overview { margin-bottom: var(--space-16); }
.registry-metric { display: grid; min-width: 0; min-height: 126px; align-content: start; gap: 8px; }
.registry-metric > span { color: var(--text-muted); font-size: 11px; }
.registry-metric strong { min-width: 0; color: var(--text-primary); font-size: 15px; line-height: 1.35; overflow-wrap: anywhere; }
.registry-metric small { color: var(--text-secondary); font-size: 12px; line-height: 1.5; overflow-wrap: anywhere; }
.registry-endpoint { color: var(--action-primary) !important; font-family: var(--font-mono); }

.registry-panel { margin-bottom: var(--space-16); padding: 0; }
.registry-panel-header { min-height: 70px; margin: 0; padding: 16px 18px; border-bottom: 1px solid var(--border-muted); gap: var(--space-16); }
.registry-panel-header p { margin: 4px 0 0; color: var(--text-secondary); font-size: 12px; line-height: 1.5; }
.section-actions, .volume-heading { display: flex; align-items: center; gap: 8px; }
.section-actions { flex-wrap: wrap; justify-content: flex-end; }
.volume-heading { justify-content: space-between; margin-bottom: 8px; }
.registry-properties { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); margin: 0; }
.registry-properties > div { min-width: 0; min-height: 88px; padding: 15px 18px; border-right: 1px solid var(--border-muted); border-bottom: 1px solid var(--border-muted); }
.registry-properties > div:nth-child(3n) { border-right: 0; }
.registry-properties > div:nth-last-child(-n + 3) { border-bottom: 0; }
.registry-properties dt { color: var(--text-muted); font-size: 10px; font-weight: 700; letter-spacing: .06em; text-transform: uppercase; }
.registry-properties dd { margin: 8px 0 0; color: var(--text-primary); font-size: 12px; line-height: 1.55; overflow-wrap: anywhere; }

.transport-options { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 8px; }
.transport-options label { display: flex; align-items: center; gap: 8px; padding: 9px 10px; border: 1px solid var(--border-muted); border-radius: var(--radius-control); font-size: 13px; }
.resource-config { margin: 20px 0; padding: 16px 0; border-top: 1px solid var(--border-muted); border-bottom: 1px solid var(--border-muted); }
.resource-config h3 { margin: 0; font-size: 14px; }
.resource-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; margin-top: 14px; }
.registry-modal { width: min(620px, calc(100vw - 32px)); }
.registry-overlay { z-index: 3000; }
.registry-notice-overlay { z-index: 3100; }
.registry-notice-modal { width: min(420px, calc(100vw - 32px)); }
.registry-notice-icon { display: grid; width: 46px; height: 46px; margin-bottom: 14px; place-items: center; border-radius: 50%; background: var(--danger-surface); color: var(--danger); }
.registry-notice-modal .modal-title { margin-bottom: 0; }
.registry-notice-text { margin: 10px 0 0; color: var(--text-secondary); font-size: 13px; line-height: 1.7; overflow-wrap: anywhere; }
.risk-confirm { display: flex; gap: 8px; margin: 0 0 14px; color: var(--danger); font-size: 13px; }
.is-spinning { animation: spin .8s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }

@media (max-width: 760px) {
  .page-header, .section-heading, .registry-panel-header { flex-direction: column; align-items: stretch; }
  .registry-properties { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .registry-properties > div:nth-child(3n) { border-right: 1px solid var(--border-muted); }
  .registry-properties > div:nth-child(2n) { border-right: 0; }
  .registry-properties > div:nth-last-child(-n + 3) { border-bottom: 1px solid var(--border-muted); }
  .registry-properties > div:nth-last-child(-n + 2) { border-bottom: 0; }
  .resource-grid { grid-template-columns: 1fr; }
}
@media (max-width: 480px) {
  .registry-properties { grid-template-columns: 1fr; }
  .registry-properties > div, .registry-properties > div:nth-child(3n) { border-right: 0; border-bottom: 1px solid var(--border-muted); }
  .registry-properties > div:last-child { border-bottom: 0; }
  .section-actions { justify-content: flex-start; }
  .section-actions .btn, .registry-panel-header > .btn { flex: 1 1 auto; }
}
</style>
