<template>
  <div>
    <div class="page-header">
      <div><h1 class="page-title">受管域名</h1><p class="page-subtitle">申请和复用命名空间内的 HTTPS 证书</p></div>
      <button class="btn btn-primary" @click="openCreate">+ 申请 HTTPS 域名</button>
    </div>
    <div v-if="error" class="k8s-banner k8s-banner-warn section-gap">{{ error }}</div>
    <section v-if="loaded && domains.length" class="card section-gap">
      <div class="table-wrap"><table class="data-table"><thead><tr><th>域名</th><th>命名空间</th><th>签发者</th><th>证书状态</th><th>TLS Secret</th><th>续期时间</th><th>入口</th><th></th></tr></thead><tbody>
        <tr v-for="domain in domains" :key="domain.id">
          <td class="cell-primary">{{ domain.hostname }}<small v-if="domain.description" class="cell-secondary">{{ domain.description }}</small></td>
          <td>{{ domain.namespace || '未绑定' }}</td><td>{{ domain.issuer_ref || '-' }}</td>
          <td><span class="badge" :class="certificateClass(domain)">{{ certificateLabel(domain) }}</span><small v-if="certificateReason(domain)" class="cell-secondary">{{ certificateReason(domain) }}</small></td>
          <td>{{ domain.tls_secret_name || '-' }}</td><td>{{ formatDate(domain.certificate?.renewal_time) }}</td><td>{{ domain.application_count || 0 }}</td>
          <td class="action-cell"><div class="btn-group"><button v-if="domain.namespace" class="btn btn-sm" @click="retry(domain)">重试签发</button><button v-if="domain.certificate_name" class="btn btn-sm" @click="openOperations(domain)">签发过程</button><button class="btn btn-sm" @click="openEdit(domain)">编辑</button><button class="btn btn-sm btn-danger" :disabled="domain.application_count > 0" @click="remove(domain)">删除</button></div></td>
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
  </div>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api/index.js'

const router = useRouter()
const route = useRoute()
const domains = ref([]), projects = ref([]), allIssuers = ref([]), loaded = ref(false), error = ref(''), modal = ref(false), editing = ref(null), saving = ref(false)
const projectID = computed(() => Number(route.query.project_id) || 0)
const environmentID = computed(() => Number(route.query.environment_id) || 0)
const currentProject = computed(() => projects.value.find(project => project.id === projectID.value) || null)
const currentEnvironment = computed(() => currentProject.value?.environments?.find(environment => environment.id === environmentID.value) || null)
const form = ref(blank())
const issuers = computed(() => allIssuers.value.filter(issuer => issuer.kind === 'ClusterIssuer' && issuer.ready))
function blank(){ return { hostname: '', environment_id: environmentID.value, issuer_ref: '', description: '', enabled: true } }
async function load(){ try { const unassigned = route.query.unassigned === 'true'; const [domainResult, projectResult] = await Promise.all([unassigned ? api.get('/domains?unassigned=true') : environmentID.value ? api.get(`/domains?environment_id=${environmentID.value}`) : api.get('/domains'), api.get('/projects')]); domains.value = domainResult || []; projects.value = projectResult || [] } catch(e) { error.value = e.message || '加载受管域名失败' } finally { loaded.value = true } }
async function loadOptions(){ allIssuers.value = await api.get('/certs/issuers') || [] }
async function openCreate(){ error.value = ''; editing.value = null; if (!currentEnvironment.value) { error.value = '请先在顶部选择项目与环境'; return } form.value = blank(); try { await loadOptions(); form.value.issuer_ref = issuers.value[0]?.name || ''; modal.value = true } catch(e) { error.value = e.message || '加载签发前置条件失败' } }
async function openEdit(domain){ error.value = ''; if (!domain.environment_id && !currentEnvironment.value) { error.value = '请先在顶部选择要重新绑定的项目与环境'; return } editing.value = domain; form.value = { hostname: domain.hostname, environment_id: domain.environment_id || environmentID.value, issuer_ref: domain.issuer_ref || '', description: domain.description || '', enabled: domain.enabled }; try { await loadOptions(); modal.value = true } catch(e) { error.value = e.message || '加载签发前置条件失败' } }
function close(){ modal.value = false; editing.value = null }
async function save(){ saving.value = true; error.value = ''; try { if(editing.value) await api.put(`/domains/${editing.value.id}`, form.value); else await api.post('/domains', form.value); close(); await load() } catch(e) { error.value = e.message || '提交域名申请失败' } finally { saving.value = false } }
async function retry(domain){ error.value = ''; try { await api.post(`/domains/${domain.id}/certificate`); await load() } catch(e) { error.value = e.message || '重新申请证书失败' } }
function openOperations(domain){ router.push(`/certs/${domain.namespace}/${domain.certificate_name}`) }
async function remove(domain){ if(domain.application_count > 0) return; if(!window.confirm(`删除受管域名 ${domain.hostname} 及其 Certificate？`)) return; try { await api.delete(`/domains/${domain.id}`); await load() } catch(e) { error.value = e.message || '删除受管域名失败' } }
function certificateLabel(domain){ if(!domain.namespace) return '未绑定'; return domain.certificate?.status === 'Ready' ? '已就绪' : domain.certificate?.status === 'Failed' ? '签发失败' : '签发中' }
function certificateClass(domain){ return certificateLabel(domain) === '已就绪' ? 'badge-online' : certificateLabel(domain) === '签发失败' ? 'badge-danger' : 'badge-deploying' }
function certificateReason(domain){ return domain.certificate?.reason || domain.certificate_error || '' }
function formatDate(value){ return value ? new Date(value).toLocaleString('zh-CN') : '-' }
onMounted(load)
watch(() => [route.query.project_id, route.query.environment_id, route.query.unassigned], load)
</script>

<style scoped>
.page-header{display:flex;justify-content:space-between;gap:var(--space-16)}.page-subtitle,.cell-secondary,.dns-note{margin:var(--space-4) 0;color:var(--text-secondary);font-size:12px}.cell-secondary{display:block;max-width:180px;overflow-wrap:anywhere}.dns-note{padding:0 var(--space-4)}.check-row{display:flex;gap:8px;align-items:center;color:var(--text-secondary);font-size:13px}@media(max-width:640px){.page-header{flex-direction:column}.page-header .btn{width:100%}}
</style>
