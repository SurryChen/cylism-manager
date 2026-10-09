<template>
  <section class="certificates-workspace">
    <SurfaceCard v-if="loaded && !ready" class="certificate-status-card">
      <template #header><div><h2 class="card-title">cert-manager {{ statusTitle }}</h2><p class="status-copy">{{ status?.message }}</p></div></template>
      <template #actions><span class="badge" :class="status?.state === 'installing' ? 'badge-deploying' : 'badge-danger'">{{ statusTitle }}</span></template>
      <p v-if="error" class="k8s-banner k8s-banner-warn">{{ error }}</p>
      <div class="modal-actions status-actions"><button v-if="status?.state === 'not_installed' && status?.installer_available" class="btn btn-primary" @click="installCertManager">安装 cert-manager</button><button class="btn" @click="refresh">重新检测</button></div>
    </SurfaceCard>

    <template v-if="loaded && ready">
      <div v-if="error" class="k8s-banner k8s-banner-warn">{{ error }}</div>
      <SurfaceCard padding="none" class="certificate-card">
        <div class="certificate-toolbar">
          <div class="certificate-toolbar-actions">
            <button class="btn" data-testid="open-issuance-config" @click="openIssuanceConfig">证书申请配置</button>
            <button class="btn btn-primary" data-testid="add-certificate" @click="openCertificate">+ 添加证书</button>
          </div>
        </div>

        <div v-if="certs.length" class="table-wrap certificate-table-wrap">
          <table class="data-table certificate-table">
            <colgroup><col class="certificate-name-column" /><col class="certificate-namespace-column" /><col class="certificate-domains-column" /><col class="certificate-issuer-column" /><col class="certificate-secret-column" /><col class="certificate-renewal-column" /><col class="certificate-status-column" /><col class="certificate-actions-column" /></colgroup>
            <thead><tr><th>名称</th><th>命名空间</th><th>域名</th><th>签发者</th><th>TLS Secret</th><th>续期时间</th><th>状态</th><th class="action-cell">操作</th></tr></thead>
            <tbody>
              <tr v-for="cert in certs" :key="cert.namespace + '/' + cert.name">
                <td class="cell-primary"><OverflowTooltip :text="cert.name" /></td>
                <td><OverflowTooltip :text="cert.namespace" /></td>
                <td><OverflowTooltip :text="cert.domains?.join(', ') || '-'" /></td>
                <td><OverflowTooltip :text="[cert.issuer, cert.issuer_kind].filter(Boolean).join(' · ') || '-'" /></td>
                <td><OverflowTooltip :text="cert.secret_name || '-'" /></td>
                <td>{{ formatDate(cert.renewal_time) }}</td>
                <td><span class="badge" :class="cert.status === 'Ready' ? 'badge-online' : cert.status === 'Failed' ? 'badge-danger' : 'badge-deploying'">{{ certificateStatusLabel(cert.status) }}</span></td>
                <td class="action-cell"><div class="certificate-row-actions"><button class="btn btn-sm" @click="router.push(`/cluster/${clusterId}/network/certificates/${encodeURIComponent(cert.namespace)}/${encodeURIComponent(cert.name)}`)">签发过程</button><button class="btn btn-sm btn-danger" @click="deleteCertificate(cert)">删除</button></div></td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-else class="empty-state certificate-empty"><FileCheck2 data-testid="certificate-empty-icon" :size="30" :stroke-width="1.5" /><span class="empty-text">尚未发现证书</span></div>
      </SurfaceCard>
    </template>

    <BaseModal :open="showIssuanceConfig" :title="configView === 'form' ? (setupIssuer ? '编辑证书申请配置' : '新增证书申请配置') : '证书申请配置'" size="large" @close="showIssuanceConfig = false">
      <div v-if="configLoading" class="empty-state config-empty"><span class="empty-text">正在读取证书配置...</span></div>
      <div v-else-if="configError" class="setup-load-error"><p class="k8s-banner k8s-banner-warn">{{ configError }}</p><button type="button" class="btn" @click="openIssuanceConfig">重新读取</button></div>
      <template v-else-if="configView === 'list'">
        <div class="setup-existing">
          <div class="setup-existing-heading"><span class="section-count">{{ issuers.length }} 项</span><button type="button" class="btn btn-primary" data-testid="add-issuer-setup" @click="startNewSetup">新增</button></div>
          <div v-if="issuers.length" class="table-wrap setup-table-wrap"><table class="data-table setup-table"><thead><tr><th>名称</th><th>类型</th><th>验证方式</th><th>状态</th><th class="action-cell">操作</th></tr></thead><tbody><tr v-for="issuer in issuers" :key="issuerKey(issuer)"><td class="cell-primary"><OverflowTooltip :text="issuer.name" /></td><td>{{ issuer.namespace || '集群级' }}</td><td>{{ issuerModeLabel(issuer.mode) }}</td><td><span class="badge" :class="issuer.ready ? 'badge-online' : 'badge-offline'">{{ issuer.ready ? '可用' : '不可用' }}</span></td><td class="action-cell"><div class="certificate-row-actions"><button v-if="canEditSetupIssuer(issuer)" type="button" class="btn btn-sm" @click="selectSetupIssuer(issuer)">编辑</button><button v-else-if="canManageIssuer(issuer) && ['self_signed', 'acme_http01', 'acme_dns01'].includes(issuer.mode)" type="button" class="btn btn-sm" @click="openIssuer(issuer)">编辑</button><button v-if="canManageIssuer(issuer)" type="button" class="btn btn-sm btn-danger" @click="pendingIssuerDelete = issuer">删除</button><span v-else class="cell-secondary">组件管理</span></div></td></tr></tbody></table></div>
          <div v-else class="setup-existing-empty">尚无证书申请配置</div>
        </div>
        <button type="button" class="advanced-toggle" @click="configView = 'advanced'">高级设置</button>
      </template>
      <template v-else-if="configView === 'form'">
        <div v-if="!providers.length" class="empty-state config-empty"><span class="empty-text">暂无可用的域名服务商</span></div>
        <form v-if="providers.length" class="certificate-setup" @submit.prevent="saveCertificateSetup">
          <div class="form-group"><label class="form-label">域名服务商</label><SelectMenu v-model="setupForm.provider" class="form-select" required aria-label="域名服务商" :disabled="Boolean(setupIssuer)" @change="selectSetupProvider"><option value="" disabled>选择域名服务商</option><option v-for="provider in providers" :key="provider.id" :value="provider.id">{{ provider.name }}</option></SelectMenu></div>
          <div v-if="selectedSetupProvider" class="setup-status"><span class="badge" :class="selectedSetupProvider.status?.ready ? 'badge-online' : 'badge-offline'">{{ providerStatusLabel(selectedSetupProvider) }}</span><span>{{ selectedSetupProvider.status?.message }}</span></div>
          <div v-if="selectedSetupProvider?.webhook && !selectedSetupProvider.status?.ready" class="setup-install"><button v-if="selectedSetupProvider.status?.state === 'not_installed'" type="button" class="btn" :disabled="Boolean(installingProvider)" @click="installProvider(selectedSetupProvider.id)">{{ installingProvider ? '安装中...' : '安装域名验证组件' }}</button><button v-else type="button" class="btn" @click="loadProviders">重新检测</button></div>
          <template v-if="selectedSetupProvider?.status?.ready">
            <div class="form-group"><label class="form-label">验证账号</label><SelectMenu v-model="setupForm.credential" class="form-select" aria-label="验证账号"><option value="new">添加账号</option><option v-for="credential in setupCredentials" :key="credential.id" :value="String(credential.id)">{{ credential.name }}</option></SelectMenu></div>
            <div v-if="setupForm.credential === 'new'" class="setup-fields"><div v-for="field in selectedSetupProvider.fields || []" :key="field.key" class="form-group"><label class="form-label" :for="`setup-${field.key}`">{{ field.label }}</label><input :id="`setup-${field.key}`" v-model.trim="setupForm.values[field.key]" class="form-input" :type="field.secret ? 'password' : 'text'" :required="field.required" autocomplete="off" /></div></div>
            <div class="form-group"><label class="form-label" for="setup-email">联系邮箱</label><input id="setup-email" v-model.trim="setupForm.email" class="form-input" type="email" required /></div>
            <div class="setup-status"><span class="badge" :class="setupIssuer?.ready ? 'badge-online' : 'badge-offline'">{{ setupIssuer?.ready ? '可用' : setupIssuer ? '待就绪' : '未配置' }}</span><span v-if="setupIssuer?.reason">{{ setupIssuer.reason }}</span></div>
            <div class="modal-actions"><button type="button" class="btn" @click="configView = 'list'">返回列表</button><button class="btn btn-primary" :disabled="submitting || !setupForm.email || !setupForm.credential">{{ submitting ? '保存中...' : '保存配置' }}</button></div>
          </template>
        </form>
        <button v-if="!selectedSetupProvider?.status?.ready" type="button" class="advanced-toggle" @click="configView = 'list'">返回列表</button>
      </template>
      <button v-if="configView === 'advanced' && !configLoading && !configError" type="button" class="advanced-toggle advanced-back" @click="configView = 'list'">返回列表</button>
      <div v-if="configView === 'advanced' && !configLoading && !configError" class="table-tabs issuance-tabs" role="tablist" aria-label="高级证书设置">
        <button class="tab-btn" :class="{ 'tab-active': issuanceConfigTab === 'issuers' }" role="tab" :aria-selected="issuanceConfigTab === 'issuers'" @click="selectIssuanceConfigTab('issuers')">签发者</button>
        <button class="tab-btn" :class="{ 'tab-active': issuanceConfigTab === 'credentials' }" role="tab" :aria-selected="issuanceConfigTab === 'credentials'" @click="selectIssuanceConfigTab('credentials')">DNS 凭据</button>
        <button class="tab-btn" :class="{ 'tab-active': issuanceConfigTab === 'providers' }" role="tab" :aria-selected="issuanceConfigTab === 'providers'" @click="selectIssuanceConfigTab('providers')">Provider 状态</button>
      </div>

      <div v-if="configView === 'advanced' && !configLoading && !configError && issuanceConfigTab === 'issuers'" class="issuance-config-content">
        <div class="config-toolbar"><span class="section-count">{{ issuers.length }} 项</span><button class="btn btn-primary" @click="openIssuer()">+ 新增签发者</button></div>
        <div v-if="issuersLoading" class="empty-state config-empty"><span class="empty-text">正在读取签发者...</span></div>
        <div v-else-if="issuers.length" class="table-wrap"><table class="data-table"><thead><tr><th>名称</th><th>模式</th><th>Provider</th><th>类型</th><th>状态</th><th class="action-cell">操作</th></tr></thead><tbody><tr v-for="issuer in issuers" :key="issuerKey(issuer)"><td class="cell-primary"><OverflowTooltip :text="issuer.name" /><small class="cell-secondary">{{ issuer.namespace || '集群级' }}</small></td><td>{{ issuerModeLabel(issuer.mode) }}</td><td><OverflowTooltip :text="providerName(issuer.dns_provider)" /></td><td>{{ issuer.kind }}</td><td><span class="badge" :class="issuer.ready ? 'badge-online' : 'badge-danger'">{{ issuer.ready ? '就绪' : '不可用' }}</span><small v-if="issuer.reason" class="cell-secondary">{{ issuer.reason }}</small></td><td class="action-cell"><div class="certificate-row-actions"><button v-if="canManageIssuer(issuer)" class="btn btn-sm" @click="openIssuer(issuer)">编辑</button><button v-if="canManageIssuer(issuer)" class="btn btn-sm btn-danger" @click="deleteIssuer(issuer)">删除</button><span v-else class="cell-secondary">组件管理</span></div></td></tr></tbody></table></div>
        <div v-else class="empty-state config-empty"><span class="empty-text">尚未配置签发者</span></div>
      </div>

      <div v-else-if="configView === 'advanced' && !configLoading && !configError && issuanceConfigTab === 'credentials'" class="issuance-config-content">
        <div class="config-toolbar"><span class="section-count">{{ credentials.length }} 项</span><button class="btn btn-primary" @click="openCredential()">+ 新增 DNS 凭据</button></div>
        <div v-if="credentialsLoading" class="empty-state config-empty"><span class="empty-text">正在读取 DNS 凭据...</span></div>
        <div v-else-if="credentials.length" class="table-wrap"><table class="data-table"><thead><tr><th>名称</th><th>Provider</th><th>命名空间</th><th>已配置字段</th><th>状态</th><th class="action-cell">操作</th></tr></thead><tbody><tr v-for="credential in credentials" :key="credential.id"><td class="cell-primary"><OverflowTooltip :text="credential.name" /></td><td><OverflowTooltip :text="providerName(credential.provider)" /></td><td><OverflowTooltip :text="credential.namespace" /></td><td><OverflowTooltip :text="configuredFieldLabels(credential).join('、') || '-'" /></td><td><span class="badge" :class="credential.enabled ? 'badge-online' : 'badge-offline'">{{ credential.enabled ? '启用' : '停用' }}</span></td><td class="action-cell"><div class="certificate-row-actions"><button class="btn btn-sm" @click="openCredential(credential)">编辑</button><button class="btn btn-sm btn-danger" @click="deleteCredential(credential)">删除</button></div></td></tr></tbody></table></div>
        <div v-else class="empty-state config-empty"><span class="empty-text">尚未配置 DNS 凭据</span></div>
      </div>

      <div v-else-if="configView === 'advanced' && !configLoading && !configError" class="issuance-config-content provider-list">
        <div v-if="providersLoading" class="empty-state config-empty"><span class="empty-text">正在读取 Provider 状态...</span></div>
        <div v-else-if="providers.length" class="provider-grid"><article v-for="provider in providers" :key="provider.id" class="provider-row"><div><strong>{{ provider.name }}</strong><p class="status-copy">{{ provider.description }}</p><p class="status-copy">{{ provider.status?.message }}</p></div><div class="provider-row-actions"><span class="badge" :class="provider.status?.ready ? 'badge-online' : provider.status?.state === 'installing' ? 'badge-deploying' : 'badge-offline'">{{ providerStatusLabel(provider) }}</span><button v-if="provider.webhook && provider.status?.state === 'not_installed'" class="btn btn-sm" :disabled="installingProvider === provider.id" @click="installProvider(provider.id)">{{ installingProvider === provider.id ? '安装中...' : '安装 Webhook' }}</button><button v-else class="btn btn-sm" @click="loadProviders">刷新</button></div></article></div>
        <div v-else class="empty-state config-empty"><span class="empty-text">尚未发现 DNS Provider</span></div>
      </div>
    </BaseModal>
    <ConfirmDialog :open="Boolean(pendingIssuerDelete)" title="删除证书申请配置" :message="`确定删除 ${pendingIssuerDelete?.name || ''}？使用此配置的证书可能无法续期。`" confirm-text="删除" :busy="deletingIssuer" @cancel="pendingIssuerDelete = null" @confirm="deleteSelectedIssuer" />

    <BaseModal :open="showCertificate" title="添加证书" @close="showCertificate = false">
      <form @submit.prevent="createCertificate"><div class="form-row"><div class="form-group"><label class="form-label">名称</label><input v-model.trim="certificateForm.name" class="form-input" placeholder="my-cert" required /></div><div class="form-group"><label class="form-label">命名空间</label><input v-model.trim="certificateForm.namespace" class="form-input" placeholder="default" required /></div></div><div class="form-group"><label class="form-label">域名</label><input v-model="certificateForm.domains" class="form-input" placeholder="example.com,*.example.com" required /></div><div class="form-group"><label class="form-label">签发者</label><SelectMenu v-model="certificateForm.issuer" class="form-select" required :disabled="issuersLoading"><option value="" disabled>{{ issuersLoading ? '正在读取可用签发者...' : '选择可用签发者' }}</option><option v-for="issuer in availableIssuers" :key="issuerKey(issuer)" :value="issuerKey(issuer)">{{ issuer.kind }} · {{ issuer.name }}</option></SelectMenu></div><div class="modal-actions"><button type="button" class="btn" @click="showCertificate = false">取消</button><button class="btn btn-primary" :disabled="submitting || issuersLoading || !availableIssuers.length">创建证书</button></div></form>
    </BaseModal>

    <BaseModal :open="showIssuer" :title="editingIssuer ? '编辑签发者' : '新增签发者'" @close="showIssuer = false">
      <form @submit.prevent="saveIssuer"><div class="form-row"><div class="form-group"><label class="form-label">名称</label><input v-model.trim="issuerForm.name" class="form-input" required :disabled="!!editingIssuer" /></div><div class="form-group"><label class="form-label">类型</label><SelectMenu v-model="issuerForm.kind" class="form-select" :disabled="!!editingIssuer"><option value="ClusterIssuer">ClusterIssuer</option><option value="Issuer">Issuer</option></SelectMenu></div></div><div v-if="issuerForm.kind === 'Issuer'" class="form-group"><label class="form-label">命名空间</label><input v-model.trim="issuerForm.namespace" class="form-input" required :disabled="!!editingIssuer" /></div><div class="form-group"><label class="form-label">签发模式</label><SelectMenu v-model="issuerForm.mode" class="form-select"><option value="acme_http01">ACME HTTP-01</option><option value="acme_dns01">DNS-01</option><option value="self_signed">自签名</option></SelectMenu></div><template v-if="issuerForm.mode !== 'self_signed'"><div class="form-group"><label class="form-label">ACME 邮箱</label><input v-model.trim="issuerForm.email" type="email" class="form-input" required /></div><div v-if="issuerForm.mode === 'acme_http01'" class="form-group"><label class="form-label">Ingress Class</label><input v-model.trim="issuerForm.ingress_class" class="form-input" placeholder="traefik" /></div><template v-else><div class="form-group"><label class="form-label">DNS Provider</label><SelectMenu v-model="issuerForm.dns_provider" class="form-select"><option value="" disabled>选择已就绪 Provider</option><option v-for="provider in readyProviders" :key="provider.id" :value="provider.id">{{ provider.name }}</option></SelectMenu></div><div class="form-group"><label class="form-label">DNS 凭据</label><SelectMenu v-model.number="issuerForm.credential_id" class="form-select" required><option :value="0" disabled>选择凭据</option><option v-for="credential in eligibleCredentials" :key="credential.id" :value="credential.id">{{ credential.name }} · {{ credential.namespace }}</option></SelectMenu></div></template></template><div class="modal-actions"><button type="button" class="btn" @click="showIssuer = false">取消</button><button class="btn btn-primary" :disabled="submitting">保存</button></div></form>
    </BaseModal>

    <BaseModal :open="showCredential" :title="editingCredential ? '编辑 DNS 凭据' : '新增 DNS 凭据'" @close="showCredential = false">
      <form @submit.prevent="saveCredential"><div class="form-row"><div class="form-group"><label class="form-label">名称</label><input v-model.trim="credentialForm.name" class="form-input" required /></div><div class="form-group"><label class="form-label">Provider</label><SelectMenu v-model="credentialForm.provider" class="form-select" :disabled="!!editingCredential"><option v-for="provider in providers" :key="provider.id" :value="provider.id">{{ provider.name }}</option></SelectMenu></div></div><div class="form-group"><label class="form-label">命名空间</label><input v-model.trim="credentialForm.namespace" class="form-input" required /></div><div v-for="field in credentialFields" :key="field.key" class="form-group"><label class="form-label">{{ field.label }}</label><input v-model="credentialForm.values[field.key]" class="form-input" :type="field.secret ? 'password' : 'text'" :placeholder="editingCredential && field.secret ? '留空则保留现有值' : ''" /></div><label class="check-row"><input v-model="credentialForm.enabled" type="checkbox" /> 启用此凭据</label><div class="modal-actions"><button type="button" class="btn" @click="showCredential = false">取消</button><button class="btn btn-primary" :disabled="submitting">保存</button></div></form>
    </BaseModal>
    <ErrorNoticeModal :open="Boolean(mutationError)" :message="mutationError" teleport @close="mutationError = ''" />
  </section>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { FileCheck2 } from 'lucide-vue-next'
import BaseModal from '../../components/BaseModal.vue'
import ConfirmDialog from '../../components/ConfirmDialog.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import OverflowTooltip from '../../components/OverflowTooltip.vue'
import SelectMenu from '../../components/SelectMenu.vue'
import SurfaceCard from '../../components/SurfaceCard.vue'
import { createCertificate as createCertificateRequest, createDNSCredential, createIssuer, deleteCertificate as removeCertificate, deleteDNSCredential, deleteIssuer as removeIssuer, getCertificateIssuers, getCertificates, getCertificateStatus, getDNSCredentials, getDNSProviders, installCertificateManager, installDNSProvider, updateDNSCredential, updateIssuer } from '../../api/certificates.js'
import { useAsyncResource } from '../../composables/useAsyncResource.js'

const router = useRouter()
const props = defineProps({ clusterId: { type: [String, Number], required: true } })
const certs = ref([])
const issuers = ref([])
const credentials = ref([])
const providers = ref([])
const status = ref(null)
const loaded = ref(false)
const statusError = ref('')
const resourceError = ref('')
const mutationError = ref('')
const submitting = ref(false)
const installingProvider = ref('')
const showIssuanceConfig = ref(false)
const configView = ref('list')
const configLoading = ref(false)
const configError = ref('')
const pendingIssuerDelete = ref(null)
const deletingIssuer = ref(false)
const issuanceConfigTab = ref('issuers')
const showCertificate = ref(false)
const showIssuer = ref(false)
const showCredential = ref(false)
const editingIssuer = ref(null)
const editingCredential = ref(null)
const certificateForm = ref(newCertificate())
const issuerForm = ref(newIssuer())
const credentialForm = ref(newCredential())
const setupForm = ref({ provider: '', issuer: '', credential: 'new', values: {}, email: '' })
const createQueryHandled = ref(false)

const error = computed(() => resourceError.value || statusError.value)
const ready = computed(() => status.value?.state === 'ready')
const statusTitle = computed(() => ({ ready: '已就绪', not_installed: '未安装', installing: '安装中', degraded: '异常', unauthorized: '未授权', unavailable: '不可用' }[status.value?.state] || '状态未知'))
const availableIssuers = computed(() => issuers.value.filter(issuer => issuer.ready && (issuer.kind === 'ClusterIssuer' || issuer.namespace === certificateForm.value.namespace)))
const readyProviders = computed(() => providers.value.filter(provider => provider.status?.ready))
const credentialFields = computed(() => providers.value.find(provider => provider.id === credentialForm.value.provider)?.fields || [])
const eligibleCredentials = computed(() => credentials.value.filter(credential => credential.enabled && credential.provider === issuerForm.value.dns_provider && (issuerForm.value.kind === 'ClusterIssuer' || credential.namespace === issuerForm.value.namespace)))
const selectedSetupProvider = computed(() => providers.value.find(provider => provider.id === setupForm.value.provider) || null)
const setupIssuer = computed(() => issuers.value.find(issuer => issuerKey(issuer) === setupForm.value.issuer) || null)
const setupCredentials = computed(() => credentials.value.filter(credential => credential.provider === setupForm.value.provider && credential.enabled && credential.namespace === 'cert-manager'))
const certificateResource = useAsyncResource(async ({ signal }) => {
  const [statusResult, certificatesResult] = await Promise.allSettled([
    getCertificateStatus({ signal }, props.clusterId),
    getCertificates({ signal }, props.clusterId),
  ])
  if (statusResult.status === 'rejected') throw statusResult.reason
  const currentStatus = statusResult.value
  if (currentStatus?.state !== 'ready') return { status: currentStatus, certificates: null }
  if (certificatesResult.status === 'rejected') throw certificatesResult.reason
  return { status: currentStatus, certificates: certificatesResult.value }
}, null)
const issuersResource = useAsyncResource(({ signal }) => getCertificateIssuers({ signal }, props.clusterId), [])
const credentialsResource = useAsyncResource(({ signal }) => getDNSCredentials({ signal }, props.clusterId), [])
const providersResource = useAsyncResource(({ signal }) => getDNSProviders({ signal }, props.clusterId), [])
const issuersLoading = issuersResource.loading
const credentialsLoading = credentialsResource.loading
const providersLoading = providersResource.loading

onMounted(refresh)
watch(() => certificateForm.value.namespace, () => {
  if (!availableIssuers.value.some(issuer => issuerKey(issuer) === certificateForm.value.issuer)) certificateForm.value.issuer = availableIssuers.value[0] ? issuerKey(availableIssuers.value[0]) : ''
})
watch(() => issuerForm.value.dns_provider, () => { issuerForm.value.credential_id = 0 })
watch(() => issuerForm.value.mode, mode => {
  if (showIssuer.value && mode === 'acme_dns01') void Promise.all([loadCredentials(), loadProviders()])
})

async function refresh() {
  statusError.value = ''
  resourceError.value = ''
  mutationError.value = ''
  const result = await certificateResource.refresh()
  if (result) {
    status.value = result.status
    if (result.certificates) {
      certs.value = result.certificates || []
      applyCreateQuery()
    }
  } else if (certificateResource.error.value) {
    resourceError.value = certificateResource.error.value.message || '加载证书管理资源失败'
  }
  loaded.value = true
}

async function loadIssuers() { resourceError.value = ''; const result = await issuersResource.refresh(); if (result !== undefined) issuers.value = result || []; else if (issuersResource.error.value) resourceError.value = issuersResource.error.value.message || '读取签发者失败' }
async function loadCredentials() { resourceError.value = ''; const result = await credentialsResource.refresh(); if (result !== undefined) credentials.value = result || []; else if (credentialsResource.error.value) resourceError.value = credentialsResource.error.value.message || '读取 DNS 凭据失败' }
async function loadProviders() { resourceError.value = ''; const result = await providersResource.refresh(); if (result !== undefined) providers.value = result || []; else if (providersResource.error.value) resourceError.value = providersResource.error.value.message || '读取 DNS Provider 失败' }
async function openCertificate() { showCertificate.value = true; await loadIssuers() }
async function openIssuanceConfig() {
  showIssuanceConfig.value = true
  configView.value = 'list'
  configLoading.value = true
  configError.value = ''
  try {
    await Promise.all([loadIssuers(), loadCredentials(), loadProviders()])
    const failures = [issuersResource.error.value, credentialsResource.error.value, providersResource.error.value].filter(Boolean)
    if (failures.length) {
      configError.value = failures.map(error => error.message || '读取证书配置失败').join('；')
      return
    }
  } finally { configLoading.value = false }
}
function canManageIssuer(issuer) { return !(issuer.namespace === 'cert-manager' && /^cylism-.*-webhook-/.test(issuer.name)) }
function canEditSetupIssuer(issuer) { return canManageIssuer(issuer) && issuer.kind === 'ClusterIssuer' && issuer.mode === 'acme_dns01' && providers.value.some(provider => provider.id === issuer.dns_provider) }
function startNewSetup() {
  setupForm.value = { provider: providers.value[0]?.id || '', issuer: '', credential: 'new', values: {}, email: '' }
  selectSetupProvider()
  configView.value = 'form'
}
function selectSetupIssuer(issuer) {
  if (!canEditSetupIssuer(issuer)) return
  setupForm.value.provider = issuer.dns_provider
  setupForm.value.issuer = issuerKey(issuer)
  selectSetupCredential(issuer)
  configView.value = 'form'
}
function selectSetupProvider() {
  setupForm.value.issuer = ''
  selectSetupCredential(null)
}
function selectSetupCredential(issuer) {
  const linked = issuer?.credential_secret_name && credentials.value.find(credential => credential.provider === setupForm.value.provider && credential.secret_name === issuer.credential_secret_name && credential.enabled)
  const first = linked || (!issuer && setupCredentials.value[0])
  setupForm.value.credential = first ? String(first.id) : 'new'
  setupForm.value.values = {}
  setupForm.value.email = issuer?.email || ''
}
async function saveCertificateSetup() {
  const provider = selectedSetupProvider.value
  if (!provider?.status?.ready || !setupForm.value.email || !setupForm.value.credential) return
  if (setupForm.value.credential !== 'new' && !setupCredentials.value.some(credential => String(credential.id) === setupForm.value.credential)) return
  if (setupForm.value.credential === 'new' && (provider.fields || []).some(field => field.required && !setupForm.value.values[field.key]?.trim())) return
  submitting.value = true
  mutationError.value = ''
  try {
    let credentialID = Number(setupForm.value.credential)
    if (setupForm.value.credential === 'new') {
      const created = await createDNSCredential({ name: `cylism-${provider.id}-dns-${Date.now()}`, namespace: 'cert-manager', provider: provider.id, values: setupForm.value.values, enabled: true }, undefined, props.clusterId)
      credentialID = created.id
    }
    const issuer = setupIssuer.value
    const baseName = `cylism-${provider.id}-certificates`
    const name = issuer?.name || (issuers.value.some(item => item.kind === 'ClusterIssuer' && item.name === baseName) ? `${baseName}-${Date.now()}` : baseName)
    const body = { name, kind: 'ClusterIssuer', mode: 'acme_dns01', dns_provider: provider.id, credential_id: credentialID, email: setupForm.value.email }
    if (issuer) await updateIssuer('ClusterIssuer', '', issuer.name, body, undefined, props.clusterId)
    else await createIssuer(body, undefined, props.clusterId)
    await Promise.all([loadIssuers(), loadCredentials(), loadProviders()])
    configView.value = 'list'
  } catch (err) { mutationError.value = err.message || '保存证书申请配置失败' } finally { submitting.value = false }
}
async function selectIssuanceConfigTab(tab) { issuanceConfigTab.value = tab; if (tab === 'issuers') await loadIssuers(); else if (tab === 'credentials') await loadCredentials(); else await loadProviders() }
async function installCertManager() { mutationError.value = ''; try { await installCertificateManager(undefined, props.clusterId); await refresh() } catch (err) { mutationError.value = err.message || '安装失败' } }
async function installProvider(id) { installingProvider.value = id; mutationError.value = ''; try { await installDNSProvider(id, undefined, props.clusterId); await loadProviders() } catch (err) { mutationError.value = err.message || '安装 Provider 失败' } finally { installingProvider.value = '' } }
async function createCertificate() { const issuer = availableIssuers.value.find(item => issuerKey(item) === certificateForm.value.issuer); if (!issuer) return; submitting.value = true; mutationError.value = ''; try { await createCertificateRequest({ name: certificateForm.value.name, namespace: certificateForm.value.namespace, domains: certificateForm.value.domains.split(',').map(item => item.trim()).filter(Boolean), issuer_ref: issuer.name, issuer_kind: issuer.kind }, undefined, props.clusterId); showCertificate.value = false; await refresh() } catch (err) { mutationError.value = err.message || '创建证书失败' } finally { submitting.value = false } }
async function openIssuer(item = null) { showIssuanceConfig.value = false; editingIssuer.value = item; issuerForm.value = item ? { name: item.name, namespace: item.namespace || '', kind: item.kind, mode: item.mode || 'acme_http01', email: item.email || '', ingress_class: 'traefik', dns_provider: item.dns_provider || '', credential_id: 0 } : newIssuer(); showIssuer.value = true; if (issuerForm.value.mode === 'acme_dns01') await Promise.all([loadCredentials(), loadProviders()]) }
async function saveIssuer() { submitting.value = true; mutationError.value = ''; try { if (editingIssuer.value) await updateIssuer(issuerForm.value.kind, issuerForm.value.namespace, issuerForm.value.name, issuerForm.value, undefined, props.clusterId); else await createIssuer(issuerForm.value, undefined, props.clusterId); showIssuer.value = false; await loadIssuers() } catch (err) { mutationError.value = err.message || '保存签发者失败' } finally { submitting.value = false } }
function deleteIssuer(item) { pendingIssuerDelete.value = item }
async function deleteSelectedIssuer() {
  const item = pendingIssuerDelete.value
  if (!item || deletingIssuer.value) return
  deletingIssuer.value = true
  mutationError.value = ''
  try {
    await removeIssuer(item.kind, item.namespace, item.name, undefined, props.clusterId)
    pendingIssuerDelete.value = null
    await loadIssuers()
  } catch (err) { mutationError.value = err.message || '删除签发者失败' } finally { deletingIssuer.value = false }
}
async function openCredential(item = null) { showIssuanceConfig.value = false; editingCredential.value = item; credentialForm.value = item ? { name: item.name, provider: item.provider, namespace: item.namespace, values: {}, enabled: item.enabled } : newCredential(); showCredential.value = true; await loadProviders() }
async function saveCredential() { submitting.value = true; mutationError.value = ''; try { const values = { ...credentialForm.value.values }; Object.keys(values).forEach(key => { if (!values[key]) delete values[key] }); const body = { ...credentialForm.value, values }; if (editingCredential.value) await updateDNSCredential(editingCredential.value.id, body, undefined, props.clusterId); else await createDNSCredential(body, undefined, props.clusterId); showCredential.value = false; await loadCredentials() } catch (err) { mutationError.value = err.message || '保存 DNS 凭据失败' } finally { submitting.value = false } }
async function deleteCredential(item) { if (!window.confirm(`删除 DNS 凭据 ${item.name}？`)) return; mutationError.value = ''; try { await deleteDNSCredential(item.id, undefined, props.clusterId); await loadCredentials() } catch (err) { mutationError.value = err.message || '删除 DNS 凭据失败' } }
async function deleteCertificate(cert) { if (!window.confirm(`删除证书 ${cert.name}？`)) return; mutationError.value = ''; try { await removeCertificate(cert.namespace, cert.name, undefined, props.clusterId); await refresh() } catch (err) { mutationError.value = err.message || '删除证书失败' } }
function applyCreateQuery() { if (createQueryHandled.value || typeof window === 'undefined' || !ready.value) return; const query = new URLSearchParams(window.location.hash.split('?')[1] || ''); if (query.get('create') !== '1') return; createQueryHandled.value = true; certificateForm.value = { ...newCertificate(), name: query.get('name') || '', namespace: query.get('namespace') || 'cylism-system', domains: query.get('domains') || '' }; showCertificate.value = true; void loadIssuers() }
function issuerKey(issuer) { return `${issuer.kind}/${issuer.namespace || '_'}/${issuer.name}` }
function providerName(id) { return providers.value.find(provider => provider.id === id)?.name || id || '-' }
function providerStatusLabel(provider) { return provider.status?.ready ? '已就绪' : ({ not_installed: '未安装', installing: '安装中' }[provider.status?.state] || '不可用') }
function issuerModeLabel(mode) { return ({ self_signed: '自签名', acme_http01: 'ACME HTTP-01', acme_dns01: 'DNS-01' }[mode] || '-') }
function configuredFieldLabels(credential) { const provider = providers.value.find(item => item.id === credential.provider); return (provider?.fields || []).filter(field => (credential.configured_fields || []).includes(field.key)).map(field => field.label) }
function certificateStatusLabel(value) { return value === 'Ready' ? '就绪' : value === 'Failed' ? '失败' : '签发中' }
function formatDate(value) { return value ? new Date(value).toLocaleString('zh-CN') : '-' }
function newCertificate() { return { name: '', namespace: 'default', domains: '', issuer: '' } }
function newIssuer() { return { name: '', namespace: '', kind: 'ClusterIssuer', mode: 'acme_http01', email: '', ingress_class: 'traefik', dns_provider: '', credential_id: 0 } }
function newCredential() { return { name: '', provider: providers.value[0]?.id || 'alidns', namespace: 'cert-manager', values: {}, enabled: true } }
</script>

<style scoped>
.certificates-workspace { min-width: 0; }
.certificate-status-card, .certificate-card, .k8s-banner { margin-top: 0; }
.k8s-banner + .certificate-card { margin-top: var(--space-20); }
.status-copy, .cell-secondary { display: block; margin: 4px 0 0; color: var(--text-secondary); font-size: 11px; }
.status-actions { justify-content: flex-start; margin-top: var(--space-16); }
.certificate-card { min-height: 260px; }
.certificate-toolbar, .config-toolbar { display: flex; min-height: 62px; align-items: center; justify-content: flex-end; gap: var(--space-12); padding: 12px var(--space-16); }
.certificate-toolbar-actions, .certificate-row-actions, .provider-row-actions { display: flex; align-items: center; justify-content: flex-end; gap: 6px; white-space: nowrap; }
.certificate-table-wrap { padding: 0 var(--space-16); }
.certificate-table { min-width: 1040px; table-layout: fixed; }
.certificate-name-column { width: 150px; }.certificate-namespace-column { width: 130px; }.certificate-domains-column { width: 200px; }.certificate-issuer-column { width: 150px; }.certificate-secret-column { width: 150px; }.certificate-renewal-column { width: 142px; }.certificate-status-column { width: 84px; }.certificate-actions-column { width: 150px; }
.certificate-empty { min-height: 170px; }
.setup-load-error { display: grid; justify-items: start; gap: var(--space-12); }
.setup-load-error .k8s-banner { margin: 0; }
.setup-existing-heading { display: flex; min-height: 42px; align-items: center; justify-content: space-between; margin-bottom: var(--space-8); font-size: 12px; }
.setup-existing-heading .section-count { color: var(--text-secondary); font-weight: 400; }
.setup-table-wrap { max-height: min(48dvh, 380px); overflow: auto; }
.setup-table { min-width: 600px; }
.setup-table .action-cell { width: 130px; }
.setup-existing-empty { padding: var(--space-20) 0; color: var(--text-secondary); font-size: 12px; text-align: center; }
.certificate-setup { display: grid; gap: var(--space-12); }
.certificate-setup .form-group { margin: 0; }
.setup-fields { display: grid; gap: var(--space-12); }
.setup-status { display: flex; align-items: center; gap: var(--space-8); color: var(--text-secondary); font-size: 12px; }
.setup-install { display: flex; justify-content: flex-start; }
.advanced-toggle { margin-top: var(--space-16); padding: 0; border: 0; background: transparent; color: var(--text-secondary); font: inherit; font-size: 12px; cursor: pointer; }
.advanced-toggle:hover { color: var(--action-primary); }
.advanced-back { margin: 0 0 var(--space-16); }
.issuance-tabs { margin-bottom: var(--space-16); }
.issuance-config-content { min-height: 260px; }
.config-toolbar { min-height: 42px; padding: 0 0 var(--space-12); }.config-toolbar .section-count { margin-right: auto; }
.config-empty { min-height: 160px; }
.provider-grid { display: grid; gap: var(--space-8); }
.provider-row { display: flex; align-items: center; justify-content: space-between; gap: var(--space-16); padding: var(--space-12); border: 1px solid var(--border-muted); border-radius: var(--radius-control); background: var(--surface-subtle); }.provider-row strong { font-size: 13px; }
.check-row { display: flex; align-items: center; gap: 8px; color: var(--text-secondary); font-size: 13px; }
@media (max-width: 640px) { .certificate-toolbar-actions { width: 100%; }.certificate-toolbar-actions .btn { flex: 1; }.setup-existing-name { display: grid; gap: 2px; }.provider-row { align-items: flex-start; flex-direction: column; }.provider-row-actions { width: 100%; justify-content: space-between; } }
</style>
