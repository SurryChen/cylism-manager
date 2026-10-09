<template>
  <div>
    <button class="back-link" @click="backToWorkspace"><ArrowLeft :size="16" />返回工作台</button>
    <SectionTabsHeader title="受管域名" :tabs="[{ id: 'domains', label: '域名' }]" active-tab="domains" />
    <FeedbackBanner v-if="readError" tone="warning" :message="readError" />
    <SurfaceCard padding="none">
      <div class="domain-toolbar"><button class="btn" @click="openImport">导入域名</button><button class="btn btn-primary" @click="openCreate">新建域名</button></div>
      <div v-if="loaded && domains.length" class="table-wrap domain-table"><table class="data-table"><thead><tr><th>域名</th><th>命名空间</th><th>解析地址</th><th>DNS</th><th>签发者</th><th>证书状态</th><th>TLS Secret</th><th>续期时间</th><th>入口</th><th></th></tr></thead><tbody>
        <tr v-for="domain in domains" :key="domain.id">
          <td class="cell-primary">{{ domain.hostname }}<small v-if="domain.certificate_ownership === 'imported'" class="cell-secondary">导入证书</small><small v-if="domain.description" class="cell-secondary">{{ domain.description }}</small></td>
          <td>{{ domain.namespace || '未绑定' }}</td><td>{{ domain.dns_addresses?.join('、') || '-' }}</td><td><span v-if="domain.cloud_connection_id" class="badge" :class="domain.dns_status === 'ready' ? 'badge-online' : 'badge-danger'">{{ domain.dns_status === 'ready' ? '已同步' : domain.dns_status === 'failed' ? '解析失败' : '待处理' }}</span><span v-else>-</span><small v-if="domain.dns_error" class="cell-secondary">{{ domain.dns_error }}</small></td><td>{{ domain.issuer_ref || '-' }}</td>
          <td><span class="badge" :class="certificateClass(domain)">{{ certificateLabel(domain) }}</span><small v-if="certificateReason(domain)" class="cell-secondary">{{ certificateReason(domain) }}</small></td>
          <td>{{ domain.tls_secret_name || '-' }}</td><td>{{ formatDate(domain.certificate?.renewal_time) }}</td><td>{{ domain.application_count || 0 }}</td>
          <td class="action-cell"><div class="btn-group"><button v-if="domain.cloud_connection_id && domain.dns_status !== 'ready'" class="btn btn-sm" @click="retryDNS(domain)">重试解析</button><button v-if="domain.namespace && domain.certificate_ownership !== 'imported'" class="btn btn-sm" @click="retry(domain)">重试签发</button><button v-if="domain.certificate_name" class="btn btn-sm" @click="openOperations">到集群查看</button><button class="btn btn-sm" @click="openEdit(domain)">编辑</button><button class="btn btn-sm btn-danger" :disabled="domain.application_count > 0" @click="remove(domain)">删除</button></div></td>
        </tr>
      </tbody></table></div>
      <EmptyState v-else :variant="loaded ? 'empty' : 'loading'" :message="loaded ? '暂无受管域名' : '加载中...'" />
    </SurfaceCard>

    <div v-if="modal" class="overlay" @click.self="close"><div class="modal"><h2 class="modal-title">{{ editing ? '编辑受管域名' : '新建域名' }}</h2><form @submit.prevent="save">
      <template v-if="!editing"><div class="form-group"><label class="form-label">云 DNS 连接</label><SelectMenu v-model.number="form.cloud_connection_id" class="form-select" required><option :value="0" disabled>选择云连接</option><option v-for="connection in dnsConnections" :key="connection.id" :value="connection.id">{{ connection.name }}</option></SelectMenu></div>
      <div class="form-group"><label class="form-label">主域名</label><SelectMenu v-model="form.dns_zone" class="form-select" required><option value="" disabled>选择主域名</option><option v-for="zone in createZones" :key="zone.name" :value="zone.name">{{ zone.name }}</option></SelectMenu></div>
      <div class="form-group"><label class="form-label">三级域名前缀</label><input v-model.trim="form.prefix" class="form-input" placeholder="api" pattern="[a-zA-Z0-9](?:[a-zA-Z0-9-]*[a-zA-Z0-9])?" required /><small v-if="form.dns_zone" class="form-hint">{{ form.prefix || 'api' }}.{{ form.dns_zone }}</small></div></template>
      <div v-else class="form-group"><label class="form-label">域名</label><input v-model.trim="form.hostname" class="form-input" disabled /></div>
      <div class="form-group"><label class="form-label">所属环境</label><input class="form-input" :value="currentEnvironment ? `${currentEnvironment.name} · ${currentEnvironment.namespace}` : '请先在顶部选择项目与环境'" disabled /></div>
      <div v-if="!editing || editing.cloud_connection_id" class="form-group"><label class="form-label">解析节点</label><div class="node-options"><label v-for="node in dnsNodes" :key="node.id" class="check-row"><input v-model="form.node_ids" type="checkbox" :value="node.id" />{{ node.name }} · {{ node.ip }}</label></div></div>
      <div class="form-group"><label class="form-label">ClusterIssuer</label><SelectMenu v-model="form.issuer_ref" class="form-select" required><option value="" disabled>选择已就绪签发者</option><option v-for="issuer in issuers" :key="issuer.name" :value="issuer.name">{{ issuer.name }}</option></SelectMenu></div>
      <div class="form-group"><label class="form-label">说明</label><input v-model.trim="form.description" class="form-input" placeholder="生产 API" /></div>
      <label class="check-row"><input v-model="form.enabled" type="checkbox" /> 启用此域名</label>
      <div class="modal-actions"><button type="button" class="btn" @click="close">取消</button><button class="btn btn-primary" :disabled="saving || !currentEnvironment || !issuers.length || (!editing && (!form.cloud_connection_id || !form.dns_zone || !form.node_ids.length))">{{ saving ? '提交中...' : '保存' }}</button></div>
    </form></div></div>

    <div v-if="importModal" class="overlay" @click.self="closeImport"><div class="modal"><h2 class="modal-title">导入域名</h2><form @submit.prevent="importCertificate">
      <div class="form-group"><label class="form-label">所属环境</label><input class="form-input" :value="currentEnvironment ? `${currentEnvironment.name} · ${currentEnvironment.namespace}` : ''" disabled /></div>
      <div class="form-group"><label class="form-label">云域名与证书</label><SelectMenu v-model="importForm.candidate_key" class="form-select" required><option value="" disabled>{{ importCandidates.length ? '选择云 DNS 与证书均存在的域名' : '没有可导入的域名' }}</option><option v-for="item in importCandidates" :key="candidateKey(item)" :value="candidateKey(item)">{{ item.certificate.domains[0] }} · {{ item.certificate.name }}</option></SelectMenu></div>
      <div v-if="selectedImportCertificate" class="certificate-preview"><span>{{ selectedImportCertificate.certificate.domains[0] }}</span><small>{{ selectedImportCertificate.certificate.issuer_kind || 'ClusterIssuer' }} · {{ selectedImportCertificate.certificate.issuer }}</small><small>当前解析: {{ selectedImportCertificate.addresses.join('、') }}</small></div>
      <div class="form-group"><label class="form-label">解析节点</label><div class="node-options"><label v-for="node in dnsNodes" :key="node.id" class="check-row"><input v-model="importForm.node_ids" type="checkbox" :value="node.id" />{{ node.name }} · {{ node.ip }}</label></div></div>
      <div class="form-group"><label class="form-label">说明</label><input v-model.trim="importForm.description" class="form-input" placeholder="导入已有证书" /></div>
      <label class="check-row"><input v-model="importForm.enabled" type="checkbox" /> 启用此域名</label>
      <p class="form-hint">导入会将业务 A 记录同步到所选节点；原 Certificate 和 TLS Secret 保持不变。</p>
      <div class="modal-actions"><button type="button" class="btn" @click="closeImport">取消</button><button class="btn btn-primary" :disabled="saving || !importForm.candidate_key || !importForm.node_ids.length">{{ saving ? '导入中...' : '确认导入' }}</button></div>
    </form></div></div>

    <ErrorNoticeModal :open="Boolean(actionError)" :message="actionError" teleport @close="actionError = ''" />
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowLeft } from 'lucide-vue-next'
import { getProjects } from '../../api/applications.js'
import { createManagedDomain, deleteManagedDomain, getClusterDomainOptions, getDomainOptions, getManagedDomains, importManagedDomainCertificate, retryManagedDomainCertificate, updateManagedDomain } from '../../api/domains.js'
import { getDomainDNSOptions, getImportableCloudDomains, retryManagedDomainDNS } from '../../api/domains.js'
import { useAsyncResource } from '../../composables/useAsyncResource.js'
import { usePolling } from '../../composables/usePolling.js'
import SurfaceCard from '../../components/SurfaceCard.vue'
import SectionTabsHeader from '../../components/SectionTabsHeader.vue'
import FeedbackBanner from '../../components/FeedbackBanner.vue'
import EmptyState from '../../components/EmptyState.vue'
import ErrorNoticeModal from '../../components/ErrorNoticeModal.vue'
import { formatDateTime as formatDate } from '../../utils/formatters.js'

const router = useRouter()
const route = useRoute()
const domains = ref([]), projects = ref([]), allIssuers = ref([]), importCandidates = ref([]), loaded = ref(false), readError = ref(''), actionError = ref(''), modal = ref(false), importModal = ref(false), editing = ref(null), saving = ref(false)
const projectID = computed(() => Number(route.query.project_id) || 0)
const environmentID = computed(() => Number(route.query.environment_id) || 0)
const currentProject = computed(() => projects.value.find(project => project.id === projectID.value) || null)
const currentEnvironment = computed(() => currentProject.value?.environments?.find(environment => environment.id === environmentID.value) || null)
function environmentByID(id){ return projects.value.flatMap(project => project.environments || []).find(environment => environment.id === id) || null }
const dnsConnections = ref([]), dnsNodes = ref([])
const form = ref(blank())
const importForm = ref(blankImport())
const createZones = computed(() => dnsConnections.value.find(connection => connection.id === form.value.cloud_connection_id)?.zones || [])
const domainResource = useAsyncResource(async ({ signal }) => {
  const unassigned = route.query.unassigned === 'true'
  const [domainResult, projectResult] = await Promise.all([
    getManagedDomains({ environmentID: environmentID.value || undefined, unassigned }, { signal }),
    getProjects({ signal }),
  ])
  return { domainResult, projectResult }
}, null)
const issuers = computed(() => allIssuers.value.filter(issuer => issuer.kind === 'ClusterIssuer' && issuer.ready))
const selectedImportCertificate = computed(() => importCandidates.value.find(item => candidateKey(item) === importForm.value.candidate_key) || null)
const statusPolling = usePolling(load, { interval: 5000 })
function blank(){ return { hostname: '', prefix: '', environment_id: environmentID.value, issuer_ref: '', description: '', enabled: true, cloud_connection_id: 0, dns_zone: '', node_ids: [] } }
function blankImport(){ return { environment_id: environmentID.value, candidate_key: '', description: '', enabled: true, node_ids: dnsNodes.value?.map(node => node.id) || [] } }
function candidateKey(item){ return `${item.cloud_connection_id}:${item.dns_zone}:${item.certificate.name}` }
async function load(){ readError.value = ''; const result = await domainResource.refresh(); if (result) { domains.value = result.domainResult || []; projects.value = result.projectResult || [] } else if (domainResource.error.value) readError.value = domainResource.error.value.message || '加载受管域名失败'; loaded.value = true; syncStatusPolling() }
async function loadOptions(environment = currentEnvironment.value){ allIssuers.value = environment?.cluster_id ? await getClusterDomainOptions(environment.cluster_id) || [] : await getDomainOptions() || [] }
async function loadDNSOptions(id = environmentID.value){ const options = await getDomainDNSOptions(id); dnsConnections.value = options?.connections || []; dnsNodes.value = options?.nodes || [] }
async function openCreate(){ actionError.value = ''; editing.value = null; if (!currentEnvironment.value) { actionError.value = '请先在顶部选择项目与环境'; return } form.value = blank(); try { await Promise.all([loadOptions(), loadDNSOptions()]); form.value.issuer_ref = issuers.value[0]?.name || ''; form.value.cloud_connection_id = dnsConnections.value[0]?.id || 0; form.value.node_ids = dnsNodes.value.map(node => node.id); modal.value = true } catch(e) { actionError.value = e.message || '加载签发前置条件失败' } }
async function openImport(){ actionError.value = ''; if (!currentEnvironment.value) { actionError.value = '请先从工作台进入对应项目与环境'; return } try { await loadDNSOptions(); importCandidates.value = await getImportableCloudDomains(environmentID.value) || []; importForm.value = blankImport(); importModal.value = true } catch(e) { actionError.value = e.message || '加载可导入域名失败' } }
async function openEdit(domain){ actionError.value = ''; if (!domain.environment_id && !currentEnvironment.value) { actionError.value = '请先在顶部选择要重新绑定的项目与环境'; return } editing.value = domain; const targetID = domain.environment_id || environmentID.value; form.value = { hostname: domain.hostname, environment_id: targetID, issuer_ref: domain.issuer_ref || '', description: domain.description || '', enabled: domain.enabled, cloud_connection_id: domain.cloud_connection_id, dns_zone: domain.dns_zone, node_ids: [...(domain.dns_node_ids || [])] }; try { await Promise.all([loadOptions(environmentByID(targetID)), loadDNSOptions(targetID)]); modal.value = true } catch(e) { actionError.value = e.message || '加载签发前置条件失败' } }
function close(){ modal.value = false; editing.value = null }
function closeImport(){ importModal.value = false; importCandidates.value = [] }
async function backToWorkspace(){ await router.push({ path: '/applications', query: { project_id: route.query.project_id, environment_id: route.query.environment_id } }) }
async function save(){ saving.value = true; actionError.value = ''; try { if(editing.value) await updateManagedDomain(editing.value.id, form.value); else await createManagedDomain({ ...form.value, hostname: `${form.value.prefix}.${form.value.dns_zone}` }); close(); await load() } catch(e) { actionError.value = e.message || '提交域名申请失败' } finally { saving.value = false } }
async function importCertificate(){ saving.value = true; actionError.value = ''; try { const candidate = selectedImportCertificate.value; if (!candidate) throw new Error('请选择可导入域名'); const { candidate_key, ...input } = importForm.value; await importManagedDomainCertificate({ ...input, certificate_name: candidate.certificate.name, cloud_connection_id: candidate.cloud_connection_id, dns_zone: candidate.dns_zone }); closeImport(); await load() } catch(e) { actionError.value = e.message || '导入域名失败' } finally { saving.value = false } }
async function retryDNS(domain){ actionError.value = ''; try { await retryManagedDomainDNS(domain.id); await load() } catch(e) { actionError.value = e.message || '同步域名解析失败' } }
async function retry(domain){ actionError.value = ''; try { await retryManagedDomainCertificate(domain.id); await load() } catch(e) { actionError.value = e.message || '重新申请证书失败' } }
function openOperations(){ router.push('/cluster?tab=list') }
async function remove(domain){ if(domain.application_count > 0) return; const suffix = domain.certificate_ownership === 'imported' ? '平台域名记录？原云 DNS 记录、Certificate 和 TLS Secret 会保留。' : '及其云 DNS 记录和 Certificate？'; if(!window.confirm(`删除受管域名 ${domain.hostname} ${suffix}`)) return; try { await deleteManagedDomain(domain.id); await load() } catch(e) { actionError.value = e.message || '删除受管域名失败' } }
function certificateLabel(domain){ if(!domain.namespace) return '未绑定'; return domain.certificate?.status === 'Ready' ? '已就绪' : domain.certificate?.status === 'Failed' ? '签发失败' : '签发中' }
function certificateClass(domain){ return certificateLabel(domain) === '已就绪' ? 'badge-online' : certificateLabel(domain) === '签发失败' ? 'badge-danger' : 'badge-deploying' }
function certificateReason(domain){ return domain.certificate?.reason || domain.certificate_error || '' }
function shouldPollStatus(domain){ return domain.namespace && domain.certificate_ownership !== 'imported' && (domain.certificate?.status === 'Issuing' || domain.certificate_error === '证书尚未创建') }
function syncStatusPolling(){ const pending = route.query.unassigned !== 'true' && domains.value.some(shouldPollStatus); if (pending) statusPolling.start(); else statusPolling.stop() }
onMounted(load)
onBeforeUnmount(statusPolling.stop)
watch(() => [route.query.project_id, route.query.environment_id, route.query.unassigned], load)
watch(() => form.value.cloud_connection_id, () => { form.value.dns_zone = createZones.value[0]?.name || '' })
</script>

<style scoped>
.back-link{display:inline-flex;align-items:center;gap:6px;margin:0 0 var(--space-16);padding:0;border:0;background:transparent;color:var(--text-secondary);font:inherit;font-size:13px;font-weight:600;cursor:pointer}.back-link:hover{color:var(--action-primary)}.back-link:focus-visible{outline:2px solid var(--focus);outline-offset:3px}.domain-toolbar{display:flex;justify-content:flex-end;gap:8px;padding:14px 20px}.domain-table{padding:0 20px}.cell-secondary,.form-hint{margin:var(--space-4) 0;color:var(--text-secondary);font-size:12px}.cell-secondary{display:block;max-width:180px;overflow-wrap:anywhere}.check-row{display:flex;gap:8px;align-items:center;color:var(--text-secondary);font-size:13px}.node-options{display:grid;gap:8px;max-height:150px;overflow:auto;padding:10px;border:1px solid var(--border-muted);border-radius:var(--radius-control)}.certificate-preview{display:grid;gap:4px;padding:10px;border:1px solid var(--border-muted);border-radius:var(--radius-control);background:var(--surface-subtle);color:var(--text-primary);font-size:12px}.certificate-preview small{color:var(--text-secondary)}@media(max-width:640px){.domain-toolbar{flex-wrap:wrap;padding:12px 14px}.domain-toolbar .btn{flex:1 1 auto}.domain-table{padding:0 14px}}
</style>
