<template>
  <div>
    <button class="back-link" @click="backToWorkspace"><ArrowLeft :size="16" />返回工作台</button>
    <div class="page-header">
      <div><h1 class="page-title">受管域名</h1><p class="page-subtitle">申请和复用命名空间内的 HTTPS 证书</p></div>
      <div class="page-actions"><button class="btn" @click="openImport">接管已有证书</button><button class="btn btn-primary" @click="openCreate">+ 申请 HTTPS 域名</button></div>
    </div>
    <div v-if="error" class="k8s-banner k8s-banner-warn section-gap">{{ error }}</div>
    <section v-if="loaded && domains.length" class="card section-gap">
      <div class="table-wrap"><table class="data-table"><thead><tr><th>域名</th><th>命名空间</th><th>签发者</th><th>证书状态</th><th>TLS Secret</th><th>续期时间</th><th>入口</th><th></th></tr></thead><tbody>
        <tr v-for="domain in domains" :key="domain.id">
          <td class="cell-primary">{{ domain.hostname }}<small v-if="domain.certificate_ownership === 'imported'" class="cell-secondary">导入证书</small><small v-if="domain.description" class="cell-secondary">{{ domain.description }}</small></td>
          <td>{{ domain.namespace || '未绑定' }}</td><td>{{ domain.issuer_ref || '-' }}</td>
          <td><span class="badge" :class="certificateClass(domain)">{{ certificateLabel(domain) }}</span><small v-if="certificateReason(domain)" class="cell-secondary">{{ certificateReason(domain) }}</small></td>
          <td>{{ domain.tls_secret_name || '-' }}</td><td>{{ formatDate(domain.certificate?.renewal_time) }}</td><td>{{ domain.application_count || 0 }}</td>
          <td class="action-cell"><div class="btn-group"><button v-if="domain.namespace && domain.certificate_ownership !== 'imported'" class="btn btn-sm" @click="retry(domain)">重试签发</button><button v-if="domain.certificate_name" class="btn btn-sm" @click="openOperations(domain)">签发过程</button><button class="btn btn-sm" @click="openEdit(domain)">编辑</button><button class="btn btn-sm btn-danger" :disabled="domain.application_count > 0" @click="remove(domain)">删除</button></div></td>
        </tr>
      </tbody></table></div>
    </section>
    <div v-else-if="loaded" class="empty-state"><span class="empty-text">尚未申请受管 HTTPS 域名</span></div>
    <p v-if="loaded" class="dns-note">证书签发会自动完成 ACME DNS-01 TXT 验证。业务 A、AAAA 或 CNAME 记录仍需指向集群公网入口。</p>

    <div v-if="modal" class="overlay" @click.self="close"><div class="modal"><h2 class="modal-title">{{ editing ? '编辑受管域名' : '申请 HTTPS 域名' }}</h2><form @submit.prevent="save">
      <div class="form-group"><label class="form-label">域名</label><input v-model.trim="form.hostname" class="form-input" placeholder="api.example.com" required :disabled="!!editing" /></div>
      <div class="form-group"><label class="form-label">所属环境</label><input class="form-input" :value="currentEnvironment ? `${currentEnvironment.name} · ${currentEnvironment.namespace}` : '请先在顶部选择项目与环境'" disabled /></div>
      <div class="form-group"><label class="form-label">ClusterIssuer</label><select v-model="form.issuer_ref" class="form-select" required><option value="" disabled>选择已就绪签发者</option><option v-for="issuer in issuers" :key="issuer.name" :value="issuer.name">{{ issuer.name }}</option></select></div>
      <div class="form-group"><label class="form-label">说明</label><input v-model.trim="form.description" class="form-input" placeholder="生产 API" /></div>
      <label class="check-row"><input v-model="form.enabled" type="checkbox" /> 启用此域名</label>
      <div class="modal-actions"><button type="button" class="btn" @click="close">取消</button><button class="btn btn-primary" :disabled="saving || !currentEnvironment || !issuers.length">{{ saving ? '提交中...' : '提交申请' }}</button></div>
    </form></div></div>

    <div v-if="importModal" class="overlay" @click.self="closeImport"><div class="modal"><h2 class="modal-title">接管已有证书</h2><form @submit.prevent="importCertificate">
      <div class="form-group"><label class="form-label">所属环境</label><input class="form-input" :value="currentEnvironment ? `${currentEnvironment.name} · ${currentEnvironment.namespace}` : ''" disabled /></div>
      <div class="form-group"><label class="form-label">Certificate</label><select v-model="importForm.certificate_name" class="form-select" required><option value="" disabled>{{ importCandidates.length ? '选择已有 Certificate' : '当前环境没有可接管证书' }}</option><option v-for="certificate in importCandidates" :key="certificate.name" :value="certificate.name">{{ certificate.name }} · {{ certificate.domains[0] }}</option></select></div>
      <div v-if="selectedImportCertificate" class="certificate-preview"><span>{{ selectedImportCertificate.domains[0] }}</span><small>{{ selectedImportCertificate.issuer_kind || 'ClusterIssuer' }} · {{ selectedImportCertificate.issuer }}</small><small>TLS Secret: {{ selectedImportCertificate.secret_name }}</small></div>
      <div class="form-group"><label class="form-label">说明</label><input v-model.trim="importForm.description" class="form-input" placeholder="接管已有证书" /></div>
      <label class="check-row"><input v-model="importForm.enabled" type="checkbox" /> 启用此域名</label>
      <p class="form-hint">接管只创建平台域名记录，不会修改、重新签发或删除原 Certificate 和 TLS Secret。</p>
      <div class="modal-actions"><button type="button" class="btn" @click="closeImport">取消</button><button class="btn btn-primary" :disabled="saving || !importForm.certificate_name">{{ saving ? '接管中...' : '确认接管' }}</button></div>
    </form></div></div>
  </div>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowLeft } from 'lucide-vue-next'
import { api } from '../api/index.js'

const router = useRouter()
const route = useRoute()
const domains = ref([]), projects = ref([]), allIssuers = ref([]), importCandidates = ref([]), loaded = ref(false), error = ref(''), modal = ref(false), importModal = ref(false), editing = ref(null), saving = ref(false)
const projectID = computed(() => Number(route.query.project_id) || 0)
const environmentID = computed(() => Number(route.query.environment_id) || 0)
const currentProject = computed(() => projects.value.find(project => project.id === projectID.value) || null)
const currentEnvironment = computed(() => currentProject.value?.environments?.find(environment => environment.id === environmentID.value) || null)
const form = ref(blank())
const importForm = ref(blankImport())
const issuers = computed(() => allIssuers.value.filter(issuer => issuer.kind === 'ClusterIssuer' && issuer.ready))
const selectedImportCertificate = computed(() => importCandidates.value.find(certificate => certificate.name === importForm.value.certificate_name) || null)
function blank(){ return { hostname: '', environment_id: environmentID.value, issuer_ref: '', description: '', enabled: true } }
function blankImport(){ return { environment_id: environmentID.value, certificate_name: '', description: '', enabled: true } }
async function load(){ try { const unassigned = route.query.unassigned === 'true'; const [domainResult, projectResult] = await Promise.all([unassigned ? api.get('/domains?unassigned=true') : environmentID.value ? api.get(`/domains?environment_id=${environmentID.value}`) : api.get('/domains'), api.get('/projects')]); domains.value = domainResult || []; projects.value = projectResult || [] } catch(e) { error.value = e.message || '加载受管域名失败' } finally { loaded.value = true } }
async function loadOptions(){ allIssuers.value = await api.get('/certs/issuers') || [] }
async function openCreate(){ error.value = ''; editing.value = null; if (!currentEnvironment.value) { error.value = '请先在顶部选择项目与环境'; return } form.value = blank(); try { await loadOptions(); form.value.issuer_ref = issuers.value[0]?.name || ''; modal.value = true } catch(e) { error.value = e.message || '加载签发前置条件失败' } }
async function openImport(){ error.value = ''; if (!currentEnvironment.value) { error.value = '请先从工作台进入对应项目与环境'; return } try { importCandidates.value = await api.get(`/domains/importable-certificates?environment_id=${environmentID.value}`) || []; importForm.value = blankImport(); importModal.value = true } catch(e) { error.value = e.message || '加载可接管证书失败' } }
async function openEdit(domain){ error.value = ''; if (!domain.environment_id && !currentEnvironment.value) { error.value = '请先在顶部选择要重新绑定的项目与环境'; return } editing.value = domain; form.value = { hostname: domain.hostname, environment_id: domain.environment_id || environmentID.value, issuer_ref: domain.issuer_ref || '', description: domain.description || '', enabled: domain.enabled }; try { await loadOptions(); modal.value = true } catch(e) { error.value = e.message || '加载签发前置条件失败' } }
function close(){ modal.value = false; editing.value = null }
function closeImport(){ importModal.value = false; importCandidates.value = [] }
async function backToWorkspace(){ await router.push({ path: '/applications', query: { project_id: route.query.project_id, environment_id: route.query.environment_id } }) }
async function save(){ saving.value = true; error.value = ''; try { if(editing.value) await api.put(`/domains/${editing.value.id}`, form.value); else await api.post('/domains', form.value); close(); await load() } catch(e) { error.value = e.message || '提交域名申请失败' } finally { saving.value = false } }
async function importCertificate(){ saving.value = true; error.value = ''; try { await api.post('/domains/import', importForm.value); closeImport(); await load() } catch(e) { error.value = e.message || '接管已有证书失败' } finally { saving.value = false } }
async function retry(domain){ error.value = ''; try { await api.post(`/domains/${domain.id}/certificate`); await load() } catch(e) { error.value = e.message || '重新申请证书失败' } }
function openOperations(domain){ router.push(`/certs/${domain.namespace}/${domain.certificate_name}`) }
async function remove(domain){ if(domain.application_count > 0) return; const suffix = domain.certificate_ownership === 'imported' ? '平台域名记录？原 Certificate 和 TLS Secret 会保留。' : '及其 Certificate？'; if(!window.confirm(`删除受管域名 ${domain.hostname} ${suffix}`)) return; try { await api.delete(`/domains/${domain.id}`); await load() } catch(e) { error.value = e.message || '删除受管域名失败' } }
function certificateLabel(domain){ if(!domain.namespace) return '未绑定'; return domain.certificate?.status === 'Ready' ? '已就绪' : domain.certificate?.status === 'Failed' ? '签发失败' : '签发中' }
function certificateClass(domain){ return certificateLabel(domain) === '已就绪' ? 'badge-online' : certificateLabel(domain) === '签发失败' ? 'badge-danger' : 'badge-deploying' }
function certificateReason(domain){ return domain.certificate?.reason || domain.certificate_error || '' }
function formatDate(value){ return value ? new Date(value).toLocaleString('zh-CN') : '-' }
onMounted(load)
watch(() => [route.query.project_id, route.query.environment_id, route.query.unassigned], load)
</script>

<style scoped>
.back-link{display:inline-flex;align-items:center;gap:6px;margin:0 0 var(--space-16);padding:0;border:0;background:transparent;color:var(--text-secondary);font:inherit;font-size:13px;font-weight:600;cursor:pointer}.back-link:hover{color:var(--action-primary)}.back-link:focus-visible{outline:2px solid var(--focus);outline-offset:3px}.page-header,.page-actions{display:flex;justify-content:space-between;gap:var(--space-16)}.page-actions{justify-content:flex-end}.page-subtitle,.cell-secondary,.dns-note,.form-hint{margin:var(--space-4) 0;color:var(--text-secondary);font-size:12px}.cell-secondary{display:block;max-width:180px;overflow-wrap:anywhere}.dns-note{padding:0 var(--space-4)}.check-row{display:flex;gap:8px;align-items:center;color:var(--text-secondary);font-size:13px}.certificate-preview{display:grid;gap:4px;padding:10px;border:1px solid var(--border-muted);border-radius:var(--radius-control);background:var(--surface-subtle);color:var(--text-primary);font-size:12px}.certificate-preview small{color:var(--text-secondary)}@media(max-width:640px){.page-header{flex-direction:column}.page-actions{width:100%}.page-actions .btn,.page-header>.btn{width:100%}}
</style>
