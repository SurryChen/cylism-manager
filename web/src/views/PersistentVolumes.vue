<template>
  <div>
    <div class="page-header"><div><h1 class="page-title">存储卷</h1><p class="page-subtitle">在环境命名空间中管理应用持久化数据</p></div><button class="btn btn-primary" :disabled="!environmentID" @click="openCreate">+ 创建存储卷</button></div>
    <div v-if="error" class="k8s-banner k8s-banner-warn section-gap">{{ error }}</div>
    <section class="storage-context"><div class="form-group"><label class="form-label">项目</label><select v-model.number="projectID" class="form-select"><option :value="0">选择项目</option><option v-for="project in projects" :key="project.id" :value="project.id">{{ project.name }}</option></select></div><div class="form-group"><label class="form-label">环境</label><select v-model.number="environmentID" class="form-select" :disabled="!selectedProject"><option :value="0">选择环境</option><option v-for="environment in selectedProject?.environments || []" :key="environment.id" :value="environment.id">{{ environment.name }} · {{ environment.namespace }}</option></select></div></section>
    <div v-if="environmentID && loaded && !claims.length" class="empty-state"><span class="empty-icon">▣</span><span class="empty-text">当前环境还没有平台托管存储卷</span></div>
    <div v-else-if="claims.length" class="card"><div class="table-wrap"><table class="data-table"><thead><tr><th>存储卷</th><th>容量</th><th>StorageClass</th><th>状态</th><th>绑定节点</th><th>数据回收</th><th>引用</th><th>操作</th></tr></thead><tbody><tr v-for="claim in claims" :key="claim.name"><td class="cell-primary">{{ claim.name }}</td><td>{{ claim.storage || '-' }}</td><td>{{ claim.storage_class_name || '默认 StorageClass' }}</td><td><span class="badge" :class="claim.phase === 'Bound' ? 'badge-online' : 'badge-deploying'">{{ claim.phase || 'Pending' }}</span></td><td><span v-if="claim.bound_node_display_name">{{ claim.bound_node_display_name }}<small class="node-meta">{{ claim.bound_node }}</small></span><span v-else-if="claim.wait_for_first_consumer">首次挂载时决定</span><span v-else>-</span></td><td><span class="badge" :class="claim.reclaim_policy === 'Delete' ? 'badge-danger' : 'badge-offline'">{{ claim.reclaim_policy || '-' }}</span></td><td>{{ claim.references?.join('、') || '-' }}</td><td><button class="icon-button danger-action" title="删除存储卷" :disabled="claim.references?.length" @click="requestDelete(claim)"><Trash2 :size="16" /></button></td></tr></tbody></table></div></div>

    <div v-if="showCreate" class="overlay" @click.self="showCreate = false"><div class="modal"><h2 class="modal-title">创建存储卷</h2><form @submit.prevent="createClaim"><div class="form-group"><label class="form-label">名称</label><input v-model.trim="form.name" class="form-input" required pattern="[a-z0-9]([-a-z0-9]*[a-z0-9])?" placeholder="karakeep-data" /></div><div class="form-group"><label class="form-label">容量</label><input v-model.trim="form.storage" class="form-input" required placeholder="5Gi" /><p class="form-hint">首期仅支持 ReadWriteOnce，挂载该卷的应用只能运行一个副本。</p></div><div class="form-group"><label class="form-label">StorageClass</label><select v-model="form.storage_class_name" class="form-select"><option value="">使用默认 StorageClass</option><option v-for="item in storageClasses" :key="item.name" :value="item.name">{{ item.name }}{{ item.is_default ? '（默认）' : '' }} · {{ item.volume_binding_mode || 'Immediate' }}</option></select></div><div class="modal-actions"><button type="button" class="btn" @click="showCreate = false">取消</button><button class="btn btn-primary" :disabled="saving">{{ saving ? '创建中...' : '创建存储卷' }}</button></div></form></div></div>
    <div v-if="deleteTarget" class="overlay" @click.self="deleteTarget = null"><div class="modal"><h2 class="modal-title">删除存储卷</h2><p class="confirm-copy">将删除 PVC“{{ deleteTarget.name }}”。{{ deleteTarget.reclaim_policy === 'Delete' ? '该 PV 的回收策略为 Delete，底层数据可能一并被清除。' : '底层数据的保留行为由 PV 回收策略决定。' }}</p><div class="modal-actions"><button class="btn" @click="deleteTarget = null">取消</button><button class="btn btn-danger" :disabled="saving" @click="deleteClaim">确认删除</button></div></div></div>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { Trash2 } from 'lucide-vue-next'
import { api } from '../api/index.js'

const projects = ref([])
const claims = ref([])
const storageClasses = ref([])
const projectID = ref(0)
const environmentID = ref(0)
const loaded = ref(false)
const error = ref('')
const saving = ref(false)
const showCreate = ref(false)
const deleteTarget = ref(null)
const form = ref(newClaimForm())
const selectedProject = computed(() => projects.value.find(project => project.id === projectID.value) || null)

function newClaimForm() { return { name: '', storage: '5Gi', storage_class_name: '' } }
async function loadProjects() { try { projects.value = await api.get('/projects') || [] } catch (e) { error.value = e.message || '加载项目失败' } }
async function loadClaims() { if (!environmentID.value) { claims.value = []; loaded.value = false; return }; loaded.value = false; error.value = ''; try { const [items, classes] = await Promise.all([api.get(`/k8s/persistent-volume-claims?environment_id=${environmentID.value}`), api.get('/k8s/storage-classes')]); claims.value = items || []; storageClasses.value = classes || []; loaded.value = true } catch (e) { error.value = e.message || '加载存储卷失败' } }
function openCreate() { form.value = newClaimForm(); showCreate.value = true }
async function createClaim() { saving.value = true; error.value = ''; try { await api.post('/k8s/persistent-volume-claims', { environment_id: environmentID.value, ...form.value }); showCreate.value = false; await loadClaims() } catch (e) { error.value = e.message || '创建存储卷失败' } finally { saving.value = false } }
function requestDelete(claim) { deleteTarget.value = claim }
async function deleteClaim() { if (!deleteTarget.value) return; saving.value = true; error.value = ''; try { await api.delete(`/k8s/persistent-volume-claims/${deleteTarget.value.name}`, { environment_id: environmentID.value, confirm_data_delete: true }); deleteTarget.value = null; await loadClaims() } catch (e) { error.value = e.message || '删除存储卷失败' } finally { saving.value = false } }

watch(projectID, () => { const environments = selectedProject.value?.environments || []; if (!environments.some(item => item.id === environmentID.value)) environmentID.value = environments.length === 1 ? environments[0].id : 0 })
watch(environmentID, loadClaims)
loadProjects()
</script>

<style scoped>
.page-header{display:flex;align-items:flex-start;justify-content:space-between;gap:var(--space-16);margin-bottom:var(--space-20)}.page-subtitle{margin:var(--space-4) 0 0;color:var(--text-secondary);font-size:13px}.storage-context{display:grid;grid-template-columns:repeat(2,minmax(0,220px));gap:var(--space-12);margin-bottom:var(--space-20);padding:var(--space-16);border:1px solid var(--border-muted);border-radius:var(--radius-control);background:var(--surface-subtle)}.storage-context .form-group{margin:0}.node-meta{display:block;margin-top:2px;color:var(--text-muted);font:10px/1 var(--font-mono)}.danger-action{color:var(--danger)}.confirm-copy{margin:0;color:var(--text-secondary);font-size:13px;line-height:1.6}@media(max-width:640px){.page-header{align-items:stretch;flex-direction:column}.page-header>.btn{width:100%}.storage-context{grid-template-columns:1fr}}
</style>
