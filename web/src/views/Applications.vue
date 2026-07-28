<template>
  <div>
    <div class="page-header"><div><h1 class="page-title">应用</h1><p class="page-subtitle">发布和运维平台托管的 Kubernetes 服务</p></div><button class="btn btn-primary" @click="showCreate = true">+ 创建应用</button></div>
    <div v-if="error" class="k8s-banner k8s-banner-warn section-gap">⚠ {{ error }}</div>
    <div class="card">
      <div v-if="loading" class="empty-state"><span class="empty-text">加载应用中...</span></div>
      <div v-else-if="applications.length === 0" class="empty-state"><span class="empty-icon">⬡</span><span class="empty-text">暂无应用</span></div>
      <div v-else class="table-wrap"><table class="data-table"><thead><tr><th>应用</th><th>环境</th><th>命名空间</th><th>访问地址</th><th>操作</th></tr></thead><tbody>
        <tr v-for="app in applications" :key="app.id" class="clickable" @click="openDetails(app)"><td class="cell-primary">{{ app.name }}</td><td>{{ app.environment?.name || '-' }}</td><td>{{ app.environment?.namespace || '-' }}</td><td>{{ endpointLabel(app) }}</td><td><button class="btn btn-sm" @click.stop="openRelease(app)">发布版本</button></td></tr>
      </tbody></table></div>
    </div>

    <div v-if="applicationDetail" class="card section-gap"><div class="detail-header"><div><h2 class="section-title">{{ applicationDetail.application.name }} 发布记录</h2><p class="section-copy">{{ applicationDetail.application.environment?.namespace }}</p></div><button class="icon-button" title="关闭发布记录" @click="closeDetails">×</button></div>
      <div v-if="applicationDetail.releases.length === 0" class="empty-state"><span class="empty-text">暂无发布记录</span></div>
      <div v-else class="table-wrap"><table class="data-table"><thead><tr><th>版本</th><th>镜像</th><th>状态</th><th>时间</th><th></th></tr></thead><tbody><tr v-for="release in applicationDetail.releases" :key="release.id" class="clickable" @click="openReleaseDetail(release)"><td>#{{ release.sequence }}</td><td class="cell-primary">{{ release.image }}</td><td><span class="badge" :class="release.status === 'succeeded' ? 'badge-online' : release.status === 'failed' ? 'badge-danger' : 'badge-offline'">{{ release.status }}</span></td><td>{{ formatTime(release.created_at) }}</td><td class="btn-group action-cell" @click.stop><button v-if="release.status === 'failed'" class="btn btn-sm" @click="retryRelease(release)">重试</button><button class="btn btn-sm" @click="rollbackRelease(release)">回滚</button></td></tr></tbody></table></div>
      <div v-if="releaseDetail" class="release-steps"><strong>Release #{{ releaseDetail.sequence }} 步骤</strong><div v-for="operation in releaseDetail.operations" :key="operation.id" class="step-row"><span :class="['status-dot', operation.status]"></span><span>{{ operation.step }}</span><span>{{ operation.status }}</span><small>{{ operation.detail || '-' }}</small></div></div>
    </div>

    <div v-if="showCreate" class="overlay" @click.self="showCreate = false"><div class="modal"><h2 class="modal-title">创建应用</h2><form @submit.prevent="createApplication">
      <div class="form-group"><label class="form-label">项目</label><input v-model="createForm.projectName" class="form-input" required placeholder="commerce" /></div>
      <div class="form-row"><div class="form-group"><label class="form-label">环境</label><input v-model="createForm.environmentName" class="form-input" required placeholder="production" /></div><div class="form-group"><label class="form-label">命名空间</label><input v-model="createForm.namespace" class="form-input" required placeholder="commerce-prod" /></div></div>
      <div class="form-group"><label class="form-label">应用名</label><input v-model="createForm.applicationName" class="form-input" required placeholder="order-api" /></div>
      <div class="modal-actions"><button type="button" class="btn" @click="showCreate = false">取消</button><button class="btn btn-primary" :disabled="submitting">{{ submitting ? '创建中...' : '创建' }}</button></div>
    </form></div></div>

    <div v-if="releaseApp" class="overlay" @click.self="releaseApp = null"><div class="modal release-modal"><h2 class="modal-title">发布 {{ releaseApp.name }}</h2><form @submit.prevent="createRelease">
      <div class="form-group"><label class="form-label">镜像</label><input v-model="releaseForm.image" class="form-input" required placeholder="registry.example.com/order-api:1.0.0" /></div>
      <div class="form-row"><div class="form-group"><label class="form-label">容器端口</label><input v-model.number="releaseForm.container_port" type="number" class="form-input" min="1" required /></div><div class="form-group"><label class="form-label">副本数</label><input v-model.number="releaseForm.replicas" type="number" class="form-input" min="1" required /></div></div>
      <div class="form-row"><div class="form-group"><label class="form-label">CPU 请求/限制</label><input v-model="releaseForm.resources.requests_cpu" class="form-input" required /><input v-model="releaseForm.resources.limits_cpu" class="form-input compact-input" required /></div><div class="form-group"><label class="form-label">内存 请求/限制</label><input v-model="releaseForm.resources.requests_memory" class="form-input" required /><input v-model="releaseForm.resources.limits_memory" class="form-input compact-input" required /></div></div>
      <div class="form-row"><div class="form-group"><label class="form-label">就绪检查</label><input v-model="releaseForm.health.readiness_path" class="form-input" required /></div><div class="form-group"><label class="form-label">存活检查</label><input v-model="releaseForm.health.liveness_path" class="form-input" required /></div></div>
      <div class="form-row"><div class="form-group"><label class="form-label">Service 端口</label><input v-model.number="releaseForm.service.port" type="number" class="form-input" required /></div><div class="form-group"><label class="form-label">暴露方式</label><select v-model="releaseForm.endpoint.exposure" class="form-select"><option value="cluster">仅集群内</option><option value="public">公网域名</option></select></div></div>
      <div v-if="releaseForm.endpoint.exposure === 'public'" class="form-row"><div class="form-group"><label class="form-label">域名</label><input v-model="releaseForm.endpoint.domain" class="form-input" required placeholder="api.example.com" /></div><div class="form-group"><label class="form-label">Issuer</label><input v-model="releaseForm.endpoint.issuer_ref" class="form-input" placeholder="letsencrypt-prod" /></div></div>
      <label v-if="releaseForm.endpoint.exposure === 'public'" class="check-row"><input v-model="releaseForm.endpoint.tls_enabled" type="checkbox" /> 启用 HTTPS</label>
      <div class="modal-actions"><button type="button" class="btn" @click="releaseApp = null">取消</button><button class="btn btn-primary" :disabled="submitting">{{ submitting ? '提交中...' : '开始发布' }}</button></div>
    </form></div></div>
  </div>
</template>

<script setup>
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { api } from '../api/index.js'

const applications = ref([])
const loading = ref(true)
const error = ref('')
const submitting = ref(false)
const showCreate = ref(false)
const releaseApp = ref(null)
const applicationDetail = ref(null)
const releaseDetail = ref(null)
let releasePoller = null
const createForm = ref({ projectName: '', environmentName: 'production', namespace: '', applicationName: '' })
const releaseForm = ref(newReleaseForm())

function newReleaseForm() { return { image: '', container_port: 8080, replicas: 1, resources: { requests_cpu: '100m', requests_memory: '128Mi', limits_cpu: '500m', limits_memory: '512Mi' }, health: { readiness_path: '/healthz', liveness_path: '/healthz' }, service: { port: 80, target_port: 8080 }, endpoint: { exposure: 'cluster', domain: '', path: '/', tls_enabled: false, issuer_ref: '' } } }
function endpointLabel(app) { const endpoint = app.endpoints?.[0]; return endpoint?.domain ? `https://${endpoint.domain}` : '集群内' }
async function fetchApplications() { loading.value = true; error.value = ''; try { applications.value = await api.get('/applications') || [] } catch (e) { error.value = e.message || '加载应用失败' } finally { loading.value = false } }
async function createApplication() { submitting.value = true; error.value = ''; try { const project = await api.post('/projects', { name: createForm.value.projectName }); const environment = await api.post(`/projects/${project.id}/environments`, { name: createForm.value.environmentName, namespace: createForm.value.namespace }); await api.post('/applications', { project_id: project.id, environment_id: environment.id, name: createForm.value.applicationName }); showCreate.value = false; createForm.value = { projectName: '', environmentName: 'production', namespace: '', applicationName: '' }; fetchApplications() } catch (e) { error.value = e.message || '创建应用失败' } finally { submitting.value = false } }
function openRelease(app) { releaseApp.value = app; releaseForm.value = newReleaseForm() }
async function createRelease() { submitting.value = true; error.value = ''; try { const applicationID = releaseApp.value.id; const release = await api.post(`/applications/${applicationID}/releases`, releaseForm.value); releaseApp.value = null; await openDetails({ id: applicationID }); await openReleaseDetail(release); fetchApplications() } catch (e) { error.value = e.message || '创建发布失败' } finally { submitting.value = false } }
async function openDetails(app) { try { applicationDetail.value = await api.get(`/applications/${app.id}`); releaseDetail.value = null } catch (e) { error.value = e.message || '加载发布记录失败' } }
function closeDetails() { applicationDetail.value = null; releaseDetail.value = null; stopPolling() }
async function openReleaseDetail(release) { if (!applicationDetail.value) return; try { releaseDetail.value = await api.get(`/applications/${applicationDetail.value.application.id}/releases/${release.id}`); pollRelease() } catch (e) { error.value = e.message || '加载发布步骤失败' } }
function pollRelease() { stopPolling(); if (!releaseDetail.value || ['succeeded', 'failed', 'rolled_back'].includes(releaseDetail.value.status)) return; releasePoller = window.setInterval(async () => { try { releaseDetail.value = await api.get(`/applications/${applicationDetail.value.application.id}/releases/${releaseDetail.value.id}`); if (['succeeded', 'failed', 'rolled_back'].includes(releaseDetail.value.status)) { stopPolling(); openDetails(applicationDetail.value.application) } } catch (e) { stopPolling() } }, 2000) }
function stopPolling() { if (releasePoller) { window.clearInterval(releasePoller); releasePoller = null } }
async function retryRelease(release) { try { const next = await api.post(`/applications/${applicationDetail.value.application.id}/releases/${release.id}/retry`); await openDetails(applicationDetail.value.application); await openReleaseDetail(next) } catch (e) { error.value = e.message || '重试失败' } }
async function rollbackRelease(release) { try { const next = await api.post(`/applications/${applicationDetail.value.application.id}/releases/${release.id}/rollback`); await openDetails(applicationDetail.value.application); await openReleaseDetail(next) } catch (e) { error.value = e.message || '回滚失败' } }
function formatTime(value) { return value ? new Date(value).toLocaleString() : '-' }
onMounted(fetchApplications)
onBeforeUnmount(stopPolling)
</script>

<style scoped>
.page-header { display:flex; align-items:flex-start; justify-content:space-between; gap:var(--space-16); }
.page-subtitle { margin:var(--space-4) 0 0; color:var(--text-secondary); font-size:13px; }
.release-modal { width:min(680px, calc(100vw - 32px)); }
.compact-input { margin-top:6px; }
.check-row { display:flex; gap:8px; align-items:center; margin:var(--space-12) 0; color:var(--text-secondary); font-size:13px; }
.detail-header { display:flex; justify-content:space-between; gap:var(--space-12); }
.section-title { margin:0; font-size:15px; }
.section-copy { margin:4px 0 0; color:var(--text-secondary); font-size:12px; }
.release-steps { margin-top:var(--space-16); display:grid; gap:8px; }
.step-row { display:grid; grid-template-columns:10px 110px 72px minmax(0, 1fr); align-items:center; gap:8px; color:var(--text-secondary); font-size:12px; }
.status-dot { width:7px; height:7px; border-radius:50%; background:var(--text-muted); }.status-dot.success { background:var(--success); }.status-dot.failed { background:var(--danger); }.status-dot.running { background:var(--warning); }
</style>
