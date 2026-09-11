<template>
  <section class="settings-section">
    <div class="section-heading">
      <div>
        <h2 class="section-title">平台入口</h2>
        <p class="settings-copy">管理域名、TLS 证书和 Ingress 现在集中在这里。</p>
      </div>
    </div>

    <section class="card platform-endpoint-card">
      <div class="card-header">
        <div>
          <h2 class="card-title">平台管理入口</h2>
          <p class="settings-copy">选择网络证书页中已就绪的证书；平台只维护 default 命名空间的 HTTPS Ingress。</p>
        </div>
        <span class="badge" :class="endpointBadgeClass">{{ endpointStateLabel }}</span>
      </div>
      <form class="platform-endpoint-form" @submit.prevent="savePlatformEndpoint">
        <div class="form-row">
          <div class="form-group">
            <label class="form-label" for="platform-endpoint-hostname">管理域名</label>
            <input id="platform-endpoint-hostname" v-model.trim="endpointForm.hostname" class="form-input" :disabled="savingEndpoint" required placeholder="console.example.com" />
          </div>
          <div class="form-group">
            <label class="form-label" for="platform-endpoint-certificate">TLS 证书</label>
            <select id="platform-endpoint-certificate" v-model="endpointForm.certificate_name" class="form-select" :disabled="savingEndpoint" required>
              <option value="">选择 default 命名空间中已就绪的证书</option>
              <option v-for="certificate in readyPlatformCertificates" :key="certificate.name" :value="certificate.name">{{ certificate.name }} · {{ certificate.domains.join(', ') }}</option>
            </select>
          </div>
        </div>
        <div class="endpoint-control-row">
          <p class="settings-copy">保存后将创建或同步平台 Ingress，并使用所选证书的 TLS Secret。</p>
          <div class="settings-action-row">
            <a v-if="platformEndpoint.url && platformEndpoint.state === 'ready'" class="btn btn-sm" :href="platformEndpoint.url" target="_blank" rel="noopener">打开入口</a>
            <button v-if="platformEndpoint.endpoint?.enabled && !platformEndpoint.ingress_ready" class="btn btn-sm" type="button" :disabled="adoptingEndpoint || syncingEndpoint || savingEndpoint" @click="adoptPlatformIngress">{{ adoptingEndpoint ? '接管中...' : '接管现有 Ingress' }}</button>
            <button v-if="platformEndpoint.endpoint?.hostname" class="btn btn-sm" type="button" :disabled="syncingEndpoint || savingEndpoint" @click="reconcilePlatformEndpoint">{{ syncingEndpoint ? '同步中...' : '重新同步' }}</button>
            <button v-if="platformEndpoint.endpoint?.enabled" class="btn btn-sm btn-danger" type="button" :disabled="savingEndpoint" @click="showDisableEndpointConfirmation = true">停用入口</button>
            <button class="btn btn-sm btn-primary" :disabled="savingEndpoint" type="submit">{{ savingEndpoint ? '保存中...' : '保存入口' }}</button>
          </div>
        </div>
      </form>
      <div v-if="platformEndpoint.endpoint?.hostname" class="platform-endpoint-status">
        <span class="detail-label">HTTPS 地址</span>
        <a v-if="platformEndpoint.url" :href="platformEndpoint.url" target="_blank" rel="noopener">{{ platformEndpoint.url }}</a>
        <span v-else>-</span>
        <span class="detail-label">Ingress</span>
        <span v-if="platformEndpoint.ingress">{{ platformEndpoint.ingress.namespace }}/{{ platformEndpoint.ingress.name }}<template v-if="platformEndpoint.ingress.ingress_class"> · {{ platformEndpoint.ingress.ingress_class }}</template></span>
        <span v-else>等待同步</span>
        <template v-if="platformEndpoint.ingress">
          <span class="detail-label">Ingress 路由</span>
          <code>{{ platformEndpoint.ingress.hostname || '-' }}{{ platformEndpoint.ingress.path || '/' }} -> {{ platformEndpoint.ingress.service_name || '-' }}:{{ platformEndpoint.ingress.service_port || '-' }}</code>
          <span class="detail-label">Ingress TLS Secret</span>
          <code>{{ platformEndpoint.ingress.tls_secret_name || '-' }}</code>
        </template>
        <span class="detail-label">TLS 证书</span>
        <span>{{ platformEndpoint.certificate?.name || platformEndpoint.endpoint.certificate_name }} · {{ platformEndpoint.certificate?.status || '等待读取' }}<template v-if="platformEndpoint.certificate?.reason"> · {{ platformEndpoint.certificate.reason }}</template></span>
        <template v-if="platformEndpoint.certificate">
          <span class="detail-label">证书到期时间</span>
          <span>{{ formatDateTime(platformEndpoint.certificate.expiry_date) }}</span>
          <span class="detail-label">下次续期时间</span>
          <span>{{ formatDateTime(platformEndpoint.certificate.renewal_time) }}</span>
        </template>
        <template v-if="platformEndpoint.certificate_error">
          <span class="detail-label">协调错误</span>
          <span class="endpoint-error-text">{{ platformEndpoint.certificate_error }}</span>
        </template>
      </div>
      <p v-if="endpointMessage" class="settings-copy platform-action-message">{{ endpointMessage }}</p>
      <p v-if="endpointError" class="settings-copy endpoint-error">{{ endpointError }}</p>
    </section>

    <Teleport to="body">
      <div v-if="showDisableEndpointConfirmation" class="overlay" @click.self="showDisableEndpointConfirmation = false">
        <div class="modal endpoint-disable-modal">
          <h2 class="modal-title">停用平台管理入口</h2>
          <p class="settings-copy">将删除平台管理的 Ingress，域名将不再通过该入口访问。所选证书和 TLS Secret 不会删除。</p>
          <div class="modal-actions">
            <button class="btn" type="button" :disabled="savingEndpoint" @click="showDisableEndpointConfirmation = false">取消</button>
            <button class="btn btn-danger" type="button" :disabled="savingEndpoint" @click="disablePlatformEndpoint">{{ savingEndpoint ? '停用中...' : '确认停用' }}</button>
          </div>
        </div>
      </div>
    </Teleport>
  </section>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { adoptPlatformIngress as adoptPlatformIngressRequest, getPlatformCertificates, getPlatformEndpoint, reconcilePlatformEndpoint as reconcilePlatformEndpointRequest, updatePlatformEndpoint } from '../../api/settings.js'
import { useAsyncResource } from '../../composables/useAsyncResource.js'
import { formatDateTime } from '../../utils/formatters.js'

const platformEndpoint = ref({ endpoint: {}, state: 'not_configured', ingress_ready: false })
const endpointForm = ref({ hostname: '', certificate_name: '' })
const certificates = ref([])
const endpointMessage = ref('')
const endpointError = ref('')
const savingEndpoint = ref(false)
const syncingEndpoint = ref(false)
const adoptingEndpoint = ref(false)
const showDisableEndpointConfirmation = ref(false)
const endpointResource = useAsyncResource(async ({ signal }) => Promise.allSettled([
  getPlatformEndpoint({ signal }),
  getPlatformCertificates({ signal }),
]), null)

const readyPlatformCertificates = computed(() => certificates.value.filter(certificate => certificate.namespace === 'default' && certificate.status === 'Ready' && certificate.domains?.length))
const endpointStateLabel = computed(() => ({ not_configured: '待配置', disabled: '已停用', ready: '已就绪', waiting_certificate: '证书未就绪', waiting_ingress: '等待 Ingress', failed: '签发失败', unavailable: '不可用' }[platformEndpoint.value.state] || '未知'))
const endpointBadgeClass = computed(() => ({ ready: 'badge-online', failed: 'badge-danger', unavailable: 'badge-danger', waiting_certificate: 'badge-deploying', waiting_ingress: 'badge-deploying' }[platformEndpoint.value.state] || 'badge-offline'))

onMounted(() => {
  void refresh({ syncForm: true })
})

async function refresh({ syncForm = false } = {}) {
  endpointError.value = ''
  const results = await endpointResource.refresh()
  if (!results) return
  const [endpointResult, certificatesResult] = results
  if (endpointResult.status === 'fulfilled') syncEndpoint(endpointResult.value, { syncForm })
  else endpointError.value = endpointResult.reason?.message || '读取平台入口失败'
  if (certificatesResult.status === 'fulfilled') certificates.value = certificatesResult.value || []
  else if (!endpointError.value) endpointError.value = certificatesResult.reason?.message || '读取可用证书失败'
}

function syncEndpoint(info, { syncForm = true } = {}) {
  platformEndpoint.value = info || { endpoint: {}, state: 'not_configured', ingress_ready: false }
  if (!syncForm) return
  endpointForm.value = {
    hostname: info?.endpoint?.hostname || '',
    certificate_name: info?.endpoint?.certificate_name || '',
  }
}

async function savePlatformEndpoint() {
  savingEndpoint.value = true
  endpointMessage.value = ''
  endpointError.value = ''
  try {
    syncEndpoint(await updatePlatformEndpoint({ ...endpointForm.value, enabled: true }))
    endpointMessage.value = '平台入口已保存，正在同步 Ingress。'
  } catch (e) { endpointError.value = e.message || '保存平台入口失败' } finally { savingEndpoint.value = false }
}

async function disablePlatformEndpoint() {
  savingEndpoint.value = true
  endpointMessage.value = ''
  endpointError.value = ''
  try {
    const endpoint = platformEndpoint.value.endpoint || {}
    syncEndpoint(await updatePlatformEndpoint({
      hostname: endpoint.hostname || endpointForm.value.hostname,
      certificate_name: endpoint.certificate_name || endpointForm.value.certificate_name,
      enabled: false,
    }))
    showDisableEndpointConfirmation.value = false
    endpointMessage.value = '平台入口已停用，Ingress 已移除。'
  } catch (e) { endpointError.value = e.message || '停用平台入口失败' } finally { savingEndpoint.value = false }
}

async function adoptPlatformIngress() {
  adoptingEndpoint.value = true
  endpointMessage.value = ''
  endpointError.value = ''
  try {
    syncEndpoint(await adoptPlatformIngressRequest())
    endpointMessage.value = '现有 Ingress 已接管并重新同步。'
  } catch (e) { endpointError.value = e.message || '接管现有 Ingress 失败' } finally { adoptingEndpoint.value = false }
}

async function reconcilePlatformEndpoint() {
  syncingEndpoint.value = true
  endpointMessage.value = ''
  endpointError.value = ''
  try {
    syncEndpoint(await reconcilePlatformEndpointRequest())
    endpointMessage.value = '平台入口已重新同步。'
  } catch (e) { endpointError.value = e.message || '重新同步平台入口失败' } finally { syncingEndpoint.value = false }
}

defineExpose({ refresh })
</script>

<style scoped src="./SystemSettings.shared.css"></style>
