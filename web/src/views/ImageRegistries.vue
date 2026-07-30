<template>
  <div>
    <button class="back-link" @click="backToWorkspace"><ArrowLeft :size="16" />返回工作台</button>
    <div class="page-header"><div><h1 class="page-title">镜像仓库</h1><p class="page-subtitle">维护全局仓库、凭据、连通状态与项目授权</p></div><button class="btn btn-primary" @click="openCreate">+ 新建镜像仓库</button></div>
    <div v-if="error" class="k8s-banner k8s-banner-warn section-gap">⚠ {{ error }}</div>

    <div v-if="loaded && registries.length > 0" class="card section-gap"><div class="table-wrap"><table class="data-table"><thead><tr><th>名称</th><th>地址</th><th>验证镜像</th><th>认证</th><th>项目授权</th><th>状态</th><th>检测结果</th><th>操作</th></tr></thead><tbody>
      <tr v-for="registry in registries" :key="registry.id"><td class="cell-primary">{{ registry.name }}</td><td>{{ registry.endpoint }}</td><td class="verification-image">{{ registry.verification_image || '-' }}</td><td><span>{{ authLabel(registry.auth_type) }}</span><small v-if="registry.credential_configured" class="credential-state">凭据已配置</small></td><td><span v-if="registry.projects?.length">{{ registry.projects.map(project => project.name).join('、') }}</span><span v-else>-</span></td><td><span class="badge" :class="registry.enabled ? 'badge-online' : 'badge-offline'">{{ registry.enabled ? '已启用' : '已停用' }}</span></td><td><div class="verification-result"><span class="badge" :class="verificationBadge(registry.last_verify_status)">{{ verificationLabel(registry.last_verify_status) }}</span><small v-if="registry.last_verified_at">{{ formatTime(registry.last_verified_at) }}</small><small v-if="registry.last_verify_error" class="verification-error">{{ registry.last_verify_error }}</small></div></td><td class="action-cell"><div class="btn-group"><button class="icon-button" title="检测镜像仓库" :aria-label="`检测 ${registry.name}`" :disabled="verifyingID === registry.id" @click="verifyRegistry(registry)"><RefreshCw :size="16" :class="{ 'is-spinning': verifyingID === registry.id }" /></button><button class="btn btn-sm" @click="openEdit(registry)">编辑</button><button class="btn btn-sm btn-danger" title="删除镜像仓库" @click="deleteTarget = registry">删除</button></div></td></tr>
    </tbody></table></div></div>

    <div v-if="showModal" class="overlay" @click.self="closeModal"><div class="modal registry-modal"><h2 class="modal-title">{{ editingRegistry ? '编辑镜像仓库' : '新建镜像仓库' }}</h2><form @submit.prevent="saveRegistry">
      <div class="form-group"><label class="form-label">名称</label><input v-model.trim="form.name" class="form-input" required placeholder="commerce-harbor" /></div>
      <div class="form-group"><label class="form-label">仓库地址</label><input v-model.trim="form.endpoint" class="form-input" required placeholder="harbor.example.com" /></div>
      <div class="form-group"><label class="form-label">验证镜像</label><input v-model.trim="form.verification_image" class="form-input" required placeholder="harbor.example.com/commerce/order-api:latest" /><p class="form-hint">检测会验证该镜像的 Manifest 与 Pull 权限，不下载镜像 Layer。</p></div>
      <div class="form-row"><div class="form-group"><label class="form-label">认证方式</label><select v-model="form.auth_type" class="form-select"><option value="anonymous">匿名访问</option><option value="basic">账号密码</option><option value="token">Token</option></select></div><div v-if="form.auth_type !== 'anonymous'" class="form-group"><label class="form-label">账号</label><input v-model.trim="form.username" class="form-input" :required="form.auth_type === 'basic'" placeholder="robot$commerce" /></div></div>
      <div v-if="form.auth_type !== 'anonymous'" class="form-group"><label class="form-label">{{ editingRegistry ? '新凭据（留空则不修改）' : '密码或 Token' }}</label><input v-model="form.credential" type="password" class="form-input" :required="!editingRegistry" autocomplete="new-password" /></div>
      <label class="check-row"><input v-model="form.enabled" type="checkbox" /> 启用此仓库</label>
      <div class="form-group"><label class="form-label">授权项目</label><div class="project-options"><label v-for="project in projects" :key="project.id" class="project-option"><input v-model="form.project_ids" type="checkbox" :value="project.id" /> {{ project.name }}</label><span v-if="projects.length === 0" class="form-hint">请先创建项目</span></div></div>
      <div class="modal-actions"><button type="button" class="btn" @click="closeModal">取消</button><button class="btn btn-primary" :disabled="submitting">{{ submitting ? '保存中...' : '保存' }}</button></div>
    </form></div></div>

    <div v-if="deleteTarget" class="overlay" @click.self="deleteTarget = null"><div class="modal"><h2 class="modal-title">删除镜像仓库</h2><p class="confirm-copy">确认删除“{{ deleteTarget.name }}”吗？已有发布记录引用时无法删除。</p><div class="modal-actions"><button class="btn" @click="deleteTarget = null">取消</button><button class="btn btn-danger" :disabled="submitting" @click="deleteRegistry">删除</button></div></div></div>
  </div>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { ArrowLeft, RefreshCw } from 'lucide-vue-next'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api/index.js'

const registries = ref([])
const projects = ref([])
const route = useRoute()
const router = useRouter()
const loaded = ref(false)
const error = ref('')
const submitting = ref(false)
const showModal = ref(false)
const editingRegistry = ref(null)
const deleteTarget = ref(null)
const verifyingID = ref(null)
const form = ref(newRegistryForm())
function newRegistryForm() { return { name: '', endpoint: '', verification_image: '', auth_type: 'anonymous', username: '', credential: '', enabled: true, project_ids: [] } }
function authLabel(type) { return type === 'basic' ? '账号密码' : type === 'token' ? 'Token' : '匿名访问' }
function verificationLabel(status) { return status === 'succeeded' ? '连通' : status === 'failed' ? '检测失败' : '未检测' }
function verificationBadge(status) { return status === 'succeeded' ? 'badge-online' : status === 'failed' ? 'badge-danger' : 'badge-offline' }
function formatTime(value) { return value ? new Date(value).toLocaleString() : '-' }
async function fetchData() { error.value = ''; try { const [registryResult, projectResult] = await Promise.all([api.get('/image-registries'), api.get('/projects')]); registries.value = registryResult || []; projects.value = projectResult || [] } catch (e) { error.value = e.message || '加载镜像仓库失败' } finally { loaded.value = true } }
function openCreate() { editingRegistry.value = null; form.value = newRegistryForm(); showModal.value = true }
function openEdit(registry) { editingRegistry.value = registry; form.value = { name: registry.name, endpoint: registry.endpoint, verification_image: registry.verification_image || '', auth_type: registry.auth_type, username: registry.username || '', credential: '', enabled: registry.enabled, project_ids: (registry.projects || []).map(project => project.id) }; showModal.value = true }
function closeModal() { showModal.value = false; editingRegistry.value = null; form.value = newRegistryForm() }
async function saveRegistry() { submitting.value = true; error.value = ''; try { const payload = { ...form.value, project_ids: [...form.value.project_ids] }; if (editingRegistry.value && !payload.credential) delete payload.credential; if (editingRegistry.value) await api.put(`/image-registries/${editingRegistry.value.id}`, payload); else await api.post('/image-registries', payload); closeModal(); await fetchData() } catch (e) { error.value = e.message || '保存镜像仓库失败' } finally { submitting.value = false } }
async function deleteRegistry() { if (!deleteTarget.value) return; submitting.value = true; error.value = ''; try { await api.delete(`/image-registries/${deleteTarget.value.id}`); deleteTarget.value = null; await fetchData() } catch (e) { error.value = e.message || '删除镜像仓库失败' } finally { submitting.value = false } }
async function verifyRegistry(registry) { verifyingID.value = registry.id; error.value = ''; try { const updated = await api.post(`/image-registries/${registry.id}/verify`); const index = registries.value.findIndex(item => item.id === registry.id); if (index >= 0) registries.value[index] = updated } catch (e) { error.value = e.message || '检测镜像仓库失败' } finally { verifyingID.value = null } }
async function backToWorkspace() { await router.push({ path: '/applications', query: { project_id: route.query.project_id, environment_id: route.query.environment_id } }) }

onMounted(fetchData)
</script>

<style scoped>
.back-link { display:inline-flex; align-items:center; gap:6px; margin:0 0 var(--space-16); padding:0; border:0; background:transparent; color:var(--text-secondary); font:inherit; font-size:13px; font-weight:600; cursor:pointer; }.back-link:hover { color:var(--action-primary); }.back-link:focus-visible { outline:2px solid var(--focus); outline-offset:3px; }.page-header { display:flex; align-items:flex-start; justify-content:space-between; gap:var(--space-16); }.page-subtitle { margin:var(--space-4) 0 0; color:var(--text-secondary); font-size:13px; }.registry-modal { width:min(560px, calc(100vw - 32px)); }.credential-state { display:block; margin-top:3px; color:var(--text-muted); font-size:11px; }.verification-image { min-width:220px; max-width:340px; overflow-wrap:anywhere; }.verification-result { display:grid; gap:3px; min-width:108px; }.verification-result small { color:var(--text-muted); font-size:11px; }.verification-result .verification-error { max-width:220px; color:var(--danger); overflow-wrap:anywhere; }.is-spinning { animation:spin .8s linear infinite; }@keyframes spin { to { transform:rotate(360deg); } }.check-row { display:flex; align-items:center; gap:8px; margin:var(--space-12) 0; color:var(--text-secondary); font-size:13px; }.project-options { display:flex; flex-wrap:wrap; gap:8px; }.project-option { display:inline-flex; align-items:center; gap:6px; padding:6px 8px; border:1px solid var(--border-muted); border-radius:var(--radius-control); color:var(--text-secondary); font-size:12px; }.form-hint { color:var(--text-muted); font-size:12px; }.confirm-copy { margin:0; color:var(--text-secondary); font-size:13px; }@media (max-width:640px) { .page-header { align-items:stretch; flex-direction:column; }.page-header .btn { width:100%; } }
</style>
